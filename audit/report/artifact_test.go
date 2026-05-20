package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/provenant/provenant/audit/pack"
	"github.com/provenant/provenant/audit/report"
	kernel "github.com/provenant/provenant/kernel/public"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// docWithFixturePath builds a minimal pack.Document whose single CapturedRead
// has the given SourcePath. The audit JSONL is structurally valid but carries
// no answer_submitted entry (derivation tree is not rendered, only source cards).
func docWithFixturePath(t *testing.T, sourcePath string) *pack.Document {
	t.Helper()

	cr := kernel.CapturedRead{
		ID:           "cr-art-001",
		RunID:        "art-run-1",
		SourcePath:   sourcePath,
		SourceName:   filepath.Base(sourcePath),
		SourceSHA256: strings.Repeat("a", 64),
		Selector:     kernel.SourceSelector{RowWindow: "2-2"},
	}
	leaf := kernel.Token{
		ID:             "tok-art-leaf",
		RunID:          "art-run-1",
		TenantID:       "tenant-art",
		ValueCanonical: "42.000000000",
		Unit:           "USD",
		Source: kernel.TokenSource{
			CapturedReadID: "cr-art-001",
			SourcePath:     sourcePath,
			OutputPath:     "/rows/0/cells/0/value",
		},
	}

	hash0 := strings.Repeat("0", 64)
	hash1 := strings.Repeat("1", 64)

	type rawEntry struct {
		Seq      uint64          `json:"seq"`
		Kind     string          `json:"kind"`
		RunID    string          `json:"run_id"`
		TenantID string          `json:"tenant_id"`
		PrevHash string          `json:"prev_hash"`
		Hash     string          `json:"hash"`
		Payload  json.RawMessage `json:"payload_deterministic"`
	}
	entries := []rawEntry{
		{Seq: 1, Kind: "run_started", RunID: "art-run-1", TenantID: "tenant-art",
			PrevHash: hash0, Hash: hash1, Payload: json.RawMessage(`{}`)},
	}
	var lines []string
	for _, e := range entries {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}

	return &pack.Document{
		SchemaVersion: pack.SchemaVersion,
		BundleSHA256:  strings.Repeat("b", 64),
		CapturedReads: []kernel.CapturedRead{cr},
		Tokens:        []kernel.Token{leaf},
		AuditJSONL:    strings.Join(lines, "\n"),
		Manifest: pack.ManifestSection{
			SchemaVersion: "provenant.audit.pack.manifest/v1",
			BundleHash:    strings.Repeat("b", 64),
			Artifacts:     []pack.ArtifactRecord{{Name: "bundle", SHA256: strings.Repeat("b", 64)}},
		},
	}
}

// ---------------------------------------------------------------------------
// TestWriteArtifactReport_RewritesFixturePaths
// ---------------------------------------------------------------------------

// TestWriteArtifactReport_RewritesFixturePaths verifies that a fixture:// path
// is rewritten to an absolute file:// path when the matching CSV is present
// in the artifact directory.
func TestWriteArtifactReport_RewritesFixturePaths(t *testing.T) {
	dir := t.TempDir()

	// Place a small foo.csv next to where report.html will be written.
	csvContent := []byte("id,amount\n1,42.00\n")
	csvPath := filepath.Join(dir, "foo.csv")
	if err := os.WriteFile(csvPath, csvContent, 0o644); err != nil {
		t.Fatalf("WriteFile(foo.csv): %v", err)
	}

	doc := docWithFixturePath(t, "fixture://foo.csv")

	if err := report.WriteArtifactReport(dir, doc); err != nil {
		t.Fatalf("WriteArtifactReport: %v", err)
	}

	// Verify report.html was written.
	outPath := filepath.Join(dir, "report.html")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(report.html): %v", err)
	}
	html := string(data)

	// The absolute file:/// path (three slashes = file:// + /abs/path) must appear.
	wantLink := "file://" + csvPath
	if !strings.Contains(html, wantLink) {
		t.Errorf("expected %q to appear in report.html; got (excerpt):\n%s",
			wantLink, excerpt(html, "sourcePath"))
	}

	// The fixture:// URI must NOT appear in any href attribute.
	if strings.Contains(html, `href="fixture://`) {
		t.Error("fixture:// URI appeared in an href in report.html")
	}

	// The input doc must not be mutated.
	if doc.CapturedReads[0].SourcePath != "fixture://foo.csv" {
		t.Errorf("WriteArtifactReport mutated input doc: SourcePath = %q", doc.CapturedReads[0].SourcePath)
	}
}

// ---------------------------------------------------------------------------
// TestWriteArtifactReport_SkipsMissingFile
// ---------------------------------------------------------------------------

// TestWriteArtifactReport_SkipsMissingFile verifies that when the matching CSV
// is NOT present in the artifact directory, the SourcePath is left empty
// (no dead file:// link in the output).
func TestWriteArtifactReport_SkipsMissingFile(t *testing.T) {
	dir := t.TempDir()
	// Do NOT write bar.csv into dir.

	doc := docWithFixturePath(t, "fixture://bar.csv")

	if err := report.WriteArtifactReport(dir, doc); err != nil {
		t.Fatalf("WriteArtifactReport: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatalf("ReadFile(report.html): %v", err)
	}
	html := string(data)

	// No href or sourcePath pointing at bar.csv via file:// should appear.
	// The JS template contains a literal 'file://' string as code — we check
	// that the absolute path to bar.csv does not appear in any link or data.
	if strings.Contains(html, "file:///") && strings.Contains(html, "bar.csv") {
		t.Error("unexpected absolute file:/// link to missing bar.csv found in report.html")
	}
	// Confirm the sourcePath JS field for the card is an empty string (not a path).
	if strings.Contains(html, `sourcePath: "/`) && strings.Contains(html, "bar.csv") {
		t.Error("sourcePath field contains an unexpected absolute path for missing bar.csv")
	}

	// fixture:// must not appear in any href.
	if strings.Contains(html, `href="fixture://`) {
		t.Error("fixture:// URI appeared in an href in report.html")
	}
}

// ---------------------------------------------------------------------------
// TestWriteArtifactReport_PreservesAbsolutePaths
// ---------------------------------------------------------------------------

// TestWriteArtifactReport_PreservesAbsolutePaths verifies that an already-absolute
// SourcePath that exists on disk is left untouched.
func TestWriteArtifactReport_PreservesAbsolutePaths(t *testing.T) {
	// Write the source file at an absolute path outside of the artifact dir.
	srcDir := t.TempDir()
	srcCSV := filepath.Join(srcDir, "real.csv")
	if err := os.WriteFile(srcCSV, []byte("id,amount\n1,10.00\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(real.csv): %v", err)
	}

	dir := t.TempDir()
	doc := docWithFixturePath(t, srcCSV) // already absolute

	if err := report.WriteArtifactReport(dir, doc); err != nil {
		t.Fatalf("WriteArtifactReport: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatalf("ReadFile(report.html): %v", err)
	}
	html := string(data)

	wantLink := "file://" + srcCSV
	if !strings.Contains(html, wantLink) {
		t.Errorf("expected %q in report.html when SourcePath is already absolute", wantLink)
	}
}

// ---------------------------------------------------------------------------
// excerpt is a tiny helper that returns up to 300 chars around the first
// occurrence of needle in s, for diagnostic output.
// ---------------------------------------------------------------------------

func excerpt(s, needle string) string {
	idx := strings.Index(s, needle)
	if idx < 0 {
		if len(s) > 300 {
			return s[:300] + "..."
		}
		return s
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + 220
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}
