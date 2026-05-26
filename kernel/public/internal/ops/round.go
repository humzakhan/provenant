package ops

import (
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

// Round implements the round.half_even op: exactly 1 input.
// Rounds the input to RoundingPolicy.Scale fractional digits using banker's
// (half-even) rounding. Unit-preserving. RoundingPolicy.Mode must be "" or
// "half_even"; any other mode returns ErrRoundingMode.
type Round struct{}

func (Round) Name() string { return "round.half_even" }

func (Round) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error) {
	if len(inputs) != 1 {
		return Evaluation{}, fmt.Errorf("%w: round.half_even requires 1 input, got %d", ErrArity, len(inputs))
	}
	switch rounding.Mode {
	case "", "half_even":
		// valid
	default:
		return Evaluation{}, fmt.Errorf("%w: round.half_even only supports mode \"\" or \"half_even\", got %q", ErrRoundingMode, rounding.Mode)
	}
	d, err := ParseCanonical(inputs[0].ValueCanonical)
	if err != nil {
		return Evaluation{}, err
	}
	// For round.half_even the caller's Scale is used verbatim. Scale 0 means
	// "round to integer", not "use default scale 9". We only fall back to the
	// canonical default when no policy was supplied at all (Mode is "" AND Scale
	// is 0), which means the caller wants the pass-through canonical form.
	scale := int32(rounding.Scale)
	if rounding.Mode == "" && scale == 0 {
		scale = defaultScale
	}
	value := d.RoundBank(scale)
	return Evaluation{Value: value, Unit: inputs[0].Unit}, nil
}
