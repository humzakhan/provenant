package report

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/memio"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/provenance"
	"github.com/provenant/provenant/reconcile/router"
)

func TestWriteIncludesRequiredEntriesAndExactBundle(t *testing.T) {
	in := testProvenancedResultSet(t, true)
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, in); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	entries := readZip(t, buf.Bytes())
	wantNames := []string{"report.md", "report.json", "mismatches.csv", "captured_reads.json", "tokens.json", "audit.jsonl", "bundle.json", "manifest.json"}
	if got := keys(entries); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("zip entries = %#v, want %#v", got, wantNames)
	}
	if !bytes.Equal(entries["bundle.json"], in.Bundle.Bytes) {
		t.Fatal("bundle.json differs from provided bundle bytes")
	}
}

func TestWriteSidecarsMatchVerifiedBundle(t *testing.T) {
	in := testProvenancedResultSet(t, true)
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, in); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	entries := readZip(t, buf.Bytes())

	state, err := bundle.Replay(in.Bundle.Bytes)
	if err != nil {
		t.Fatalf("bundle.Replay() error = %v", err)
	}
	var gotTokens []kernel.Token
	if err := json.Unmarshal(entries["tokens.json"], &gotTokens); err != nil {
		t.Fatalf("tokens.json unmarshal error = %v", err)
	}
	if !reflect.DeepEqual(gotTokens, state.Tokens) {
		t.Fatalf("tokens.json = %#v, want replayed %#v", gotTokens, state.Tokens)
	}

	wantAudit, err := buildAuditJSONL(state.AuditEntries)
	if err != nil {
		t.Fatalf("buildAuditJSONL() error = %v", err)
	}
	if !bytes.Equal(entries["audit.jsonl"], wantAudit) {
		t.Fatalf("audit.jsonl = %s, want %s", entries["audit.jsonl"], wantAudit)
	}
	if !bytes.Equal(entries["bundle.json"], in.Bundle.Bytes) {
		t.Fatal("bundle.json differs from provided bundle bytes")
	}
}

func TestWriteSidecarDivergenceFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*provenance.ProvenancedResultSet)
	}{
		{name: "token", mutate: func(in *provenance.ProvenancedResultSet) { in.Tokens[0].ValueCanonical = "11.00" }},
		{name: "captured read", mutate: func(in *provenance.ProvenancedResultSet) { in.CapturedReads[0].SourceHash = strings.Repeat("b", 64) }},
		{name: "audit", mutate: func(in *provenance.ProvenancedResultSet) { in.AuditEntries[0].Seq++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := testProvenancedResultSet(t, true)
			tt.mutate(in)
			var buf bytes.Buffer
			if err := Write(context.Background(), &buf, in); err == nil {
				t.Fatal("Write() error = nil, want fail-closed sidecar mismatch")
			}
		})
	}
}

func TestWriteManifestHashes(t *testing.T) {
	in := testProvenancedResultSet(t, true)
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, in); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	entries := readZip(t, buf.Bytes())
	var got manifest
	if err := json.Unmarshal(entries["manifest.json"], &got); err != nil {
		t.Fatalf("manifest unmarshal error = %v", err)
	}
	if got.BundleHash != sha256HexForTest(in.Bundle.Bytes) {
		t.Fatalf("bundle hash = %s, want %s", got.BundleHash, sha256HexForTest(in.Bundle.Bytes))
	}
	if len(got.Artifacts) != 7 {
		t.Fatalf("manifest artifacts = %d, want 7", len(got.Artifacts))
	}
	for _, artifact := range got.Artifacts {
		data, ok := entries[artifact.Name]
		if !ok {
			t.Fatalf("manifest names missing zip entry %q", artifact.Name)
		}
		if artifact.Size != len(data) {
			t.Fatalf("%s size = %d, want %d", artifact.Name, artifact.Size, len(data))
		}
		if artifact.SHA256 != sha256HexForTest(data) {
			t.Fatalf("%s hash = %s, want %s", artifact.Name, artifact.SHA256, sha256HexForTest(data))
		}
	}
	if _, ok := entries["manifest.json"]; !ok {
		t.Fatal("manifest.json missing")
	}
}

func TestWriteInvalidOrMissingBundleFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*provenance.ProvenancedResultSet)
	}{
		{name: "missing", mutate: func(in *provenance.ProvenancedResultSet) { in.Bundle = nil }},
		{name: "empty", mutate: func(in *provenance.ProvenancedResultSet) { in.Bundle.Bytes = nil }},
		{name: "hash mismatch", mutate: func(in *provenance.ProvenancedResultSet) { in.Bundle.SHA256 = strings.Repeat("0", 64) }},
		{name: "tampered", mutate: func(in *provenance.ProvenancedResultSet) { in.Bundle.Bytes = append(in.Bundle.Bytes, ' ') }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := testProvenancedResultSet(t, true)
			tt.mutate(in)
			var buf bytes.Buffer
			if err := Write(context.Background(), &buf, in); err == nil {
				t.Fatal("Write() error = nil, want fail-closed bundle error")
			}
		})
	}
}

func TestWriteMismatchCSVBehavior(t *testing.T) {
	in := testProvenancedResultSet(t, true)
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, in); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	entries := readZip(t, buf.Bytes())
	got := string(entries["mismatches.csv"])
	want := "result_id,outcome,key,field,left,right,reason\nres_mismatch,mismatch,\"[{\"\"field\"\":\"\"id\"\",\"\"value\"\":\"\"a\"\"}]\",amount,10.00,11.00,amount mismatch\nres_missing_right,missing_right,[],,,,not on right\n"
	if got != want {
		t.Fatalf("mismatches.csv = %q, want %q", got, want)
	}

	in.Results.Results = []engine.Result{{ID: "res_matched", Outcome: engine.OutcomeMatched}}
	buf.Reset()
	if err := Write(context.Background(), &buf, in); err != nil {
		t.Fatalf("Write() matched-only error = %v", err)
	}
	entries = readZip(t, buf.Bytes())
	if got, want := string(entries["mismatches.csv"]), "result_id,outcome,key,field,left,right,reason\n"; got != want {
		t.Fatalf("matched-only mismatches.csv = %q, want %q", got, want)
	}
}

func TestWriteDeterministicRepeatedOutput(t *testing.T) {
	in := testProvenancedResultSet(t, true)
	var first bytes.Buffer
	if err := Write(context.Background(), &first, in); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	var second bytes.Buffer
	if err := Write(context.Background(), &second, in); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("Write output changed across repeated calls")
	}
}

func testProvenancedResultSet(t *testing.T, includeMismatch bool) *provenance.ProvenancedResultSet {
	t.Helper()
	run := kernel.Run{ID: "run-1", TenantID: "tenant-1", Status: kernel.RunRunning}
	read := kernel.CapturedRead{
		ID:            "cr-left",
		RunID:         run.ID,
		SourcePath:    "left.csv",
		Selector:      kernel.SourceSelector{RowWindow: "2:"},
		ReaderName:    "extractcsvrows",
		ReaderVersion: "test",
		SourceSHA256:  strings.Repeat("a", 64),
		OutputSHA256:  sha256HexForTest([]byte(`{"value":"10.00"}`)),
		Output:        json.RawMessage(`{"value":"10.00"}`),
	}
	binding := memio.NewBinding()
	minted, err := kernel.MintFromCapturedRead(context.Background(), binding, binding, binding, fixedClock{}, run, kernel.MintFromCapturedReadRequest{
		CapturedRead: read,
		Source:       kernel.Source{Path: read.SourcePath, SHA256: read.SourceSHA256},
		OutputPath:   "$.value",
	})
	if err != nil {
		t.Fatalf("MintFromCapturedRead() error = %v", err)
	}
	bundleBytes, err := bundle.Emit(bundle.State{Run: run, CapturedReads: []kernel.CapturedRead{read}, Tokens: []kernel.Token{minted.Token}, AuditEntries: []kernel.AuditEntry{minted.AuditEntry}})
	if err != nil {
		t.Fatalf("bundle.Emit() error = %v", err)
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("bundle.Verify() error = %v", err)
	}
	results := []engine.Result{{ID: "res_matched", Outcome: engine.OutcomeMatched}}
	if includeMismatch {
		results = append(results,
			engine.Result{ID: "res_mismatch", Outcome: engine.OutcomeMismatch, Key: []engine.KeyValue{{Field: "id", Value: "a"}}, Differences: []engine.Difference{{Field: "amount", Left: "10.00", Right: "11.00"}}, Reason: "amount mismatch"},
			engine.Result{ID: "res_missing_right", Outcome: engine.OutcomeMissingRight, Reason: "not on right"},
		)
	}
	return &provenance.ProvenancedResultSet{
		Results:       engine.ResultSet{RunID: run.ID, Mode: "exact_match", LeftFileID: "left", RightFileID: "right", Results: results},
		CapturedReads: []provenance.CapturedReadRef{{FileID: "left", CapturedReadID: read.ID, SourcePath: read.SourcePath, SourceHash: read.SourceSHA256, Selector: read.Selector, Reader: read.ReaderName, ReaderVersion: read.ReaderVersion}},
		Tokens:        []kernel.Token{minted.Token},
		AuditEntries:  []kernel.AuditEntry{minted.AuditEntry},
		Bundle:        &provenance.BundleRef{Bytes: bundleBytes, SHA256: sha256HexForTest(bundleBytes)},
		Evidence:      []provenance.ResultEvidence{{ResultID: "res_matched", RowRefs: []provenance.RowEvidenceRef{{Side: "left", FileID: "left", RowKey: "row:1", Position: router.RowPosition{Row: 1}, CapturedReadID: read.ID, SourceHash: read.SourceSHA256}}}},
	}
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0).UTC() }

func readZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	entries := map[string][]byte{}
	for _, file := range reader.File {
		body, err := file.Open()
		if err != nil {
			t.Fatalf("open %s error = %v", file.Name, err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatalf("read %s error = %v", file.Name, err)
		}
		entries[file.Name] = data
	}
	return entries
}

func keys(entries map[string][]byte) []string {
	ordered := []string{"report.md", "report.json", "mismatches.csv", "captured_reads.json", "tokens.json", "audit.jsonl", "bundle.json", "manifest.json"}
	got := make([]string, 0, len(entries))
	for _, name := range ordered {
		if _, ok := entries[name]; ok {
			got = append(got, name)
		}
	}
	return got
}

func sha256HexForTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
