package bundle_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/bundle"
	"github.com/humzakhan/provenant/kernel/public/extractcsvrows"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/extractjsonl"
	"github.com/humzakhan/provenant/kernel/public/extractspreadsheet"
	"github.com/humzakhan/provenant/kernel/public/internal/phaseone"
	"github.com/humzakhan/provenant/kernel/public/internal/testxlsx"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

func TestEmitVerifyReplay(t *testing.T) {
	t.Parallel()

	run, binding := buildPhaseOneState(t)
	state := snapshotState(t, binding, run)

	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	bundleBytesAgain, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("second Emit() error = %v", err)
	}
	if !bytes.Equal(bundleBytes, bundleBytesAgain) {
		t.Fatal("Emit() bytes are not deterministic")
	}

	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if replayed.Run != run {
		t.Fatalf("Replay().Run = %#v, want %#v", replayed.Run, run)
	}
	if len(replayed.CapturedReads) != 2 {
		t.Fatalf("len(Replay().CapturedReads) = %d, want 2", len(replayed.CapturedReads))
	}
	if len(replayed.Tokens) != 2 {
		t.Fatalf("len(Replay().Tokens) = %d, want 2", len(replayed.Tokens))
	}
	if len(replayed.AuditEntries) != 2 {
		t.Fatalf("len(Replay().AuditEntries) = %d, want 2", len(replayed.AuditEntries))
	}
	readIDs := make(map[string]struct{}, len(replayed.CapturedReads))
	for _, read := range replayed.CapturedReads {
		readIDs[read.ID] = struct{}{}
	}
	for _, token := range replayed.Tokens {
		if _, ok := readIDs[token.Source.CapturedReadID]; !ok {
			t.Fatalf("Replay() token captured read = %s, want one of stored reads", token.Source.CapturedReadID)
		}
		if !token.CreatedAt.IsZero() {
			t.Fatalf("Replay() token CreatedAt = %s, want zero", token.CreatedAt)
		}
		if token.Derivation != nil {
			t.Fatalf("Replay() token Derivation = %#v, want nil", token.Derivation)
		}
	}
	if replayed.AuditEntries[1].Seq != 2 {
		t.Fatalf("Replay() audit seq = %d, want 2", replayed.AuditEntries[1].Seq)
	}
	if !bytes.Equal(replayed.AuditEntries[1].PrevHash, replayed.AuditEntries[0].Hash) {
		t.Fatalf("Replay() second prev_hash = %x, want first hash %x", replayed.AuditEntries[1].PrevHash, replayed.AuditEntries[0].Hash)
	}

	replayedBytes, err := bundle.Emit(replayed)
	if err != nil {
		t.Fatalf("Emit(Replay()) error = %v", err)
	}
	if !bytes.Equal(bundleBytes, replayedBytes) {
		t.Fatal("Emit(Replay()) bytes differ from original bundle")
	}
}

func TestVerifyFailsClosedOnTamperAndTruncation(t *testing.T) {
	t.Parallel()

	run, binding := buildPhaseOneState(t)
	state := snapshotState(t, binding, run)

	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(t *testing.T, doc bundleDocument) []byte
		wantErr error
	}{
		{
			name: "token value tampered",
			mutate: func(t *testing.T, doc bundleDocument) []byte {
				doc.Tokens[0].ValueCanonical = "999.990000000"
				return encodeDocument(t, doc)
			},
			wantErr: bundle.ErrTamperDetected,
		},
		{
			name: "captured read output tampered",
			mutate: func(t *testing.T, doc bundleDocument) []byte {
				doc.CapturedReads[0].Output = []byte(`{"json_pointer":"/invoice/items/0/amount","value":"999.99"}`)
				return encodeDocument(t, doc)
			},
			wantErr: bundle.ErrTamperDetected,
		},
		{
			name: "audit chain truncated",
			mutate: func(t *testing.T, doc bundleDocument) []byte {
				doc.AuditJSONL = strings.TrimSuffix(doc.AuditJSONL, "\n")
				return encodeDocument(t, doc)
			},
			wantErr: bundle.ErrChainTruncated,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := decodeDocument(t, bundleBytes)
			mutated := tt.mutate(t, doc)

			err := bundle.Verify(mutated)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Verify() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyFailsWhenAuditedTokenDivergesFromCapturedReadOutput(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	read := doc.CapturedReads[0]
	evidence, err := phaseone.ReconstructMintEvidence(run.ID, read.ID, read.Output, "$.value", "")
	if err != nil {
		t.Fatalf("ReconstructMintEvidence() error = %v", err)
	}
	if evidence.ValueCanonical != doc.Tokens[0].ValueCanonical {
		t.Fatalf("baseline canonical value = %s, want %s", doc.Tokens[0].ValueCanonical, evidence.ValueCanonical)
	}

	doc.Tokens[0].ID = "tok_diverged"
	doc.Tokens[0].ValueCanonical = "999.990000000"
	mutateFirstAuditPayload(t, &doc, func(payload map[string]any) {
		payload["token_id"] = "tok_diverged"
		payload["source_value_raw"] = "999.99"
		payload["value_canonical"] = "999.990000000"
	})

	err = bundle.Verify(encodeDocument(t, doc))
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestVerifyAllowsMultipleTokensFromOneCapturedRead(t *testing.T) {
	t.Parallel()

	doc := multipleTokensOneCapturedReadDocument(t)
	doc.CapturedReads = []kernel.CapturedRead{capturedReadByID(t, doc.CapturedReads, doc.Tokens[0].Source.CapturedReadID)}

	if err := bundle.Verify(encodeDocument(t, doc)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsUnreferencedCapturedRead(t *testing.T) {
	t.Parallel()

	doc := multipleTokensOneCapturedReadDocument(t)

	err := bundle.Verify(encodeDocument(t, doc))
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

// TestReplayIgnoresUnauditedTokenFields asserts that unaudited but
// invariant-safe token fields (such as CreatedAt) are not trusted across the
// bundle boundary: Replay reconstructs them deterministically from audit
// payloads. Fields that would violate the Token leaf-vs-derived invariant
// (Derivation set on a token whose audit kind is token_mint) are rejected
// outright — see TestReplayRejectsLeafTokenWithDerivation.
func TestReplayIgnoresUnauditedTokenFields(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	doc.Tokens[0].CreatedAt = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

	bundleBytes := encodeDocument(t, doc)
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if !replayed.Tokens[0].CreatedAt.IsZero() {
		t.Fatalf("Replay() token CreatedAt = %s, want zero", replayed.Tokens[0].CreatedAt)
	}
}

// TestReplayRejectsLeafTokenWithDerivation asserts the Phase 01 invariant
// that a token minted via token_mint must not also carry a Derivation
// (the leaf-vs-derived invariant documented on kernel.Token).
func TestReplayRejectsLeafTokenWithDerivation(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	doc.Tokens[0].Derivation = &kernel.Derivation{
		Operation: "sum",
		Inputs:    []string{"tok_a", "tok_b"},
		FormulaID: "arith.sum",
		Evaluated: doc.Tokens[0].ValueCanonical,
	}

	err := bundle.Verify(encodeDocument(t, doc))
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestEmitReplayPreservesJSONLCapturedReadMetadata(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_jsonl", TenantID: "tenant_1", Status: kernel.RunRunning}
	sourceBytes := []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}\n{\"amount\":\"3.30\",\"note\":\"c\"}\n{\"amount\":\"4.40\",\"note\":\"d\"}\n{\"amount\":\"5.50\",\"note\":\"e\"}")
	source := sourceWithHash("fixture://amounts.jsonl", sourceBytes)
	binding := memio.NewBinding(source)
	selector := kernel.SourceSelector{
		LineWindow:  "1-1",
		Cursor:      "line:2",
		JSONPointer: "/amount",
	}
	clock := &sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)}}

	result, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractjsonl.Reader{},
		binding,
		binding,
		clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://amounts.jsonl",
			Selector:   selector,
			OutputPath: "$.value",
		},
	)
	if err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}
	wantOutput := []byte(`{"cursor":"line:2","effective_line_end":3,"effective_line_start":3,"line_window":"1-1","next_cursor":"line:3","normalized_value":"3.30","raw_value":"\"3.30\"","record_count":1,"record_order":"source_line_asc","records":[{"byte_end":74,"byte_start":68,"json_pointer":"/amount","line":3,"normalized_value":"3.30","raw_value":"\"3.30\"","value":"3.30","window_index":0}],"source_hash":"` + source.Source.SHA256 + `","value":"3.30"}`)
	if !bytes.Equal(result.ReadResult.Output, wantOutput) {
		t.Fatalf("ReadResult.Output = %s, want %s", result.ReadResult.Output, wantOutput)
	}

	state := snapshotState(t, binding, run)
	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed.CapturedReads) != 1 {
		t.Fatalf("len(Replay().CapturedReads) = %d, want 1", len(replayed.CapturedReads))
	}

	got := replayed.CapturedReads[0]
	if got.ID != result.ReadResult.CapturedRead.ID {
		t.Fatalf("Replay().CapturedReads[0].ID = %s, want %s", got.ID, result.ReadResult.CapturedRead.ID)
	}
	if got.Selector != selector {
		t.Fatalf("Replay().CapturedReads[0].Selector = %#v, want %#v", got.Selector, selector)
	}
	if got.Origin != "fixture://amounts.jsonl#line=3-3&ptr=/amount&byte=68-74" {
		t.Fatalf("Replay().CapturedReads[0].Origin = %s", got.Origin)
	}
	if string(got.ReaderOptions) != string(result.ReadResult.CapturedRead.ReaderOptions) {
		t.Fatalf("Replay().CapturedReads[0].ReaderOptions = %s, want %s", got.ReaderOptions, result.ReadResult.CapturedRead.ReaderOptions)
	}
	if string(got.Locator) != string(result.ReadResult.CapturedRead.Locator) {
		t.Fatalf("Replay().CapturedReads[0].Locator = %s, want %s", got.Locator, result.ReadResult.CapturedRead.Locator)
	}
	if !bytes.Equal(got.Output, wantOutput) {
		t.Fatalf("Replay().CapturedReads[0].Output = %s, want %s", got.Output, wantOutput)
	}
}

func TestEmitReplayPreservesCSVCapturedReadMetadata(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_csv", TenantID: "tenant_1", Status: kernel.RunRunning}
	sourceBytes := []byte("account,amount,note\ncash,1.10,a\nar,2.20,b\nap,3.30,c")
	source := sourceWithHash("fixture://amounts.csv", sourceBytes)
	binding := memio.NewBinding(source)
	selector := kernel.SourceSelector{
		RowWindow:        "1-1",
		Cursor:           "row:2",
		ColumnProjection: "amount",
		HeaderRow:        1,
		Dialect:          "excel",
		Encoding:         "utf-8",
	}
	clock := &sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)}}

	result, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractcsvrows.Reader{},
		binding,
		binding,
		clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://amounts.csv",
			Selector:   selector,
			OutputPath: "$.value",
		},
	)
	if err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}
	wantOutput := []byte(`{"column_count":1,"column_order":"source_column_asc","column_projection":"amount","cursor":"row:2","effective_row_end":3,"effective_row_start":3,"header":{"cells":[{"column":2,"normalized_value":"amount","raw_value":"amount","value":"amount"}],"row":1},"header_row":1,"next_cursor":"row:3","normalized_value":"2.20","raw_value":"2.20","row_count":1,"row_order":"source_row_asc","row_window":"1-1","rows":[{"cells":[{"column":2,"header":"amount","normalized_value":"2.20","raw_value":"2.20","row":3,"value":"2.20"}],"row":3,"window_index":0}],"source_hash":"` + source.Source.SHA256 + `","value":"2.20"}`)
	if !bytes.Equal(result.ReadResult.Output, wantOutput) {
		t.Fatalf("ReadResult.Output = %s, want %s", result.ReadResult.Output, wantOutput)
	}

	state := snapshotState(t, binding, run)
	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed.CapturedReads) != 1 {
		t.Fatalf("len(Replay().CapturedReads) = %d, want 1", len(replayed.CapturedReads))
	}

	got := replayed.CapturedReads[0]
	if got.ID != result.ReadResult.CapturedRead.ID {
		t.Fatalf("Replay().CapturedReads[0].ID = %s, want %s", got.ID, result.ReadResult.CapturedRead.ID)
	}
	if got.Selector != selector {
		t.Fatalf("Replay().CapturedReads[0].Selector = %#v, want %#v", got.Selector, selector)
	}
	if got.Origin != "fixture://amounts.csv#row=3&col=2&header_row=1&header=amount" {
		t.Fatalf("Replay().CapturedReads[0].Origin = %s", got.Origin)
	}
	if string(got.ReaderOptions) != string(result.ReadResult.CapturedRead.ReaderOptions) {
		t.Fatalf("Replay().CapturedReads[0].ReaderOptions = %s, want %s", got.ReaderOptions, result.ReadResult.CapturedRead.ReaderOptions)
	}
	if string(got.Locator) != string(result.ReadResult.CapturedRead.Locator) {
		t.Fatalf("Replay().CapturedReads[0].Locator = %s, want %s", got.Locator, result.ReadResult.CapturedRead.Locator)
	}
	if !bytes.Equal(got.Output, wantOutput) {
		t.Fatalf("Replay().CapturedReads[0].Output = %s, want %s", got.Output, wantOutput)
	}
}

func TestEmitReplayPreservesSpreadsheetCapturedReadMetadata(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_sheet", TenantID: "tenant_1", Status: kernel.RunRunning}
	sourceBytes := spreadsheetWorkbookBytes(t)
	source := sourceWithHash("fixture://amounts.xlsx", sourceBytes)
	binding := memio.NewBinding(source)
	selector := kernel.SourceSelector{
		TableName:        "Amounts",
		RowWindow:        "3-3",
		ColumnProjection: "B",
	}
	clock := &sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)}}

	result, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractspreadsheet.Reader{},
		binding,
		binding,
		clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: source.Source.Path,
			Selector:   selector,
			OutputPath: "$.value",
		},
	)
	if err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}

	wantOutput := []byte(`{"column_count":1,"column_order":"source_column_asc","column_projection":"B","effective_row_end":3,"effective_row_start":3,"formula_policy":"cached_only","normalized_value":"2.20","range":"A1:B3","raw_value":"=B2*2","row_count":1,"row_order":"source_row_asc","row_window":"3-3","rows":[{"cells":[{"a1":"B3","column":2,"formula":"=B2*2","formula_cached_value":"2.20","normalized_value":"2.20","raw_value":"=B2*2","row":3,"sheet":"TrialBalance","value":"2.20"}],"row":3,"sheet":"TrialBalance","window_index":0}],"sheet":"TrialBalance","source_hash":"` + source.Source.SHA256 + `","table_name":"Amounts","value":"2.20"}`)
	if !bytes.Equal(result.ReadResult.Output, wantOutput) {
		t.Fatalf("ReadResult.Output = %s, want %s", result.ReadResult.Output, wantOutput)
	}

	state := snapshotState(t, binding, run)
	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed.CapturedReads) != 1 {
		t.Fatalf("len(Replay().CapturedReads) = %d, want 1", len(replayed.CapturedReads))
	}

	got := replayed.CapturedReads[0]
	if got.ID != result.ReadResult.CapturedRead.ID {
		t.Fatalf("Replay().CapturedReads[0].ID = %s, want %s", got.ID, result.ReadResult.CapturedRead.ID)
	}
	if got.Selector != selector {
		t.Fatalf("Replay().CapturedReads[0].Selector = %#v, want %#v", got.Selector, selector)
	}
	if got.Origin != "fixture://amounts.xlsx#sheet=TrialBalance&row=3&col=2&a1=B3" {
		t.Fatalf("Replay().CapturedReads[0].Origin = %s", got.Origin)
	}
	if string(got.ReaderOptions) != string(result.ReadResult.CapturedRead.ReaderOptions) {
		t.Fatalf("Replay().CapturedReads[0].ReaderOptions = %s, want %s", got.ReaderOptions, result.ReadResult.CapturedRead.ReaderOptions)
	}
	if string(got.Locator) != string(result.ReadResult.CapturedRead.Locator) {
		t.Fatalf("Replay().CapturedReads[0].Locator = %s, want %s", got.Locator, result.ReadResult.CapturedRead.Locator)
	}
	if !bytes.Equal(got.Output, wantOutput) {
		t.Fatalf("Replay().CapturedReads[0].Output = %s, want %s", got.Output, wantOutput)
	}
}

func TestVerifyRejectsUnknownTokenMintPayloadField(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	mutateFirstAuditPayload(t, &doc, func(payload map[string]any) {
		payload["unexpected"] = "extra-field"
	})

	err := bundle.Verify(encodeDocument(t, doc))
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestVerifyAcceptsSupportedNonJSONPointerSelectorPayload(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	selector := kernel.SourceSelector{
		LineWindow: "1-100",
		Cursor:     "line:100",
		JSONPath:   "$.invoice.items[0].amount",
		ByteSpan:   "0-24",
	}
	doc.CapturedReads[0].Selector = selector
	doc.Tokens[0].Source.SourceSelector = selector
	mutateFirstAuditPayload(t, &doc, func(payload map[string]any) {
		payload["source_selector"] = selector
	})

	if err := bundle.Verify(encodeDocument(t, doc)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsAmbiguousSelectorPayload(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	selector := kernel.SourceSelector{
		LineWindow: "1-100",
		RowWindow:  "1-10",
	}
	doc.CapturedReads[0].Selector = selector
	doc.Tokens[0].Source.SourceSelector = selector
	mutateFirstAuditPayload(t, &doc, func(payload map[string]any) {
		payload["source_selector"] = selector
	})

	err := bundle.Verify(encodeDocument(t, doc))
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

type bundleDocument struct {
	Format        string                `json:"format"`
	Run           kernel.Run            `json:"run"`
	CapturedReads []kernel.CapturedRead `json:"captured_reads"`
	Tokens        []kernel.Token        `json:"tokens"`
	AuditJSONL    string                `json:"audit_jsonl"`
}

type sequenceClock struct {
	times []time.Time
	index int
}

func (c *sequenceClock) Now() time.Time {
	if c.index >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	current := c.times[c.index]
	c.index++
	return current
}

func buildPhaseOneState(t *testing.T) (kernel.Run, *memio.Binding) {
	t.Helper()

	return buildStateForRequests(t, []kernel.ReadAndMintRequest{
		{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount"},
			OutputPath: "$.value",
		},
		{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/1/amount"},
			OutputPath: "$.value",
		},
	})
}

func buildSingleMintState(t *testing.T) (kernel.Run, *memio.Binding) {
	t.Helper()

	return buildStateForRequests(t, []kernel.ReadAndMintRequest{
		{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount"},
			OutputPath: "$.value",
		},
	})
}

func multipleTokensOneCapturedReadDocument(t *testing.T) bundleDocument {
	t.Helper()

	run, binding := buildStateForRequests(t, []kernel.ReadAndMintRequest{
		{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount"},
			OutputPath: "$.value",
		},
		{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/1/amount"},
			OutputPath: "/normalized_value",
		},
	})
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))
	if len(doc.CapturedReads) != 2 || len(doc.Tokens) != 2 {
		t.Fatalf("baseline captured reads/tokens = %d/%d, want 2/2", len(doc.CapturedReads), len(doc.Tokens))
	}

	read := capturedReadByID(t, doc.CapturedReads, doc.Tokens[0].Source.CapturedReadID)
	evidence, err := phaseone.ReconstructMintEvidence(run.ID, read.ID, read.Output, "/normalized_value", "")
	if err != nil {
		t.Fatalf("ReconstructMintEvidence() error = %v", err)
	}
	doc.Tokens[1].ID = evidence.TokenID
	doc.Tokens[1].Source.CapturedReadID = read.ID
	doc.Tokens[1].Source.SourceSelector = read.Selector
	doc.Tokens[1].Source.Origin = read.Origin
	doc.Tokens[1].ValueCanonical = evidence.ValueCanonical
	mutateAuditPayload(t, &doc, 1, func(payload map[string]any) {
		payload["token_id"] = evidence.TokenID
		payload["captured_read_id"] = read.ID
		payload["source_path"] = read.SourcePath
		payload["source_selector"] = read.Selector
		payload["origin"] = read.Origin
		payload["reader_name"] = read.ReaderName
		payload["reader_version"] = read.ReaderVersion
		payload["reader_options"] = json.RawMessage(read.ReaderOptions)
		payload["locator"] = json.RawMessage(read.Locator)
		payload["source_sha256"] = read.SourceSHA256
		payload["output_sha256"] = read.OutputSHA256
		payload["source_value_raw"] = evidence.SourceValueRaw
		payload["value_canonical"] = evidence.ValueCanonical
	})

	return doc
}

func capturedReadByID(t *testing.T, reads []kernel.CapturedRead, id string) kernel.CapturedRead {
	t.Helper()

	for _, read := range reads {
		if read.ID == id {
			return read
		}
	}
	t.Fatalf("captured read %s not found", id)
	return kernel.CapturedRead{}
}

func buildStateForRequests(t *testing.T, requests []kernel.ReadAndMintRequest) (kernel.Run, *memio.Binding) {
	t.Helper()

	sourceBytes := []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)
	binding := memio.NewBinding(sourceWithHash("fixture://invoice.json", sourceBytes))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	clock := &sequenceClock{times: []time.Time{
		time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 18, 10, 5, 0, 0, time.UTC),
	}}
	for _, request := range requests {
		if _, err := kernel.ReadAndMint(
			context.Background(),
			binding,
			binding,
			extractjson.Reader{},
			binding,
			binding,
			clock,
			run,
			request,
		); err != nil {
			t.Fatalf("ReadAndMint() error = %v", err)
		}
	}

	return run, binding
}

func snapshotState(t *testing.T, binding *memio.Binding, run kernel.Run) bundle.State {
	t.Helper()

	var state bundle.State
	if err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		reads, err := binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		tokens, err := binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		auditEntries, err := binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		state = bundle.State{
			Run:           run,
			CapturedReads: reads,
			Tokens:        tokens,
			AuditEntries:  auditEntries,
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
	return state
}

func mustEmitState(t *testing.T, state bundle.State) []byte {
	t.Helper()

	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	return bundleBytes
}

func sourceWithHash(path string, data []byte) kernel.SourceBytes {
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:      path,
			MediaType: "application/json",
			SHA256:    sha256Hex(data),
		},
		Bytes: data,
	}
}

func spreadsheetWorkbookBytes(t *testing.T) []byte {
	t.Helper()

	raw, err := testxlsx.Build(testxlsx.Workbook{
		Sheets: []testxlsx.Sheet{
			{
				Name:      "TrialBalance",
				Dimension: "A1:B3",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "1.10"},
					{A1: "A3", Value: "ar"},
					{A1: "B3", Formula: "=B2*2", CachedValue: "2.20"},
				},
				Tables: []testxlsx.Table{{Name: "Amounts", Range: "A1:B3"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("testxlsx.Build() error = %v", err)
	}
	return raw
}

func decodeDocument(t *testing.T, bundleBytes []byte) bundleDocument {
	t.Helper()

	var doc bundleDocument
	if err := json.Unmarshal(bundleBytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal(bundleBytes) error = %v", err)
	}
	return doc
}

func encodeDocument(t *testing.T, doc bundleDocument) []byte {
	t.Helper()

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal(doc) error = %v", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatalf("jcs.Transform(doc) error = %v", err)
	}
	return canonical
}

func mutateFirstAuditPayload(t *testing.T, doc *bundleDocument, mutate func(payload map[string]any)) {
	t.Helper()
	mutateAuditPayload(t, doc, 0, mutate)
}

func mutateAuditPayload(t *testing.T, doc *bundleDocument, index int, mutate func(payload map[string]any)) {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(doc.AuditJSONL, "\n"), "\n")
	if index < 0 || index >= len(lines) || lines[index] == "" {
		t.Fatalf("AuditJSONL has no line %d", index)
	}

	line := decodeAuditLine(t, lines[index])

	var payload map[string]any
	if err := json.Unmarshal(line.PayloadDeterministic, &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	mutate(payload)

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(payload) error = %v", err)
	}
	line.PayloadDeterministic, err = jcs.Transform(rawPayload)
	if err != nil {
		t.Fatalf("jcs.Transform(payload) error = %v", err)
	}
	line.Hash = recomputeAuditHash(t, line)

	rawLine, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("json.Marshal(line) error = %v", err)
	}
	canonicalLine, err := jcs.Transform(rawLine)
	if err != nil {
		t.Fatalf("jcs.Transform(line) error = %v", err)
	}
	lines[index] = string(canonicalLine)
	doc.AuditJSONL = strings.Join(lines, "\n") + "\n"
}

type auditLineDocument struct {
	At                      string           `json:"at"`
	Hash                    string           `json:"hash"`
	Kind                    kernel.EntryKind `json:"kind"`
	PayloadDeterministic    json.RawMessage  `json:"payload_deterministic"`
	PayloadNonDeterministic json.RawMessage  `json:"payload_non_deterministic,omitempty"`
	PrevHash                string           `json:"prev_hash"`
	RunID                   string           `json:"run_id"`
	Seq                     uint64           `json:"seq"`
	TenantID                string           `json:"tenant_id"`
}

func decodeAuditLine(t *testing.T, line string) auditLineDocument {
	t.Helper()

	var parsed auditLineDocument
	dec := json.NewDecoder(strings.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("Decode(audit line) error = %v", err)
	}
	return parsed
}

func recomputeAuditHash(t *testing.T, line auditLineDocument) string {
	t.Helper()

	prevHash, err := hex.DecodeString(line.PrevHash)
	if err != nil {
		t.Fatalf("DecodeString(prev_hash) error = %v", err)
	}

	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, line.Seq)

	h := sha256.New()
	h.Write(prevHash)
	h.Write(seqBytes)
	h.Write([]byte(line.RunID))
	h.Write([]byte(line.TenantID))
	h.Write([]byte(line.Kind))
	h.Write(line.PayloadDeterministic)
	return hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
