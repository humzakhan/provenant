package bundle_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/provenant/provenant/kernel/public/bundle"
)

func TestPhaseOneCoreLoopEndToEnd(t *testing.T) {
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

func TestPhaseOneCoreLoopFailsClosedOnCapturedReadHashMutation(t *testing.T) {
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
