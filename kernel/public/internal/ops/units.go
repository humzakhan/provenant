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
