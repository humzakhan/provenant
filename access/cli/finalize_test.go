package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizeReconcileReportFileNoOverwriteFailsWhenDestinationAppears(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tmpPath := filepath.Join(dir, ".report.zip.tmp")
	outPath := filepath.Join(dir, "report.zip")
	if err := os.WriteFile(tmpPath, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFile(tmp) error = %v", err)
	}
	if err := os.WriteFile(outPath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile(out) error = %v", err)
	}

	err := finalizeReconcileReportFile(tmpPath, outPath, false)
	var existsErr outputExistsError
	if !errors.As(err, &existsErr) {
		t.Fatalf("finalizeReconcileReportFile() error = %v, want outputExistsError", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(out) error = %v", err)
	}
	if string(got) != "existing" {
		t.Fatalf("output = %q, want existing", got)
	}
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("Stat(tmp) error = %v", err)
	}
}
