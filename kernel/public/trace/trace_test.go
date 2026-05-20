package trace_test

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

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
	"github.com/provenant/provenant/kernel/public/trace"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var traceTestClock = fixedClock{t: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}

// sourceBytes builds a SourceBytes for a JSON snippet.
func sourceBytes(path string, content []byte) kernel.SourceBytes {
	sum := sha256.Sum256(content)
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:   path,
			SHA256: hex.EncodeToString(sum[:]),
		},
		Bytes: content,
	}
}

// mintLeafInBinding mints a leaf token into binding at seq=1 (genesis run).
// Returns the token and audit entry.
func mintLeafInBinding(
	t *testing.T,
	binding *memio.Binding,
	run kernel.Run,
	src kernel.SourceBytes,
	pointer, unit string,
) (kernel.Token, kernel.AuditEntry) {
	t.Helper()
	result, err := kernel.ReadAndMint(
		context.Background(),
		binding, binding,
		extractjson.Reader{},
		binding, binding,
		traceTestClock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: src.Source.Path,
			Selector:   kernel.SourceSelector{JSONPointer: pointer},
			OutputPath: "$.value",
			Unit:       unit,
		},
	)
	if err != nil {
		t.Fatalf("mintLeafInBinding ReadAndMint: %v", err)
	}
	return result.Token, result.AuditEntry
}

// getReads fetches captured reads from a binding.
func getReads(t *testing.T, b *memio.Binding, run kernel.Run) []kernel.CapturedRead {
	t.Helper()
	var reads []kernel.CapturedRead
	if err := b.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		var err error
		reads, err = b.CapturedReads(ctx, tx, run.ID)
		return err
	}); err != nil {
		t.Fatalf("getReads: %v", err)
	}
	return reads
}

// canonicalJSON marshals v to JCS-canonical JSON.
func canonicalJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("canonicalJSON json.Marshal: %v", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatalf("canonicalJSON jcs.Transform: %v", err)
	}
	return canonical
}

// genesisHash mirrors the bundle's internal genesis hash.
func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

// auditHash mirrors the bundle's internal auditHash.
func auditHash(prevHash []byte, seq uint64, runID, tenantID string, kind kernel.EntryKind, hashInput []byte) []byte {
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

// deriveTokenID computes a short test token ID from its inputs.
func deriveTokenID(runID, tenantID, op string, inputIDs []string, value, unit string) string {
	h := sha256.New()
	h.Write([]byte(runID))
	h.Write([]byte(tenantID))
	h.Write([]byte(op))
	for _, id := range inputIDs {
		h.Write([]byte(id))
	}
	h.Write([]byte(value))
	h.Write([]byte(unit))
	return "tok_" + hex.EncodeToString(h.Sum(nil))[:24]
}

// buildDeriveEntry constructs a derived token + audit entry at the given seq,
// chaining from prevHash.
func buildDeriveEntry(
	t *testing.T,
	run kernel.Run,
	op string,
	inputs []kernel.Token,
	evaluatedValue, unit string,
	prevHash []byte,
	seq uint64,
) (kernel.Token, kernel.AuditEntry) {
	t.Helper()
	return buildDeriveEntryRounding(t, run, op, inputs, evaluatedValue, unit, prevHash, seq,
		kernel.RoundingPolicy{Mode: "half_even", Scale: 9})
}

func buildDeriveEntryRounding(
	t *testing.T,
	run kernel.Run,
	op string,
	inputs []kernel.Token,
	evaluatedValue, unit string,
	prevHash []byte,
	seq uint64,
	rounding kernel.RoundingPolicy,
) (kernel.Token, kernel.AuditEntry) {
	t.Helper()

	inputIDs := make([]string, len(inputs))
	for i, tok := range inputs {
		inputIDs[i] = tok.ID
	}

	tokenID := deriveTokenID(run.ID, run.TenantID, op, inputIDs, evaluatedValue, unit)

	token := kernel.Token{
		ID:             tokenID,
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: evaluatedValue,
		Unit:           unit,
		Derivation: &kernel.Derivation{
			Operation: op,
			Inputs:    append([]string(nil), inputIDs...),
			FormulaID: op,
			Rounding:  rounding,
			Evaluated: evaluatedValue,
		},
	}

	payloadMap := map[string]any{
		"evaluated":  evaluatedValue,
		"formula_id": op,
		"inputs":     inputIDs,
		"operation":  op,
		"token_id":   tokenID,
	}
	if unit != "" {
		payloadMap["unit"] = unit
	}
	if rounding.Mode != "" || rounding.Scale != 0 {
		payloadMap["rounding"] = map[string]any{
			"mode":  rounding.Mode,
			"scale": rounding.Scale,
		}
	}

	det := canonicalJSON(t, payloadMap)
	hash := auditHash(prevHash, seq, run.ID, run.TenantID, kernel.EntryKindTokenDerive, det)

	entry := kernel.AuditEntry{
		Seq:      seq,
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     kernel.EntryKindTokenDerive,
		At:       traceTestClock.Now(),
		PrevHash: append([]byte(nil), prevHash...),
		Hash:     hash,
		Payload: kernel.PayloadEnvelope{
			Deterministic:      det,
			HashInputCanonical: det,
		},
	}
	return token, entry
}

// rebuildLeafEntry re-issues a leaf's audit entry at the correct seq and prevHash
// for multi-leaf bundles. The binding already holds the captured read; we just
// need to recompute the chain link.
func rebuildLeafEntry(
	t *testing.T,
	run kernel.Run,
	origEntry kernel.AuditEntry,
	prevHash []byte,
	seq uint64,
) kernel.AuditEntry {
	t.Helper()
	det := origEntry.Payload.Deterministic
	hash := auditHash(prevHash, seq, run.ID, run.TenantID, kernel.EntryKindTokenMint, det)
	return kernel.AuditEntry{
		Seq:      seq,
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     kernel.EntryKindTokenMint,
		At:       origEntry.At,
		PrevHash: append([]byte(nil), prevHash...),
		Hash:     hash,
		Payload:  origEntry.Payload,
	}
}

// concat concatenates multiple slices.
func concat[T any](ss ...[]T) []T {
	var result []T
	for _, s := range ss {
		result = append(result, s...)
	}
	return result
}

// assertContrib finds a leaf by token ID and asserts its Contribution.
func assertContrib(t *testing.T, leaves []trace.LeafContribution, tokenID, want string) {
	t.Helper()
	for _, lc := range leaves {
		if lc.TokenID == tokenID {
			if lc.Contribution != want {
				t.Errorf("leaf %s contribution = %q, want %q", tokenID, lc.Contribution, want)
			}
			return
		}
	}
	t.Errorf("leaf %q not found in contributions (have %d leaves)", tokenID, len(leaves))
}

// assertRoundingResidual finds the synthetic <rounding> contribution and
// checks its value.
func assertRoundingResidual(t *testing.T, leaves []trace.LeafContribution, want string) {
	t.Helper()
	for _, lc := range leaves {
		if lc.SourceName == "<rounding>" {
			if lc.Contribution != want {
				t.Errorf("<rounding> contribution = %q, want %q", lc.Contribution, want)
			}
			return
		}
	}
	t.Errorf("no <rounding> synthetic contribution found in %d leaves", len(leaves))
}

func hasRoundingResidual(leaves []trace.LeafContribution) bool {
	for _, lc := range leaves {
		if lc.SourceName == "<rounding>" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestTraceLeaf: tracing a leaf token returns a single-node graph with one
// leaf contribution equal to the leaf's own canonical value.
func TestTraceLeaf(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_leaf", TenantID: "tenant_tr", Status: kernel.RunRunning}
	src := sourceBytes("fixture://leaf.json", []byte(`{"amount":42.5}`))
	binding := memio.NewBinding(src)

	leaf, _ := mintLeafInBinding(t, binding, run, src, "/amount", "USD")

	var reads []kernel.CapturedRead
	var tokens []kernel.Token
	var entries []kernel.AuditEntry
	_ = binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		var err error
		reads, err = binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		tokens, err = binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		entries, err = binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		return err
	})

	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: reads,
		Tokens:        tokens,
		AuditEntries:  entries,
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, leaf.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	if g.Root != leaf.ID {
		t.Errorf("Root = %q, want %q", g.Root, leaf.ID)
	}
	if len(g.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(g.Nodes))
	}
	if g.Nodes[0].Kind != "leaf" {
		t.Errorf("Node.Kind = %q, want leaf", g.Nodes[0].Kind)
	}
	if len(g.Edges) != 0 {
		t.Errorf("Edges count = %d, want 0", len(g.Edges))
	}
	if len(g.Leaves) != 1 {
		t.Fatalf("Leaves count = %d, want 1", len(g.Leaves))
	}
	if g.Leaves[0].Contribution != leaf.ValueCanonical {
		t.Errorf("leaf contribution = %q, want %q", g.Leaves[0].Contribution, leaf.ValueCanonical)
	}
	if g.Leaves[0].TokenID != leaf.ID {
		t.Errorf("leaf token_id = %q, want %q", g.Leaves[0].TokenID, leaf.ID)
	}
	if g.Nodes[0].AuditSeq == 0 {
		t.Error("Node.AuditSeq should be > 0")
	}
}

// TestTraceSubtract: calc.subtract(a, b) gives a positive, b negative; sum == root.
func TestTraceSubtract(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_sub", TenantID: "tenant_tr", Status: kernel.RunRunning}

	src1 := sourceBytes("fixture://sub_a.json", []byte(`{"val":100}`))
	bind1 := memio.NewBinding(src1)
	leafA, entryA1 := mintLeafInBinding(t, bind1, run, src1, "/val", "USD")

	src2 := sourceBytes("fixture://sub_b.json", []byte(`{"val":30}`))
	bind2 := memio.NewBinding(src2)
	leafB, entryB1 := mintLeafInBinding(t, bind2, run, src2, "/val", "USD")

	// Re-chain entryA at seq=1 from genesis, entryB at seq=2 from entryA.
	prevA := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, entryA1, prevA, 1)
	entryB := rebuildLeafEntry(t, run, entryB1, entryA.Hash, 2)

	// derive = subtract(A, B) = 70 USD
	derivedTok, deriveEntry := buildDeriveEntry(t, run, "calc.subtract",
		[]kernel.Token{leafA, leafB}, "70.000000000", "USD", entryB.Hash, 3)

	allReads := concat(
		getReads(t, bind1, run),
		getReads(t, bind2, run),
	)
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        []kernel.Token{leafA, leafB, derivedTok},
		AuditEntries:  []kernel.AuditEntry{entryA, entryB, deriveEntry},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	if len(g.Leaves) != 2 {
		t.Fatalf("Leaves count = %d, want 2", len(g.Leaves))
	}
	assertContrib(t, g.Leaves, leafA.ID, "100.000000000")
	assertContrib(t, g.Leaves, leafB.ID, "-30.000000000")
	if g.RootValue != "70.000000000" {
		t.Errorf("RootValue = %q, want 70.000000000", g.RootValue)
	}
}

// TestTraceSum5: calc.sum of 5 inputs — contributions sum to root.
func TestTraceSum5(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_sum5", TenantID: "tenant_tr", Status: kernel.RunRunning}

	var leafTokens []kernel.Token
	var allEntries []kernel.AuditEntry
	var allReads []kernel.CapturedRead

	var prevHash []byte
	prevHash = genesisHash(run.TenantID)

	for i := 0; i < 5; i++ {
		letter := string(rune('a' + i))
		content := []byte(`{"val":10}`)
		path := "fixture://sum_" + letter + ".json"
		src := sourceBytes(path, content)
		b := memio.NewBinding(src)
		tok, rawEntry := mintLeafInBinding(t, b, run, src, "/val", "USD")
		seq := uint64(i + 1)
		entry := rebuildLeafEntry(t, run, rawEntry, prevHash, seq)
		leafTokens = append(leafTokens, tok)
		allEntries = append(allEntries, entry)
		allReads = append(allReads, getReads(t, b, run)...)
		prevHash = entry.Hash
	}

	derivedTok, deriveEntry := buildDeriveEntry(t, run, "calc.sum",
		leafTokens, "50.000000000", "USD", prevHash, 6)
	allEntries = append(allEntries, deriveEntry)

	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        append(leafTokens, derivedTok),
		AuditEntries:  allEntries,
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	if len(g.Leaves) != 5 {
		t.Fatalf("Leaves count = %d, want 5", len(g.Leaves))
	}
	for _, lc := range g.Leaves {
		if lc.Contribution != "10.000000000" {
			t.Errorf("leaf %s contribution = %q, want 10.000000000", lc.TokenID, lc.Contribution)
		}
	}
}

// TestTraceNestedDerive: calc.add(A, calc.subtract(B, C)).
// Leaves: A=+100, B=+50, C=-20; sum=130.
func TestTraceNestedDerive(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_nested", TenantID: "tenant_tr", Status: kernel.RunRunning}

	srcA := sourceBytes("fixture://n_a.json", []byte(`{"val":100}`))
	srcB := sourceBytes("fixture://n_b.json", []byte(`{"val":50}`))
	srcC := sourceBytes("fixture://n_c.json", []byte(`{"val":20}`))

	bindA := memio.NewBinding(srcA)
	bindB := memio.NewBinding(srcB)
	bindC := memio.NewBinding(srcC)

	leafA, rawA := mintLeafInBinding(t, bindA, run, srcA, "/val", "USD")
	leafB, rawB := mintLeafInBinding(t, bindB, run, srcB, "/val", "USD")
	leafC, rawC := mintLeafInBinding(t, bindC, run, srcC, "/val", "USD")

	prevA := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, rawA, prevA, 1)
	entryB := rebuildLeafEntry(t, run, rawB, entryA.Hash, 2)
	entryC := rebuildLeafEntry(t, run, rawC, entryB.Hash, 3)

	// derive1 = subtract(B, C) = 30
	derive1, entry4 := buildDeriveEntry(t, run, "calc.subtract",
		[]kernel.Token{leafB, leafC}, "30.000000000", "USD", entryC.Hash, 4)
	// derive2 = add(A, derive1) = 130
	derive2, entry5 := buildDeriveEntry(t, run, "calc.add",
		[]kernel.Token{leafA, derive1}, "130.000000000", "USD", entry4.Hash, 5)

	allReads := concat(
		getReads(t, bindA, run),
		getReads(t, bindB, run),
		getReads(t, bindC, run),
	)
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        []kernel.Token{leafA, leafB, leafC, derive1, derive2},
		AuditEntries:  []kernel.AuditEntry{entryA, entryB, entryC, entry4, entry5},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, derive2.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	if len(g.Leaves) != 3 {
		t.Fatalf("Leaves count = %d, want 3", len(g.Leaves))
	}
	assertContrib(t, g.Leaves, leafA.ID, "100.000000000")
	assertContrib(t, g.Leaves, leafB.ID, "50.000000000")
	assertContrib(t, g.Leaves, leafC.ID, "-20.000000000")
}

// TestTraceMultiplyUSDByDim: currency × dimensionless.
// Currency leaves scaled; dimensionless leaves = "0".
func TestTraceMultiplyUSDByDim(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_mul", TenantID: "tenant_tr", Status: kernel.RunRunning}

	srcA := sourceBytes("fixture://mul_a.json", []byte(`{"val":100}`))
	srcB := sourceBytes("fixture://mul_b.json", []byte(`{"val":1.5}`))

	bindA := memio.NewBinding(srcA)
	bindB := memio.NewBinding(srcB)

	leafA, rawA := mintLeafInBinding(t, bindA, run, srcA, "/val", "USD")
	leafB, rawB := mintLeafInBinding(t, bindB, run, srcB, "/val", "")

	prevA := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, rawA, prevA, 1)
	entryB := rebuildLeafEntry(t, run, rawB, entryA.Hash, 2)

	derivedTok, entry3 := buildDeriveEntry(t, run, "calc.multiply",
		[]kernel.Token{leafA, leafB}, "150.000000000", "USD", entryB.Hash, 3)

	allReads := concat(
		getReads(t, bindA, run),
		getReads(t, bindB, run),
	)
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        []kernel.Token{leafA, leafB, derivedTok},
		AuditEntries:  []kernel.AuditEntry{entryA, entryB, entry3},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	if len(g.Leaves) != 2 {
		t.Fatalf("Leaves count = %d, want 2", len(g.Leaves))
	}
	// USD leaf (A) × 1.5 = 150
	assertContrib(t, g.Leaves, leafA.ID, "150.000000000")
	// Dimensionless leaf (B) contributes 0
	assertContrib(t, g.Leaves, leafB.ID, "0.000000000")
	if g.RootValue != "150.000000000" {
		t.Errorf("RootValue = %q, want 150.000000000", g.RootValue)
	}
}

// TestTraceDivide: numerator leaves / denominator; denom leaves = 0; rounding residual.
func TestTraceDivide(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_div", TenantID: "tenant_tr", Status: kernel.RunRunning}

	srcA := sourceBytes("fixture://div_a.json", []byte(`{"val":100}`))
	srcB := sourceBytes("fixture://div_b.json", []byte(`{"val":3}`))

	bindA := memio.NewBinding(srcA)
	bindB := memio.NewBinding(srcB)

	leafA, rawA := mintLeafInBinding(t, bindA, run, srcA, "/val", "USD")
	leafB, rawB := mintLeafInBinding(t, bindB, run, srcB, "/val", "")

	prevA := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, rawA, prevA, 1)
	entryB := rebuildLeafEntry(t, run, rawB, entryA.Hash, 2)

	// 100 / 3 = 33.333333333 (half-even at 9dp).
	derivedTok, entry3 := buildDeriveEntry(t, run, "calc.divide",
		[]kernel.Token{leafA, leafB}, "33.333333333", "USD", entryB.Hash, 3)

	allReads := concat(
		getReads(t, bindA, run),
		getReads(t, bindB, run),
	)
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        []kernel.Token{leafA, leafB, derivedTok},
		AuditEntries:  []kernel.AuditEntry{entryA, entryB, entry3},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	// Denominator leaves contribute 0.
	assertContrib(t, g.Leaves, leafB.ID, "0.000000000")

	// Rounding residual must be present (100/3 is non-terminating).
	if !hasRoundingResidual(g.Leaves) {
		t.Error("expected synthetic <rounding> residual for 100/3")
	}
}

// TestTraceRoundHalfEven: synthetic <rounding> captures residual; invariant holds.
func TestTraceRoundHalfEven(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_round", TenantID: "tenant_tr", Status: kernel.RunRunning}

	// Leaf: 100.5 USD  →  rounds to 100 USD under half_even at scale 0.
	srcA := sourceBytes("fixture://round_a.json", []byte(`{"val":100.5}`))
	bindA := memio.NewBinding(srcA)
	leafA, rawA := mintLeafInBinding(t, bindA, run, srcA, "/val", "USD")

	prevA := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, rawA, prevA, 1)

	// round.half_even(scale=0): 100.500000000 → 100 (banker's, 100 is even).
	roundedTok, entry2 := buildDeriveEntryRounding(t, run, "round.half_even",
		[]kernel.Token{leafA}, "100.000000000", "USD", entryA.Hash, 2,
		kernel.RoundingPolicy{Mode: "half_even", Scale: 0})

	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: getReads(t, bindA, run),
		Tokens:        []kernel.Token{leafA, roundedTok},
		AuditEntries:  []kernel.AuditEntry{entryA, entry2},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, roundedTok.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}

	// leaf value = 100.500000000; rounded result = 100.000000000.
	// residual = 100.000000000 - 100.500000000 = -0.500000000.
	if !hasRoundingResidual(g.Leaves) {
		t.Error("expected synthetic <rounding> residual for round.half_even")
	}
	assertRoundingResidual(t, g.Leaves, "-0.500000000")
	assertContrib(t, g.Leaves, leafA.ID, "100.500000000")
}

// TestTraceFilterSumWhere: filter.sum_where matched inputs contribute their
// values; unmatched inputs contribute "0". The matched list is read from the
// deterministic audit payload (authoritative), not heuristically reconstructed.
func TestTraceFilterSumWhere(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_filter", TenantID: "tenant_tr", Status: kernel.RunRunning}

	// Three USD leaves; matched = {A, C} (so B is excluded by the predicate).
	// This is the case the old heuristic would have mis-attributed:
	// sum(unit-matched) = 100+50+30 = 180 != evaluated 130, and falling back to
	// "treat all candidates as matched" would have given a sum of 180, failing
	// the invariant. The audit-payload-authoritative read gives the correct
	// contributions A=+100, B=0, C=+30, total = 130.
	srcA := sourceBytes("fixture://flt_a.json", []byte(`{"val":100}`))
	srcB := sourceBytes("fixture://flt_b.json", []byte(`{"val":50}`))
	srcC := sourceBytes("fixture://flt_c.json", []byte(`{"val":30}`))

	bindA := memio.NewBinding(srcA)
	bindB := memio.NewBinding(srcB)
	bindC := memio.NewBinding(srcC)

	leafA, rawA := mintLeafInBinding(t, bindA, run, srcA, "/val", "USD")
	leafB, rawB := mintLeafInBinding(t, bindB, run, srcB, "/val", "USD")
	leafC, rawC := mintLeafInBinding(t, bindC, run, srcC, "/val", "USD")

	prev := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, rawA, prev, 1)
	entryB := rebuildLeafEntry(t, run, rawB, entryA.Hash, 2)
	entryC := rebuildLeafEntry(t, run, rawC, entryB.Hash, 3)

	// Build a derived filter.sum_where token where matched = {A, C}.
	inputs := []kernel.Token{leafA, leafB, leafC}
	inputIDs := []string{leafA.ID, leafB.ID, leafC.ID}
	matched := []string{leafA.ID, leafC.ID}
	evaluated := "130.000000000"
	unit := "USD"

	tokenID := deriveTokenID(run.ID, run.TenantID, "filter.sum_where", inputIDs, evaluated, unit)
	derivedTok := kernel.Token{
		ID:             tokenID,
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: evaluated,
		Unit:           unit,
		Derivation: &kernel.Derivation{
			Operation: "filter.sum_where",
			Inputs:    append([]string(nil), inputIDs...),
			FormulaID: "filter.sum_where",
			Rounding:  kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
			Evaluated: evaluated,
		},
	}

	payloadMap := map[string]any{
		"evaluated":  evaluated,
		"formula_id": "filter.sum_where",
		"inputs":     inputIDs,
		"matched":    matched,
		"operation":  "filter.sum_where",
		"predicate": map[string]any{
			"field": "row.tag",
			"op":    "in",
			"values": []string{"keep_a", "keep_c"},
		},
		"rounding": map[string]any{
			"mode":  "half_even",
			"scale": 9,
		},
		"token_id": tokenID,
		"unit":     unit,
	}
	det := canonicalJSON(t, payloadMap)
	hash := auditHash(entryC.Hash, 4, run.ID, run.TenantID, kernel.EntryKindTokenDerive, det)
	deriveEntry := kernel.AuditEntry{
		Seq:      4,
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     kernel.EntryKindTokenDerive,
		At:       traceTestClock.Now(),
		PrevHash: append([]byte(nil), entryC.Hash...),
		Hash:     hash,
		Payload: kernel.PayloadEnvelope{
			Deterministic:      det,
			HashInputCanonical: det,
		},
	}
	_ = inputs

	allReads := concat(
		getReads(t, bindA, run),
		getReads(t, bindB, run),
		getReads(t, bindC, run),
	)
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        []kernel.Token{leafA, leafB, leafC, derivedTok},
		AuditEntries:  []kernel.AuditEntry{entryA, entryB, entryC, deriveEntry},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if g.RootValue != evaluated {
		t.Errorf("RootValue = %q, want %q", g.RootValue, evaluated)
	}
	assertContrib(t, g.Leaves, leafA.ID, "100.000000000")
	assertContrib(t, g.Leaves, leafB.ID, "0.000000000")
	assertContrib(t, g.Leaves, leafC.ID, "30.000000000")
}

// TestTraceUnknownToken: returns ErrTokenNotFound.
func TestTraceUnknownToken(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_unk", TenantID: "tenant_tr", Status: kernel.RunRunning}
	src := sourceBytes("fixture://unk.json", []byte(`{"val":1}`))
	binding := memio.NewBinding(src)
	mintLeafInBinding(t, binding, run, src, "/val", "USD")

	var reads []kernel.CapturedRead
	var tokens []kernel.Token
	var entries []kernel.AuditEntry
	_ = binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		var err error
		reads, err = binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		tokens, err = binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		entries, err = binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		return err
	})
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: reads,
		Tokens:        tokens,
		AuditEntries:  entries,
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	_, traceErr := trace.Trace(bundleBytes, "tok_does_not_exist")
	if traceErr == nil {
		t.Fatal("expected error for unknown token, got nil")
	}
	if !errors.Is(traceErr, trace.ErrTokenNotFound) {
		t.Errorf("error = %v, want ErrTokenNotFound", traceErr)
	}
}

// TestTraceTamperedBundle: bundle.Verify fails first; trace never runs.
func TestTraceTamperedBundle(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_tamp", TenantID: "tenant_tr", Status: kernel.RunRunning}
	src := sourceBytes("fixture://tamp.json", []byte(`{"val":1}`))
	binding := memio.NewBinding(src)
	leaf, _ := mintLeafInBinding(t, binding, run, src, "/val", "USD")

	var reads []kernel.CapturedRead
	var tokens []kernel.Token
	var entries []kernel.AuditEntry
	_ = binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		var err error
		reads, err = binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		tokens, err = binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		entries, err = binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		return err
	})
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: reads,
		Tokens:        tokens,
		AuditEntries:  entries,
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	// Corrupt the bundle.
	tampered := bytes.Clone(bundleBytes)
	for i := 10; i < 30; i++ {
		tampered[i] ^= 0xFF
	}

	_, traceErr := trace.Trace(tampered, leaf.ID)
	if traceErr == nil {
		t.Fatal("expected error for tampered bundle, got nil")
	}
}

// TestTraceDeterminism: two Trace calls return byte-identical canonical JSON.
func TestTraceDeterminism(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_tr_det", TenantID: "tenant_tr", Status: kernel.RunRunning}

	srcA := sourceBytes("fixture://det_a.json", []byte(`{"val":10}`))
	srcB := sourceBytes("fixture://det_b.json", []byte(`{"val":20}`))
	bindA := memio.NewBinding(srcA)
	bindB := memio.NewBinding(srcB)

	leafA, rawA := mintLeafInBinding(t, bindA, run, srcA, "/val", "USD")
	leafB, rawB := mintLeafInBinding(t, bindB, run, srcB, "/val", "USD")

	prevA := genesisHash(run.TenantID)
	entryA := rebuildLeafEntry(t, run, rawA, prevA, 1)
	entryB := rebuildLeafEntry(t, run, rawB, entryA.Hash, 2)

	derivedTok, entry3 := buildDeriveEntry(t, run, "calc.add",
		[]kernel.Token{leafA, leafB}, "30.000000000", "USD", entryB.Hash, 3)

	allReads := concat(getReads(t, bindA, run), getReads(t, bindB, run))
	bundleBytes, err := bundle.Emit(bundle.State{
		Run:           run,
		CapturedReads: allReads,
		Tokens:        []kernel.Token{leafA, leafB, derivedTok},
		AuditEntries:  []kernel.AuditEntry{entryA, entryB, entry3},
	})
	if err != nil {
		t.Fatalf("bundle.Emit: %v", err)
	}

	g1, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() #1 error = %v", err)
	}
	g2, err := trace.Trace(bundleBytes, derivedTok.ID)
	if err != nil {
		t.Fatalf("Trace() #2 error = %v", err)
	}

	b1, err := trace.MarshalCanonical(g1)
	if err != nil {
		t.Fatalf("MarshalCanonical #1: %v", err)
	}
	b2, err := trace.MarshalCanonical(g2)
	if err != nil {
		t.Fatalf("MarshalCanonical #2: %v", err)
	}

	if !bytes.Equal(b1, b2) {
		t.Errorf("Trace output is not byte-identical across two calls:\n#1: %s\n#2: %s", b1, b2)
	}
}
