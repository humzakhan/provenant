package phaseone

import (
	"errors"
	"testing"
)

func TestExtractMintValueEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  []byte
		want    string
		wantErr error
	}{
		{
			name:   "numeric prefers raw exponential lexeme",
			output: []byte(`{"value":1000,"raw_value":"1e3"}`),
			want:   "1e3",
		},
		{
			name:   "numeric preserves signed zero from raw value",
			output: []byte(`{"value":0,"raw_value":"-0.0"}`),
			want:   "-0.0",
		},
		{
			name:   "numeric preserves trailing zeros from raw value",
			output: []byte(`{"value":1.23,"raw_value":"1.2300"}`),
			want:   "1.2300",
		},
		{
			name:   "numeric falls back to decoded number without raw value",
			output: []byte(`{"value":1000}`),
			want:   "1000",
		},
		{
			name:   "string ignores quoted raw json text",
			output: []byte(`{"value":"56.78","raw_value":"\"56.78\""}`),
			want:   "56.78",
		},
		{
			name:    "object remains fail closed",
			output:  []byte(`{"value":{"amount":1000},"raw_value":"{\"amount\":1000}"}`),
			wantErr: ErrMintValueNotNumeric,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ExtractMintValue(tt.output, "$.value")
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ExtractMintValue() error = nil, want %v", tt.wantErr)
				}
				if got != "" {
					t.Fatalf("ExtractMintValue() value = %q, want empty on error", got)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ExtractMintValue() error = %v, want wrapped %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractMintValue() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ExtractMintValue() = %q, want %q", got, tt.want)
			}
		})
	}
}
