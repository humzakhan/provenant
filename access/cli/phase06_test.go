package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/provenant/provenant/access/cli"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
)

func newPhase06Runner() cli.Runner {
	return cli.Runner{
		Now: func() time.Time {
			return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		},
	}
}

// ---------------------------------------------------------------------------
// read-mint with source_name: captured read carries SourceName; bundle verifies
// ---------------------------------------------------------------------------

func TestReadMintWithSourceName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "sn_test.bundle.json")
	sourcesPath := filepath.Join(dir, "sources.json")
	runner := newPhase06Runner()

	run := kernel.Run{ID: "run_sn_1", TenantID: "tenant_p06", Status: kernel.RunRunning}
	sourceContent := []byte(`{"revenue":777}`)

	// Write sources.json pointing at extract.json reader with the source path.
	sourceJSON := `{
  "schema_version": "provenant.sources/v1",
  "sources": {
    "ledger.revenue": {
      "reader": "extract.json",
      "path": "fixture://revenue.json",
      "selector": {"json_pointer": "/revenue"},
      "output_path": "$.value",
      "unit": "USD"
    }
  }
}`
	if err := os.WriteFile(sourcesPath, []byte(sourceJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(sources.json) error = %v", err)
	}

	// 1. run start
	code, out := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start exit code = %d, output = %s", code, out)
	}

	// 2. read-mint using source_name.
	// The request carries source_name; --sources flag is injected in the CLI args.
	src := sourceWithHash("fixture://revenue.json", sourceContent)
	code, out = runCLI(t, runner,
		[]string{"read-mint", "--sources", sourcesPath, "--request", "-"},
		map[string]any{
			"run_file":    bundlePath,
			"source_name": "ledger.revenue",
			"source":      src,
			"run":         run,
			// reader and read fields are intentionally absent; resolved from registry.
		},
	)
	if code != 0 {
		t.Fatalf("read-mint (source_name) exit code = %d, output = %s", code, out)
	}
	mintResp := decodeJSON[readMintResp](t, out)
	if mintResp.Token.ID == "" {
		t.Fatal("read-mint: token.id is empty")
	}
	if mintResp.Seq != 2 {
		t.Fatalf("read-mint seq = %d, want 2", mintResp.Seq)
	}

	// 3. run finish
	code, out = runCLI(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run finish exit code = %d, output = %s", code, out)
	}
	finishResp := decodeJSON[runBundleResp](t, out)

	// 4. verify the bundle (source_name must not break verification).
	bundleBytes, err := os.ReadFile(finishResp.BundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundle) error = %v", err)
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("bundle.Verify() error = %v", err)
	}

	// 5. replay and confirm source_name is persisted on the captured read.
	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("bundle.Replay() error = %v", err)
	}
	if len(state.CapturedReads) != 1 {
		t.Fatalf("replay captured reads = %d, want 1", len(state.CapturedReads))
	}
	if state.CapturedReads[0].SourceName != "ledger.revenue" {
		t.Errorf("captured_read.source_name = %q, want %q",
			state.CapturedReads[0].SourceName, "ledger.revenue")
	}
}

// ---------------------------------------------------------------------------
// read-mint with source_name: unknown source name returns user error (exit 2)
// ---------------------------------------------------------------------------

func TestReadMintUnknownSourceName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "sn_unknown.bundle.json")
	sourcesPath := filepath.Join(dir, "sources.json")
	runner := newPhase06Runner()

	run := kernel.Run{ID: "run_sn_unk", TenantID: "tenant_p06", Status: kernel.RunRunning}

	if err := os.WriteFile(sourcesPath, []byte(`{"schema_version":"provenant.sources/v1","sources":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	src := sourceWithHash("fixture://x.json", []byte(`{"x":1}`))
	code, out := runCLI(t, runner,
		[]string{"read-mint", "--sources", sourcesPath, "--request", "-"},
		map[string]any{
			"run_file":    bundlePath,
			"source_name": "does.not.exist",
			"source":      src,
			"run":         run,
		},
	)
	if code != 2 {
		t.Fatalf("read-mint (unknown source_name) exit code = %d, want 2; output = %s", code, out)
	}
}

// ---------------------------------------------------------------------------
// Backward compat: pre-Phase-06 bundles (no source_name) still verify
// ---------------------------------------------------------------------------

func TestBackwardCompatBundleNoSourceName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "legacy.bundle.json")
	runner := newPhase06Runner()
	run := kernel.Run{ID: "run_legacy_1", TenantID: "tenant_p06", Status: kernel.RunRunning}

	// Build a bundle using the raw path (no source_name), which mirrors the
	// pre-Phase-06 behaviour.
	buildSimpleBundle(t, runner, bundlePath, run)

	bundleBytes, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundle) error = %v", err)
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("bundle.Verify() on pre-06 bundle error = %v", err)
	}

	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("bundle.Replay() on pre-06 bundle error = %v", err)
	}
	if len(state.CapturedReads) != 1 {
		t.Fatalf("replay captured reads = %d, want 1", len(state.CapturedReads))
	}
	// source_name must be empty for legacy bundles.
	if state.CapturedReads[0].SourceName != "" {
		t.Errorf("legacy bundle: captured_read.source_name = %q, want empty", state.CapturedReads[0].SourceName)
	}
}

// ---------------------------------------------------------------------------
// pack CLI verb: happy path
// ---------------------------------------------------------------------------

func TestPackHappyPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "pack_in.bundle.json")
	packOutPath := filepath.Join(dir, "audit.pack.json")
	runner := newPhase06Runner()
	run := kernel.Run{ID: "run_pack_1", TenantID: "tenant_p06", Status: kernel.RunRunning}

	buildSimpleBundle(t, runner, bundlePath, run)

	code, out := runCLI(t, runner, []string{"pack", "--bundle", bundlePath, "--out", packOutPath}, nil)
	if code != 0 {
		t.Fatalf("pack exit code = %d, output = %s", code, out)
	}

	// The response should carry pack_path and pack_sha256.
	var resp struct {
		PackPath   string `json:"pack_path"`
		PackSHA256 string `json:"pack_sha256"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("json.Unmarshal(pack response) error = %v, input = %s", err, out)
	}
	if resp.PackPath == "" {
		t.Fatal("pack: pack_path is empty")
	}
	if resp.PackSHA256 == "" {
		t.Fatal("pack: pack_sha256 is empty")
	}

	// The output file must exist and parse as provenant.audit.pack/v1.
	packData, err := os.ReadFile(packOutPath)
	if err != nil {
		t.Fatalf("ReadFile(pack) error = %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(packData, &doc); err != nil {
		t.Fatalf("json.Unmarshal(pack file) error = %v", err)
	}
	var schemaVersion string
	if err := json.Unmarshal(doc["schema_version"], &schemaVersion); err != nil {
		t.Fatalf("unmarshal schema_version: %v", err)
	}
	if schemaVersion != "provenant.audit.pack/v1" {
		t.Errorf("schema_version = %q, want provenant.audit.pack/v1", schemaVersion)
	}
}

// ---------------------------------------------------------------------------
// pack CLI verb: missing --bundle flag returns user error
// ---------------------------------------------------------------------------

func TestPackMissingBundleFlag(t *testing.T) {
	t.Parallel()

	runner := newPhase06Runner()
	code, _ := runCLI(t, runner, []string{"pack", "--out", "/tmp/nope.json"}, nil)
	if code != 2 {
		t.Fatalf("pack missing --bundle: exit code = %d, want 2", code)
	}
}

// ---------------------------------------------------------------------------
// Helper: runCLI with nil body sends empty stdin
// ---------------------------------------------------------------------------

func init() {
	// Ensure the overloaded runCLI in this file handles nil body.
	// (Actual nil-body handling is in the existing runCLI helper via json.Marshal(nil) => "null".)
	_ = cli.Runner{}
}
