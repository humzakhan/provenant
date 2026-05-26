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
// is rewritten to a relative basename href when the matching CSV is present
// in the artifact directory, producing href="foo.csv" (not file:// + abs path).
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

	// The relative basename must appear as an href (no file:// prefix).
	wantHref := `href="foo.csv"`
	if !strings.Contains(html, wantHref) {
		t.Errorf("expected %q to appear in report.html; got (excerpt):\n%s",
			wantHref, excerpt(html, "sourcePath"))
	}

	// No absolute machine path should appear.
	if strings.Contains(html, dir) {
		t.Errorf("absolute artifact dir path %q should not appear in report.html", dir)
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

	// No href linking to bar.csv should appear since the file is not present.
	if strings.Contains(html, `href="bar.csv"`) {
		t.Error("unexpected href link to missing bar.csv found in report.html")
	}
	// The localHref JS field must be empty when the file is absent.
	if strings.Contains(html, `localHref: "bar.csv"`) {
		t.Error("localHref JS field is non-empty for missing bar.csv")
	}

	// fixture:// must not appear in any href.
	if strings.Contains(html, `href="fixture://`) {
		t.Error("fixture:// URI appeared in an href in report.html")
	}
}

// ---------------------------------------------------------------------------
// TestWriteArtifactReport_OmitsPathNotInArtifactDir
// ---------------------------------------------------------------------------

// TestWriteArtifactReport_OmitsPathNotInArtifactDir verifies that a SourcePath
// whose basename does not exist inside the artifact directory produces no link
// (SourcePath is set to ""), even if the file exists elsewhere on disk.
// WriteArtifactReport only links files co-located with report.html.
func TestWriteArtifactReport_OmitsPathNotInArtifactDir(t *testing.T) {
	// Write the source file at a path outside of the artifact dir.
	srcDir := t.TempDir()
	srcCSV := filepath.Join(srcDir, "real.csv")
	if err := os.WriteFile(srcCSV, []byte("id,amount\n1,10.00\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(real.csv): %v", err)
	}

	// Artifact dir does NOT contain real.csv.
	dir := t.TempDir()
	doc := docWithFixturePath(t, srcCSV)

	if err := report.WriteArtifactReport(dir, doc); err != nil {
		t.Fatalf("WriteArtifactReport: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatalf("ReadFile(report.html): %v", err)
	}
	html := string(data)

	// No href="real.csv" should appear since real.csv is not in dir.
	if strings.Contains(html, `href="real.csv"`) {
		t.Error("unexpected link to real.csv that is not co-located in the artifact dir")
	}
	// No machine-specific absolute path should appear.
	if strings.Contains(html, srcDir) {
		t.Errorf("absolute srcDir path %q should not appear in report.html", srcDir)
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
