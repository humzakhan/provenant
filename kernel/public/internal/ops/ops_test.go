package ops_test

import (
	"errors"
	"testing"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/internal/ops"
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
// All() registry
// ---------------------------------------------------------------------------

func TestAllRegistersThreeOps(t *testing.T) {
	t.Parallel()

	all := ops.All()
	if len(all) != 3 {
		t.Fatalf("len(All()) = %d, want 3", len(all))
	}
	names := make(map[string]bool, len(all))
	for _, op := range all {
		names[op.Name()] = true
	}
	for _, want := range []string{"calc.add", "calc.subtract", "calc.sum"} {
		if !names[want] {
			t.Errorf("op %q not in All()", want)
		}
	}
}
