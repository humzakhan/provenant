package ops

import (
	"fmt"

	kernel "github.com/provenant/provenant/kernel/public"
)

// Add implements the calc.add op: exactly 2 inputs, same unit,
// result = inputs[0] + inputs[1].
type Add struct{}

func (Add) Name() string { return "calc.add" }

func (Add) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) != 2 {
		return Evaluation{}, fmt.Errorf("%w: calc.add requires 2 inputs, got %d", ErrArity, len(inputs))
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
	value := ApplyRounding(a.Add(b), rounding)
	return Evaluation{Value: value, Unit: unit}, nil
}
