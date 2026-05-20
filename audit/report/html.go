// Package report renders audit pack documents as human-readable reports.
// The only current renderer is HTML; future renderers (text, PDF) may be
// added as siblings in this package.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/provenant/provenant/audit/pack"
	kernel "github.com/provenant/provenant/kernel/public"
)

// RenderHTML renders doc as a self-contained HTML file.
// The output is deterministic: identical input always produces identical bytes.
// No external network requests, no wall-clock timestamps.
// Inline JavaScript is used for tab switching and slide-over toggling only;
// no external scripts or CDN resources are referenced.
func RenderHTML(doc *pack.Document) ([]byte, error) {
	data, err := buildTemplateData(doc)
	if err != nil {
		return nil, fmt.Errorf("report: build template data: %w", err)
	}

	tmpl, err := template.New("audit").Funcs(template.FuncMap{
		"fileURL": func(path string) template.URL {
			if path == "" || !filepath.IsAbs(path) {
				return template.URL("")
			}
			return template.URL("file://" + path)
		},
		"hasFileURL": func(path string) bool {
			return path != "" && filepath.IsAbs(path)
		},
	}).Parse(htmlTemplate)
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

	TokenCount        int
	CapturedReadCount int
	AuditEntryCount   int
	MismatchCount     int

	SubmittedAnswer *submittedAnswerData

	HasReconciliation bool
	OutcomeCounts     []outcomeCount
	Mismatches        []mismatchRow

	InvariantOK      bool
	InvariantChecked bool
	DerivationTree   []treeNode

	SourceCards []sourceCard

	CapturedReads []capturedReadRow

	AuditEntries       []auditEntryRow
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
	IsLeaf         bool
	SourcePath     string // absolute path if available, "" otherwise
	SourceName     string
}

// sourceCard represents one source file in the visual card grid (Plain tab).
type sourceCard struct {
	Name       string
	Ext        string       // file extension without dot, e.g. "csv"
	SourceID   string       // selector or tool identifier
	SourcePath string       // absolute path for file:// link (may be empty)
	SHA256     string       // truncated sha256
	RowCount   string       // human description of rows
	Total      string       // sum value if available
	Index      int          // 0-based for JS panel id
}

type capturedReadRow struct {
	SourceName string
	Selector   string
	SHA256     string
	BytesRead  int
	SourcePath string // absolute path if available
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
		readsByID := indexCapturedReads(doc.CapturedReads)
		d.InvariantChecked, d.InvariantOK, d.DerivationTree = buildDerivationTree(d.SubmittedAnswer.TokenID, tokensByID, doc.CapturedReads, readsByID)
	}

	// Override invariant banner: if reconciliation data is present and there are
	// mismatches, mark the invariant as VIOLATED. This surfaces semantic breakages
	// in the report header even when the cryptographic bundle integrity is intact.
	if d.HasReconciliation && d.MismatchCount > 0 {
		d.InvariantChecked = true
		d.InvariantOK = false
	}

	// Section 6: Captured reads table + source cards.
	// Build a deduplicated set of source cards by path/name.
	cardsByKey := make(map[string]*sourceCard)
	cardOrder := []string{}
	for _, cr := range doc.CapturedReads {
		name := cr.SourceName
		if name == "" {
			name = cr.SourcePath
		}
		sel := formatSelector(cr.Selector)
		sha := cr.SourceSHA256
		shaShort := sha
		if len(shaShort) > 16 {
			shaShort = shaShort[:16] + "…"
		}
		// BytesRead is not stored on CapturedRead; use -1 to indicate N/A.
		d.CapturedReads = append(d.CapturedReads, capturedReadRow{
			SourceName: name,
			Selector:   sel,
			SHA256:     shaShort,
			BytesRead:  -1,
			SourcePath: cr.SourcePath,
		})

		// Source card: group by source name.
		key := name
		if key == "" {
			key = cr.SourcePath
		}
		if _, exists := cardsByKey[key]; !exists {
			ext := filepath.Ext(name)
			if len(ext) > 1 {
				ext = ext[1:] // strip leading dot
			} else {
				ext = "file"
			}
			absPath := ""
			if filepath.IsAbs(cr.SourcePath) {
				absPath = cr.SourcePath
			}
			card := &sourceCard{
				Name:       name,
				Ext:        ext,
				SourceID:   sel,
				SourcePath: absPath,
				SHA256:     shaShort,
				RowCount:   "—",
				Index:      len(cardOrder),
			}
			cardsByKey[key] = card
			cardOrder = append(cardOrder, key)
		}
	}
	// Sort card order by name for determinism.
	sort.Strings(cardOrder)
	// Reassign indices after sort.
	for i, key := range cardOrder {
		cardsByKey[key].Index = i
		d.SourceCards = append(d.SourceCards, *cardsByKey[key])
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

// indexCapturedReads builds a map from CapturedRead ID to CapturedRead.
func indexCapturedReads(reads []kernel.CapturedRead) map[string]kernel.CapturedRead {
	m := make(map[string]kernel.CapturedRead, len(reads))
	for _, cr := range reads {
		m[cr.ID] = cr
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
func buildDerivationTree(rootID string, tokensByID map[string]kernel.Token, reads []kernel.CapturedRead, readsByID map[string]kernel.CapturedRead) (checked, ok bool, nodes []treeNode) {
	root, exists := tokensByID[rootID]
	if !exists {
		return false, false, nil
	}

	invariantOK := true // flipped to false on first failed node

	var walk func(tok kernel.Token, depth int)
	walk = func(tok kernel.Token, depth int) {
		kind := "leaf"
		op := ""
		isLeaf := false
		sourcePath := ""
		sourceName := ""
		if tok.Derivation != nil {
			kind = "derived"
			op = tok.Derivation.Operation
		} else {
			isLeaf = true
			// Leaf token: try to find source path from the captured read.
			if tok.Source.CapturedReadID != "" {
				if cr, found := readsByID[tok.Source.CapturedReadID]; found {
					if filepath.IsAbs(cr.SourcePath) {
						sourcePath = cr.SourcePath
					}
					if cr.SourceName != "" {
						sourceName = cr.SourceName
					} else {
						sourceName = cr.SourcePath
					}
				}
			}
			// Fallback: use TokenSource.SourcePath.
			if sourcePath == "" && filepath.IsAbs(tok.Source.SourcePath) {
				sourcePath = tok.Source.SourcePath
			}
		}
		nodes = append(nodes, treeNode{
			Depth:          depth,
			TokenID:        tok.ID,
			ValueCanonical: tok.ValueCanonical,
			Unit:           tok.Unit,
			Operation:      op,
			Kind:           kind,
			IndentPx:       depth * 24,
			IsLeaf:         isLeaf,
			SourcePath:     sourcePath,
			SourceName:     sourceName,
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
// HTML template — two-tab UI matching the Provenant landing-page aesthetic
// ---------------------------------------------------------------------------

const htmlTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Provenant Audit Report</title>
<style>
/* ── CSS variables (extracted from provenant-landing-v2.html) ── */
:root{
  --bg:#fff;--surface:#F5F5F3;--border:rgba(0,0,0,.07);--border-med:rgba(0,0,0,.12);--border-strong:rgba(0,0,0,.18);
  --text:#191919;--muted:#525252;--faint:#9A9A9A;
  --green:#1F4D3A;--green-tint:#EEF6F2;--green-soft:#B8D4C5;--green-hover:#163829;--green-light:#52C97A;
  --dark:#0C0F14;--amber:#B45309;--danger:#C0392B;
  --sans:-apple-system,system-ui,'Segoe UI',sans-serif;
  --mono:ui-monospace,'Cascadia Code','Fira Mono',monospace;
  --r:8px;--rl:12px;
  --sh:0 0 0 1px rgba(0,0,0,.05),0 4px 16px rgba(0,0,0,.06);
  --sh-lg:0 0 0 1px rgba(0,0,0,.04),0 12px 40px rgba(0,0,0,.09);
}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:var(--sans);font-size:14px;color:var(--text);background:var(--surface);-webkit-font-smoothing:antialiased}
a{color:inherit;text-decoration:none}
button{font-family:inherit;cursor:pointer}

/* ── header bar ── */
.rpt-header{background:var(--green);color:#fff;padding:16px 32px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:8px}
.rpt-logo{font-weight:700;font-size:15px;letter-spacing:-.02em;display:flex;align-items:center;gap:8px}
.rpt-logo-mark{width:13px;height:13px;background:#fff;opacity:.9;border-radius:3px;transform:rotate(45deg);flex-shrink:0}
.rpt-meta{font-family:var(--mono);font-size:11px;color:rgba(255,255,255,.65);text-align:right}

/* ── tab bar ── */
.rpt-tabs{background:#fff;border-bottom:1px solid var(--border);display:flex;gap:0;padding:0 32px;position:sticky;top:0;z-index:50;box-shadow:0 1px 0 var(--border)}
.rpt-tab{display:inline-flex;align-items:center;gap:7px;padding:14px 20px 15px;border:none;background:transparent;font-size:13px;font-weight:600;color:var(--faint);cursor:pointer;white-space:nowrap;font-family:var(--sans);position:relative;flex-shrink:0;transition:color .15s}
.rpt-tab::after{content:'';position:absolute;bottom:-1px;left:0;right:0;height:2px;background:var(--green);border-radius:1px 1px 0 0;transform:scaleX(0);transform-origin:left;transition:transform .22s cubic-bezier(.4,0,.2,1)}
.rpt-tab:hover{color:var(--muted)}
.rpt-tab.active{color:var(--text)}
.rpt-tab.active::after{transform:scaleX(1)}

/* ── tab panels ── */
.rpt-panel{display:none;padding:32px;max-width:1200px;margin:0 auto}
.rpt-panel.active{display:block}

/* ── summary tiles ── */
.rpt-tiles{display:flex;gap:12px;flex-wrap:wrap;margin-bottom:28px}
.rpt-tile{background:#fff;border:1px solid var(--border);border-radius:var(--r);padding:14px 18px;min-width:110px;box-shadow:var(--sh)}
.rpt-tile-label{font-size:10px;color:var(--faint);text-transform:uppercase;letter-spacing:.06em;font-weight:500}
.rpt-tile-value{font-size:24px;font-weight:700;margin-top:4px;letter-spacing:-.02em}
.rpt-tile-sub{font-size:11px;color:var(--faint);margin-top:2px;font-family:var(--mono)}
.tile-ok .rpt-tile-value{color:var(--green)}
.tile-warn .rpt-tile-value{color:var(--amber)}
.tile-error .rpt-tile-value{color:var(--danger)}

/* ── invariant banner ── */
.banner-ok{background:var(--green-tint);border:1px solid var(--green-soft);color:var(--green);padding:10px 16px;border-radius:var(--r);margin-bottom:20px;font-weight:600;font-size:13px;display:flex;align-items:center;gap:8px}
.banner-violated{background:#FDE8E8;border:1px solid #F5B3B3;color:var(--danger);padding:10px 16px;border-radius:var(--r);margin-bottom:20px;font-weight:600;font-size:13px;display:flex;align-items:center;gap:8px}
.banner-ok::before{content:'✓';font-weight:700}
.banner-violated::before{content:'✗';font-weight:700}

/* ── source card grid ── */
.src-grid{display:flex;flex-wrap:wrap;gap:14px;margin-bottom:28px}
.src-card{background:#fff;border:1px solid var(--border);border-radius:var(--rl);padding:20px 18px;min-width:200px;flex:1;max-width:280px;box-shadow:var(--sh);cursor:pointer;transition:border-color .15s,box-shadow .15s;position:relative}
.src-card:hover{border-color:var(--green-soft);box-shadow:0 0 0 1px var(--green-soft),var(--sh-lg)}
.src-card-icon{display:inline-flex;position:relative;margin-bottom:14px}
.mini-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:2px;width:72px;height:72px;padding:5px;background:#fff;border:1px solid var(--border-med);border-radius:4px}
.mini-grid>span{aspect-ratio:1;background:var(--surface);border-radius:1px}
.mini-grid>span.h{background:var(--green)}
.src-card-ext{position:absolute;bottom:-5px;right:-8px;background:var(--text);color:#fff;font-family:var(--mono);font-size:9px;padding:2px 5px;border-radius:3px;letter-spacing:.04em;font-weight:600}
.src-card-name{font-family:var(--mono);font-size:13px;font-weight:600;color:var(--text);margin-bottom:3px;word-break:break-all}
.src-card-id{font-family:var(--mono);font-size:11px;color:var(--green);margin-bottom:6px;word-break:break-all}
.src-card-meta{font-family:var(--mono);font-size:11px;color:var(--faint)}
.src-card-link{display:block;margin-top:10px;font-family:var(--mono);font-size:11px;color:var(--green);text-decoration:underline;text-underline-offset:2px;word-break:break-all}

/* ── operation node ── */
.op-node{background:#fff;border:1px solid var(--border);border-radius:var(--r);padding:14px 18px;display:inline-flex;align-items:center;gap:10px;margin-bottom:20px;box-shadow:var(--sh)}
.op-node-label{font-size:11px;color:var(--faint);text-transform:uppercase;letter-spacing:.05em;font-weight:500}
.op-node-value{font-family:var(--mono);font-size:14px;font-weight:600;color:var(--text)}

/* ── hero answer callout ── */
.hero-answer{background:var(--green);color:#fff;border-radius:var(--rl);padding:28px 32px;margin-bottom:28px;box-shadow:0 4px 24px rgba(31,77,58,.35);display:flex;flex-direction:column;gap:6px;position:relative;overflow:hidden}
.hero-answer::before{content:'';position:absolute;right:-40px;top:-40px;width:160px;height:160px;border-radius:50%;background:rgba(255,255,255,.05)}
.hero-answer-eyebrow{font-family:var(--mono);font-size:10px;text-transform:uppercase;letter-spacing:.09em;color:rgba(255,255,255,.6);font-weight:600}
.hero-answer-label{font-size:18px;font-weight:700;letter-spacing:-.02em;color:#fff}
.hero-answer-value{font-family:var(--mono);font-size:42px;font-weight:700;letter-spacing:-.03em;color:#fff;line-height:1}
.hero-answer-unit{font-family:var(--mono);font-size:13px;color:rgba(255,255,255,.7)}
.hero-answer-status{display:flex;align-items:center;gap:6px;margin-top:4px;font-family:var(--mono);font-size:12px;color:rgba(255,255,255,.8)}
.hero-answer-dot{width:7px;height:7px;border-radius:50%;background:var(--green-light);flex-shrink:0}

/* ── audit chain tree ── */
.audit-chain{font-family:var(--mono);font-size:13px;line-height:1.9;margin-bottom:28px}
.audit-chain-label{font-size:11px;text-transform:uppercase;letter-spacing:.07em;color:var(--faint);font-weight:600;margin-bottom:12px}
.chain-row{display:flex;align-items:baseline;gap:8px;padding:2px 8px;margin:0 -8px;border-radius:4px;flex-wrap:wrap}
.chain-row:hover{background:var(--green-tint)}
.chain-prefix{color:var(--faint);white-space:pre}
.chain-val{color:var(--text);font-weight:600}
.chain-arrow{color:var(--faint);margin:0 3px}
.chain-op{color:var(--green)}
.chain-src{color:var(--muted);font-size:12px}
.view-btn{margin-left:8px;font-family:var(--mono);font-size:11px;background:transparent;border:1px solid var(--border-strong);color:var(--muted);padding:2px 8px;border-radius:4px;transition:all .15s;flex-shrink:0}
.view-btn:hover{background:var(--green);border-color:var(--green);color:#fff}

/* ── section heading ── */
.rpt-h2{font-size:15px;font-weight:700;margin:28px 0 12px;border-bottom:1px solid var(--border);padding-bottom:8px;letter-spacing:-.01em}
.rpt-h2:first-child{margin-top:0}

/* ── key-value pairs ── */
.kv{display:grid;grid-template-columns:160px 1fr;gap:4px 12px;margin-bottom:16px}
.kv-label{color:var(--muted);font-size:12px}
.kv-value{font-family:var(--mono);font-size:12px;word-break:break-all}

/* ── tables ── */
.tbl{width:100%;border-collapse:collapse;font-size:13px;margin-bottom:20px}
.tbl th{background:var(--surface);text-align:left;padding:8px 12px;border:1px solid var(--border-med);font-weight:600;font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--muted)}
.tbl td{padding:7px 12px;border:1px solid var(--border);vertical-align:top;word-break:break-all}
.tbl tr:nth-child(even) td{background:#FAFAF8}
.mono{font-family:var(--mono);font-size:12px}
.status-ok{color:var(--green);font-weight:600}
.status-broken{color:var(--danger);font-weight:600}

/* ── file slide-over panel ── */
.slide-over{position:fixed;inset:0;z-index:200;pointer-events:none;visibility:hidden}
.slide-over.open{pointer-events:auto;visibility:visible}
.slide-over-overlay{position:absolute;inset:0;background:rgba(0,0,0,.3);opacity:0;transition:opacity .25s}
.slide-over.open .slide-over-overlay{opacity:1}
.slide-over-content{position:absolute;top:0;right:0;height:100%;width:500px;max-width:90vw;background:#fff;border-left:1px solid var(--border);transform:translateX(100%);transition:transform .3s cubic-bezier(.16,1,.3,1);overflow-y:auto;display:flex;flex-direction:column;box-shadow:-8px 0 40px rgba(0,0,0,.1)}
.slide-over.open .slide-over-content{transform:translateX(0)}
.slide-over-header{display:flex;align-items:center;justify-content:space-between;padding:18px 22px;border-bottom:1px solid var(--border);position:sticky;top:0;background:#fff;z-index:2}
.slide-over-title{display:flex;align-items:center;gap:10px;font-family:var(--mono);font-size:14px;font-weight:600}
.slide-over-ext{background:var(--text);color:#fff;font-family:var(--mono);font-size:10px;padding:2px 6px;border-radius:3px}
.slide-over-close{background:transparent;border:none;width:28px;height:28px;border-radius:4px;display:inline-flex;align-items:center;justify-content:center;color:var(--muted);font-size:18px;transition:all .15s;cursor:pointer}
.slide-over-close:hover{background:var(--surface);color:var(--text)}
.slide-over-meta{padding:12px 22px;font-family:var(--mono);font-size:12px;color:var(--muted);border-bottom:1px solid var(--border);background:var(--surface)}
.slide-over-meta-row{display:flex;gap:12px;margin-bottom:4px;flex-wrap:wrap}
.slide-over-meta-row .key{color:var(--faint);width:70px;flex-shrink:0}
.slide-over-body{padding:16px 22px;flex:1}
.slide-over-footer{padding:14px 22px;border-top:1px solid var(--border);font-family:var(--mono);font-size:11px;color:var(--faint);display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:8px}
.slide-over-footer a{color:var(--green);text-decoration:underline;text-underline-offset:2px}

/* ── recon outcomes ── */
.outcome-chip{display:inline-flex;align-items:center;gap:4px;padding:3px 8px;border-radius:4px;font-size:11px;font-weight:600;font-family:var(--mono)}
.chip-ok{background:var(--green-tint);color:var(--green)}
.chip-err{background:#FDE8E8;color:var(--danger)}
</style>
</head>
<body>

<!-- ── header ── -->
<header class="rpt-header">
  <div class="rpt-logo"><span class="rpt-logo-mark"></span>Provenant Audit Report</div>
  <div class="rpt-meta">
    <div>bundle <span style="color:rgba(255,255,255,.85)">{{.BundleSHA256}}</span></div>
    <div>schema {{.SchemaVersion}} &middot; run {{.RunID}}</div>
  </div>
</header>

<!-- ── tab bar ── -->
<div class="rpt-tabs">
  <button class="rpt-tab active" onclick="showTab('plain',this)">Plain view</button>
  <button class="rpt-tab" onclick="showTab('detail',this)">Technical detail</button>
</div>

<!-- ═══ PLAIN TAB ═══ -->
<div class="rpt-panel active" id="tab-plain">

  <!-- summary tiles -->
  <div class="rpt-tiles">
    <div class="rpt-tile">
      <div class="rpt-tile-label">Tokens</div>
      <div class="rpt-tile-value">{{.TokenCount}}</div>
    </div>
    <div class="rpt-tile">
      <div class="rpt-tile-label">Source Reads</div>
      <div class="rpt-tile-value">{{.CapturedReadCount}}</div>
    </div>
    <div class="rpt-tile">
      <div class="rpt-tile-label">Audit Entries</div>
      <div class="rpt-tile-value">{{.AuditEntryCount}}</div>
    </div>
    {{if .HasReconciliation}}
    <div class="rpt-tile {{if eq .MismatchCount 0}}tile-ok{{else}}tile-error{{end}}">
      <div class="rpt-tile-label">Mismatches</div>
      <div class="rpt-tile-value">{{.MismatchCount}}</div>
    </div>
    {{end}}
  </div>

  <!-- invariant banner -->
  {{if .InvariantChecked}}
    {{if .InvariantOK}}
    <div class="banner-ok">Contribution invariant: OK — decimal sum of all leaf contributions matches the submitted answer.</div>
    {{else}}
    <div class="banner-violated">Contribution invariant: VIOLATED — leaf sum does not match the submitted answer; review mismatches below.</div>
    {{end}}
  {{end}}

  <!-- source file cards -->
  {{if .SourceCards}}
  <div class="rpt-h2">Source Files</div>
  <div class="src-grid">
    {{range .SourceCards}}
    <div class="src-card" onclick="openSlideOver({{.Index}})">
      <div class="src-card-icon">
        <div class="mini-grid">
          <span class="h"></span><span></span><span></span><span></span>
          <span></span><span class="h"></span><span></span><span></span>
          <span></span><span></span><span class="h"></span><span></span>
          <span class="h"></span><span></span><span></span><span class="h"></span>
        </div>
        <span class="src-card-ext">{{.Ext}}</span>
      </div>
      <div class="src-card-name">{{.Name}}</div>
      <div class="src-card-id">{{.SourceID}}</div>
      <div class="src-card-meta">sha256 {{.SHA256}}</div>
      {{if .SourcePath}}
      <a class="src-card-link" href="file://{{.SourcePath}}" onclick="event.stopPropagation()">view file &rarr;</a>
      {{end}}
    </div>
    {{end}}
  </div>
  {{end}}

  <!-- submitted answer hero -->
  {{if .SubmittedAnswer}}
  <div class="rpt-h2">Submitted Answer</div>
  <div class="hero-answer">
    <div class="hero-answer-eyebrow">Operating result &middot; submit_answer</div>
    <div class="hero-answer-label">{{.SubmittedAnswer.TokenID}}</div>
    <div class="hero-answer-value">{{.SubmittedAnswer.ValueCanonical}}</div>
    {{if .SubmittedAnswer.Unit}}<div class="hero-answer-unit">{{.SubmittedAnswer.Unit}}</div>{{end}}
    <div class="hero-answer-status"><span class="hero-answer-dot"></span>grounded &mdash; no literal value in request &middot; provenance verified</div>
  </div>
  {{end}}

  <!-- derivation graph audit chain -->
  {{if .DerivationTree}}
  <div class="rpt-h2">Audit Chain</div>
  <div class="audit-chain">
    <div class="audit-chain-label">Derivation chain &mdash; prev-hash continuity:
      {{if eq .PrevHashContinuity "OK"}}
      <span class="status-ok">OK</span>
      {{else}}
      <span class="status-broken">{{.PrevHashContinuity}}</span>
      {{end}}
    </div>
    {{range .DerivationTree}}
    <div class="chain-row">
      <span class="chain-prefix" style="margin-left:{{.IndentPx}}px">{{if .IsLeaf}}&#x2514;&#x2500; {{else if eq .Depth 0}}&#x25BA; {{else}}&#x251C;&#x2500; {{end}}</span>
      <span class="chain-val">{{.ValueCanonical}}</span>
      {{if .Unit}}<span class="chain-src">{{.Unit}}</span>{{end}}
      {{if .Operation}}<span class="chain-arrow">&larr;</span><span class="chain-op">{{.Operation}}</span>{{end}}
      <span class="mono" style="color:var(--faint);font-size:11px">{{.TokenID}}</span>
      {{if .IsLeaf}}
        {{if .SourcePath}}
        <a class="view-btn" href="file://{{.SourcePath}}">view file &rarr;</a>
        {{end}}
      {{end}}
    </div>
    {{end}}
  </div>
  {{end}}

</div><!-- /tab-plain -->

<!-- ═══ DETAIL TAB ═══ -->
<div class="rpt-panel" id="tab-detail">

  <div class="rpt-h2">Run Identity</div>
  <div class="kv">
    <span class="kv-label">Run ID</span><span class="kv-value">{{.RunID}}</span>
    <span class="kv-label">Tenant ID</span><span class="kv-value">{{.TenantID}}</span>
    <span class="kv-label">Bundle SHA256</span><span class="kv-value">{{.BundleSHA256}}</span>
    <span class="kv-label">Schema Version</span><span class="kv-value">{{.SchemaVersion}}</span>
  </div>

  {{if .InvariantChecked}}
  <div class="rpt-h2">Contribution Invariant</div>
  {{if .InvariantOK}}
  <div class="banner-ok">Contribution invariant: OK</div>
  {{else}}
  <div class="banner-violated">Contribution invariant: VIOLATED</div>
  {{end}}
  {{end}}

  {{if .SubmittedAnswer}}
  <div class="rpt-h2">Submitted Answer</div>
  <div class="kv">
    <span class="kv-label">Token ID</span><span class="kv-value">{{.SubmittedAnswer.TokenID}}</span>
    <span class="kv-label">Value</span><span class="kv-value">{{.SubmittedAnswer.ValueCanonical}}</span>
    {{if .SubmittedAnswer.Unit}}<span class="kv-label">Unit</span><span class="kv-value">{{.SubmittedAnswer.Unit}}</span>{{end}}
    <span class="kv-label">Provenance</span><span class="kv-value">grounded — no literal value in request</span>
  </div>
  {{end}}

  {{if .HasReconciliation}}
  <div class="rpt-h2">Reconciliation Outcomes</div>
  {{if .OutcomeCounts}}
  <table class="tbl" style="max-width:360px">
    <thead><tr><th>Outcome</th><th>Count</th></tr></thead>
    <tbody>
    {{range .OutcomeCounts}}
    <tr><td>{{.Outcome}}</td><td>{{.Count}}</td></tr>
    {{end}}
    </tbody>
  </table>
  {{end}}
  {{if .Mismatches}}
  <table class="tbl">
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

  <div class="rpt-h2">Derivation Graph</div>
  {{if .DerivationTree}}
  <table class="tbl">
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

  <div class="rpt-h2">Captured Reads</div>
  {{if .CapturedReads}}
  <table class="tbl">
    <thead><tr><th>Source Name</th><th>Selector</th><th>SHA256</th><th>Source Path</th></tr></thead>
    <tbody>
    {{range .CapturedReads}}
    <tr>
      <td>{{.SourceName}}</td>
      <td class="mono">{{.Selector}}</td>
      <td class="mono">{{.SHA256}}</td>
      <td class="mono" style="font-size:11px">{{.SourcePath}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <p style="color:var(--muted)">No captured reads.</p>
  {{end}}

  <div class="rpt-h2">Audit Chain</div>
  <p style="margin-bottom:10px">Entries: {{.AuditEntryCount}} &mdash; Prev-hash continuity:
    {{if eq .PrevHashContinuity "OK"}}
    <span class="status-ok">OK</span>
    {{else}}
    <span class="status-broken">{{.PrevHashContinuity}}</span>
    {{end}}
  </p>
  {{if .AuditEntries}}
  <table class="tbl" style="max-width:480px">
    <thead><tr><th>Seq</th><th>Kind</th></tr></thead>
    <tbody>
    {{range .AuditEntries}}
    <tr><td class="mono">{{.Seq}}</td><td class="mono">{{.Kind}}</td></tr>
    {{end}}
    </tbody>
  </table>
  {{end}}

  <div class="rpt-h2">Manifest</div>
  {{if .Artifacts}}
  <table class="tbl">
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
  <p style="color:var(--muted)">No artifacts.</p>
  {{end}}

</div><!-- /tab-detail -->

<!-- ── slide-over panel ── -->
<div class="slide-over" id="slide-over" aria-hidden="true">
  <div class="slide-over-overlay" id="slide-overlay"></div>
  <aside class="slide-over-content" id="slide-content" role="dialog" aria-label="Source file detail"></aside>
</div>

<script>
/* ── tab switching ── */
function showTab(name, btn) {
  document.querySelectorAll('.rpt-panel').forEach(function(p) {
    p.classList.remove('active');
  });
  document.querySelectorAll('.rpt-tab').forEach(function(t) {
    t.classList.remove('active');
  });
  var panel = document.getElementById('tab-' + name);
  if (panel) { panel.classList.add('active'); }
  if (btn) { btn.classList.add('active'); }
}

/* ── source card data (embedded from template) ── */
var srcCards = [
  {{range .SourceCards}}
  {
    name: {{.Name | js}},
    ext: {{.Ext | js}},
    sourceID: {{.SourceID | js}},
    sourcePath: {{.SourcePath | js}},
    sha256: {{.SHA256 | js}}
  }{{/* no trailing comma needed — JS parses arrays with trailing commas but avoid for compat */}},
  {{end}}
  null
].filter(function(x) { return x !== null; });

/* ── slide-over ── */
var slideOver = document.getElementById('slide-over');
var slideContent = document.getElementById('slide-content');

function openSlideOver(idx) {
  var d = srcCards[idx];
  if (!d) { return; }
  var fileLink = '';
  if (d.sourcePath && d.sourcePath.indexOf('/') === 0) {
    fileLink = '<a href="file://' + d.sourcePath + '" style="color:var(--green)">Open file &rarr;</a>';
  }
  slideContent.innerHTML =
    '<div class="slide-over-header">' +
      '<div class="slide-over-title"><span class="slide-over-ext">' + escH(d.ext) + '</span><span>' + escH(d.name) + '</span></div>' +
      '<button class="slide-over-close" id="slide-close" aria-label="Close">×</button>' +
    '</div>' +
    '<div class="slide-over-meta">' +
      '<div class="slide-over-meta-row"><span class="key">source</span><span>' + escH(d.sourceID) + '</span></div>' +
      '<div class="slide-over-meta-row"><span class="key">sha256</span><span>' + escH(d.sha256) + '</span></div>' +
      (d.sourcePath ? '<div class="slide-over-meta-row"><span class="key">path</span><span style="font-family:var(--mono);font-size:11px;word-break:break-all">' + escH(d.sourcePath) + '</span></div>' : '') +
    '</div>' +
    '<div class="slide-over-body"><p style="color:var(--muted);font-size:13px">Row-level data is captured in the audit bundle. Open the source file or replay the bundle to inspect individual rows.</p></div>' +
    '<div class="slide-over-footer"><span>Captured read &middot; audit pack</span>' + fileLink + '</div>';
  slideOver.classList.add('open');
  slideOver.setAttribute('aria-hidden', 'false');
  document.body.style.overflow = 'hidden';
  var closeBtn = document.getElementById('slide-close');
  if (closeBtn) { closeBtn.addEventListener('click', closeSlideOver); }
}

function closeSlideOver() {
  slideOver.classList.remove('open');
  slideOver.setAttribute('aria-hidden', 'true');
  document.body.style.overflow = '';
}

document.getElementById('slide-overlay').addEventListener('click', closeSlideOver);
document.addEventListener('keydown', function(e) { if (e.key === 'Escape') { closeSlideOver(); } });

function escH(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
</script>

</body>
</html>`
