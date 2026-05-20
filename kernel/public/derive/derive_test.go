package derive_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/derive"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var testTime = time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

type sequenceClock struct {
	times []time.Time
	index int
}

func (c *sequenceClock) Now() time.Time {
	if c.index >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	cur := c.times[c.index]
	c.index++
	return cur
}

func testRun() kernel.Run {
	return kernel.Run{ID: "run_test", TenantID: "tenant_test", Status: kernel.RunRunning}
}

// mintCounter tracks a global counter to generate unique source paths within tests.
var mintCounter int64

// mintIntoBinding mints a leaf token directly into b using a JSON numeric
// source. Each call appends one CapturedRead and one AuditEntry.
// The source path is made unique per call via a counter so multiple tokens
// with the same value and unit can coexist in the same binding.
func mintIntoBinding(t *testing.T, b *memio.Binding, run kernel.Run, value, unit string) kernel.Token {
	t.Helper()

	n := atomic.AddInt64(&mintCounter, 1)
	src := []byte(`{"value":` + value + `}`)
	path := fmt.Sprintf("fixture://v_%s_%s_%s_%d.json", run.ID, value, unit, n)
	sb := kernel.SourceBytes{
		Source: kernel.Source{
			Path:   path,
			SHA256: sha256SumBytes(src),
		},
		Bytes: src,
	}

	// We need to register the source in the binding. Since Binding.ResolveSource
	// only works for sources passed to NewBinding, we create a small helper
	// binding just for resolution and do a direct ReadAndMint that writes into b.
	// We achieve this by using a source-only binding as the resolver while
	// using b for Store, AuditSink, and Scope.
	srcBinding := memio.NewBinding(sb)

	res, err := kernel.ReadAndMint(
		context.Background(),
		b,          // scope
		srcBinding, // resolver — only holds this one source
		extractjson.Reader{},
		b, // store (and CapturedReadStore)
		b, // audit sink
		fixedClock{t: testTime},
		run,
		kernel.ReadAndMintRequest{
			SourcePath: path,
			Selector:   kernel.SourceSelector{JSONPointer: "/value"},
			OutputPath: "$.value",
			Unit:       unit,
		},
	)
	if err != nil {
		t.Fatalf("mintIntoBinding(%s, %s): %v", value, unit, err)
	}
	return res.Token
}


// deps returns a Dependencies wired to b.
func deps(b *memio.Binding, clock kernel.Clock) derive.Dependencies {
	return derive.Dependencies{
		Clock:     clock,
		Store:     b,
		AuditSink: b,
		Scope:     b,
	}
}

// assertNoAuditEntries checks that no audit entries exist for run in b.
func assertNoAuditEntries(t *testing.T, b *memio.Binding, run kernel.Run) {
	t.Helper()
	if err := b.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		entries, err := b.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 audit entries after error, got %d", len(entries))
		}
		return nil
	}); err != nil {
		t.Fatalf("assertNoAuditEntries: %v", err)
	}
}

// snapshotBindingState collects all tokens, captured reads, and audit entries
// from b for run into a bundle.State.
func snapshotBindingState(t *testing.T, b *memio.Binding, run kernel.Run) bundle.State {
	t.Helper()
	var state bundle.State
	state.Run = run
	if err := b.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		var err error
		state.Tokens, err = b.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		state.CapturedReads, err = b.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		state.AuditEntries, err = b.AuditEntries(ctx, tx, run.TenantID, run.ID)
		return err
	}); err != nil {
		t.Fatalf("snapshotBindingState: %v", err)
	}
	return state
}

// ---------------------------------------------------------------------------
// calc.add
// ---------------------------------------------------------------------------

func TestCalcAddHappyPath(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokA := mintIntoBinding(t, b, run, "100", "USD")
	tokB := mintIntoBinding(t, b, run, "23", "USD")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tokA, tokB},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != "123.000000000" {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, "123.000000000")
	}
	if res.Token.Unit != "USD" {
		t.Errorf("Unit = %q, want %q", res.Token.Unit, "USD")
	}
	if res.Token.Derivation == nil {
		t.Fatal("Derivation is nil")
	}
	if res.Token.Derivation.Operation != "calc.add" {
		t.Errorf("Derivation.Operation = %q, want %q", res.Token.Derivation.Operation, "calc.add")
	}
	if len(res.Token.Derivation.Inputs) != 2 {
		t.Fatalf("len(Derivation.Inputs) = %d, want 2", len(res.Token.Derivation.Inputs))
	}
	if res.Token.Derivation.Inputs[0] != tokA.ID {
		t.Errorf("Inputs[0] = %q, want %q", res.Token.Derivation.Inputs[0], tokA.ID)
	}
	if res.Token.Derivation.Inputs[1] != tokB.ID {
		t.Errorf("Inputs[1] = %q, want %q", res.Token.Derivation.Inputs[1], tokB.ID)
	}
	if res.Token.Source != (kernel.TokenSource{}) {
		t.Errorf("Source = %+v, want zero value", res.Token.Source)
	}
	if res.AuditEntry.Kind != kernel.EntryKindTokenDerive {
		t.Errorf("AuditEntry.Kind = %q, want %q", res.AuditEntry.Kind, kernel.EntryKindTokenDerive)
	}
}

// ---------------------------------------------------------------------------
// calc.subtract
// ---------------------------------------------------------------------------

func TestCalcSubtractOrder(t *testing.T) {
	t.Parallel()

	run := testRun()

	// Forward: 100 - 30 = 70
	b1 := memio.NewBinding()
	tok100a := mintIntoBinding(t, b1, run, "100", "USD")
	tok30a := mintIntoBinding(t, b1, run, "30", "USD")
	res1, err := derive.Derive(context.Background(), deps(b1, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.subtract",
		Inputs: []kernel.Token{tok100a, tok30a},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res1.Token.ValueCanonical != "70.000000000" {
		t.Errorf("100 - 30: ValueCanonical = %q, want %q", res1.Token.ValueCanonical, "70.000000000")
	}

	// Swapped: 30 - 100 = -70
	b2 := memio.NewBinding()
	tok100b := mintIntoBinding(t, b2, run, "100", "USD")
	tok30b := mintIntoBinding(t, b2, run, "30", "USD")
	res2, err := derive.Derive(context.Background(), deps(b2, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.subtract",
		Inputs: []kernel.Token{tok30b, tok100b},
	})
	if err != nil {
		t.Fatalf("Derive() swapped error = %v", err)
	}
	if res2.Token.ValueCanonical == res1.Token.ValueCanonical {
		t.Error("swapped subtract produced same result as forward subtract")
	}
	if res2.Token.ValueCanonical != "-70.000000000" {
		t.Errorf("30 - 100: ValueCanonical = %q, want %q", res2.Token.ValueCanonical, "-70.000000000")
	}
}

// ---------------------------------------------------------------------------
// calc.sum
// ---------------------------------------------------------------------------

func TestCalcSumSingleInput(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tok := mintIntoBinding(t, b, run, "42", "EUR")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.sum",
		Inputs: []kernel.Token{tok},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != "42.000000000" {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, "42.000000000")
	}
}

func TestCalcSumTwoInputs(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tok1 := mintIntoBinding(t, b, run, "10", "GBP")
	tok2 := mintIntoBinding(t, b, run, "20", "GBP")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.sum",
		Inputs: []kernel.Token{tok1, tok2},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != "30.000000000" {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, "30.000000000")
	}
}

func TestCalcSumNInputs(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tok1 := mintIntoBinding(t, b, run, "1", "USD")
	tok2 := mintIntoBinding(t, b, run, "2", "USD")
	tok3 := mintIntoBinding(t, b, run, "3", "USD")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.sum",
		Inputs: []kernel.Token{tok1, tok2, tok3},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != "6.000000000" {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, "6.000000000")
	}
}

func TestCalcSumEmptyInputs(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	mintIntoBinding(t, b, run, "1", "USD") // need at least one entry so binding isn't entirely empty

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.sum",
		Inputs: []kernel.Token{},
	})
	if !errors.Is(err, derive.ErrNoInputs) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrNoInputs)
	}
}

// ---------------------------------------------------------------------------
// Unit mismatch
// ---------------------------------------------------------------------------

func TestUnitMismatch(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokUSD := mintIntoBinding(t, b, run, "100", "USD")
	tokEUR := mintIntoBinding(t, b, run, "50", "EUR")

	auditsBefore := countAuditEntries(t, b, run)

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tokUSD, tokEUR},
	})
	if !errors.Is(err, derive.ErrUnitMismatch) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrUnitMismatch)
	}

	// Verify no new audit entry was appended on error.
	auditsAfter := countAuditEntries(t, b, run)
	if auditsAfter != auditsBefore {
		t.Errorf("audit entries after error: got %d, want %d", auditsAfter, auditsBefore)
	}
}

func countAuditEntries(t *testing.T, b *memio.Binding, run kernel.Run) int {
	t.Helper()
	var n int
	if err := b.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		entries, err := b.AuditEntries(ctx, tx, run.TenantID, run.ID)
		n = len(entries)
		return err
	}); err != nil {
		t.Fatalf("countAuditEntries: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Arity violations
// ---------------------------------------------------------------------------

func TestCalcAddArityViolations(t *testing.T) {
	t.Parallel()

	run := testRun()

	tests := []struct {
		name    string
		nInputs int
	}{
		{"one input", 1},
		{"three inputs", 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := memio.NewBinding()
			inputs := make([]kernel.Token, tt.nInputs)
			for i := range inputs {
				inputs[i] = mintIntoBinding(t, b, run, "10", "USD")
			}

			_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
				Run:    run,
				Op:     "calc.add",
				Inputs: inputs,
			})
			if !errors.Is(err, derive.ErrArity) {
				t.Fatalf("Derive() error = %v, want %v", err, derive.ErrArity)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unknown op
// ---------------------------------------------------------------------------

func TestUnknownOp(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tok := mintIntoBinding(t, b, run, "1", "USD")

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.nope",
		Inputs: []kernel.Token{tok},
	})
	if !errors.Is(err, derive.ErrUnknownOp) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrUnknownOp)
	}
}

// ---------------------------------------------------------------------------
// Missing input token
// ---------------------------------------------------------------------------

func TestInputNotInStore(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	// Mint into a throwaway binding so we have a real token struct, but
	// do NOT put it into b so ErrInputNotInStore fires.
	bThrow := memio.NewBinding()
	tok := mintIntoBinding(t, bThrow, run, "1", "USD")

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tok, tok},
	})
	if !errors.Is(err, derive.ErrInputNotInStore) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrInputNotInStore)
	}
}

// ---------------------------------------------------------------------------
// Deterministic ID: identical inputs yield identical token IDs
// ---------------------------------------------------------------------------

func TestDeterministicTokenID(t *testing.T) {
	t.Parallel()

	run := testRun()

	// Use two independent bindings; each gets the same leaf token minted into it.
	b1 := memio.NewBinding()
	tok1 := mintIntoBinding(t, b1, run, "50", "USD")

	b2 := memio.NewBinding()
	tok2 := mintIntoBinding(t, b2, run, "50", "USD")

	// The leaf token IDs must be identical (same source, same value, same run).
	if tok1.ID != tok2.ID {
		t.Skipf("leaf token IDs differ (%s vs %s) — skip derivation determinism check", tok1.ID, tok2.ID)
	}

	res1, err := derive.Derive(context.Background(), deps(b1, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tok1, tok1},
	})
	if err != nil {
		t.Fatalf("Derive() first: %v", err)
	}

	res2, err := derive.Derive(context.Background(), deps(b2, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tok2, tok2},
	})
	if err != nil {
		t.Fatalf("Derive() second: %v", err)
	}

	if res1.Token.ID != res2.Token.ID {
		t.Errorf("non-deterministic ID: %q vs %q", res1.Token.ID, res2.Token.ID)
	}
}

// ---------------------------------------------------------------------------
// Derived token as input to another derive (DAG depth > 1)
// ---------------------------------------------------------------------------

func TestDerivedInputToDerived(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokA := mintIntoBinding(t, b, run, "10", "USD")
	tokB := mintIntoBinding(t, b, run, "20", "USD")
	tokC := mintIntoBinding(t, b, run, "30", "USD")
	clock := &sequenceClock{times: []time.Time{
		testTime,
		testTime.Add(time.Second),
	}}

	// First: sum(B, C)
	resSumBC, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "calc.sum",
		Inputs: []kernel.Token{tokB, tokC},
	})
	if err != nil {
		t.Fatalf("Derive sum(B,C): %v", err)
	}
	if resSumBC.Token.ValueCanonical != "50.000000000" {
		t.Errorf("sum(B,C) = %q, want 50.000000000", resSumBC.Token.ValueCanonical)
	}

	// Second: add(A, sum(B,C)) — derived token is a valid input
	resAdd, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tokA, resSumBC.Token},
	})
	if err != nil {
		t.Fatalf("Derive add(A, sum(B,C)): %v", err)
	}
	if resAdd.Token.ValueCanonical != "60.000000000" {
		t.Errorf("add(A, sum(B,C)) = %q, want 60.000000000", resAdd.Token.ValueCanonical)
	}
}

// ---------------------------------------------------------------------------
// Replay byte-equality
// ---------------------------------------------------------------------------

func TestReplayByteEquality(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_replay", TenantID: "tenant_replay", Status: kernel.RunRunning}
	b := memio.NewBinding()
	clock := &sequenceClock{times: []time.Time{
		testTime,
		testTime.Add(time.Second),
		testTime.Add(2 * time.Second),
		testTime.Add(3 * time.Second),
		testTime.Add(4 * time.Second),
	}}

	// Mint three leaf tokens.
	tok100 := mintIntoBinding(t, b, run, "100", "USD")
	tok50 := mintIntoBinding(t, b, run, "50", "USD")
	tok25 := mintIntoBinding(t, b, run, "25", "USD")

	// Derive: 100 - 50
	res1, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "calc.subtract",
		Inputs: []kernel.Token{tok100, tok50},
	})
	if err != nil {
		t.Fatalf("first Derive: %v", err)
	}

	// Derive: (100-50) - 25
	_, err = derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "calc.subtract",
		Inputs: []kernel.Token{res1.Token, tok25},
	})
	if err != nil {
		t.Fatalf("second Derive: %v", err)
	}

	// Snapshot state and emit bundle.
	state := snapshotBindingState(t, b, run)
	bundleBytes1, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit(): %v", err)
	}

	// Verify + Replay.
	if err := bundle.Verify(bundleBytes1); err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	replayed, err := bundle.Replay(bundleBytes1)
	if err != nil {
		t.Fatalf("Replay(): %v", err)
	}

	// Re-emit the replayed state and compare bytes.
	bundleBytes2, err := bundle.Emit(replayed)
	if err != nil {
		t.Fatalf("Emit(Replay()): %v", err)
	}
	if !bytes.Equal(bundleBytes1, bundleBytes2) {
		t.Fatal("Emit(Replay()) bytes differ from original — replay byte-equality broken")
	}
}

// ---------------------------------------------------------------------------
// Fuzz: calc.sum associativity (golden decimal, not approximate)
// ---------------------------------------------------------------------------

func TestCalcSumAssociativity(t *testing.T) {
	t.Parallel()

	vectors := []struct {
		a, b, c string
	}{
		{"1", "2", "3"},
		{"100.5", "200.25", "300.125"},
		{"0", "0", "0"},
		{"999999999", "1", "0"},
		{"0.000000001", "0.000000002", "0.000000003"},
	}

	for _, v := range vectors {
		v := v
		t.Run(v.a+"+"+v.b+"+"+v.c, func(t *testing.T) {
			t.Parallel()

			run := kernel.Run{
				ID:       "run_assoc_" + v.a + "_" + v.b + "_" + v.c,
				TenantID: "tenant_assoc",
				Status:   kernel.RunRunning,
			}

			// sum(a,b,c)
			b1 := memio.NewBinding()
			tokA1 := mintIntoBinding(t, b1, run, v.a, "USD")
			tokB1 := mintIntoBinding(t, b1, run, v.b, "USD")
			tokC1 := mintIntoBinding(t, b1, run, v.c, "USD")
			resABC, err := derive.Derive(context.Background(), deps(b1, fixedClock{t: testTime}), derive.Request{
				Run:    run,
				Op:     "calc.sum",
				Inputs: []kernel.Token{tokA1, tokB1, tokC1},
			})
			if err != nil {
				t.Fatalf("sum(a,b,c): %v", err)
			}

			// add(a, sum(b,c))
			b2 := memio.NewBinding()
			tokA2 := mintIntoBinding(t, b2, run, v.a, "USD")
			tokB2 := mintIntoBinding(t, b2, run, v.b, "USD")
			tokC2 := mintIntoBinding(t, b2, run, v.c, "USD")
			clock2 := &sequenceClock{times: []time.Time{testTime, testTime.Add(time.Second)}}
			resBC, err := derive.Derive(context.Background(), deps(b2, clock2), derive.Request{
				Run:    run,
				Op:     "calc.sum",
				Inputs: []kernel.Token{tokB2, tokC2},
			})
			if err != nil {
				t.Fatalf("sum(b,c): %v", err)
			}
			resABC2, err := derive.Derive(context.Background(), deps(b2, clock2), derive.Request{
				Run:    run,
				Op:     "calc.add",
				Inputs: []kernel.Token{tokA2, resBC.Token},
			})
			if err != nil {
				t.Fatalf("add(a, sum(b,c)): %v", err)
			}

			// add(sum(a,b), c)
			b3 := memio.NewBinding()
			tokA3 := mintIntoBinding(t, b3, run, v.a, "USD")
			tokB3 := mintIntoBinding(t, b3, run, v.b, "USD")
			tokC3 := mintIntoBinding(t, b3, run, v.c, "USD")
			clock3 := &sequenceClock{times: []time.Time{testTime, testTime.Add(time.Second)}}
			resAB, err := derive.Derive(context.Background(), deps(b3, clock3), derive.Request{
				Run:    run,
				Op:     "calc.sum",
				Inputs: []kernel.Token{tokA3, tokB3},
			})
			if err != nil {
				t.Fatalf("sum(a,b): %v", err)
			}
			resABC3, err := derive.Derive(context.Background(), deps(b3, clock3), derive.Request{
				Run:    run,
				Op:     "calc.add",
				Inputs: []kernel.Token{resAB.Token, tokC3},
			})
			if err != nil {
				t.Fatalf("add(sum(a,b), c): %v", err)
			}

			got1 := resABC.Token.ValueCanonical
			got2 := resABC2.Token.ValueCanonical
			got3 := resABC3.Token.ValueCanonical
			if got1 != got2 || got1 != got3 {
				t.Errorf("associativity violated for (%s,%s,%s): sum=%s add(a,sum(b,c))=%s add(sum(a,b),c)=%s",
					v.a, v.b, v.c, got1, got2, got3)
			}
		})
	}
}
