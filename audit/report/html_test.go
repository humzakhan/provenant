package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/provenant/provenant/audit/pack"
	"github.com/provenant/provenant/audit/report"
	kernel "github.com/provenant/provenant/kernel/public"
)

// ---------------------------------------------------------------------------
// Fixture builder
//
// minimalDoc builds a tiny pack.Document with:
//   - 2 leaf tokens
//   - 1 derived (add) token
//   - 1 captured read per leaf token
//   - 4 audit entries (run_started, token_mint x2, answer_submitted)
//   - a manifest with 1 artifact
//
// The fixture is intentionally small (no network calls, no CSV files, no CLI).
// ---------------------------------------------------------------------------

func minimalDoc(t *testing.T) *pack.Document {
	t.Helper()

	cr1 := kernel.CapturedRead{
		ID:           "cr-001",
		RunID:        "test-run-1",
		SourcePath:   "fixture://data.json",
		SourceName:   "data.json",
		SourceSHA256: "aaaa" + strings.Repeat("0", 60),
		Selector:     kernel.SourceSelector{JSONPointer: "/a"},
	}
	cr2 := kernel.CapturedRead{
		ID:           "cr-002",
		RunID:        "test-run-1",
		SourcePath:   "fixture://data.json",
		SourceName:   "data.json",
		SourceSHA256: "bbbb" + strings.Repeat("0", 60),
		Selector:     kernel.SourceSelector{JSONPointer: "/b"},
	}

	leaf1 := kernel.Token{
		ID:             "tok-leaf-1",
		RunID:          "test-run-1",
		TenantID:       "tenant-test",
		ValueCanonical: "100.000000000",
		Unit:           "USD",
		Source: kernel.TokenSource{
			CapturedReadID: "cr-001",
			SourcePath:     "fixture://data.json",
			OutputPath:     "$.a",
		},
	}
	leaf2 := kernel.Token{
		ID:             "tok-leaf-2",
		RunID:          "test-run-1",
		TenantID:       "tenant-test",
		ValueCanonical: "200.000000000",
		Unit:           "USD",
		Source: kernel.TokenSource{
			CapturedReadID: "cr-002",
			SourcePath:     "fixture://data.json",
			OutputPath:     "$.b",
		},
	}
	derived := kernel.Token{
		ID:             "tok-derived-1",
		RunID:          "test-run-1",
		TenantID:       "tenant-test",
		ValueCanonical: "300.000000000",
		Unit:           "USD",
		Derivation: &kernel.Derivation{
			Operation: "calc.add",
			Inputs:    []string{"tok-leaf-1", "tok-leaf-2"},
			Evaluated: "300.000000000",
		},
	}

	// Build a minimal audit_jsonl string with 4 entries.
	// The prev_hash chain is fake but structurally valid for rendering.
	hash0 := strings.Repeat("0", 64)
	hash1 := strings.Repeat("1", 64)
	hash2 := strings.Repeat("2", 64)
	hash3 := strings.Repeat("3", 64)

	type rawEntry struct {
		Seq      uint64          `json:"seq"`
		Kind     string          `json:"kind"`
		RunID    string          `json:"run_id"`
		TenantID string          `json:"tenant_id"`
		PrevHash string          `json:"prev_hash"`
		Hash     string          `json:"hash"`
		Payload  json.RawMessage `json:"payload_deterministic"`
	}

	answerPayload, _ := json.Marshal(map[string]string{
		"label":           "total",
		"token_id":        "tok-derived-1",
		"unit":            "USD",
		"value_canonical": "300.000000000",
	})

	entries := []rawEntry{
		{Seq: 1, Kind: "run_started", RunID: "test-run-1", TenantID: "tenant-test",
			PrevHash: hash0, Hash: hash1, Payload: json.RawMessage(`{}`)},
		{Seq: 2, Kind: "token_mint", RunID: "test-run-1", TenantID: "tenant-test",
			PrevHash: hash1, Hash: hash2, Payload: json.RawMessage(`{}`)},
		{Seq: 3, Kind: "answer_submitted", RunID: "test-run-1", TenantID: "tenant-test",
			PrevHash: hash2, Hash: hash3, Payload: answerPayload},
		{Seq: 4, Kind: "run_completed", RunID: "test-run-1", TenantID: "tenant-test",
			PrevHash: hash3, Hash: strings.Repeat("4", 64), Payload: json.RawMessage(`{}`)},
	}

	var lines []string
	for _, e := range entries {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}
	auditJSONL := strings.Join(lines, "\n")

	return &pack.Document{
		SchemaVersion: pack.SchemaVersion,
		BundleSHA256:  strings.Repeat("a", 64),
		CapturedReads: []kernel.CapturedRead{cr1, cr2},
		Tokens:        []kernel.Token{leaf1, leaf2, derived},
		AuditJSONL:    auditJSONL,
		Manifest: pack.ManifestSection{
			SchemaVersion: "provenant.audit.pack.manifest/v1",
			BundleHash:    strings.Repeat("a", 64),
			Artifacts: []pack.ArtifactRecord{
				{Name: "bundle", SHA256: strings.Repeat("a", 64)},
			},
		},
	}
}

// docWithMismatch returns a doc with a Report section that has one mismatch,
// which should drive the mismatch tile count to 1 and show VIOLATED banner.
func docWithMismatch(t *testing.T) *pack.Document {
	t.Helper()
	doc := minimalDoc(t)

	// Build reconciliation JSON with one OutcomeMismatch record.
	reconJSON := json.RawMessage(`{
		"results": [
			{
				"id": "res-001",
				"outcome": "mismatch",
				"key_field": "id",
				"key_value": "ROW-42",
				"differences": [
					{"field": "amount", "left": "100.00", "right": "100.01", "reason": "amount delta 0.01 exceeds tolerance 0.00"}
				]
			}
		]
	}`)

	doc.Report = &pack.ReportSection{
		Markdown: "# Reconciliation\nOne mismatch found.",
		JSON:     reconJSON,
	}
	return doc
}

// docInvariantViolated returns a doc where the contribution invariant is
// structurally violated: the derived token declares ValueCanonical="300.000000000"
// but its two leaf inputs sum to only "250.000000000" (100 + 150). No
// Report.JSON is set — the violation is detected by the renderer's own
// decimal sum check inside buildDerivationTree.
func docInvariantViolated(t *testing.T) *pack.Document {
	t.Helper()

	cr1 := kernel.CapturedRead{
		ID:           "cr-v01",
		RunID:        "test-run-v",
		SourcePath:   "fixture://data.json",
		SourceName:   "data.json",
		SourceSHA256: "aaaa" + strings.Repeat("0", 60),
		Selector:     kernel.SourceSelector{JSONPointer: "/a"},
	}
	cr2 := kernel.CapturedRead{
		ID:           "cr-v02",
		RunID:        "test-run-v",
		SourcePath:   "fixture://data.json",
		SourceName:   "data.json",
		SourceSHA256: "bbbb" + strings.Repeat("0", 60),
		Selector:     kernel.SourceSelector{JSONPointer: "/b"},
	}

	leaf1 := kernel.Token{
		ID:             "tok-vleaf-1",
		RunID:          "test-run-v",
		TenantID:       "tenant-test",
		ValueCanonical: "100.000000000",
		Unit:           "USD",
		Source: kernel.TokenSource{
			CapturedReadID: "cr-v01",
			SourcePath:     "fixture://data.json",
			OutputPath:     "$.a",
		},
	}
	leaf2 := kernel.Token{
		ID:             "tok-vleaf-2",
		RunID:          "test-run-v",
		TenantID:       "tenant-test",
		ValueCanonical: "150.000000000", // 100 + 150 = 250 ≠ 300 → violation
		Unit:           "USD",
		Source: kernel.TokenSource{
			CapturedReadID: "cr-v02",
			SourcePath:     "fixture://data.json",
			OutputPath:     "$.b",
		},
	}
	// Derived token claims 300 but inputs only sum to 250.
	derived := kernel.Token{
		ID:             "tok-vderived-1",
		RunID:          "test-run-v",
		TenantID:       "tenant-test",
		ValueCanonical: "300.000000000",
		Unit:           "USD",
		Derivation: &kernel.Derivation{
			Operation: "calc.add",
			Inputs:    []string{"tok-vleaf-1", "tok-vleaf-2"},
			Evaluated: "300.000000000",
		},
	}

	hash0 := strings.Repeat("0", 64)
	hash1 := strings.Repeat("1", 64)
	hash2 := strings.Repeat("2", 64)
	hash3 := strings.Repeat("3", 64)

	type rawEntry struct {
		Seq      uint64          `json:"seq"`
		Kind     string          `json:"kind"`
		RunID    string          `json:"run_id"`
		TenantID string          `json:"tenant_id"`
		PrevHash string          `json:"prev_hash"`
		Hash     string          `json:"hash"`
		Payload  json.RawMessage `json:"payload_deterministic"`
	}

	answerPayload, _ := json.Marshal(map[string]string{
		"label":           "total",
		"token_id":        "tok-vderived-1",
		"unit":            "USD",
		"value_canonical": "300.000000000",
	})

	entries := []rawEntry{
		{Seq: 1, Kind: "run_started", RunID: "test-run-v", TenantID: "tenant-test",
			PrevHash: hash0, Hash: hash1, Payload: json.RawMessage(`{}`)},
		{Seq: 2, Kind: "token_mint", RunID: "test-run-v", TenantID: "tenant-test",
			PrevHash: hash1, Hash: hash2, Payload: json.RawMessage(`{}`)},
		{Seq: 3, Kind: "answer_submitted", RunID: "test-run-v", TenantID: "tenant-test",
			PrevHash: hash2, Hash: hash3, Payload: answerPayload},
		{Seq: 4, Kind: "run_completed", RunID: "test-run-v", TenantID: "tenant-test",
			PrevHash: hash3, Hash: strings.Repeat("4", 64), Payload: json.RawMessage(`{}`)},
	}

	var lines []string
	for _, e := range entries {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}
	auditJSONL := strings.Join(lines, "\n")

	return &pack.Document{
		SchemaVersion: pack.SchemaVersion,
		BundleSHA256:  strings.Repeat("b", 64),
		CapturedReads: []kernel.CapturedRead{cr1, cr2},
		Tokens:        []kernel.Token{leaf1, leaf2, derived},
		AuditJSONL:    auditJSONL,
		Manifest: pack.ManifestSection{
			SchemaVersion: "provenant.audit.pack.manifest/v1",
			BundleHash:    strings.Repeat("b", 64),
			Artifacts: []pack.ArtifactRecord{
				{Name: "bundle", SHA256: strings.Repeat("b", 64)},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// TestRenderHTMLGolden
// ---------------------------------------------------------------------------

func TestRenderHTMLGolden(t *testing.T) {
	doc := minimalDoc(t)
	got, err := report.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}

	goldenPath := "testdata/golden.html"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden written: %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to regenerate)", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("RenderHTML() output differs from golden %s (run with UPDATE_GOLDEN=1 to regenerate)", goldenPath)
	}
}

// ---------------------------------------------------------------------------
// TestRenderHTMLDeterministic
// ---------------------------------------------------------------------------

func TestRenderHTMLDeterministic(t *testing.T) {
	doc := minimalDoc(t)

	got1, err := report.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML() #1 error = %v", err)
	}
	got2, err := report.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML() #2 error = %v", err)
	}

	if !bytes.Equal(got1, got2) {
		t.Error("RenderHTML() is not deterministic: two calls on identical input differ")
	}
}

// ---------------------------------------------------------------------------
// TestRenderHTMLNoExternalAssets
// ---------------------------------------------------------------------------

func TestRenderHTMLNoExternalAssets(t *testing.T) {
	doc := minimalDoc(t)
	out, err := report.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	html := string(out)

	if strings.Contains(html, "<script") {
		t.Error("rendered HTML contains <script> tag")
	}
	if strings.Contains(html, "http://") {
		t.Error("rendered HTML contains http:// URL")
	}
	if strings.Contains(html, "https://") {
		t.Error("rendered HTML contains https:// URL")
	}
}

// ---------------------------------------------------------------------------
// TestRenderHTMLContributionInvariantOK
// ---------------------------------------------------------------------------

func TestRenderHTMLContributionInvariantOK(t *testing.T) {
	// minimalDoc has leaf sums (100+200=300) that exactly equal the derived
	// ValueCanonical (300). The renderer's decimal sum check must pass → OK banner.
	doc := minimalDoc(t)
	out, err := report.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if !strings.Contains(string(out), "Contribution invariant: OK") {
		t.Error("expected 'Contribution invariant: OK' in output")
	}
}

// ---------------------------------------------------------------------------
// TestRenderHTMLContributionInvariantViolated
// ---------------------------------------------------------------------------

func TestRenderHTMLContributionInvariantViolated(t *testing.T) {
	// docInvariantViolated has leaf sums (100+150=250) that do NOT equal the
	// declared derived ValueCanonical (300). No Report.JSON is present — the
	// VIOLATED banner must come from the renderer's own decimal sum check.
	doc := docInvariantViolated(t)
	out, err := report.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if !strings.Contains(string(out), "VIOLATED") {
		t.Error("expected 'VIOLATED' in output")
	}
}
