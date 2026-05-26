package ops

import (
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/shopspring/decimal"
)

// Sum implements the calc.sum op: 1..N inputs of the same unit,
// result = sum(inputs). Order is preserved in the audit entry but does not
// affect the computed value.
type Sum struct{}

func (Sum) Name() string { return "calc.sum" }

func (Sum) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) == 0 {
		return Evaluation{}, fmt.Errorf("%w: calc.sum requires at least 1 input", ErrNoInputs)
	}
	unit, err := RequireSameUnit(inputs)
	if err != nil {
		return Evaluation{}, err
	}
	total := decimal.Zero
	for _, tok := range inputs {
		d, err := ParseCanonical(tok.ValueCanonical)
		if err != nil {
			return Evaluation{}, err
		}
		total = total.Add(d)
	}
	value := ApplyRounding(total, rounding)
	return Evaluation{Value: value, Unit: unit}, nil
}
