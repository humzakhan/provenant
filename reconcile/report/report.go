package report

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/provenance"
)

var zipModified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type manifest struct {
	BundleHash string             `json:"bundle_hash"`
	Artifacts  []manifestArtifact `json:"artifacts"`
}

type manifestArtifact struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type reportJSON struct {
	RunID         string            `json:"run_id"`
	Mode          string            `json:"mode"`
	LeftFileID    string            `json:"left_file_id,omitempty"`
	RightFileID   string            `json:"right_file_id,omitempty"`
	ResultCount   int               `json:"result_count"`
	OutcomeCounts map[string]int    `json:"outcome_counts"`
	Mismatches    []mismatchSummary `json:"mismatches"`
}

type mismatchSummary struct {
	ResultID string              `json:"result_id"`
	Outcome  engine.Outcome      `json:"outcome"`
	Key      []engine.KeyValue   `json:"key,omitempty"`
	Reason   string              `json:"reason,omitempty"`
	Diffs    []engine.Difference `json:"differences,omitempty"`
}

type auditLine struct {
	At                      string           `json:"at"`
	Hash                    string           `json:"hash"`
	Kind                    kernel.EntryKind `json:"kind"`
	PayloadDeterministic    json.RawMessage  `json:"payload_deterministic"`
	PayloadNonDeterministic json.RawMessage  `json:"payload_non_deterministic,omitempty"`
	PrevHash                string           `json:"prev_hash"`
	RunID                   string           `json:"run_id"`
	Seq                     uint64           `json:"seq"`
	TenantID                string           `json:"tenant_id"`
}

// Write writes a deterministic reconcile report ZIP to w.
func Write(ctx context.Context, w io.Writer, in *provenance.ProvenancedResultSet) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("reconcile_report_failed: missing writer")
	}
	if in == nil {
		return fmt.Errorf("reconcile_report_failed: missing provenanced result set")
	}
	if in.Bundle == nil || len(in.Bundle.Bytes) == 0 {
		return fmt.Errorf("reconcile_report_failed: missing bundle")
	}
	if got := sha256Hex(in.Bundle.Bytes); in.Bundle.SHA256 != "" && in.Bundle.SHA256 != got {
		return fmt.Errorf("reconcile_report_failed: bundle hash mismatch")
	}
	if err := bundle.Verify(in.Bundle.Bytes); err != nil {
		return fmt.Errorf("reconcile_report_failed: verify bundle: %w", err)
	}

	artifacts := make([]artifact, 0, 8)
	add := func(name string, data []byte) {
		artifacts = append(artifacts, artifact{name: name, data: data})
	}

	report, err := buildReportJSON(in)
	if err != nil {
		return err
	}
	mismatches, err := buildMismatchesCSV(in)
	if err != nil {
		return err
	}
	capturedReads, err := marshalJSON(in.CapturedReads)
	if err != nil {
		return fmt.Errorf("reconcile_report_failed: captured reads: %w", err)
	}
	tokens, err := marshalJSON(in.Tokens)
	if err != nil {
		return fmt.Errorf("reconcile_report_failed: tokens: %w", err)
	}
	audit, err := buildAuditJSONL(in.AuditEntries)
	if err != nil {
		return err
	}

	add("report.md", buildReportMarkdown(report))
	add("report.json", mustMarshalReport(report))
	add("mismatches.csv", mismatches)
	add("captured_reads.json", capturedReads)
	add("tokens.json", tokens)
	add("audit.jsonl", audit)
	add("bundle.json", append([]byte(nil), in.Bundle.Bytes...))
	manifestData, err := buildManifest(artifacts, sha256Hex(in.Bundle.Bytes))
	if err != nil {
		return err
	}
	add("manifest.json", manifestData)

	return writeZip(w, artifacts)
}

type artifact struct {
	name string
	data []byte
}

func buildReportJSON(in *provenance.ProvenancedResultSet) (reportJSON, error) {
	out := reportJSON{
		RunID:         in.Results.RunID,
		Mode:          in.Results.Mode,
		LeftFileID:    in.Results.LeftFileID,
		RightFileID:   in.Results.RightFileID,
		ResultCount:   len(in.Results.Results),
		OutcomeCounts: map[string]int{},
	}
	for _, result := range in.Results.Results {
		out.OutcomeCounts[string(result.Outcome)]++
		if result.Outcome == engine.OutcomeMatched {
			continue
		}
		out.Mismatches = append(out.Mismatches, mismatchSummary{
			ResultID: result.ID,
			Outcome:  result.Outcome,
			Key:      append([]engine.KeyValue(nil), result.Key...),
			Reason:   result.Reason,
			Diffs:    append([]engine.Difference(nil), result.Differences...),
		})
	}
	sort.Slice(out.Mismatches, func(i, j int) bool { return out.Mismatches[i].ResultID < out.Mismatches[j].ResultID })
	return out, nil
}

func buildReportMarkdown(report reportJSON) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Reconcile Report\n\n")
	fmt.Fprintf(&buf, "- Run ID: %s\n", report.RunID)
	fmt.Fprintf(&buf, "- Mode: %s\n", report.Mode)
	fmt.Fprintf(&buf, "- Result count: %d\n", report.ResultCount)
	fmt.Fprintf(&buf, "- Mismatch count: %d\n", len(report.Mismatches))
	keys := make([]string, 0, len(report.OutcomeCounts))
	for key := range report.OutcomeCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(&buf, "\n## Outcomes\n\n")
	for _, key := range keys {
		fmt.Fprintf(&buf, "- %s: %d\n", key, report.OutcomeCounts[key])
	}
	fmt.Fprintf(&buf, "\n## Mismatches\n\n")
	if len(report.Mismatches) == 0 {
		fmt.Fprintf(&buf, "No non-matched results.\n")
		return buf.Bytes()
	}
	for _, mismatch := range report.Mismatches {
		fmt.Fprintf(&buf, "- %s: %s", mismatch.ResultID, mismatch.Outcome)
		if mismatch.Reason != "" {
			fmt.Fprintf(&buf, " (%s)", mismatch.Reason)
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func mustMarshalReport(report reportJSON) []byte {
	data, err := marshalJSON(report)
	if err != nil {
		panic(err)
	}
	return data
}

func buildMismatchesCSV(in *provenance.ProvenancedResultSet) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"result_id", "outcome", "key", "field", "left", "right", "reason"}); err != nil {
		return nil, fmt.Errorf("reconcile_report_failed: mismatches csv: %w", err)
	}
	results := append([]engine.Result(nil), in.Results.Results...)
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	for _, result := range results {
		if result.Outcome == engine.OutcomeMatched {
			continue
		}
		resultKey := result.Key
		if resultKey == nil {
			resultKey = []engine.KeyValue{}
		}
		key, err := marshalCompactString(resultKey)
		if err != nil {
			return nil, fmt.Errorf("reconcile_report_failed: mismatch key: %w", err)
		}
		if len(result.Differences) == 0 {
			if err := writer.Write([]string{result.ID, string(result.Outcome), key, "", "", "", result.Reason}); err != nil {
				return nil, fmt.Errorf("reconcile_report_failed: mismatches csv: %w", err)
			}
			continue
		}
		diffs := append([]engine.Difference(nil), result.Differences...)
		sort.Slice(diffs, func(i, j int) bool { return diffs[i].Field < diffs[j].Field })
		for _, diff := range diffs {
			if err := writer.Write([]string{result.ID, string(result.Outcome), key, diff.Field, diff.Left, diff.Right, result.Reason}); err != nil {
				return nil, fmt.Errorf("reconcile_report_failed: mismatches csv: %w", err)
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("reconcile_report_failed: mismatches csv: %w", err)
	}
	return buf.Bytes(), nil
}

func buildAuditJSONL(entries []kernel.AuditEntry) ([]byte, error) {
	entries = append([]kernel.AuditEntry(nil), entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Seq < entries[j].Seq })
	var buf bytes.Buffer
	for _, entry := range entries {
		line := auditLine{
			At:                   entry.At.UTC().Format(time.RFC3339Nano),
			Hash:                 hex.EncodeToString(entry.Hash),
			Kind:                 entry.Kind,
			PayloadDeterministic: append(json.RawMessage(nil), entry.Payload.Deterministic...),
			PrevHash:             hex.EncodeToString(entry.PrevHash),
			RunID:                entry.RunID,
			Seq:                  entry.Seq,
			TenantID:             entry.TenantID,
		}
		if len(entry.Payload.NonDeterministic) > 0 {
			line.PayloadNonDeterministic = append(json.RawMessage(nil), entry.Payload.NonDeterministic...)
		}
		data, err := json.Marshal(line)
		if err != nil {
			return nil, fmt.Errorf("reconcile_report_failed: audit jsonl: %w", err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func buildManifest(artifacts []artifact, bundleHash string) ([]byte, error) {
	out := manifest{BundleHash: bundleHash, Artifacts: make([]manifestArtifact, 0, len(artifacts)+1)}
	for _, artifact := range artifacts {
		out.Artifacts = append(out.Artifacts, manifestArtifact{Name: artifact.name, Size: len(artifact.data), SHA256: sha256Hex(artifact.data)})
	}
	sort.Slice(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].Name < out.Artifacts[j].Name })
	return marshalJSON(out)
}

func writeZip(w io.Writer, artifacts []artifact) error {
	zw := zip.NewWriter(w)
	for _, artifact := range artifacts {
		header := &zip.FileHeader{Name: artifact.name, Method: zip.Store}
		header.SetModTime(zipModified)
		entry, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("reconcile_report_failed: create zip entry %q: %w", artifact.name, err)
		}
		if _, err := entry.Write(artifact.data); err != nil {
			_ = zw.Close()
			return fmt.Errorf("reconcile_report_failed: write zip entry %q: %w", artifact.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("reconcile_report_failed: close zip: %w", err)
	}
	return nil
}

func marshalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func marshalCompactString(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
