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
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/extractjsonl"
	"github.com/provenant/provenant/kernel/public/internal/phaseone"
	"github.com/provenant/provenant/kernel/public/memio"
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

func TestReplayIgnoresUnauditedTokenFields(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	doc.Tokens[0].CreatedAt = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	doc.Tokens[0].Derivation = &kernel.Derivation{
		Operation: "sum",
		Inputs:    []string{"tok_a", "tok_b"},
		FormulaID: "arith.sum",
	}

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
	if replayed.Tokens[0].Derivation != nil {
		t.Fatalf("Replay() token Derivation = %#v, want nil", replayed.Tokens[0].Derivation)
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

	lines := strings.Split(strings.TrimSuffix(doc.AuditJSONL, "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("AuditJSONL has no lines")
	}

	line := decodeAuditLine(t, lines[0])

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
	lines[0] = string(canonicalLine)
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
