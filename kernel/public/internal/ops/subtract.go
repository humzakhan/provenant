package ops

import (
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

// Subtract implements the calc.subtract op: exactly 2 inputs, same unit,
// result = inputs[0] - inputs[1] (minuend - subtrahend). Order is significant.
type Subtract struct{}

func (Subtract) Name() string { return "calc.subtract" }

func (Subtract) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) != 2 {
		return Evaluation{}, fmt.Errorf("%w: calc.subtract requires 2 inputs, got %d", ErrArity, len(inputs))
	}
	unit, err := RequireSameUnit(inputs)
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
	value := ApplyRounding(a.Sub(b), rounding)
	return Evaluation{Value: value, Unit: unit}, nil
}
