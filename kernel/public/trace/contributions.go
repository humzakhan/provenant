package trace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

const canonicalScale = int32(9)

// parseCanonical parses a canonical decimal string as stored in Token.ValueCanonical.
func parseCanonical(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("trace: parse decimal %q: %w", s, err)
	}
	return d, nil
}

// formatCanonical serialises a decimal to a 9-fractional-digit fixed-point
// string, matching the canonical form used by kernel/public/internal/ops.
// Negative zero (e.g. -0.000000000) is normalised to positive zero.
func formatCanonical(d decimal.Decimal) string {
	rounded := d.RoundBank(canonicalScale)
	text := rounded.StringFixedBank(canonicalScale)
	// Normalise -0.000...0 → 0.000...0.
	// Only do this when the text is exactly "-0." followed by all zeros.
	if text == "-0."+strings.Repeat("0", int(canonicalScale)) {
		return "0." + strings.Repeat("0", int(canonicalScale))
	}
	return text
}

// marshalJSON marshals v to JSON bytes using encoding/json.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// unmarshalLoose decodes JSON bytes into v, allowing unknown fields (unlike
// the strict decoder used in bundle parsing).
func unmarshalLoose(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
