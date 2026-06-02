// Package ops houses the pure per-op implementations for the kernel derivation
// engine. Each op is a stateless function over input tokens; no I/O, no clock.
package ops

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

const defaultScale = 9

// ParseCanonical parses a canonical decimal string from a token's ValueCanonical
// field. It returns ErrDecimalParse on failure.
func ParseCanonical(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%w: %q: %v", ErrDecimalParse, s, err)
	}
	return d, nil
}

// FormatCanonical serialises a decimal to a 9-fractional-digit fixed-point
// string, matching the canonical form used by leaf tokens.
func FormatCanonical(d decimal.Decimal) string {
	rounded := d.RoundBank(defaultScale)
	text := rounded.StringFixedBank(defaultScale)
	if strings.HasPrefix(text, "-0.") {
		return "0." + strings.Repeat("0", defaultScale)
	}
	return text
}

// ApplyRounding applies the caller-supplied RoundingPolicy to d. When the
// policy is the zero value, or when Mode is empty, half-even at scale 9 is
// used (the canonical form). Scale 0 is treated as the default scale 9.
func ApplyRounding(d decimal.Decimal, policy kernel.RoundingPolicy) decimal.Decimal {
	scale := int32(policy.Scale)
	if scale == 0 {
		scale = defaultScale
	}
	switch policy.Mode {
	case "", "half_even":
		return d.RoundBank(scale)
	case "half_up":
		return d.Round(scale)
	case "down":
		return d.RoundDown(scale)
	default:
		// Unknown modes are rejected upstream by the engine; fall back to
		// half_even so the op itself is never stranded.
		return d.RoundBank(scale)
	}
}
