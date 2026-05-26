package ops

import (
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/shopspring/decimal"
)

// Divide implements the calc.divide op: exactly 2 inputs.
// result = inputs[0] / inputs[1].
//
// Rounding is mandatory: callers must supply a non-zero Scale to force
// explicit precision. Division by zero returns ErrDivisionByZero.
type Divide struct{}

func (Divide) Name() string { return "calc.divide" }

func (Divide) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) != 2 {
		return Evaluation{}, fmt.Errorf("%w: calc.divide requires 2 inputs, got %d", ErrArity, len(inputs))
	}
	if rounding.Scale == 0 {
		return Evaluation{}, fmt.Errorf("%w: calc.divide requires an explicit rounding scale", ErrRoundingRequired)
	}
	unit, err := DivideUnit(inputs[0].Unit, inputs[1].Unit)
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
	if b.Equal(decimal.Zero) {
		return Evaluation{}, ErrDivisionByZero
	}
	value := ApplyRounding(a.Div(b), rounding)
	return Evaluation{Value: value, Unit: unit}, nil
}
