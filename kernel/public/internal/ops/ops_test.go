package ops_test

import (
	"errors"
	"testing"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/internal/ops"
)

func tok(value, unit string) kernel.Token {
	return kernel.Token{
		ID:             "tok_" + value + "_" + unit,
		RunID:          "run_test",
		TenantID:       "tenant_test",
		ValueCanonical: value,
		Unit:           unit,
	}
}

var defaultRounding = kernel.RoundingPolicy{}

// ---------------------------------------------------------------------------
// RequireSameUnit
// ---------------------------------------------------------------------------

func TestRequireSameUnit(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		_, err := ops.RequireSameUnit(nil)
		if !errors.Is(err, ops.ErrNoInputs) {
			t.Fatalf("got %v, want %v", err, ops.ErrNoInputs)
		}
	})

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		unit, err := ops.RequireSameUnit([]kernel.Token{tok("1.000000000", "USD")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if unit != "USD" {
			t.Fatalf("unit = %q, want %q", unit, "USD")
		}
	})

	t.Run("same units", func(t *testing.T) {
		t.Parallel()
		unit, err := ops.RequireSameUnit([]kernel.Token{tok("1.000000000", "USD"), tok("2.000000000", "USD")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if unit != "USD" {
			t.Fatalf("unit = %q, want %q", unit, "USD")
		}
	})

	t.Run("mismatched units", func(t *testing.T) {
		t.Parallel()
		_, err := ops.RequireSameUnit([]kernel.Token{tok("1.000000000", "USD"), tok("2.000000000", "EUR")})
		if !errors.Is(err, ops.ErrUnitMismatch) {
			t.Fatalf("got %v, want %v", err, ops.ErrUnitMismatch)
		}
	})
}

// ---------------------------------------------------------------------------
// FormatCanonical
// ---------------------------------------------------------------------------

func TestFormatCanonical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"100", "100.000000000"},
		{"0", "0.000000000"},
		{"1.5", "1.500000000"},
		{"-70", "-70.000000000"},
		{"0.000000001", "0.000000001"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			d, err := ops.ParseCanonical(tt.input)
			if err != nil {
				t.Fatalf("ParseCanonical(%q): %v", tt.input, err)
			}
			got := ops.FormatCanonical(d)
			if got != tt.want {
				t.Fatalf("FormatCanonical(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Add
// ---------------------------------------------------------------------------

func TestAddHappyPath(t *testing.T) {
	t.Parallel()

	ev, err := ops.Add{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("23.000000000", "USD")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Add.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "123.000000000" {
		t.Errorf("value = %s, want 123.000000000", ops.FormatCanonical(ev.Value))
	}
	if ev.Unit != "USD" {
		t.Errorf("unit = %q, want %q", ev.Unit, "USD")
	}
}

func TestAddArityOne(t *testing.T) {
	t.Parallel()

	_, err := ops.Add{}.Evaluate([]kernel.Token{tok("1.000000000", "USD")}, defaultRounding)
	if !errors.Is(err, ops.ErrArity) {
		t.Fatalf("got %v, want %v", err, ops.ErrArity)
	}
}

func TestAddArityThree(t *testing.T) {
	t.Parallel()

	_, err := ops.Add{}.Evaluate(
		[]kernel.Token{tok("1.000000000", "USD"), tok("2.000000000", "USD"), tok("3.000000000", "USD")},
		defaultRounding,
	)
	if !errors.Is(err, ops.ErrArity) {
		t.Fatalf("got %v, want %v", err, ops.ErrArity)
	}
}

func TestAddUnitMismatch(t *testing.T) {
	t.Parallel()

	_, err := ops.Add{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("50.000000000", "EUR")},
		defaultRounding,
	)
	if !errors.Is(err, ops.ErrUnitMismatch) {
		t.Fatalf("got %v, want %v", err, ops.ErrUnitMismatch)
	}
}

// ---------------------------------------------------------------------------
// Subtract
// ---------------------------------------------------------------------------

func TestSubtractOrder(t *testing.T) {
	t.Parallel()

	ev, err := ops.Subtract{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("30.000000000", "USD")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Subtract.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "70.000000000" {
		t.Errorf("100-30 = %s, want 70.000000000", ops.FormatCanonical(ev.Value))
	}

	ev2, err := ops.Subtract{}.Evaluate(
		[]kernel.Token{tok("30.000000000", "USD"), tok("100.000000000", "USD")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Subtract.Evaluate() swapped: %v", err)
	}
	if ops.FormatCanonical(ev2.Value) != "-70.000000000" {
		t.Errorf("30-100 = %s, want -70.000000000", ops.FormatCanonical(ev2.Value))
	}
}

// ---------------------------------------------------------------------------
// Sum
// ---------------------------------------------------------------------------

func TestSumSingle(t *testing.T) {
	t.Parallel()

	ev, err := ops.Sum{}.Evaluate(
		[]kernel.Token{tok("42.000000000", "EUR")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Sum.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "42.000000000" {
		t.Errorf("sum(42) = %s, want 42.000000000", ops.FormatCanonical(ev.Value))
	}
}

func TestSumMultiple(t *testing.T) {
	t.Parallel()

	ev, err := ops.Sum{}.Evaluate(
		[]kernel.Token{tok("1.000000000", "USD"), tok("2.000000000", "USD"), tok("3.000000000", "USD")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Sum.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "6.000000000" {
		t.Errorf("sum(1,2,3) = %s, want 6.000000000", ops.FormatCanonical(ev.Value))
	}
}

func TestSumEmptyReturnsErrNoInputs(t *testing.T) {
	t.Parallel()

	_, err := ops.Sum{}.Evaluate(nil, defaultRounding)
	if !errors.Is(err, ops.ErrNoInputs) {
		t.Fatalf("got %v, want %v", err, ops.ErrNoInputs)
	}
}

func TestSumUnitMismatch(t *testing.T) {
	t.Parallel()

	_, err := ops.Sum{}.Evaluate(
		[]kernel.Token{tok("1.000000000", "USD"), tok("2.000000000", "EUR")},
		defaultRounding,
	)
	if !errors.Is(err, ops.ErrUnitMismatch) {
		t.Fatalf("got %v, want %v", err, ops.ErrUnitMismatch)
	}
}

// ---------------------------------------------------------------------------
// calc.multiply
// ---------------------------------------------------------------------------

func TestMultiplyUSDByDimensionless(t *testing.T) {
	t.Parallel()

	// USD × ""  → USD
	ev, err := ops.Multiply{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("2.000000000", "")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Multiply.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "200.000000000" {
		t.Errorf("value = %s, want 200.000000000", ops.FormatCanonical(ev.Value))
	}
	if ev.Unit != "USD" {
		t.Errorf("unit = %q, want %q", ev.Unit, "USD")
	}

	// ""  × USD → USD (reverse order)
	ev2, err := ops.Multiply{}.Evaluate(
		[]kernel.Token{tok("2.000000000", ""), tok("100.000000000", "USD")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Multiply.Evaluate() reversed: %v", err)
	}
	if ev2.Unit != "USD" {
		t.Errorf("reversed unit = %q, want %q", ev2.Unit, "USD")
	}
	if ops.FormatCanonical(ev2.Value) != "200.000000000" {
		t.Errorf("reversed value = %s, want 200.000000000", ops.FormatCanonical(ev2.Value))
	}
}

func TestMultiplyDimensionlessByDimensionless(t *testing.T) {
	t.Parallel()

	ev, err := ops.Multiply{}.Evaluate(
		[]kernel.Token{tok("3.000000000", ""), tok("4.000000000", "")},
		defaultRounding,
	)
	if err != nil {
		t.Fatalf("Multiply.Evaluate(): %v", err)
	}
	if ev.Unit != "" {
		t.Errorf("unit = %q, want %q", ev.Unit, "")
	}
}

func TestMultiplyUSDByUSDReturnsErrUnitMismatch(t *testing.T) {
	t.Parallel()

	_, err := ops.Multiply{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("2.000000000", "USD")},
		defaultRounding,
	)
	if !errors.Is(err, ops.ErrUnitMismatch) {
		t.Fatalf("got %v, want %v", err, ops.ErrUnitMismatch)
	}
}

func TestMultiplyUSDByEURReturnsErrUnitMismatch(t *testing.T) {
	t.Parallel()

	_, err := ops.Multiply{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("2.000000000", "EUR")},
		defaultRounding,
	)
	if !errors.Is(err, ops.ErrUnitMismatch) {
		t.Fatalf("got %v, want %v", err, ops.ErrUnitMismatch)
	}
}

// ---------------------------------------------------------------------------
// calc.divide
// ---------------------------------------------------------------------------

func TestDivideUSDByUSDReturnsDimensionless(t *testing.T) {
	t.Parallel()

	rounding := kernel.RoundingPolicy{Mode: "half_even", Scale: 9}
	ev, err := ops.Divide{}.Evaluate(
		[]kernel.Token{tok("300.000000000", "USD"), tok("100.000000000", "USD")},
		rounding,
	)
	if err != nil {
		t.Fatalf("Divide.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "3.000000000" {
		t.Errorf("value = %s, want 3.000000000", ops.FormatCanonical(ev.Value))
	}
	if ev.Unit != "" {
		t.Errorf("unit = %q, want empty (dimensionless)", ev.Unit)
	}
}

func TestDivideByZeroReturnsErrDivisionByZero(t *testing.T) {
	t.Parallel()

	_, err := ops.Divide{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("0.000000000", "USD")},
		kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
	)
	if !errors.Is(err, ops.ErrDivisionByZero) {
		t.Fatalf("got %v, want %v", err, ops.ErrDivisionByZero)
	}
}

func TestDivideMissingScaleReturnsErrRoundingRequired(t *testing.T) {
	t.Parallel()

	_, err := ops.Divide{}.Evaluate(
		[]kernel.Token{tok("100.000000000", "USD"), tok("3.000000000", "USD")},
		kernel.RoundingPolicy{}, // scale == 0 → ErrRoundingRequired
	)
	if !errors.Is(err, ops.ErrRoundingRequired) {
		t.Fatalf("got %v, want %v", err, ops.ErrRoundingRequired)
	}
}

// ---------------------------------------------------------------------------
// round.half_even
// ---------------------------------------------------------------------------

func TestRoundHalfEvenBoundary(t *testing.T) {
	t.Parallel()

	// Banker's rounding spec vectors from the phase 03 plan.
	tests := []struct {
		input string
		scale int
		want  string
	}{
		{"2.5", 0, "2"},   // round-half-even: 2.5 → 2 (2 is even)
		{"3.5", 0, "4"},   // round-half-even: 3.5 → 4 (4 is even)
		{"-2.5", 0, "-2"}, // round-half-even: -2.5 → -2 (2 is even)
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			rounding := kernel.RoundingPolicy{Mode: "half_even", Scale: tt.scale}
			ev, err := ops.Round{}.Evaluate(
				[]kernel.Token{tok(tt.input, "USD")},
				rounding,
			)
			if err != nil {
				t.Fatalf("Round.Evaluate(%q, scale %d): %v", tt.input, tt.scale, err)
			}
			got := ev.Value.StringFixed(int32(tt.scale))
			if got != tt.want {
				t.Errorf("Round(%q, %d) = %q, want %q", tt.input, tt.scale, got, tt.want)
			}
			if ev.Unit != "USD" {
				t.Errorf("unit = %q, want USD", ev.Unit)
			}
		})
	}
}

func TestRoundHalfEvenUnsupportedModeReturnsErrRoundingMode(t *testing.T) {
	t.Parallel()

	_, err := ops.Round{}.Evaluate(
		[]kernel.Token{tok("2.5", "USD")},
		kernel.RoundingPolicy{Mode: "half_up", Scale: 0},
	)
	if !errors.Is(err, ops.ErrRoundingMode) {
		t.Fatalf("got %v, want %v", err, ops.ErrRoundingMode)
	}
}

// ---------------------------------------------------------------------------
// select.passthrough
// ---------------------------------------------------------------------------

func TestPassthroughPreservesValueAndUnit(t *testing.T) {
	t.Parallel()

	input := tok("99.500000000", "GBP")
	ev, err := ops.Passthrough{}.Evaluate([]kernel.Token{input}, defaultRounding)
	if err != nil {
		t.Fatalf("Passthrough.Evaluate(): %v", err)
	}
	if ops.FormatCanonical(ev.Value) != "99.500000000" {
		t.Errorf("value = %s, want 99.500000000", ops.FormatCanonical(ev.Value))
	}
	if ev.Unit != "GBP" {
		t.Errorf("unit = %q, want GBP", ev.Unit)
	}
}

func TestPassthroughArityTwo(t *testing.T) {
	t.Parallel()

	_, err := ops.Passthrough{}.Evaluate(
		[]kernel.Token{tok("1.000000000", "USD"), tok("2.000000000", "USD")},
		defaultRounding,
	)
	if !errors.Is(err, ops.ErrArity) {
		t.Fatalf("got %v, want %v", err, ops.ErrArity)
	}
}

// ---------------------------------------------------------------------------
// All() registry
// ---------------------------------------------------------------------------

func TestAllRegistersEightOps(t *testing.T) {
	t.Parallel()

	all := ops.All()
	if len(all) != 8 {
		t.Fatalf("len(All()) = %d, want 8", len(all))
	}
	names := make(map[string]bool, len(all))
	for _, op := range all {
		names[op.Name()] = true
	}
	for _, want := range []string{
		"calc.add", "calc.subtract", "calc.sum",
		"calc.multiply", "calc.divide",
		"select.passthrough", "round.half_even",
		"filter.sum_where",
	} {
		if !names[want] {
			t.Errorf("op %q not in All()", want)
		}
	}
}
