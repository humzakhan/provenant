package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/provenant/provenant/audit/pack"
	kernel "github.com/provenant/provenant/kernel/public"
)

// WriteArtifactReport renders an HTML report for doc where any SourcePath
// referencing a file co-located with the report is rewritten to a relative
// basename (e.g. "payroll.csv"), so that href links in the HTML resolve
// correctly relative to the report's own directory when opened in a browser.
// The rendered bytes are written to filepath.Join(dir, "report.html").
//
// Semantics:
//   - dir must be an existing directory (returns error otherwise).
//   - The input doc is not mutated; a shallow clone of CapturedReads is used.
//   - For each CapturedRead, the basename is derived by stripping any scheme
//     prefix (e.g. "fixture://foo.csv" → "foo.csv") and taking filepath.Base.
//     If a file with that basename exists beside the report in dir, SourcePath
//     is set to just that basename (a relative path). Otherwise it is set to ""
//     so the renderer renders no link.
//   - Calls RenderHTML on the cloned doc and writes to filepath.Join(dir, "report.html").
func WriteArtifactReport(dir string, doc *pack.Document) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("report.WriteArtifactReport: stat dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("report.WriteArtifactReport: %q is not a directory", dir)
	}

	// Shallow-clone the document so the caller's doc is not mutated.
	cloned := *doc
	reads := make([]kernel.CapturedRead, len(doc.CapturedReads))
	copy(reads, doc.CapturedReads)

	for i, cr := range reads {
		// Derive the local basename by stripping any scheme prefix.
		base := filepath.Base(stripScheme(cr.SourcePath))
		if base == "" || base == "." {
			reads[i].SourcePath = ""
			continue
		}
		candidate := filepath.Join(dir, base)
		if _, err := os.Stat(candidate); err != nil {
			// File not present beside the report — omit the link.
			reads[i].SourcePath = ""
			continue
		}
		// Set to relative basename only — the HTML renderer produces href="payroll.csv"
		// which resolves relative to report.html in the same directory.
		reads[i].SourcePath = base
	}
	cloned.CapturedReads = reads

	html, err := RenderHTML(&cloned)
	if err != nil {
		return fmt.Errorf("report.WriteArtifactReport: render HTML: %w", err)
	}

	outPath := filepath.Join(dir, "report.html")
	if err := os.WriteFile(outPath, html, 0o644); err != nil {
		return fmt.Errorf("report.WriteArtifactReport: write %q: %w", outPath, err)
	}
	return nil
}

// stripScheme removes a leading "<scheme>://" prefix from path.
// If no scheme is present the path is returned unchanged.
func stripScheme(path string) string {
	if idx := strings.Index(path, "://"); idx >= 0 {
		return path[idx+3:]
	}
	return path
}
