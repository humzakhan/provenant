package grounding

import (
	"testing"
	"time"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

func TestCheckAnswerGroundedPass(t *testing.T) {
	t.Parallel()

	answer := "Revenue was 42 and fee was $0.50."
	tokens := []kernel.Token{
		{ID: "tok_42", ValueCanonical: "42.000000000", CreatedAt: time.Unix(0, 0)},
		{ID: "tok_050", ValueCanonical: "0.500000000", CreatedAt: time.Unix(0, 0)},
	}

	verdict, err := CheckAnswer(answer, tokens)
	if err != nil {
		t.Fatalf("CheckAnswer() error = %v", err)
	}
	if !verdict.OK || !verdict.Pass || verdict.Fail {
		t.Fatalf("verdict flags = %#v, want ok/pass true and fail false", verdict)
	}
	if len(verdict.GroundedClaims) != 2 {
		t.Fatalf("len(GroundedClaims) = %d, want 2", len(verdict.GroundedClaims))
	}
	if len(verdict.UngroundedClaims) != 0 {
		t.Fatalf("UngroundedClaims = %#v, want none", verdict.UngroundedClaims)
	}

	want := []struct {
		value   string
		raw     string
		tokenID string
	}{
		{value: "42.000000000", raw: "42", tokenID: "tok_42"},
		{value: "0.500000000", raw: "$0.50", tokenID: "tok_050"},
	}
	for i, got := range verdict.GroundedClaims {
		if got.Value != want[i].value || got.Raw != want[i].raw || got.TokenID != want[i].tokenID {
			t.Fatalf("GroundedClaims[%d] = %#v, want value=%q raw=%q token_id=%q", i, got, want[i].value, want[i].raw, want[i].tokenID)
		}
		if answer[got.Start:got.End] != got.Raw {
			t.Fatalf("GroundedClaims[%d] span = %q, want %q", i, answer[got.Start:got.End], got.Raw)
		}
	}
}

func TestCheckAnswerUngroundedFail(t *testing.T) {
	t.Parallel()

	answer := "Revenue was 42 and margin was 7."
	tokens := []kernel.Token{
		{ID: "tok_42", ValueCanonical: "42.000000000", CreatedAt: time.Unix(0, 0)},
	}

	verdict, err := CheckAnswer(answer, tokens)
	if err != nil {
		t.Fatalf("CheckAnswer() error = %v", err)
	}
	if verdict.OK || verdict.Pass || !verdict.Fail {
		t.Fatalf("verdict flags = %#v, want ok/pass false and fail true", verdict)
	}
	if len(verdict.GroundedClaims) != 1 {
		t.Fatalf("len(GroundedClaims) = %d, want 1", len(verdict.GroundedClaims))
	}
	if len(verdict.UngroundedClaims) != 1 {
		t.Fatalf("len(UngroundedClaims) = %d, want 1", len(verdict.UngroundedClaims))
	}

	got := verdict.UngroundedClaims[0]
	if got.Value != "7.000000000" || got.Raw != "7" {
		t.Fatalf("UngroundedClaims[0] = %#v, want value=7.000000000 raw=7", got)
	}
	if got.Reason == "" {
		t.Fatal("UngroundedClaims[0].Reason is empty")
	}
	if answer[got.Start:got.End] != got.Raw {
		t.Fatalf("UngroundedClaims[0] span = %q, want %q", answer[got.Start:got.End], got.Raw)
	}
}
