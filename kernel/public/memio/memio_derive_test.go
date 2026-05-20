package memio_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/derive"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

// TestBindingDerive exercises the memio.Binding.Derive convenience wrapper.
func TestBindingDerive(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_derive_memio", TenantID: "tenant_derive_memio", Status: kernel.RunRunning}
	clock := &memioDeriveFixedClock{t: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}

	src := []byte(`{"value":100}`)
	sb := kernel.SourceBytes{
		Source: kernel.Source{Path: "fixture://derive_memio.json", SHA256: sha256HexDeriveMemio(src)},
		Bytes:  src,
	}
	b := memio.NewBinding(sb)

	res, err := kernel.ReadAndMint(
		context.Background(),
		b, b,
		extractjson.Reader{},
		b, b,
		clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://derive_memio.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/value"},
			OutputPath: "$.value",
			Unit:       "USD",
		},
	)
	if err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}

	derived, err := b.Derive(context.Background(), clock, run, derive.Request{
		Op:     "calc.add",
		Inputs: []kernel.Token{res.Token, res.Token},
	})
	if err != nil {
		t.Fatalf("Binding.Derive() error = %v", err)
	}
	if derived.Token.ValueCanonical != "200.000000000" {
		t.Errorf("ValueCanonical = %q, want 200.000000000", derived.Token.ValueCanonical)
	}
	if derived.Token.Unit != "USD" {
		t.Errorf("Unit = %q, want USD", derived.Token.Unit)
	}
	if derived.AuditEntry.Kind != kernel.EntryKindTokenDerive {
		t.Errorf("AuditEntry.Kind = %q, want %q", derived.AuditEntry.Kind, kernel.EntryKindTokenDerive)
	}
}

type memioDeriveFixedClock struct{ t time.Time }

func (c *memioDeriveFixedClock) Now() time.Time { return c.t }

func sha256HexDeriveMemio(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
