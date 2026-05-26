package ops

import (
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

// Multiply implements the calc.multiply op: exactly 2 inputs.
// result = inputs[0] * inputs[1].
// Rounding defaults to half_even at scale 9. The audit entry preserves
// the supplied input order; the value is commutative.
type Multiply struct{}

func (Multiply) Name() string { return "calc.multiply" }

func (Multiply) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) != 2 {
		return Evaluation{}, fmt.Errorf("%w: calc.multiply requires 2 inputs, got %d", ErrArity, len(inputs))
	}
	unit, err := MultiplyUnit(inputs[0].Unit, inputs[1].Unit)
	if err != nil {
		return Evaluation{}, err
	}
	a, err := ParseCanonical(inputs[0].ValueCanonical)
	if err != nil {
		return Evaluation{}, err
	}
	b, err := ParseCanonical(inputs[1].ValueCanonical)
	if err != nil {
		return Evaluation{}, err
	}
	value := ApplyRounding(a.Mul(b), rounding)
	return Evaluation{Value: value, Unit: unit}, nil
}
