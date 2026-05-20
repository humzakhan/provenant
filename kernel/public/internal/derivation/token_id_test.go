package derivation_test

import (
	"testing"

	"github.com/provenant/provenant/kernel/public/internal/derivation"
)

func TestDeriveTokenIDDeterministic(t *testing.T) {
	t.Parallel()

	first := derivation.DeriveTokenID("run_1", "tenant_1", "calc.add", []string{"tok_a", "tok_b"}, "200.000000000", "USD")
	second := derivation.DeriveTokenID("run_1", "tenant_1", "calc.add", []string{"tok_a", "tok_b"}, "200.000000000", "USD")
	if first != second {
		t.Fatalf("DeriveTokenID() not deterministic: %q vs %q", first, second)
	}
}

func TestDeriveTokenIDPrefix(t *testing.T) {
	t.Parallel()

	id := derivation.DeriveTokenID("run_1", "tenant_1", "calc.add", []string{"tok_a"}, "100.000000000", "USD")
	if len(id) < 4 || id[:4] != "tok_" {
		t.Fatalf("DeriveTokenID() = %q, want prefix tok_", id)
	}
	// "tok_" (4) + hex-encoded 16 bytes (32) = 36 chars
	const wantLen = 36
	if len(id) != wantLen {
		t.Fatalf("DeriveTokenID() len = %d, want %d", len(id), wantLen)
	}
}

func TestDeriveTokenIDGolden(t *testing.T) {
	t.Parallel()

	// Fixed golden vector — must be stable across Go versions and platforms.
	// Computed once from the reference implementation and locked in.
	id := derivation.DeriveTokenID(
		"run_abc",
		"tenant_xyz",
		"calc.add",
		[]string{"tok_001", "tok_002"},
		"300.000000000",
		"USD",
	)
	const want = "tok_cb408cdc2e9216d6ae0bd14071b70049"
	if id != want {
		t.Fatalf("DeriveTokenID() = %q, want %q", id, want)
	}
}

func TestDeriveTokenIDDistinct(t *testing.T) {
	t.Parallel()

	// Each case differs by exactly one field from the base; every ID must be unique.
	type vector struct {
		name          string
		run, tenant   string
		op            string
		inputs        []string
		value, unit   string
	}
	tests := []vector{
		{"base", "run_1", "tenant_1", "calc.add", []string{"tok_a", "tok_b"}, "200.000000000", "USD"},
		{"diff run", "run_2", "tenant_1", "calc.add", []string{"tok_a", "tok_b"}, "200.000000000", "USD"},
		{"diff tenant", "run_1", "tenant_2", "calc.add", []string{"tok_a", "tok_b"}, "200.000000000", "USD"},
		{"diff op", "run_1", "tenant_1", "calc.sum", []string{"tok_a", "tok_b"}, "200.000000000", "USD"},
		{"diff inputs order", "run_1", "tenant_1", "calc.add", []string{"tok_b", "tok_a"}, "200.000000000", "USD"},
		{"diff value", "run_1", "tenant_1", "calc.add", []string{"tok_a", "tok_b"}, "100.000000000", "USD"},
		{"diff unit", "run_1", "tenant_1", "calc.add", []string{"tok_a", "tok_b"}, "200.000000000", "EUR"},
	}

	seen := make(map[string]string, len(tests))
	for _, tt := range tests {
		id := derivation.DeriveTokenID(tt.run, tt.tenant, tt.op, tt.inputs, tt.value, tt.unit)
		if prev, ok := seen[id]; ok {
			t.Errorf("DeriveTokenID() collision between %q and %q (id=%s)", tt.name, prev, id)
		}
		seen[id] = tt.name
	}
}
