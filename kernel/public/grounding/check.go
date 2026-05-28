package grounding

import kernel "github.com/humzakhan/provenant/kernel/public"

// Verdict reports whether every extracted numeric claim is grounded by a token.
type Verdict struct {
	OK               bool              `json:"ok"`
	Pass             bool              `json:"pass"`
	Fail             bool              `json:"fail"`
	GroundedClaims   []GroundedClaim   `json:"grounded_claims"`
	UngroundedClaims []UngroundedClaim `json:"ungrounded_claims"`
}

// GroundedClaim is a numeric claim backed by a matching token.
type GroundedClaim struct {
	Value   string `json:"value"`
	Raw     string `json:"raw"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	TokenID string `json:"token_id"`
}

// UngroundedClaim is a numeric claim with no matching token.
type UngroundedClaim struct {
	Value  string `json:"value"`
	Raw    string `json:"raw"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Reason string `json:"reason"`
}

// CheckAnswer verifies that every extracted numeric claim has at least one
// token with the same canonical value.
func CheckAnswer(answer string, tokens []kernel.Token) (Verdict, error) {
	tokenIDsByValue := make(map[string]string, len(tokens))
	for _, tok := range tokens {
		if _, exists := tokenIDsByValue[tok.ValueCanonical]; exists {
			continue
		}
		tokenIDsByValue[tok.ValueCanonical] = tok.ID
	}

	claims := ExtractNumericClaims(answer)
	verdict := Verdict{
		GroundedClaims:   make([]GroundedClaim, 0, len(claims)),
		UngroundedClaims: make([]UngroundedClaim, 0),
	}
	for _, claim := range claims {
		tokenID, ok := tokenIDsByValue[claim.ValueCanonical]
		if ok {
			verdict.GroundedClaims = append(verdict.GroundedClaims, GroundedClaim{
				Value:   claim.ValueCanonical,
				Raw:     claim.Raw,
				Start:   claim.Start,
				End:     claim.End,
				TokenID: tokenID,
			})
			continue
		}
		verdict.UngroundedClaims = append(verdict.UngroundedClaims, UngroundedClaim{
			Value:  claim.ValueCanonical,
			Raw:    claim.Raw,
			Start:  claim.Start,
			End:    claim.End,
			Reason: "no token with matching value_canonical",
		})
	}

	verdict.OK = len(verdict.UngroundedClaims) == 0
	verdict.Pass = verdict.OK
	verdict.Fail = !verdict.OK
	return verdict, nil
}
