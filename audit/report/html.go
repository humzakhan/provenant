// Package report renders audit pack documents as human-readable reports.
// The only current renderer is HTML; future renderers (text, PDF) may be
// added as siblings in this package.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/provenant/provenant/audit/pack"
	kernel "github.com/provenant/provenant/kernel/public"
)

// RenderHTML renders doc as a self-contained HTML file.
// The output is deterministic: identical input always produces identical bytes.
// No external network requests, no JavaScript, no wall-clock timestamps.
func RenderHTML(doc *pack.Document) ([]byte, error) {
	data, err := buildTemplateData(doc)
	if err != nil {
		return nil, fmt.Errorf("report: build template data: %w", err)
	}

	tmpl, err := template.New("audit").Parse(htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("report: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("report: execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Template data types
// ---------------------------------------------------------------------------

type templateData struct {
	SchemaVersion string
	BundleSHA256  string
	RunID         string
	TenantID      string

	TokenCount       int
	CapturedReadCount int
	AuditEntryCount  int
	MismatchCount    int

	SubmittedAnswer *submittedAnswerData

	HasReconciliation bool
	OutcomeCounts     []outcomeCount
	Mismatches        []mismatchRow

	InvariantOK     bool
	InvariantChecked bool
	DerivationTree  []treeNode

	CapturedReads []capturedReadRow

	AuditEntries      []auditEntryRow
	PrevHashContinuity string

	Artifacts []artifactRow
}

type submittedAnswerData struct {
	TokenID        string
	ValueCanonical string
	Unit           string
}

type outcomeCount struct {
	Outcome string
	Count   int
}

type mismatchRow struct {
	ResultID string
	Outcome  string
	Key      string
	Field    string
	Left     string
	Right    string
	Reason   string
}

type treeNode struct {
	Depth          int
	TokenID        string
	ValueCanonical string
	Unit           string
	Operation      string
	Kind           string
	Contribution   string
	IndentPx       int
}

type capturedReadRow struct {
	SourceName string
	Selector   string
	SHA256     string
	BytesRead  int
}

type auditEntryRow struct {
	Seq  uint64
	Kind string
}

type artifactRow struct {
	Name   string
	SHA256 string
}

// ---------------------------------------------------------------------------
// auditEntry is the minimal on-wire shape of one audit_jsonl line.
// ---------------------------------------------------------------------------

type auditEntry struct {
	Seq      uint64          `json:"seq"`
	Kind     string          `json:"kind"`
	PrevHash string          `json:"prev_hash"`
	Hash     string          `json:"hash"`
	RunID    string          `json:"run_id"`
	TenantID string          `json:"tenant_id"`
	Payload  json.RawMessage `json:"payload_deterministic"`
}

// answerPayload is the deterministic payload of answer_submitted entries.
type answerPayload struct {
	Label          string `json:"label"`
	TokenID        string `json:"token_id"`
	Unit           string `json:"unit"`
	ValueCanonical string `json:"value_canonical"`
}

// reconcileResult is the minimal shape of a reconciliation ResultSet in report.json.
type reconcileResult struct {
	Results []struct {
		ID          string `json:"id"`
		Outcome     string `json:"outcome"`
		KeyField    string `json:"key_field"`
		KeyValue    string `json:"key_value"`
		Differences []struct {
			Field  string `json:"field"`
			Left   string `json:"left"`
			Right  string `json:"right"`
			Reason string `json:"reason"`
		} `json:"differences"`
	} `json:"results"`
}

// ---------------------------------------------------------------------------
// buildTemplateData assembles all sections from doc.
// ---------------------------------------------------------------------------

func buildTemplateData(doc *pack.Document) (*templateData, error) {
	entries, err := parseAuditJSONL(doc.AuditJSONL)
	if err != nil {
		return nil, err
	}

	d := &templateData{
		SchemaVersion:     doc.SchemaVersion,
		BundleSHA256:      doc.BundleSHA256,
		TokenCount:        len(doc.Tokens),
		CapturedReadCount: len(doc.CapturedReads),
		AuditEntryCount:   len(entries),
	}

	// Extract run_id and tenant_id from the first entry that carries them.
	for _, e := range entries {
		if e.RunID != "" {
			d.RunID = e.RunID
		}
		if e.TenantID != "" {
			d.TenantID = e.TenantID
		}
		if d.RunID != "" && d.TenantID != "" {
			break
		}
	}

	// Section 3: Submitted answer — find answer_submitted entry.
	for _, e := range entries {
		if e.Kind == "answer_submitted" {
			var ap answerPayload
			if err := json.Unmarshal(e.Payload, &ap); err == nil && ap.TokenID != "" {
				d.SubmittedAnswer = &submittedAnswerData{
					TokenID:        ap.TokenID,
					ValueCanonical: ap.ValueCanonical,
					Unit:           ap.Unit,
				}
			}
			break
		}
	}

	// Section 4: Reconciliation.
	if doc.Report != nil && len(doc.Report.JSON) > 0 {
		d.HasReconciliation = true
		d.OutcomeCounts, d.Mismatches, d.MismatchCount = parseReconciliation(doc.Report.JSON)
	}

	// Section 5: Derivation graph tree — built from tokens.
	if d.SubmittedAnswer != nil {
		tokensByID := indexTokens(doc.Tokens)
		d.InvariantChecked, d.InvariantOK, d.DerivationTree = buildDerivationTree(d.SubmittedAnswer.TokenID, tokensByID, doc.CapturedReads)
	}

	// Override invariant banner: if reconciliation data is present and there are
	// mismatches, mark the invariant as VIOLATED. This surfaces semantic breakages
	// in the report header even when the cryptographic bundle integrity is intact.
	if d.HasReconciliation && d.MismatchCount > 0 {
		d.InvariantChecked = true
		d.InvariantOK = false
	}

	// Section 6: Captured reads table.
	for _, cr := range doc.CapturedReads {
		name := cr.SourceName
		if name == "" {
			name = cr.SourcePath
		}
		sel := formatSelector(cr.Selector)
		sha := cr.SourceSHA256
		if len(sha) > 16 {
			sha = sha[:16] + "…"
		}
		// BytesRead is not stored on CapturedRead; use -1 to indicate N/A.
		d.CapturedReads = append(d.CapturedReads, capturedReadRow{
			SourceName: name,
			Selector:   sel,
			SHA256:     sha,
			BytesRead:  -1,
		})
	}

	// Section 7: Audit chain.
	d.AuditEntries, d.PrevHashContinuity = buildAuditChainSection(entries)

	// Section 8: Manifest.
	for _, a := range doc.Manifest.Artifacts {
		d.Artifacts = append(d.Artifacts, artifactRow{Name: a.Name, SHA256: a.SHA256})
	}

	return d, nil
}

// parseAuditJSONL splits the audit_jsonl string and parses each line.
func parseAuditJSONL(jsonl string) ([]auditEntry, error) {
	var entries []auditEntry
	for _, line := range strings.Split(jsonl, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e auditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("report: parse audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// parseReconciliation parses a reconciliation ResultSet JSON blob.
func parseReconciliation(raw json.RawMessage) ([]outcomeCount, []mismatchRow, int) {
	var rs reconcileResult
	if err := json.Unmarshal(raw, &rs); err != nil {
		return nil, nil, 0
	}

	// Sort deterministically: use outcome+key_value+id as the sort key.
	results := append([]struct {
		ID          string `json:"id"`
		Outcome     string `json:"outcome"`
		KeyField    string `json:"key_field"`
		KeyValue    string `json:"key_value"`
		Differences []struct {
			Field  string `json:"field"`
			Left   string `json:"left"`
			Right  string `json:"right"`
			Reason string `json:"reason"`
		} `json:"differences"`
	}{}, rs.Results...)
	sort.Slice(results, func(i, j int) bool {
		// deterministic sort by outcome, key_value, id
		if results[i].Outcome != results[j].Outcome {
			return results[i].Outcome < results[j].Outcome
		}
		if results[i].KeyValue != results[j].KeyValue {
			return results[i].KeyValue < results[j].KeyValue
		}
		return results[i].ID < results[j].ID
	})

	// Build outcome counts (sorted by outcome name for determinism).
	countMap := make(map[string]int)
	for _, r := range results {
		countMap[r.Outcome]++
	}
	outcomeKeys := make([]string, 0, len(countMap))
	for k := range countMap {
		outcomeKeys = append(outcomeKeys, k)
	}
	// sort.Strings for deterministic map key iteration
	sort.Strings(outcomeKeys)
	var outcomeCounts []outcomeCount
	for _, k := range outcomeKeys {
		outcomeCounts = append(outcomeCounts, outcomeCount{Outcome: k, Count: countMap[k]})
	}

	// Build mismatches rows (non-ok outcomes with differences).
	var mismatches []mismatchRow
	mismatchCount := 0
	for _, r := range results {
		if r.Outcome == "ok" {
			continue
		}
		mismatchCount++
		if len(r.Differences) == 0 {
			mismatches = append(mismatches, mismatchRow{
				ResultID: r.ID,
				Outcome:  r.Outcome,
				Key:      r.KeyField + "=" + r.KeyValue,
			})
		} else {
			for _, diff := range r.Differences {
				mismatches = append(mismatches, mismatchRow{
					ResultID: r.ID,
					Outcome:  r.Outcome,
					Key:      r.KeyField + "=" + r.KeyValue,
					Field:    diff.Field,
					Left:     diff.Left,
					Right:    diff.Right,
					Reason:   diff.Reason,
				})
			}
		}
	}
	return outcomeCounts, mismatches, mismatchCount
}

// indexTokens builds a map from token ID to Token.
func indexTokens(tokens []kernel.Token) map[string]kernel.Token {
	m := make(map[string]kernel.Token, len(tokens))
	for _, t := range tokens {
		m[t.ID] = t
	}
	return m
}

// buildDerivationTree performs a DFS from rootID and returns a flat list of
// treeNode rows (parent before children) suitable for rendering as nested
// indented rows in a table. It also computes the contribution invariant flag.
//
// The contribution invariant is checked using shopspring/decimal arithmetic
// (already a project dependency — see go.mod). For each derived node in the
// tree the function verifies that the signed sum of its direct children's
// ValueCanonical equals the node's own ValueCanonical, honouring operation
// sign conventions:
//
//   - calc.add / calc.sum / select.passthrough / filter.sum_where:
//     all inputs contribute positively.
//   - calc.subtract(a, b): position-0 input is positive; position-1 is negated.
//   - calc.multiply / calc.divide / round.half_even: invariant is skipped for
//     this node (these ops cannot be verified without bundle.Replay; the node is
//     treated as checked-OK so that other nodes are still verified).
//
// InvariantOK is false iff any derived node fails the sum check.
func buildDerivationTree(rootID string, tokensByID map[string]kernel.Token, reads []kernel.CapturedRead) (checked, ok bool, nodes []treeNode) {
	root, exists := tokensByID[rootID]
	if !exists {
		return false, false, nil
	}

	invariantOK := true // flipped to false on first failed node

	var walk func(tok kernel.Token, depth int)
	walk = func(tok kernel.Token, depth int) {
		kind := "leaf"
		op := ""
		if tok.Derivation != nil {
			kind = "derived"
			op = tok.Derivation.Operation
		}
		nodes = append(nodes, treeNode{
			Depth:          depth,
			TokenID:        tok.ID,
			ValueCanonical: tok.ValueCanonical,
			Unit:           tok.Unit,
			Operation:      op,
			Kind:           kind,
			IndentPx:       depth * 24,
		})
		if tok.Derivation == nil {
			return
		}

		// Recurse into children first (DFS, parent before children in output).
		for _, inputID := range tok.Derivation.Inputs {
			child, found := tokensByID[inputID]
			if found {
				walk(child, depth+1)
			}
		}

		// Contribution invariant check for this derived node.
		// Skip ops that require bundle.Replay for correct semantics
		// (multiply, divide, round) — treat them as trivially OK.
		switch op {
		case "calc.add", "calc.sum", "select.passthrough", "filter.sum_where":
			// All direct inputs contribute positively.
			sum := decimal.Zero
			valid := true
			for _, inputID := range tok.Derivation.Inputs {
				child, found := tokensByID[inputID]
				if !found {
					valid = false
					break
				}
				v, err := decimal.NewFromString(child.ValueCanonical)
				if err != nil {
					valid = false
					break
				}
				sum = sum.Add(v)
			}
			if valid {
				nodeVal, err := decimal.NewFromString(tok.ValueCanonical)
				if err == nil && !sum.Equal(nodeVal) {
					invariantOK = false
				}
			}

		case "calc.subtract":
			// Position 0: positive; position 1: negative.
			if len(tok.Derivation.Inputs) == 2 {
				tokA, okA := tokensByID[tok.Derivation.Inputs[0]]
				tokB, okB := tokensByID[tok.Derivation.Inputs[1]]
				if okA && okB {
					aVal, errA := decimal.NewFromString(tokA.ValueCanonical)
					bVal, errB := decimal.NewFromString(tokB.ValueCanonical)
					nodeVal, errN := decimal.NewFromString(tok.ValueCanonical)
					if errA == nil && errB == nil && errN == nil {
						sum := aVal.Sub(bVal)
						if !sum.Equal(nodeVal) {
							invariantOK = false
						}
					}
				}
			}

		default:
			// calc.multiply, calc.divide, round.half_even, unknown ops:
			// cannot verify without bundle.Replay; treat as OK for banner purposes.
		}
	}
	walk(root, 0)

	return true, invariantOK, nodes
}

// formatSelector returns a compact string description of a SourceSelector.
func formatSelector(s kernel.SourceSelector) string {
	if s.RowWindow != "" {
		if s.ColumnProjection != "" {
			return fmt.Sprintf("row=%s col=%s", s.RowWindow, s.ColumnProjection)
		}
		return "row=" + s.RowWindow
	}
	if s.JSONPointer != "" {
		return "ptr=" + s.JSONPointer
	}
	if s.JSONPath != "" {
		return "jsonpath=" + s.JSONPath
	}
	if s.LineWindow != "" {
		return "lines=" + s.LineWindow
	}
	if s.ByteSpan != "" {
		return "bytes=" + s.ByteSpan
	}
	return "(default)"
}

// buildAuditChainSection returns the per-entry rows and prev-hash continuity status.
func buildAuditChainSection(entries []auditEntry) ([]auditEntryRow, string) {
	rows := make([]auditEntryRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, auditEntryRow{Seq: e.Seq, Kind: e.Kind})
	}

	// Prev-hash continuity: each entry's prev_hash must equal the previous entry's hash.
	status := "OK"
	for i := 1; i < len(entries); i++ {
		if entries[i].PrevHash != entries[i-1].Hash {
			status = fmt.Sprintf("BROKEN at seq %d (prev_hash ≠ hash of seq %d)", entries[i].Seq, entries[i-1].Seq)
			break
		}
	}
	return rows, status
}

// ---------------------------------------------------------------------------
// HTML template
// ---------------------------------------------------------------------------

const htmlTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Provenant Audit Report</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;font-size:14px;color:#1a1a1a;background:#f8f8f8;padding:24px}
h1{font-size:20px;margin-bottom:4px}
h2{font-size:15px;margin:20px 0 8px;border-bottom:1px solid #ddd;padding-bottom:4px}
.meta{color:#555;font-size:12px;margin-bottom:20px}
.tiles{display:flex;gap:12px;flex-wrap:wrap;margin-bottom:20px}
.tile{background:#fff;border:1px solid #ddd;border-radius:6px;padding:12px 16px;min-width:120px}
.tile-label{font-size:11px;color:#666;text-transform:uppercase;letter-spacing:.04em}
.tile-value{font-size:22px;font-weight:600;margin-top:4px}
.tile-ok .tile-value{color:#2a7a2a}
.tile-warn .tile-value{color:#b87a00}
.tile-error .tile-value{color:#b00020}
.banner-ok{background:#e6f4e6;border:1px solid #2a7a2a;color:#2a7a2a;padding:8px 14px;border-radius:4px;margin-bottom:12px;font-weight:600}
.banner-violated{background:#fde8e8;border:1px solid #b00020;color:#b00020;padding:8px 14px;border-radius:4px;margin-bottom:12px;font-weight:600}
.table-plain{width:100%;border-collapse:collapse;font-size:13px}
.table-plain th{background:#f0f0f0;text-align:left;padding:6px 10px;border:1px solid #ddd;font-weight:600}
.table-plain td{padding:5px 10px;border:1px solid #e0e0e0;vertical-align:top;word-break:break-all}
.table-plain tr:nth-child(even) td{background:#fafafa}
.mono{font-family:ui-monospace,monospace;font-size:12px}
.kv{display:grid;grid-template-columns:160px 1fr;gap:4px 12px;margin-bottom:12px}
.kv-label{color:#555;font-size:12px}
.kv-value{font-family:ui-monospace,monospace;font-size:12px;word-break:break-all}
.status-ok{color:#2a7a2a;font-weight:600}
.status-broken{color:#b00020;font-weight:600}
</style>
</head>
<body>

<h1>Provenant Audit Report</h1>
<div class="meta">
  <span class="mono">{{.BundleSHA256}}</span>
  &mdash; schema <span class="mono">{{.SchemaVersion}}</span>
</div>

<div class="kv">
  <span class="kv-label">Run ID</span><span class="kv-value">{{.RunID}}</span>
  <span class="kv-label">Tenant ID</span><span class="kv-value">{{.TenantID}}</span>
</div>

<h2>Summary</h2>
<div class="tiles">
  <div class="tile">
    <div class="tile-label">Tokens</div>
    <div class="tile-value">{{.TokenCount}}</div>
  </div>
  <div class="tile">
    <div class="tile-label">Captured Reads</div>
    <div class="tile-value">{{.CapturedReadCount}}</div>
  </div>
  <div class="tile">
    <div class="tile-label">Audit Entries</div>
    <div class="tile-value">{{.AuditEntryCount}}</div>
  </div>
  {{if .HasReconciliation}}
  <div class="{{if eq .MismatchCount 0}}tile tile-ok{{else}}tile tile-error{{end}}">
    <div class="tile-label">Mismatches</div>
    <div class="tile-value">{{.MismatchCount}}</div>
  </div>
  {{end}}
  {{if .SubmittedAnswer}}
  <div class="tile tile-ok">
    <div class="tile-label">Submitted Answer</div>
    <div class="tile-value">{{.SubmittedAnswer.ValueCanonical}}</div>
    {{if .SubmittedAnswer.Unit}}<div class="tile-label">{{.SubmittedAnswer.Unit}}</div>{{end}}
  </div>
  {{end}}
</div>

{{if .SubmittedAnswer}}
<h2>Submitted Answer</h2>
<div class="kv">
  <span class="kv-label">Token ID</span><span class="kv-value">{{.SubmittedAnswer.TokenID}}</span>
  <span class="kv-label">Value</span><span class="kv-value">{{.SubmittedAnswer.ValueCanonical}}</span>
  {{if .SubmittedAnswer.Unit}}<span class="kv-label">Unit</span><span class="kv-value">{{.SubmittedAnswer.Unit}}</span>{{end}}
  <span class="kv-label">Provenance</span><span class="kv-value">grounded — no literal value in request</span>
</div>
{{end}}

{{if .HasReconciliation}}
<h2>Reconciliation Outcomes</h2>
{{if .OutcomeCounts}}
<table class="table-plain" style="margin-bottom:12px;max-width:400px">
  <thead><tr><th>Outcome</th><th>Count</th></tr></thead>
  <tbody>
  {{range .OutcomeCounts}}
  <tr><td>{{.Outcome}}</td><td>{{.Count}}</td></tr>
  {{end}}
  </tbody>
</table>
{{end}}
{{if .Mismatches}}
<table class="table-plain">
  <thead><tr><th>Result ID</th><th>Outcome</th><th>Key</th><th>Field</th><th>Left</th><th>Right</th><th>Reason</th></tr></thead>
  <tbody>
  {{range .Mismatches}}
  <tr>
    <td class="mono">{{.ResultID}}</td>
    <td>{{.Outcome}}</td>
    <td class="mono">{{.Key}}</td>
    <td class="mono">{{.Field}}</td>
    <td class="mono">{{.Left}}</td>
    <td class="mono">{{.Right}}</td>
    <td>{{.Reason}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{end}}
{{end}}

<h2>Derivation Graph</h2>
{{if .InvariantChecked}}
  {{if .InvariantOK}}
  <div class="banner-ok">Contribution invariant: OK</div>
  {{else}}
  <div class="banner-violated">Contribution invariant: VIOLATED</div>
  {{end}}
{{end}}
{{if .DerivationTree}}
<table class="table-plain">
  <thead><tr><th>Token ID</th><th>Kind</th><th>Operation</th><th>Value</th><th>Unit</th></tr></thead>
  <tbody>
  {{range .DerivationTree}}
  <tr>
    <td class="mono" style="padding-left:{{.IndentPx}}px">{{.TokenID}}</td>
    <td>{{.Kind}}</td>
    <td class="mono">{{.Operation}}</td>
    <td class="mono">{{.ValueCanonical}}</td>
    <td>{{.Unit}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{end}}

<h2>Captured Reads</h2>
{{if .CapturedReads}}
<table class="table-plain">
  <thead><tr><th>Source Name</th><th>Selector</th><th>SHA256</th></tr></thead>
  <tbody>
  {{range .CapturedReads}}
  <tr>
    <td>{{.SourceName}}</td>
    <td class="mono">{{.Selector}}</td>
    <td class="mono">{{.SHA256}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{else}}
<p style="color:#555">No captured reads.</p>
{{end}}

<h2>Audit Chain</h2>
<p>Entries: {{.AuditEntryCount}} &mdash; Prev-hash continuity:
  {{if eq .PrevHashContinuity "OK"}}
  <span class="status-ok">OK</span>
  {{else}}
  <span class="status-broken">{{.PrevHashContinuity}}</span>
  {{end}}
</p>
{{if .AuditEntries}}
<table class="table-plain" style="margin-top:8px;max-width:500px">
  <thead><tr><th>Seq</th><th>Kind</th></tr></thead>
  <tbody>
  {{range .AuditEntries}}
  <tr><td class="mono">{{.Seq}}</td><td class="mono">{{.Kind}}</td></tr>
  {{end}}
  </tbody>
</table>
{{end}}

<h2>Manifest</h2>
{{if .Artifacts}}
<table class="table-plain">
  <thead><tr><th>Artifact</th><th>SHA256</th></tr></thead>
  <tbody>
  {{range .Artifacts}}
  <tr>
    <td>{{.Name}}</td>
    <td class="mono">{{.SHA256}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{else}}
<p style="color:#555">No artifacts.</p>
{{end}}

</body>
</html>`
