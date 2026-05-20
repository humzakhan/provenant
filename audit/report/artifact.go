package report

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/provenant/provenant/audit/pack"
	kernel "github.com/provenant/provenant/kernel/public"
)

// WriteArtifactReport renders an HTML report for doc where any non-absolute
// SourcePath (e.g. "fixture://foo.csv") is rewritten to an absolute path
// under dir, matching files copied alongside the report. The rendered bytes
// are written to filepath.Join(dir, "report.html").
//
// Semantics:
//   - dir must be an existing directory (returns error otherwise).
//   - The input doc is not mutated; a shallow clone of CapturedReads is used.
//   - For each CapturedRead whose SourcePath is not filepath.IsAbs()-true,
//     the path is replaced with filepath.Join(dir, filepath.Base(stripScheme(SourcePath))).
//     If the resulting file does not exist on disk, the path is left empty
//     (so the renderer skips the file:// link gracefully).
//   - Calls RenderHTML on the cloned doc and writes to filepath.Join(dir, "report.html").
func WriteArtifactReport(dir string, doc *pack.Document) error {
	// Resolve dir to an absolute path so all generated file:// links are absolute.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("report.WriteArtifactReport: resolve dir %q: %w", dir, err)
	}
	dir = absDir

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
		if filepath.IsAbs(cr.SourcePath) {
			// Already absolute — check existence; if present, keep as-is.
			if _, err := os.Stat(cr.SourcePath); errors.Is(err, os.ErrNotExist) {
				reads[i].SourcePath = ""
			}
			continue
		}
		// Non-absolute (e.g. "fixture://foo.csv"): strip scheme and derive local path.
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
		reads[i].SourcePath = candidate
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
