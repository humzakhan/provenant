package pack_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/provenant/provenant/audit/pack"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildTestBundle creates a verifiable bundle with one minted token.
func buildTestBundle(t *testing.T) []byte {
	t.Helper()

	sourceData := []byte(`{"amount":"42.00"}`)
	sb := kernel.SourceBytes{
		Source: kernel.Source{
			Path:      "fixture://test.json",
			MediaType: "application/json",
			SHA256:    sha256Hex(sourceData),
		},
		Bytes: sourceData,
	}

	binding := memio.NewBinding(sb)
	run := kernel.Run{ID: "pack_test_run_1", TenantID: "pack_test_tenant", Status: kernel.RunRunning}
	clock := fixedClock{t: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}

	if _, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractjson.Reader{},
		binding,
		binding,
		clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://test.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/amount"},
			OutputPath: "$.value",
			Unit:       "USD",
		},
	); err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}

	state := snapshotBinding(t, binding, run)
	b, err := bundle.Emit(state)
	if err != nil {
		t.Fatalf("bundle.Emit() error = %v", err)
	}
	return b
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func snapshotBinding(t *testing.T, binding *memio.Binding, run kernel.Run) bundle.State {
	t.Helper()
	var state bundle.State
	if err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		reads, err := binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		tokens, err := binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		entries, err := binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		state = bundle.State{
			Run:           run,
			CapturedReads: reads,
			Tokens:        tokens,
			AuditEntries:  entries,
		}
		return nil
	}); err != nil {
		t.Fatalf("snapshotBinding WithTx error = %v", err)
	}
	return state
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Tests: determinism
// ---------------------------------------------------------------------------

func TestBuildDeterminism(t *testing.T) {
	t.Parallel()

	bundleBytes := buildTestBundle(t)

	pack1, err := pack.Build(bundleBytes, pack.Options{SkipSidecarDiscovery: true})
	if err != nil {
		t.Fatalf("Build() #1 error = %v", err)
	}
	pack2, err := pack.Build(bundleBytes, pack.Options{SkipSidecarDiscovery: true})
	if err != nil {
		t.Fatalf("Build() #2 error = %v", err)
	}

	if string(pack1) != string(pack2) {
		t.Fatal("Build() is not deterministic: two builds from the same bundle differ")
	}
}

// ---------------------------------------------------------------------------
// Tests: pack verifies bundle (tamper detection)
// ---------------------------------------------------------------------------

func TestBuildTamperedBundleReturnsError(t *testing.T) {
	t.Parallel()

	bundleBytes := buildTestBundle(t)

	// Tamper by flipping a byte in the middle of the bundle.
	tampered := append([]byte(nil), bundleBytes...)
	tampered[len(tampered)/2] ^= 0xFF

	_, err := pack.Build(tampered, pack.Options{SkipSidecarDiscovery: true})
	if err == nil {
		t.Fatal("Build(tampered) expected error, got nil")
	}
	if !errors.Is(err, bundle.ErrTamperDetected) {
		t.Errorf("Build(tampered) error = %v, want wrapping bundle.ErrTamperDetected", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: happy path – valid schema_version and key fields present
// ---------------------------------------------------------------------------

func TestBuildHappyPath(t *testing.T) {
	t.Parallel()

	bundleBytes := buildTestBundle(t)
	packBytes, err := pack.Build(bundleBytes, pack.Options{SkipSidecarDiscovery: true})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Decode as raw map to verify top-level structure.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(packBytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal(pack) error = %v", err)
	}

	for _, key := range []string{"schema_version", "bundle_sha256", "bundle", "manifest"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("pack document missing key %q", key)
		}
	}

	var schemaVersion string
	if err := json.Unmarshal(doc["schema_version"], &schemaVersion); err != nil {
		t.Fatalf("unmarshal schema_version: %v", err)
	}
	if schemaVersion != pack.SchemaVersion {
		t.Errorf("schema_version = %q, want %q", schemaVersion, pack.SchemaVersion)
	}
}

// ---------------------------------------------------------------------------
// Tests: report section omitted when no sidecars
// ---------------------------------------------------------------------------

func TestBuildOmitsReportWhenNoSidecars(t *testing.T) {
	t.Parallel()

	bundleBytes := buildTestBundle(t)
	packBytes, err := pack.Build(bundleBytes, pack.Options{SkipSidecarDiscovery: true})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(packBytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	// report key must be absent (omitempty).
	if _, ok := doc["report"]; ok {
		t.Error("report section present when no sidecars supplied")
	}
}

// ---------------------------------------------------------------------------
// Tests: report section present when sidecars supplied
// ---------------------------------------------------------------------------

func TestBuildIncludesReportWhenSidecarsSupplied(t *testing.T) {
	t.Parallel()

	bundleBytes := buildTestBundle(t)
	packBytes, err := pack.Build(bundleBytes, pack.Options{
		SkipSidecarDiscovery: true,
		ReportMarkdown:       []byte("# Report\nAll clear."),
		ReportJSON:           json.RawMessage(`{"ok":true}`),
		MismatchesCSV:        []byte("id,outcome\n"),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(packBytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if _, ok := doc["report"]; !ok {
		t.Error("report section missing when sidecars supplied")
	}
}

// ---------------------------------------------------------------------------
// Tests: Decode rejects unknown schema_version
// ---------------------------------------------------------------------------

func TestDecodeUnknownSchemaVersion(t *testing.T) {
	t.Parallel()

	doc := `{"schema_version":"provenant.audit.pack/v2","bundle_sha256":"abc","manifest":{}}`
	_, err := pack.Decode(strings.NewReader(doc))
	if !errors.Is(err, pack.ErrUnknownSchema) {
		t.Errorf("Decode(unknown version) error = %v, want ErrUnknownSchema", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: Decode happy path round-trip
// ---------------------------------------------------------------------------

func TestDecodeHappyPath(t *testing.T) {
	t.Parallel()

	bundleBytes := buildTestBundle(t)
	packBytes, err := pack.Build(bundleBytes, pack.Options{SkipSidecarDiscovery: true})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	_, err = pack.Decode(strings.NewReader(string(packBytes)))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
}
