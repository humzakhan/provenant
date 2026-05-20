package ops

import (
	"fmt"

	kernel "github.com/provenant/provenant/kernel/public"
)

// RequireSameUnit returns the shared unit of all inputs, or an error if the
// inputs are empty or carry different units. It is used by all additive ops.
func RequireSameUnit(inputs []kernel.Token) (string, error) {
	if len(inputs) == 0 {
		return "", ErrNoInputs
	}
	unit := inputs[0].Unit
	for _, t := range inputs[1:] {
		if t.Unit != unit {
			return "", fmt.Errorf("%w: %s vs %s", ErrUnitMismatch, unit, t.Unit)
		}
	}
	return unit, nil
}

// MultiplyUnit resolves the output unit for a * b.
//
// Unit algebra:
//
//	USD × ""  → USD   (currency × dimensionless)
//	""  × USD → USD   (dimensionless × currency)
//	""  × ""  → ""    (dimensionless × dimensionless)
//	USD × USD → ErrUnitMismatch  (squared currency not modelled)
//	USD × EUR → ErrUnitMismatch  (mixed currencies)
func MultiplyUnit(a, b string) (string, error) {
	switch {
	case a == "" && b == "":
		return "", nil
	case a == "":
		return b, nil
	case b == "":
		return a, nil
	case a == b:
		return "", fmt.Errorf("%w: %s × %s (squared currency)", ErrUnitMismatch, a, b)
	default:
		return "", fmt.Errorf("%w: %s × %s", ErrUnitMismatch, a, b)
	}
}

// DivideUnit resolves the output unit for a / b.
//
// Unit algebra:
//
//	USD / USD → ""    (ratio)
//	USD / ""  → USD   (currency / dimensionless)
//	""  / ""  → ""    (dimensionless / dimensionless)
//	""  / USD → ErrUnitMismatch  (1/USD not modelled)
//	USD / EUR → ErrUnitMismatch  (mixed currencies)
func DivideUnit(a, b string) (string, error) {
	switch {
	case a == "" && b == "":
		return "", nil
	case b == "":
		return a, nil
	case a == b:
		return "", nil
	case a == "":
		return "", fmt.Errorf("%w: 1/%s (inverse currency not modelled)", ErrUnitMismatch, b)
	default:
		return "", fmt.Errorf("%w: %s / %s", ErrUnitMismatch, a, b)
	}
}
