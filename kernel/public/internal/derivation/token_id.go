package derivation

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// DeriveTokenID computes the deterministic token ID for a derived token using
// the same "tok_" + hexFirst16(sha256(...)) convention as leaf tokens.
//
// The hash input is: runID || NUL || tenantID || NUL || op || NUL ||
// join(inputTokenIDs, NUL) || NUL || valueCanonical || NUL || unit
//
// NUL (\x00) is used as the field separator; it cannot appear in any of
// these string values, so the encoding is injective.
func DeriveTokenID(runID, tenantID, op string, inputIDs []string, valueCanonical, unit string) string {
	var b strings.Builder
	b.WriteString(runID)
	b.WriteByte(0)
	b.WriteString(tenantID)
	b.WriteByte(0)
	b.WriteString(op)
	b.WriteByte(0)
	b.WriteString(strings.Join(inputIDs, "\x00"))
	b.WriteByte(0)
	b.WriteString(valueCanonical)
	b.WriteByte(0)
	b.WriteString(unit)

	sum := sha256.Sum256([]byte(b.String()))
	return "tok_" + hex.EncodeToString(sum[:16])
}
