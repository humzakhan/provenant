package grounding

import "testing"

func TestExtractNumericClaims(t *testing.T) {
	answer := "Revenue was $1,247,322.18 and margin value was 47322.18. EU cash was €1.234.567,89; yen was ¥1,200. Count was 5."

	claims := ExtractNumericClaims(answer)
	want := []NumericClaim{
		{Raw: "$1,247,322.18", ValueCanonical: "1247322.180000000"},
		{Raw: "47322.18", ValueCanonical: "47322.180000000"},
		{Raw: "€1.234.567,89", ValueCanonical: "1234567.890000000"},
		{Raw: "¥1,200", ValueCanonical: "1200.000000000"},
		{Raw: "5", ValueCanonical: "5.000000000"},
	}
	if len(claims) != len(want) {
		t.Fatalf("len(claims) = %d, want %d: %#v", len(claims), len(want), claims)
	}
	for i := range want {
		if claims[i].Raw != want[i].Raw {
			t.Errorf("claims[%d].Raw = %q, want %q", i, claims[i].Raw, want[i].Raw)
		}
		if claims[i].ValueCanonical != want[i].ValueCanonical {
			t.Errorf("claims[%d].ValueCanonical = %q, want %q", i, claims[i].ValueCanonical, want[i].ValueCanonical)
		}
		if gotRaw := answer[claims[i].Start:claims[i].End]; gotRaw != claims[i].Raw {
			t.Errorf("claims[%d] span extracts %q, want %q", i, gotRaw, claims[i].Raw)
		}
	}
}

func TestExtractNumericClaimsIgnoresKnownFalsePositives(t *testing.T) {
	answer := `1. Summary
On 2026-05-28 and May 28, 2026 we shipped version 1.2 and v1.2.3 after checking [4].
The 1st sample had 12.5% uplift and 47 bps movement.
Calendar year 2026 is context, not a claim.
Manual note 999 [[unverified]] is escaped.
Small ratio 0.78 is below the PR1 plain-number threshold.`

	claims := ExtractNumericClaims(answer)
	if len(claims) != 0 {
		t.Fatalf("claims = %#v, want none", claims)
	}
}

func TestExtractNumericClaimsPlainThresholdAndRounding(t *testing.T) {
	answer := "The rounded amount is 2.3456789125 and the small fee is $0.78."

	claims := ExtractNumericClaims(answer)
	want := []NumericClaim{
		{Raw: "2.3456789125", ValueCanonical: "2.345678912"},
		{Raw: "$0.78", ValueCanonical: "0.780000000"},
	}
	if len(claims) != len(want) {
		t.Fatalf("len(claims) = %d, want %d: %#v", len(claims), len(want), claims)
	}
	for i := range want {
		if claims[i].Raw != want[i].Raw || claims[i].ValueCanonical != want[i].ValueCanonical {
			t.Errorf("claims[%d] = %#v, want %#v", i, claims[i], want[i])
		}
	}
}

func TestExtractNumericClaimsValidatesGroupingSpaces(t *testing.T) {
	answer := "Accepted 1 234 and 1 234 567. Rejected 12 34 and 123 45."

	claims := ExtractNumericClaims(answer)
	want := []NumericClaim{
		{Raw: "1 234", ValueCanonical: "1234.000000000"},
		{Raw: "1 234 567", ValueCanonical: "1234567.000000000"},
	}
	if len(claims) != len(want) {
		t.Fatalf("len(claims) = %d, want %d: %#v", len(claims), len(want), claims)
	}
	for i := range want {
		if claims[i].Raw != want[i].Raw || claims[i].ValueCanonical != want[i].ValueCanonical {
			t.Errorf("claims[%d] = %#v, want %#v", i, claims[i], want[i])
		}
	}
}

func TestExtractNumericClaimsCurrencyFractionsAndCitationClusters(t *testing.T) {
	answer := "Fees were $.50 and £.99 after discounts; references [1, 2] and [4] are citations."

	claims := ExtractNumericClaims(answer)
	want := []NumericClaim{
		{Raw: "$.50", ValueCanonical: "0.500000000"},
		{Raw: "£.99", ValueCanonical: "0.990000000"},
	}
	if len(claims) != len(want) {
		t.Fatalf("len(claims) = %d, want %d: %#v", len(claims), len(want), claims)
	}
	for i := range want {
		if claims[i].Raw != want[i].Raw || claims[i].ValueCanonical != want[i].ValueCanonical {
			t.Errorf("claims[%d] = %#v, want %#v", i, claims[i], want[i])
		}
	}
}

func TestExtractNumericClaimsRejectsAmbiguousSingleDotEuro(t *testing.T) {
	answer := "Ignored €1.234. Accepted €12.34 and €1.234,56."

	claims := ExtractNumericClaims(answer)
	want := []NumericClaim{
		{Raw: "€12.34", ValueCanonical: "12.340000000"},
		{Raw: "€1.234,56", ValueCanonical: "1234.560000000"},
	}
	if len(claims) != len(want) {
		t.Fatalf("len(claims) = %d, want %d: %#v", len(claims), len(want), claims)
	}
	for i := range want {
		if claims[i].Raw != want[i].Raw || claims[i].ValueCanonical != want[i].ValueCanonical {
			t.Errorf("claims[%d] = %#v, want %#v", i, claims[i], want[i])
		}
	}
}
