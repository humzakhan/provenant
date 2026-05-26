package ops

import (
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

// Passthrough implements the select.passthrough op: exactly 1 input.
// Returns a derived token whose value and unit equal the input. Used to
// re-anchor a leaf token under a logical name, giving the audit chain an
// explicit re-anchor event.
type Passthrough struct{}

func (Passthrough) Name() string { return "select.passthrough" }

func (Passthrough) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) != 1 {
		return Evaluation{}, fmt.Errorf("%w: select.passthrough requires 1 input, got %d", ErrArity, len(inputs))
	}
	d, err := ParseCanonical(inputs[0].ValueCanonical)
	if err != nil {
		return Evaluation{}, err
	}
	// Passthrough preserves the value exactly; rounding is a no-op here because
	// the value is already in canonical form. We still call ApplyRounding so that
	// any explicit caller-supplied policy is honoured.
	value := ApplyRounding(d, rounding)
	return Evaluation{Value: value, Unit: inputs[0].Unit}, nil
}
