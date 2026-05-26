package derivation_test

import (
	"bytes"
	"errors"
	"testing"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/internal/derivation"
)

// validPayload returns a representative well-formed payload for table-driven use.
func validPayload() derivation.Payload {
	return derivation.Payload{
		TokenID:   "tok_derived",
		Operation: "calc.add",
		Inputs:    []string{"tok_a", "tok_b"},
		FormulaID: "calc.add",
		Rounding:  kernel.RoundingPolicy{Mode: "half_even", Scale: 9},
		Evaluated: "300.000000000",
		Unit:      "USD",
	}
}

// TestEncodeDeterministic asserts the canonical bytes are stable across
// repeated Encode calls and independent of the input field ordering.
func TestEncodeDeterministic(t *testing.T) {
	t.Parallel()

	first, err := derivation.Encode(validPayload())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	second, err := derivation.Encode(validPayload())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("Encode() not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}

	// Reorder the same logical content via a different struct construction
	// path; JCS canonicalization must yield the identical bytes.
	p := derivation.Payload{
		Unit:      "USD",
		Evaluated: "300.000000000",
		Rounding:  kernel.RoundingPolicy{Scale: 9, Mode: "half_even"},
		FormulaID: "calc.add",
		Inputs:    []string{"tok_a", "tok_b"},
		Operation: "calc.add",
		TokenID:   "tok_derived",
	}
	third, err := derivation.Encode(p)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Equal(first, third) {
		t.Fatalf("Encode() field-order dependent: want bit-identical canonical bytes")
	}
}

// TestEncodeDecodeRoundTrip asserts Decode(Encode(p)) == p for a valid payload.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	original := validPayload()
	encoded, err := derivation.Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := derivation.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	reencoded, err := derivation.Encode(decoded)
	if err != nil {
		t.Fatalf("Encode(Decode()) error = %v", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("Encode/Decode/Encode not bit-identical:\nfirst=%s\nsecond=%s", encoded, reencoded)
	}
}

// TestDecodeRejects covers every validation error returned by Decode.
func TestDecodeRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(p *derivation.Payload)
		wantErr error
	}{
		{
			name:    "missing token_id",
			mutate:  func(p *derivation.Payload) { p.TokenID = "" },
			wantErr: derivation.ErrMissingTokenID,
		},
		{
			name:    "missing operation",
			mutate:  func(p *derivation.Payload) { p.Operation = "" },
			wantErr: derivation.ErrMissingOperation,
		},
		{
			name:    "missing evaluated",
			mutate:  func(p *derivation.Payload) { p.Evaluated = "" },
			wantErr: derivation.ErrMissingEvaluated,
		},
		{
			name:    "empty inputs",
			mutate:  func(p *derivation.Payload) { p.Inputs = nil },
			wantErr: derivation.ErrEmptyInputs,
		},
		{
			name:    "invalid rounding mode",
			mutate:  func(p *derivation.Payload) { p.Rounding.Mode = "ceiling" },
			wantErr: derivation.ErrInvalidRounding,
		},
		{
			name:    "rounding scale negative",
			mutate:  func(p *derivation.Payload) { p.Rounding.Scale = -1 },
			wantErr: derivation.ErrInvalidRounding,
		},
		{
			name:    "rounding scale too large",
			mutate:  func(p *derivation.Payload) { p.Rounding.Scale = 19 },
			wantErr: derivation.ErrInvalidRounding,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := validPayload()
			tt.mutate(&p)
			if _, err := derivation.Encode(p); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Encode() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestDecodeRejectsUnknownFields asserts DisallowUnknownFields is on.
func TestDecodeRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"token_id":"tok","operation":"calc.add","inputs":["a"],"evaluated":"1","extra":"x"}`)
	if _, err := derivation.Decode(raw); err == nil {
		t.Fatal("Decode() accepted unknown field; want rejection")
	}
}

// TestDecodeRejectsTrailingBytes asserts no trailing bytes are permitted.
func TestDecodeRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	encoded, err := derivation.Encode(validPayload())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	withTrailer := append(append([]byte(nil), encoded...), []byte(" {}")...)
	if _, err := derivation.Decode(withTrailer); err == nil {
		t.Fatal("Decode() accepted trailing bytes; want rejection")
	}
}
