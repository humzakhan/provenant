package derive_test

// Phase 03 tests: calc.multiply, calc.divide, round.half_even,
// select.passthrough, filter.sum_where, and replay byte-equality extension.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/derive"
	"github.com/provenant/provenant/kernel/public/extractcsvrows"
	"github.com/provenant/provenant/kernel/public/memio"
)

// ---------------------------------------------------------------------------
// calc.multiply
// ---------------------------------------------------------------------------

func TestCalcMultiplyUSDByDimensionless(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokUSD := mintIntoBinding(t, b, run, "100", "USD")
	tokDim := mintIntoBinding(t, b, run, "2", "")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.multiply",
		Inputs: []kernel.Token{tokUSD, tokDim},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != "200.000000000" {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, "200.000000000")
	}
	if res.Token.Unit != "USD" {
		t.Errorf("Unit = %q, want %q", res.Token.Unit, "USD")
	}

	// reverse: dimensionless × USD → USD
	b2 := memio.NewBinding()
	tokDim2 := mintIntoBinding(t, b2, run, "2", "")
	tokUSD2 := mintIntoBinding(t, b2, run, "100", "USD")
	res2, err := derive.Derive(context.Background(), deps(b2, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.multiply",
		Inputs: []kernel.Token{tokDim2, tokUSD2},
	})
	if err != nil {
		t.Fatalf("Derive() reversed error = %v", err)
	}
	if res2.Token.Unit != "USD" {
		t.Errorf("reversed Unit = %q, want %q", res2.Token.Unit, "USD")
	}
}

func TestCalcMultiplyUSDByUSDReturnsErrUnitMismatch(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokA := mintIntoBinding(t, b, run, "100", "USD")
	tokB := mintIntoBinding(t, b, run, "2", "USD")

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.multiply",
		Inputs: []kernel.Token{tokA, tokB},
	})
	if !errors.Is(err, derive.ErrUnitMismatch) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrUnitMismatch)
	}
}

// ---------------------------------------------------------------------------
// calc.divide
// ---------------------------------------------------------------------------

func TestCalcDivideUSDByUSDReturnsDimensionless(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokA := mintIntoBinding(t, b, run, "300", "USD")
	tokB := mintIntoBinding(t, b, run, "100", "USD")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:      run,
		Op:       "calc.divide",
		Inputs:   []kernel.Token{tokA, tokB},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != "3.000000000" {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, "3.000000000")
	}
	if res.Token.Unit != "" {
		t.Errorf("Unit = %q, want empty (dimensionless)", res.Token.Unit)
	}
}

func TestCalcDivideByZeroReturnsErrDivisionByZero(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokA := mintIntoBinding(t, b, run, "100", "USD")
	tokZero := mintIntoBinding(t, b, run, "0", "USD")

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:      run,
		Op:       "calc.divide",
		Inputs:   []kernel.Token{tokA, tokZero},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
	})
	if !errors.Is(err, derive.ErrDivisionByZero) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrDivisionByZero)
	}
}

func TestCalcDivideMissingScaleReturnsErrRoundingRequired(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	tokA := mintIntoBinding(t, b, run, "100", "USD")
	tokB := mintIntoBinding(t, b, run, "3", "USD")

	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "calc.divide",
		Inputs: []kernel.Token{tokA, tokB},
		// No rounding policy → ErrRoundingRequired
	})
	if !errors.Is(err, derive.ErrRoundingRequired) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrRoundingRequired)
	}
}

// ---------------------------------------------------------------------------
// round.half_even
// ---------------------------------------------------------------------------

func TestRoundHalfEvenBoundaryViaDeriveEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		scale int
		want  string
	}{
		{"2.5", 0, "2.000000000"},  // banker's: 2.5 → 2 (even neighbor), padded by FormatCanonical
		{"3.5", 0, "4.000000000"},  // banker's: 3.5 → 4 (even neighbor)
		{"-2.5", 0, "-2.000000000"}, // banker's: -2.5 → -2 (even neighbor)
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("round(%s,scale=%d)", tt.input, tt.scale), func(t *testing.T) {
			t.Parallel()

			run := kernel.Run{
				ID:       "run_round_" + tt.input,
				TenantID: "tenant_round",
				Status:   kernel.RunRunning,
			}
			b := memio.NewBinding()
			tok := mintIntoBinding(t, b, run, tt.input, "USD")

			res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
				Run:      run,
				Op:       "round.half_even",
				Inputs:   []kernel.Token{tok},
				Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: tt.scale},
			})
			if err != nil {
				t.Fatalf("Derive() error = %v", err)
			}
			if res.Token.Unit != "USD" {
				t.Errorf("Unit = %q, want USD", res.Token.Unit)
			}
			if res.Token.ValueCanonical != tt.want {
				t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, tt.want)
			}
		})
	}
}

// TestRoundHalfEvenBoundaryValues verifies the banker's rounding vectors at
// scale 0 directly via the op level (not the full engine) for precision.
func TestRoundHalfEvenBoundaryValues(t *testing.T) {
	t.Parallel()

	run := testRun()
	b25 := memio.NewBinding()
	tok25 := mintIntoBinding(t, b25, run, "2.5", "USD")
	res25, err := derive.Derive(context.Background(), deps(b25, fixedClock{t: testTime}), derive.Request{
		Run:      run,
		Op:       "round.half_even",
		Inputs:   []kernel.Token{tok25},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 0},
	})
	if err != nil {
		t.Fatalf("round(2.5,0): %v", err)
	}
	// After RoundBank(0) on 2.5 → 2, FormatCanonical(2) = "2.000000000"
	if res25.Token.ValueCanonical != "2.000000000" {
		t.Errorf("round(2.5, 0) = %q, want 2.000000000", res25.Token.ValueCanonical)
	}

	b35 := memio.NewBinding()
	tok35 := mintIntoBinding(t, b35, run, "3.5", "USD")
	res35, err := derive.Derive(context.Background(), deps(b35, fixedClock{t: testTime}), derive.Request{
		Run:      run,
		Op:       "round.half_even",
		Inputs:   []kernel.Token{tok35},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 0},
	})
	if err != nil {
		t.Fatalf("round(3.5,0): %v", err)
	}
	if res35.Token.ValueCanonical != "4.000000000" {
		t.Errorf("round(3.5, 0) = %q, want 4.000000000", res35.Token.ValueCanonical)
	}

	// -2.5 → -2 (banker's rounding on negative half: away from zero only when
	// the integer part is odd; |-2| = 2 is even → round toward zero = -2)
	b_neg := memio.NewBinding()
	tokNeg := mintIntoBinding(t, b_neg, run, "-2.5", "USD")
	resNeg, err := derive.Derive(context.Background(), deps(b_neg, fixedClock{t: testTime}), derive.Request{
		Run:      run,
		Op:       "round.half_even",
		Inputs:   []kernel.Token{tokNeg},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 0},
	})
	if err != nil {
		t.Fatalf("round(-2.5,0): %v", err)
	}
	if resNeg.Token.ValueCanonical != "-2.000000000" {
		t.Errorf("round(-2.5, 0) = %q, want -2.000000000", resNeg.Token.ValueCanonical)
	}
}

// ---------------------------------------------------------------------------
// select.passthrough
// ---------------------------------------------------------------------------

func TestSelectPassthrough(t *testing.T) {
	t.Parallel()

	run := testRun()
	b := memio.NewBinding()
	src := mintIntoBinding(t, b, run, "99.5", "GBP")

	res, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:    run,
		Op:     "select.passthrough",
		Inputs: []kernel.Token{src},
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	if res.Token.ValueCanonical != src.ValueCanonical {
		t.Errorf("ValueCanonical = %q, want %q", res.Token.ValueCanonical, src.ValueCanonical)
	}
	if res.Token.Unit != "GBP" {
		t.Errorf("Unit = %q, want GBP", res.Token.Unit)
	}
	if res.Token.Derivation == nil {
		t.Fatal("Derivation is nil")
	}
	if len(res.Token.Derivation.Inputs) != 1 {
		t.Errorf("len(Derivation.Inputs) = %d, want 1", len(res.Token.Derivation.Inputs))
	}
	if res.AuditEntry.Kind != kernel.EntryKindTokenDerive {
		t.Errorf("AuditEntry.Kind = %q, want %q", res.AuditEntry.Kind, kernel.EntryKindTokenDerive)
	}
}

// ---------------------------------------------------------------------------
// filter.sum_where helpers
// ---------------------------------------------------------------------------

// mintCSVRowToken mints a leaf token whose source is a single-row CSV using
// extract.csv_rows. It writes the captured read and token into b and returns
// the minted token. The row contains an "account" and an "amount" column.
func mintCSVRowToken(t *testing.T, b *memio.Binding, run kernel.Run, account, amount string, n int) kernel.Token {
	t.Helper()

	csvBytes := []byte(fmt.Sprintf("account,amount\n%s,%s\n", account, amount))
	srcPath := fmt.Sprintf("fixture://csv_row_%s_%s_%d.csv", account, amount, n)
	srcSHA := sha256Hex(csvBytes)
	sb := kernel.SourceBytes{
		Source: kernel.Source{Path: srcPath, SHA256: srcSHA},
		Bytes:  csvBytes,
	}
	bSrc := memio.NewBinding(sb)

	// No ColumnProjection so both "account" and "amount" columns are returned.
	// The predicate will test the "account" cell (cells[0]) and the minted value
	// comes from the "amount" cell (cells[1]).
	res, err := kernel.ReadAndMint(
		context.Background(),
		b,    // scope
		bSrc, // resolver
		extractcsvrows.Reader{},
		b, b,
		fixedClock{t: testTime},
		run,
		kernel.ReadAndMintRequest{
			SourcePath: srcPath,
			Selector: kernel.SourceSelector{
				RowWindow: "2-2",
				HeaderRow: 1,
				Dialect:   "excel",
				Encoding:  "utf-8",
			},
			OutputPath: "/rows/0/cells/1/value",
			Unit:       "USD",
		},
	)
	if err != nil {
		t.Fatalf("mintCSVRowToken(%s, %s): %v", account, amount, err)
	}
	return res.Token
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// filter.sum_where: happy path
// ---------------------------------------------------------------------------

func TestFilterSumWhereHappyPath(t *testing.T) {
	t.Parallel()

	// Three CSV-row leaves with accounts 4000, 4010, 4020.
	// Predicate: account in ["4000","4010"] → selects tok4000 + tok4010.
	run := kernel.Run{ID: "run_filter_happy", TenantID: "tenant_filter", Status: kernel.RunRunning}
	b := memio.NewBinding()

	tok4000 := mintCSVRowToken(t, b, run, "4000", "100", 1)
	tok4010 := mintCSVRowToken(t, b, run, "4010", "50", 2)
	tok4020 := mintCSVRowToken(t, b, run, "4020", "25", 3)

	predicate := &kernel.RowPredicate{
		Field:  "row.account",
		Op:     "in",
		Values: []string{"4000", "4010"},
	}

	clock := &sequenceClock{times: []time.Time{testTime, testTime.Add(time.Second)}}

	res, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:       run,
		Op:        "filter.sum_where",
		Inputs:    []kernel.Token{tok4000, tok4010, tok4020},
		Predicate: predicate,
	})
	if err != nil {
		t.Fatalf("Derive() error = %v", err)
	}
	// Expected: 100 + 50 = 150
	if res.Token.ValueCanonical != "150.000000000" {
		t.Errorf("ValueCanonical = %q, want 150.000000000", res.Token.ValueCanonical)
	}
	if res.Token.Unit != "USD" {
		t.Errorf("Unit = %q, want USD", res.Token.Unit)
	}
	if res.Token.Derivation == nil {
		t.Fatal("Derivation is nil")
	}
	// All three are listed as inputs.
	if len(res.Token.Derivation.Inputs) != 3 {
		t.Errorf("len(Inputs) = %d, want 3", len(res.Token.Derivation.Inputs))
	}

	// Verify bundle round-trip to confirm matched/predicate in payload.
	state := snapshotBindingState(t, b, run)
	bundleBytes, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit(): %v", err)
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("Verify(): %v", err)
	}

	// Confirm matched appears verbatim in the deterministic payload.
	rawPayload := res.AuditEntry.Payload.Deterministic
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := payload["matched"]; !ok {
		t.Error("matched field missing from audit payload")
	}
	if _, ok := payload["predicate"]; !ok {
		t.Error("predicate field missing from audit payload")
	}
}

// ---------------------------------------------------------------------------
// filter.sum_where: mixed lineage → ErrAmbiguousLineage
// ---------------------------------------------------------------------------

func TestFilterSumWhereMixedLineageReturnsErrAmbiguousLineage(t *testing.T) {
	t.Parallel()

	// Create a derived token whose inputs come from two different leaf reads.
	// When we use it as a filter.sum_where input it should fail with
	// ErrAmbiguousLineage because it traces to two leaf captured reads.
	run := kernel.Run{ID: "run_filter_ambiguous", TenantID: "tenant_filter_ambiguous", Status: kernel.RunRunning}
	b := memio.NewBinding()

	tokA := mintCSVRowToken(t, b, run, "4000", "100", 1)
	tokB := mintCSVRowToken(t, b, run, "4010", "50", 2)

	// Derive a token that has two CSV-row ancestors.
	clock := &sequenceClock{times: []time.Time{testTime, testTime.Add(time.Second)}}
	sumRes, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "calc.add",
		Inputs: []kernel.Token{tokA, tokB},
	})
	if err != nil {
		t.Fatalf("Derive add: %v", err)
	}

	// Now use the derived (multi-leaf) token as an input to filter.sum_where.
	predicate := &kernel.RowPredicate{Field: "row.account", Op: "eq", Value: "4000"}
	_, err = derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:       run,
		Op:        "filter.sum_where",
		Inputs:    []kernel.Token{sumRes.Token},
		Predicate: predicate,
	})
	if !errors.Is(err, derive.ErrAmbiguousLineage) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrAmbiguousLineage)
	}
}

// ---------------------------------------------------------------------------
// filter.sum_where: no matches → ErrNoMatchingInputs
// ---------------------------------------------------------------------------

func TestFilterSumWhereNoMatchesReturnsErrNoMatchingInputs(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_filter_nomatch", TenantID: "tenant_filter_nomatch", Status: kernel.RunRunning}
	b := memio.NewBinding()

	tok4000 := mintCSVRowToken(t, b, run, "4000", "100", 1)
	tok4010 := mintCSVRowToken(t, b, run, "4010", "50", 2)

	// Predicate that matches nothing.
	predicate := &kernel.RowPredicate{Field: "row.account", Op: "eq", Value: "9999"}
	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:       run,
		Op:        "filter.sum_where",
		Inputs:    []kernel.Token{tok4000, tok4010},
		Predicate: predicate,
	})
	if !errors.Is(err, derive.ErrNoMatchingInputs) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrNoMatchingInputs)
	}
}

// ---------------------------------------------------------------------------
// filter.sum_where: JSON leaf → ErrPredicateNotRowShaped
// ---------------------------------------------------------------------------

func TestFilterSumWhereJSONLeafReturnsErrPredicateNotRowShaped(t *testing.T) {
	t.Parallel()

	// mintIntoBinding uses extract.json (a non-row reader).
	run := kernel.Run{ID: "run_filter_json", TenantID: "tenant_filter_json", Status: kernel.RunRunning}
	b := memio.NewBinding()
	tokJSON := mintIntoBinding(t, b, run, "100", "USD")

	predicate := &kernel.RowPredicate{Field: "row.account", Op: "eq", Value: "4000"}
	_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
		Run:       run,
		Op:        "filter.sum_where",
		Inputs:    []kernel.Token{tokJSON},
		Predicate: predicate,
	})
	if !errors.Is(err, derive.ErrPredicateNotRowShaped) {
		t.Fatalf("Derive() error = %v, want %v", err, derive.ErrPredicateNotRowShaped)
	}
}

// ---------------------------------------------------------------------------
// Fuzz: RowPredicate.Op — only "eq" and "in" accepted
// ---------------------------------------------------------------------------

func FuzzRowPredicateOp(f *testing.F) {
	// Seed corpus: valid ops and some invalid ones.
	f.Add("eq")
	f.Add("in")
	f.Add("")
	f.Add("gt")
	f.Add("gte")
	f.Add("not")
	f.Add("contains")
	f.Add("regex")
	f.Add("OR")

	f.Fuzz(func(t *testing.T, opStr string) {
		run := kernel.Run{ID: "run_fuzz_predop", TenantID: "tenant_fuzz", Status: kernel.RunRunning}
		b := memio.NewBinding()
		tok := mintCSVRowToken(t, b, run, "4000", "100", 99)
		predicate := &kernel.RowPredicate{Field: "row.account", Op: opStr, Value: "4000"}

		_, err := derive.Derive(context.Background(), deps(b, fixedClock{t: testTime}), derive.Request{
			Run:       run,
			Op:        "filter.sum_where",
			Inputs:    []kernel.Token{tok},
			Predicate: predicate,
		})
		if opStr == "eq" || opStr == "in" {
			// Valid ops must not return ErrPredicateOp.
			if errors.Is(err, derive.ErrPredicateOp) {
				t.Errorf("op %q should be valid but got ErrPredicateOp", opStr)
			}
		} else {
			// Any other op must return ErrPredicateOp (fail-closed).
			if err == nil || !errors.Is(err, derive.ErrPredicateOp) {
				t.Errorf("op %q should be rejected with ErrPredicateOp but got: %v", opStr, err)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Replay byte-equality extended to Phase 03 ops
// ---------------------------------------------------------------------------

func TestReplayByteEqualityPhase03(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_replay_p3", TenantID: "tenant_replay_p3", Status: kernel.RunRunning}
	b := memio.NewBinding()
	clock := &sequenceClock{times: []time.Time{
		testTime,
		testTime.Add(time.Second),
		testTime.Add(2 * time.Second),
		testTime.Add(3 * time.Second),
		testTime.Add(4 * time.Second),
		testTime.Add(5 * time.Second),
		testTime.Add(6 * time.Second),
		testTime.Add(7 * time.Second),
	}}

	// Mint leaf tokens.
	tok100 := mintIntoBinding(t, b, run, "100", "USD")
	tok50 := mintIntoBinding(t, b, run, "50", "USD")
	tok2 := mintIntoBinding(t, b, run, "2", "")

	// calc.multiply: 100 USD × 2 → 200 USD
	resMul, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "calc.multiply",
		Inputs: []kernel.Token{tok100, tok2},
	})
	if err != nil {
		t.Fatalf("Derive multiply: %v", err)
	}

	// calc.divide: 200 USD / 50 USD → 4 (dimensionless), scale 9
	_, err = derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:      run,
		Op:       "calc.divide",
		Inputs:   []kernel.Token{resMul.Token, tok50},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
	})
	if err != nil {
		t.Fatalf("Derive divide: %v", err)
	}

	// select.passthrough: 50 USD → 50 USD
	resPass, err := derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:    run,
		Op:     "select.passthrough",
		Inputs: []kernel.Token{tok50},
	})
	if err != nil {
		t.Fatalf("Derive passthrough: %v", err)
	}

	// round.half_even: 50 USD → 50 USD at scale 2
	_, err = derive.Derive(context.Background(), deps(b, clock), derive.Request{
		Run:      run,
		Op:       "round.half_even",
		Inputs:   []kernel.Token{resPass.Token},
		Rounding: kernel.RoundingPolicy{Mode: "half_even", Scale: 2},
	})
	if err != nil {
		t.Fatalf("Derive round: %v", err)
	}

	// Snapshot and emit bundle.
	state := snapshotBindingState(t, b, run)
	bundleBytes1, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("Emit(): %v", err)
	}
	if err := bundle.Verify(bundleBytes1); err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	replayed, err := bundle.Replay(bundleBytes1)
	if err != nil {
		t.Fatalf("Replay(): %v", err)
	}
	bundleBytes2, err := bundle.Emit(replayed)
	if err != nil {
		t.Fatalf("Emit(Replay()): %v", err)
	}
	if !bytes.Equal(bundleBytes1, bundleBytes2) {
		t.Fatal("Emit(Replay()) bytes differ — Phase 03 replay byte-equality broken")
	}
}
