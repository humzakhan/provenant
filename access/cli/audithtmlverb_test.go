package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/humzakhan/provenant/access/cli"
	kernel "github.com/humzakhan/provenant/kernel/public"
)

// newAuditHTMLRunner returns a Runner with a fixed clock for audit-html tests.
func newAuditHTMLRunner() cli.Runner {
	return cli.Runner{
		Now: func() time.Time {
			return time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
		},
	}
}

// buildPackForAuditHTMLTest builds a minimal bundle + pack in dir.
// Returns the path to the written pack file.
func buildPackForAuditHTMLTest(t *testing.T, runner cli.Runner, dir string, run kernel.Run) string {
	t.Helper()

	bundlePath := filepath.Join(dir, "test.bundle.json")
	packPath := filepath.Join(dir, "test.pack.json")

	buildSimpleBundle(t, runner, bundlePath, run)

	code, out := runCLI(t, runner, []string{"pack", "--bundle", bundlePath, "--out", packPath}, nil)
	if code != 0 {
		t.Fatalf("pack: exit code = %d, output = %s", code, out)
	}

	return packPath
}

// ---------------------------------------------------------------------------
// TestAuditHTMLVerbHappyPath: audit-html writes valid HTML to --out
// ---------------------------------------------------------------------------

func TestAuditHTMLVerbHappyPath(t *testing.T) {
	t.Parallel()

	runner := newAuditHTMLRunner()
	dir := t.TempDir()
	run := kernel.Run{ID: "run_audithtml_1", TenantID: "tenant_audithtml", Status: kernel.RunRunning}

	packPath := buildPackForAuditHTMLTest(t, runner, dir, run)
	outPath := filepath.Join(dir, "report.html")

	code, out := runCLI(t, runner, []string{"audit-html", "--pack", packPath, "--out", outPath}, nil)
	if code != 0 {
		t.Fatalf("audit-html: exit code = %d, output = %s", code, out)
	}

	htmlBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", outPath, err)
	}
	if !bytes.Contains(htmlBytes, []byte("<!doctype html>")) {
		t.Error("output HTML does not contain <!doctype html>")
	}
	if !bytes.Contains(htmlBytes, []byte("Provenant Audit Report")) {
		t.Error("output HTML does not contain report title")
	}
	// Determinism: a second invocation with the same inputs must produce identical bytes.
	out2Path := filepath.Join(dir, "report2.html")
	code2, _ := runCLI(t, runner, []string{"audit-html", "--pack", packPath, "--out", out2Path}, nil)
	if code2 != 0 {
		t.Fatalf("audit-html (second run): exit code = %d", code2)
	}
	html2, err := os.ReadFile(out2Path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", out2Path, err)
	}
	if !bytes.Equal(htmlBytes, html2) {
		t.Error("audit-html output is not deterministic: two runs produced different bytes")
	}
}

// ---------------------------------------------------------------------------
// TestAuditHTMLVerbBrokenPack: corrupt pack yields nonzero exit + clear stderr
// ---------------------------------------------------------------------------

func TestAuditHTMLVerbBrokenPack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write a file that looks like JSON but has an invalid schema_version.
	corruptPackPath := filepath.Join(dir, "corrupt.pack.json")
	corruptJSON := []byte(`{"schema_version":"bad","bundle_sha256":"","tokens":[],"captured_reads":[],"audit_jsonl":"","manifest":{"schema_version":"","bundle_hash":"","artifacts":[]}}`)
	if err := os.WriteFile(corruptPackPath, corruptJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(corrupt pack): %v", err)
	}

	outPath := filepath.Join(dir, "report.html")
	var stdout, stderr bytes.Buffer
	code := cli.Main([]string{"audit-html", "--pack", corruptPackPath, "--out", outPath}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatal("audit-html on corrupt pack: expected nonzero exit code")
	}
	stderrStr := stderr.String()
	if stderrStr == "" {
		t.Error("audit-html on corrupt pack: expected non-empty stderr")
	}
}

// ---------------------------------------------------------------------------
// TestAuditHTMLVerbMissingPack: missing path yields exit 2
// ---------------------------------------------------------------------------

func TestAuditHTMLVerbMissingPack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nonexistentPath := filepath.Join(dir, "does_not_exist.pack.json")
	outPath := filepath.Join(dir, "report.html")

	var stdout, stderr bytes.Buffer
	code := cli.Main([]string{"audit-html", "--pack", nonexistentPath, "--out", outPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("audit-html missing --pack: exit code = %d, want 2; stderr = %s", code, stderr.String())
	}
}
