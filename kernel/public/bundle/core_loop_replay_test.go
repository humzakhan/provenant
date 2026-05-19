package bundle_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/internal/phaseone"
	"github.com/provenant/provenant/kernel/public/memio"
)

func TestBundleReplayReconstructsCoreLoopState(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	state := snapshotState(t, binding, run)

	if len(state.CapturedReads) != 1 {
		t.Fatalf("len(state.CapturedReads) = %d, want 1", len(state.CapturedReads))
	}
	if state.CapturedReads[0].ReaderName != "extract.json" {
		t.Fatalf("CapturedReads[0].ReaderName = %q, want %q", state.CapturedReads[0].ReaderName, "extract.json")
	}
	if len(state.Tokens) != 1 {
		t.Fatalf("len(state.Tokens) = %d, want 1", len(state.Tokens))
	}
	if len(state.AuditEntries) != 1 {
		t.Fatalf("len(state.AuditEntries) = %d, want 1", len(state.AuditEntries))
	}
	if state.Tokens[0].Source.CapturedReadID != state.CapturedReads[0].ID {
		t.Fatalf("Tokens[0].Source.CapturedReadID = %s, want %s", state.Tokens[0].Source.CapturedReadID, state.CapturedReads[0].ID)
	}

	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
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
	if len(replayed.CapturedReads) != 1 {
		t.Fatalf("len(Replay().CapturedReads) = %d, want 1", len(replayed.CapturedReads))
	}
	if len(replayed.Tokens) != 1 {
		t.Fatalf("len(Replay().Tokens) = %d, want 1", len(replayed.Tokens))
	}
	if len(replayed.AuditEntries) != 1 {
		t.Fatalf("len(Replay().AuditEntries) = %d, want 1", len(replayed.AuditEntries))
	}
	if replayed.CapturedReads[0].ReaderName != "extract.json" {
		t.Fatalf("Replay().CapturedReads[0].ReaderName = %q, want %q", replayed.CapturedReads[0].ReaderName, "extract.json")
	}
	if replayed.CapturedReads[0].OutputSHA256 != state.CapturedReads[0].OutputSHA256 {
		t.Fatalf("Replay().CapturedReads[0].OutputSHA256 = %s, want %s", replayed.CapturedReads[0].OutputSHA256, state.CapturedReads[0].OutputSHA256)
	}
	if replayed.Tokens[0].ID != state.Tokens[0].ID {
		t.Fatalf("Replay().Tokens[0].ID = %s, want %s", replayed.Tokens[0].ID, state.Tokens[0].ID)
	}
	if replayed.Tokens[0].ValueCanonical != state.Tokens[0].ValueCanonical {
		t.Fatalf("Replay().Tokens[0].ValueCanonical = %s, want %s", replayed.Tokens[0].ValueCanonical, state.Tokens[0].ValueCanonical)
	}
	if replayed.Tokens[0].Source != state.Tokens[0].Source {
		t.Fatalf("Replay().Tokens[0].Source = %#v, want %#v", replayed.Tokens[0].Source, state.Tokens[0].Source)
	}
	if replayed.AuditEntries[0].Seq != 1 {
		t.Fatalf("Replay().AuditEntries[0].Seq = %d, want 1", replayed.AuditEntries[0].Seq)
	}
	if !bytes.Equal(replayed.AuditEntries[0].Hash, state.AuditEntries[0].Hash) {
		t.Fatalf("Replay().AuditEntries[0].Hash = %x, want %x", replayed.AuditEntries[0].Hash, state.AuditEntries[0].Hash)
	}

	replayedBytes, err := bundle.Emit(replayed)
	if err != nil {
		t.Fatalf("Emit(Replay()) error = %v", err)
	}
	if !bytes.Equal(replayedBytes, bundleBytes) {
		t.Fatal("Emit(Replay()) bytes differ from original bundle")
	}
}

func TestBundleVerifyRejectsCapturedReadOutputHashMutation(t *testing.T) {
	t.Parallel()

	run, binding := buildSingleMintState(t)
	doc := decodeDocument(t, mustEmitState(t, snapshotState(t, binding, run)))

	original := doc.CapturedReads[0].OutputSHA256
	if original == "" {
		t.Fatal("CapturedReads[0].OutputSHA256 is empty")
	}
	if original[len(original)-1] == '0' {
		doc.CapturedReads[0].OutputSHA256 = original[:len(original)-1] + "1"
	} else {
		doc.CapturedReads[0].OutputSHA256 = original[:len(original)-1] + "0"
	}

	err := bundle.Verify(encodeDocument(t, doc))
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestBundleReplayPreservesNumericJSONValueEvidence(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"totals":[1e3,1.2300]}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)
	rawValue := `1e3`
	byteStart, byteEnd := mustUniqueBundleSpan(t, sourceBytes, rawValue)
	selector := kernel.SourceSelector{
		JSONPointer: "/invoice/totals/0",
		JSONPath:    "$.invoice.totals[0]",
		ByteSpan:    fmt.Sprintf("%d-%d", byteStart, byteEnd),
	}

	binding := memio.NewBinding(source)
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	clock := &sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)}}

	result, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractjson.Reader{},
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

	wantOutput := []byte(fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/invoice/totals/0","jsonpath":"$.invoice.totals[0]","normalized_value":1000,"raw_value":"1e3","source_hash":"%s","value":1000}`, byteEnd, byteStart, source.Source.SHA256))
	if !bytes.Equal(result.ReadResult.Output, wantOutput) {
		t.Fatalf("ReadResult.Output = %s, want %s", result.ReadResult.Output, wantOutput)
	}
	if result.Token.ValueCanonical != "1000.000000000" {
		t.Fatalf("Token.ValueCanonical = %s, want %s", result.Token.ValueCanonical, "1000.000000000")
	}

	evidence, err := phaseone.ReconstructMintEvidence(run.ID, result.ReadResult.CapturedRead.ID, result.ReadResult.Output, "$.value", "")
	if err != nil {
		t.Fatalf("ReconstructMintEvidence() error = %v", err)
	}
	if evidence.SourceValueRaw != "1000" {
		t.Fatalf("MintEvidence.SourceValueRaw = %s, want %s", evidence.SourceValueRaw, "1000")
	}
	if evidence.ValueCanonical != result.Token.ValueCanonical {
		t.Fatalf("MintEvidence.ValueCanonical = %s, want %s", evidence.ValueCanonical, result.Token.ValueCanonical)
	}

	state := snapshotState(t, binding, run)
	bundleBytes := mustEmitState(t, state)
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed.CapturedReads) != 1 {
		t.Fatalf("len(Replay().CapturedReads) = %d, want 1", len(replayed.CapturedReads))
	}
	if !bytes.Equal(replayed.CapturedReads[0].Output, wantOutput) {
		t.Fatalf("Replay().CapturedReads[0].Output = %s, want %s", replayed.CapturedReads[0].Output, wantOutput)
	}
	if replayed.CapturedReads[0].Selector != selector {
		t.Fatalf("Replay().CapturedReads[0].Selector = %#v, want %#v", replayed.CapturedReads[0].Selector, selector)
	}
	wantOrigin := fmt.Sprintf("fixture://invoice.json#ptr=/invoice/totals/0&byte=%d-%d", byteStart, byteEnd)
	if replayed.CapturedReads[0].Origin != wantOrigin {
		t.Fatalf("Replay().CapturedReads[0].Origin = %s, want %s", replayed.CapturedReads[0].Origin, wantOrigin)
	}
	if replayed.Tokens[0].ValueCanonical != "1000.000000000" {
		t.Fatalf("Replay().Tokens[0].ValueCanonical = %s, want %s", replayed.Tokens[0].ValueCanonical, "1000.000000000")
	}
}

func mustUniqueBundleSpan(t *testing.T, source []byte, raw string) (int, int) {
	t.Helper()

	start := strings.Index(string(source), raw)
	if start < 0 {
		t.Fatalf("raw value %q not found in %s", raw, source)
	}
	if strings.Count(string(source), raw) != 1 {
		t.Fatalf("raw value %q appears multiple times in %s", raw, source)
	}
	return start, start + len(raw)
}
