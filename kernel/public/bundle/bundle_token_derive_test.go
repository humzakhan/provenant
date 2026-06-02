package bundle_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gowebpki/jcs"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/bundle"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

// buildDeriveState constructs a bundle.State that contains one real leaf
// token produced by ReadAndMint and one derived token. It returns the state
// and the emitted canonical bundle bytes.
//
// The leaf token is minted from a JSON source file and has seq 1. The derived
// token uses operation "calc.add" over the leaf (twice) and has seq 2.
func buildDeriveState(t *testing.T) (bundle.State, []byte) {
	t.Helper()

	run := kernel.Run{ID: "run_derive", TenantID: "tenant_derive", Status: kernel.RunRunning}
	sourceBytes := []byte(`{"invoice":{"total":"100.00"}}`)
	src := kernel.SourceBytes{
		Source: kernel.Source{
			Path:      "fixture://derive.json",
			MediaType: "application/json",
			SHA256:    sha256HexDeriveTest(sourceBytes),
		},
		Bytes: sourceBytes,
	}
	binding := memio.NewBinding(src)
	clock := &sequenceClock{times: []time.Time{
		time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
	}}

	result, err := kernel.ReadAndMint(
		context.Background(),
		binding, binding,
		extractjson.Reader{},
		binding, binding,
		clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://derive.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
			OutputPath: "$.value",
			Unit:       "USD",
		},
	)
	if err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}

	leafToken := result.Token
	leafEntry := result.AuditEntry

	// Build the token_derive deterministic payload for seq 2.
	// The derived token is "calc.add" of the leaf token with itself.
	// evaluated = leaf.ValueCanonical (100.000000000) + leaf.ValueCanonical = 200.000000000
	derivedValue := "200.000000000"
	derivedTokenID := "tok_derive_test_00000000000000"

	derivedToken := kernel.Token{
		ID:             derivedTokenID,
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: derivedValue,
		Unit:           "USD",
		Derivation: &kernel.Derivation{
			Operation: "calc.add",
			Inputs:    []string{leafToken.ID, leafToken.ID},
			FormulaID: "calc.add",
			Rounding:  kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
			Evaluated: derivedValue,
		},
	}

	derivePayloadMap := map[string]any{
		"evaluated":  derivedValue,
		"formula_id": "calc.add",
		"inputs":     []string{leafToken.ID, leafToken.ID},
		"operation":  "calc.add",
		"rounding":   map[string]any{"mode": "half_even", "scale": 9},
		"token_id":   derivedTokenID,
		"unit":       "USD",
	}
	deriveDet := canonicalJSONDeriveTest(t, derivePayloadMap)
	seq2Hash := deriveTestAuditHash(leafEntry.Hash, 2, run.ID, run.TenantID, kernel.EntryKindTokenDerive, deriveDet)

	deriveEntry := kernel.AuditEntry{
		Seq:      2,
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     kernel.EntryKindTokenDerive,
		At:       time.Date(2026, 5, 20, 0, 1, 0, 0, time.UTC),
		PrevHash: leafEntry.Hash,
		Hash:     seq2Hash,
		Payload: kernel.PayloadEnvelope{
			Deterministic:      deriveDet,
			HashInputCanonical: deriveDet,
		},
	}

	reads, err := readsFromBinding(t, binding, run.ID)
	if err != nil {
		t.Fatalf("reads from binding error = %v", err)
	}

	state := bundle.State{
		Run:           run,
		CapturedReads: reads,
		Tokens:        []kernel.Token{leafToken, derivedToken},
		AuditEntries:  []kernel.AuditEntry{leafEntry, deriveEntry},
	}

	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	return state, bundleBytes
}

func readsFromBinding(t *testing.T, binding *memio.Binding, runID string) ([]kernel.CapturedRead, error) {
	t.Helper()
	var reads []kernel.CapturedRead
	err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		var err error
		reads, err = binding.CapturedReads(ctx, tx, runID)
		return err
	})
	return reads, err
}

// TestDerivedTokenBundleVerifyAndReplay is the primary positive-path test.
func TestDerivedTokenBundleVerifyAndReplay(t *testing.T) {
	t.Parallel()

	_, bundleBytes := buildDeriveState(t)

	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	replayed, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed.Tokens) != 2 {
		t.Fatalf("len(Replay().Tokens) = %d, want 2", len(replayed.Tokens))
	}
	if len(replayed.AuditEntries) != 2 {
		t.Fatalf("len(Replay().AuditEntries) = %d, want 2", len(replayed.AuditEntries))
	}

	// Find the derived token in the replayed state.
	var derived *kernel.Token
	for i := range replayed.Tokens {
		if replayed.Tokens[i].Derivation != nil {
			tok := replayed.Tokens[i]
			derived = &tok
		}
	}
	if derived == nil {
		t.Fatal("Replay() produced no derived token")
		return
	}
	derivation := derived.Derivation
	if derivation == nil {
		t.Fatal("Replay() derived token has nil Derivation")
		return
	}
	if derivation.Operation != "calc.add" {
		t.Fatalf("Derivation.Operation = %q, want %q", derivation.Operation, "calc.add")
	}
	if derived.ValueCanonical != "200.000000000" {
		t.Fatalf("ValueCanonical = %q, want %q", derived.ValueCanonical, "200.000000000")
	}
	if derived.Source != (kernel.TokenSource{}) {
		t.Fatalf("derived token Source = %#v, want zero value", derived.Source)
	}

	// Emit(Replay()) must produce bit-identical bytes.
	rebundled, err := bundle.Emit(replayed)
	if err != nil {
		t.Fatalf("Emit(Replay()) error = %v", err)
	}
	if !bytes.Equal(bundleBytes, rebundled) {
		t.Fatal("Emit(Replay()) bytes differ from original bundle")
	}
}

// TestDerivedTokenNegativeCases covers every rejection listed in the phase spec.
func TestDerivedTokenNegativeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(t *testing.T, doc *bundleDocument)
	}{
		{
			name: "forward ref: input token not yet seen",
			mutate: func(t *testing.T, doc *bundleDocument) {
				mutateAuditPayload(t, doc, 1, func(p map[string]any) {
					p["inputs"] = []string{"tok_does_not_exist_yet"}
				})
			},
		},
		{
			name: "empty inputs array",
			mutate: func(t *testing.T, doc *bundleDocument) {
				mutateAuditPayload(t, doc, 1, func(p map[string]any) {
					p["inputs"] = []string{}
				})
			},
		},
		{
			name: "unknown rounding mode",
			mutate: func(t *testing.T, doc *bundleDocument) {
				mutateAuditPayload(t, doc, 1, func(p map[string]any) {
					p["rounding"] = map[string]any{"mode": "ceiling", "scale": 9}
				})
			},
		},
		{
			name: "evaluated != token ValueCanonical",
			mutate: func(t *testing.T, doc *bundleDocument) {
				// Change the token's values without changing the payload, so
				// validateDerivedTokenAgainstPayload catches the mismatch.
				for i, tok := range doc.Tokens {
					if tok.Derivation != nil {
						doc.Tokens[i].ValueCanonical = "999.000000000"
						doc.Tokens[i].Derivation.Evaluated = "999.000000000"
					}
				}
			},
		},
		{
			name: "mixed Source and Derivation: leaf token has non-nil Derivation",
			mutate: func(t *testing.T, doc *bundleDocument) {
				// The first token in the deterministic-sorted slice is the leaf.
				// Attach a Derivation to it so the leaf-mint validator rejects.
				for i, tok := range doc.Tokens {
					if tok.Derivation == nil {
						doc.Tokens[i].Derivation = &kernel.Derivation{
							Operation: "calc.add",
							Inputs:    []string{tok.ID},
							Evaluated: tok.ValueCanonical,
						}
						break
					}
				}
			},
		},
		{
			name: "mixed Source and Derivation: derived token has non-zero Source",
			mutate: func(t *testing.T, doc *bundleDocument) {
				for i, tok := range doc.Tokens {
					if tok.Derivation != nil {
						doc.Tokens[i].Source = kernel.TokenSource{
							CapturedReadID: doc.CapturedReads[0].ID,
							SourcePath:     doc.CapturedReads[0].SourcePath,
							SourceSelector: doc.CapturedReads[0].Selector,
							OutputPath:     "$.value",
						}
					}
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, bundleBytes := buildDeriveState(t)
			doc := decodeDocument(t, bundleBytes)
			tt.mutate(t, &doc)

			err := bundle.Verify(encodeDocument(t, doc))
			if !errors.Is(err, bundle.ErrTamperDetected) {
				t.Fatalf("Verify() error = %v, want %v", err, bundle.ErrTamperDetected)
			}
		})
	}
}

// TestLeafOnlyBundleBackwardCompat is the golden regression: a leaf-only
// bundle (no derive entries) must still verify and replay byte-identically.
func TestLeafOnlyBundleBackwardCompat(t *testing.T) {
	t.Parallel()

	run, binding := buildPhaseOneState(t)
	state := snapshotState(t, binding, run)

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
	rebundled, err := bundle.Emit(replayed)
	if err != nil {
		t.Fatalf("Emit(Replay()) error = %v", err)
	}
	if !bytes.Equal(bundleBytes, rebundled) {
		t.Fatal("Emit(Replay()) bytes differ — leaf-only backward compat broken")
	}
}

// ---------------------------------------------------------------------------
// Package-local test helpers (names avoid clashing with bundle_test.go)
// ---------------------------------------------------------------------------

func sha256HexDeriveTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func deriveTestAuditHash(prevHash []byte, seq uint64, runID, tenantID string, kind kernel.EntryKind, hashInput []byte) []byte {
	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, seq)
	h := sha256.New()
	h.Write(prevHash)
	h.Write(seqBytes)
	h.Write([]byte(runID))
	h.Write([]byte(tenantID))
	h.Write([]byte(kind))
	h.Write(hashInput)
	return h.Sum(nil)
}

func canonicalJSONDeriveTest(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatalf("jcs.Transform() error = %v", err)
	}
	return canonical
}
