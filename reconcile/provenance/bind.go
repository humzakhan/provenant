package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/memio"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"
)

type Options struct{}

type ProvenancedResultSet struct {
	Results       engine.ResultSet    `json:"results"`
	CapturedReads []CapturedReadRef   `json:"captured_reads"`
	Tokens        []kernel.Token      `json:"tokens,omitempty"`
	AuditEntries  []kernel.AuditEntry `json:"audit_entries,omitempty"`
	Bundle        *BundleRef          `json:"bundle,omitempty"`
	Evidence      []ResultEvidence    `json:"evidence"`
}

type BundleRef struct {
	Bytes  []byte `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type CapturedReadRef struct {
	FileID         string                `json:"file_id"`
	CapturedReadID string                `json:"captured_read_id"`
	SourcePath     string                `json:"source_path"`
	SourceHash     string                `json:"source_hash"`
	Selector       kernel.SourceSelector `json:"selector"`
	Reader         string                `json:"reader"`
	ReaderVersion  string                `json:"reader_version"`
}

type ResultEvidence struct {
	ResultID  string             `json:"result_id"`
	RowRefs   []RowEvidenceRef   `json:"row_refs,omitempty"`
	ValueRefs []ValueEvidenceRef `json:"value_refs,omitempty"`
}

type RowEvidenceRef struct {
	Side           string             `json:"side"`
	FileID         string             `json:"file_id"`
	RowKey         string             `json:"row_key"`
	Position       router.RowPosition `json:"position"`
	CapturedReadID string             `json:"captured_read_id"`
	SourceHash     string             `json:"source_hash"`
}

type ValueEvidenceRef struct {
	Side           string `json:"side"`
	FileID         string `json:"file_id"`
	RowKey         string `json:"row_key"`
	LogicalField   string `json:"logical_field"`
	SourceField    string `json:"source_field"`
	Role           string `json:"role"`
	Raw            string `json:"raw,omitempty"`
	RawExact       bool   `json:"raw_exact,omitempty"`
	Canonical      string `json:"canonical"`
	CapturedReadID string `json:"captured_read_id"`
	SourceHash     string `json:"source_hash"`
	Selector       string `json:"selector"`
	OutputPath     string `json:"output_path,omitempty"`
	TokenID        string `json:"token_id,omitempty"`
}

func Bind(ctx context.Context, req request.Request, files []router.NormalizedFile, results *engine.ResultSet, opts Options) (*ProvenancedResultSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		return nil, fmt.Errorf("provenance_bind_failed: missing result set")
	}

	scope, err := newBindingScope(req, *results)
	if err != nil {
		return nil, err
	}
	index, captured, err := newFileIndex(files, scope.files)
	if err != nil {
		return nil, err
	}
	bound := &ProvenancedResultSet{
		Results:       copyResultSet(*results),
		CapturedReads: captured,
		Evidence:      make([]ResultEvidence, 0, len(results.Results)),
	}

	fields := fieldsForResults(req)
	for _, result := range results.Results {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		evidence, err := bindResult(result, fields, index, scope)
		if err != nil {
			return nil, err
		}
		bound.Evidence = append(bound.Evidence, evidence)
	}
	if err := mintAmountEvidence(ctx, req, bound, index); err != nil {
		return nil, err
	}
	return bound, nil
}

func mintAmountEvidence(ctx context.Context, req request.Request, bound *ProvenancedResultSet, idx fileIndex) error {
	if bound == nil {
		return nil
	}

	run := kernel.Run{ID: req.Run.ID, TenantID: req.Run.TenantID, Status: kernel.RunRunning}
	binding := memio.NewBinding()
	clock := fixedClock{at: time.Unix(0, 0).UTC()}
	minted := map[string]kernel.MintFromCapturedReadResult{}
	mintedReads := map[string]kernel.CapturedRead{}

	for evidenceIndex := range bound.Evidence {
		for valueIndex := range bound.Evidence[evidenceIndex].ValueRefs {
			ref := &bound.Evidence[evidenceIndex].ValueRefs[valueIndex]
			if ref.Role != request.RoleAmount {
				continue
			}

			read, outputPath, err := capturedReadOutputPathForValueRef(*ref, idx)
			if err != nil {
				return err
			}
			ref.OutputPath = outputPath

			key := read.ID + "\x00" + outputPath
			mintedResult, exists := minted[key]
			if !exists {
				result, err := kernel.MintFromCapturedRead(
					ctx,
					binding,
					binding,
					binding,
					clock,
					run,
					kernel.MintFromCapturedReadRequest{
						CapturedRead: read,
						Source:       kernel.Source{Path: read.SourcePath, SHA256: read.SourceSHA256},
						OutputPath:   outputPath,
					},
				)
				if err != nil {
					return fmt.Errorf("provenance_bind_failed: mint amount token for file %q row %q field %q: %w", ref.FileID, ref.RowKey, ref.LogicalField, err)
				}
				minted[key] = result
				mintedReads[read.ID] = read
				mintedResult = result
			}
			ref.TokenID = mintedResult.Token.ID
		}
	}

	if len(minted) == 0 {
		return nil
	}

	tokens, audits, err := mintedState(ctx, binding, run)
	if err != nil {
		return err
	}
	reads := make([]kernel.CapturedRead, 0, len(mintedReads))
	for _, read := range mintedReads {
		reads = append(reads, cloneCapturedRead(read))
	}
	sort.Slice(reads, func(i, j int) bool { return reads[i].ID < reads[j].ID })

	bundleBytes, err := bundle.Emit(bundle.State{Run: run, CapturedReads: reads, Tokens: tokens, AuditEntries: audits})
	if err != nil {
		return fmt.Errorf("provenance_bind_failed: emit bundle: %w", err)
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		return fmt.Errorf("provenance_bind_failed: verify bundle: %w", err)
	}

	bound.Tokens = tokens
	bound.AuditEntries = audits
	bound.Bundle = &BundleRef{Bytes: bundleBytes, SHA256: sha256Hex(bundleBytes)}
	return nil
}

func capturedReadOutputPathForValueRef(ref ValueEvidenceRef, idx fileIndex) (kernel.CapturedRead, string, error) {
	file, ok := idx.files[ref.FileID]
	if !ok {
		return kernel.CapturedRead{}, "", fmt.Errorf("provenance_bind_failed: missing file provenance for %q", ref.FileID)
	}
	read := file.file.CapturedRead
	if read.ID != ref.CapturedReadID {
		return kernel.CapturedRead{}, "", fmt.Errorf("provenance_bind_failed: captured read mismatch for file %q", ref.FileID)
	}
	if capturedReadValueMatchesRef(read.Output, "$.value", ref) {
		return cloneCapturedRead(read), "$.value", nil
	}
	row, ok := file.rows[ref.RowKey]
	if !ok {
		return kernel.CapturedRead{}, "", fmt.Errorf("provenance_bind_failed: file %q missing row %q provenance", ref.FileID, ref.RowKey)
	}
	outputPath, ok := outputPathForCSVOrSpreadsheetValue(read.Output, row, ref)
	if !ok {
		return kernel.CapturedRead{}, "", fmt.Errorf("provenance_bind_failed: cannot map captured read output path for file %q row %q field %q", ref.FileID, ref.RowKey, ref.LogicalField)
	}
	if !capturedReadValueMatchesRef(read.Output, outputPath, ref) {
		return kernel.CapturedRead{}, "", fmt.Errorf("provenance_bind_failed: captured read value mismatch for file %q row %q field %q", ref.FileID, ref.RowKey, ref.LogicalField)
	}
	return cloneCapturedRead(read), outputPath, nil
}

func capturedReadValueMatchesRef(output json.RawMessage, outputPath string, ref ValueEvidenceRef) bool {
	value, ok := capturedOutputValue(output, outputPath)
	if !ok {
		return false
	}
	return value == ref.Canonical || value == ref.Raw
}

func capturedOutputValue(output json.RawMessage, outputPath string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(string(output)))
	dec.UseNumber()
	var payload any
	if err := dec.Decode(&payload); err != nil {
		return "", false
	}
	if outputPath == "$.value" {
		object, ok := payload.(map[string]any)
		if !ok {
			return "", false
		}
		payload, ok = object["value"]
		if !ok {
			return "", false
		}
	} else {
		var ok bool
		payload, ok = jsonPointerValue(payload, outputPath)
		if !ok {
			return "", false
		}
	}
	switch typed := payload.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func outputPathForCSVOrSpreadsheetValue(output json.RawMessage, row router.NormalizedRow, ref ValueEvidenceRef) (string, bool) {
	var payload struct {
		Rows []struct {
			Row         int `json:"row"`
			WindowIndex int `json:"window_index"`
			Cells       []struct {
				Column int    `json:"column"`
				Header string `json:"header"`
				A1     string `json:"a1"`
				Value  any    `json:"value"`
			} `json:"cells"`
		} `json:"rows"`
	}
	dec := json.NewDecoder(strings.NewReader(string(output)))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return "", false
	}
	for rowIndex, outputRow := range payload.Rows {
		if row.Position.Row != 0 && outputRow.Row != row.Position.Row {
			continue
		}
		if row.Position.Row == 0 && outputRow.WindowIndex != row.Position.Index {
			continue
		}
		for cellIndex, cell := range outputRow.Cells {
			if !cellMatchesRef(cell.Header, cell.A1, cell.Column, ref.SourceField) {
				continue
			}
			path := fmt.Sprintf("/rows/%d/cells/%d/value", rowIndex, cellIndex)
			if value, ok := capturedValueString(cell.Value); ok && (value == ref.Canonical || value == ref.Raw) {
				return path, true
			}
		}
	}
	return "", false
}

func cellMatchesRef(header, a1 string, column int, source string) bool {
	if header != "" && header == source {
		return true
	}
	if a1 != "" && strings.TrimRight(a1, "0123456789") == source {
		return true
	}
	parsed, err := strconv.Atoi(source)
	return err == nil && parsed == column
}

func capturedValueString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func jsonPointerValue(value any, pointer string) (any, bool) {
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	current := value
	for _, token := range strings.Split(pointer[1:], "/") {
		decoded := strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[decoded]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(decoded)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func mintedState(ctx context.Context, binding *memio.Binding, run kernel.Run) ([]kernel.Token, []kernel.AuditEntry, error) {
	var tokens []kernel.Token
	var audits []kernel.AuditEntry
	if err := binding.WithTx(ctx, func(ctx context.Context, tx kernel.Tx) error {
		var err error
		tokens, err = binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		audits, err = binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		return err
	}); err != nil {
		return nil, nil, fmt.Errorf("provenance_bind_failed: collect minted state: %w", err)
	}
	return tokens, audits, nil
}

type fixedClock struct {
	at time.Time
}

func (c fixedClock) Now() time.Time {
	return c.at
}

func cloneCapturedRead(read kernel.CapturedRead) kernel.CapturedRead {
	cloned := read
	cloned.ReaderOptions = append([]byte(nil), read.ReaderOptions...)
	cloned.Locator = append([]byte(nil), read.Locator...)
	cloned.Output = append([]byte(nil), read.Output...)
	return cloned
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type fileIndex struct {
	files map[string]fileEvidence
}

type bindingScope struct {
	left  string
	right string
	files map[string]struct{}
}

type fileEvidence struct {
	file router.NormalizedFile
	rows map[string]router.NormalizedRow
}

func newBindingScope(req request.Request, results engine.ResultSet) (bindingScope, error) {
	scope := bindingScope{
		left:  req.Reconcile.Left,
		right: req.Reconcile.Right,
		files: map[string]struct{}{},
	}
	for _, result := range results.Results {
		if result.Left != nil {
			if err := scope.addRef("left", *result.Left); err != nil {
				return bindingScope{}, err
			}
		}
		if result.Right != nil {
			if err := scope.addRef("right", *result.Right); err != nil {
				return bindingScope{}, err
			}
		}
		for _, ref := range result.GroupRowKeys {
			if _, err := scope.groupSide(ref); err != nil {
				return bindingScope{}, err
			}
			scope.files[ref.FileID] = struct{}{}
		}
	}
	return scope, nil
}

func (s bindingScope) addRef(side string, ref engine.RowRef) error {
	expected := s.left
	if side == "right" {
		expected = s.right
	}
	if ref.FileID != expected {
		return fmt.Errorf("provenance_bind_failed: %s row ref file %q outside reconcile scope %q", side, ref.FileID, expected)
	}
	s.files[ref.FileID] = struct{}{}
	return nil
}

func (s bindingScope) groupSide(ref engine.RowRef) (string, error) {
	if ref.FileID == s.left {
		return "left_group", nil
	}
	if ref.FileID == s.right {
		return "right_group", nil
	}
	return "", fmt.Errorf("provenance_bind_failed: group row ref file %q outside reconcile scope %q/%q", ref.FileID, s.left, s.right)
}

func newFileIndex(files []router.NormalizedFile, include map[string]struct{}) (fileIndex, []CapturedReadRef, error) {
	idx := fileIndex{files: make(map[string]fileEvidence, len(include))}
	captured := make([]CapturedReadRef, 0, len(include))
	for _, file := range files {
		if _, ok := include[file.FileID]; !ok {
			continue
		}
		if strings.TrimSpace(file.FileID) == "" {
			return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: missing file_id")
		}
		if _, exists := idx.files[file.FileID]; exists {
			return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: duplicate file_id %q", file.FileID)
		}
		if strings.TrimSpace(file.CapturedRead.ID) == "" {
			return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: file %q missing captured_read_id", file.FileID)
		}
		if strings.TrimSpace(file.SourceHash) == "" || strings.TrimSpace(file.CapturedRead.SourceSHA256) == "" || file.SourceHash != file.CapturedRead.SourceSHA256 {
			return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: file %q missing source hash provenance", file.FileID)
		}
		if strings.TrimSpace(file.CapturedRead.SourcePath) == "" {
			return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: file %q missing source path provenance", file.FileID)
		}

		rows := make(map[string]router.NormalizedRow, len(file.Rows))
		for _, row := range file.Rows {
			if strings.TrimSpace(row.RowKey) == "" {
				return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: file %q missing row key provenance", file.FileID)
			}
			if _, exists := rows[row.RowKey]; exists {
				return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: file %q duplicate row key %q", file.FileID, row.RowKey)
			}
			rows[row.RowKey] = row
		}

		idx.files[file.FileID] = fileEvidence{file: file, rows: rows}
		captured = append(captured, CapturedReadRef{
			FileID:         file.FileID,
			CapturedReadID: file.CapturedRead.ID,
			SourcePath:     file.CapturedRead.SourcePath,
			SourceHash:     file.CapturedRead.SourceSHA256,
			Selector:       file.CapturedRead.Selector,
			Reader:         file.CapturedRead.ReaderName,
			ReaderVersion:  file.CapturedRead.ReaderVersion,
		})
	}
	for fileID := range include {
		if _, ok := idx.files[fileID]; !ok {
			return fileIndex{}, nil, fmt.Errorf("provenance_bind_failed: missing file provenance for %q", fileID)
		}
	}
	sort.Slice(captured, func(i, j int) bool { return captured[i].FileID < captured[j].FileID })
	return idx, captured, nil
}

func bindResult(result engine.Result, fields resultFields, idx fileIndex, scope bindingScope) (ResultEvidence, error) {
	evidence := ResultEvidence{ResultID: result.ID}
	seenRows := map[string]struct{}{}
	seenValues := map[string]struct{}{}

	if result.Left != nil {
		if err := appendRowEvidence(&evidence, seenRows, "left", *result.Left, idx); err != nil {
			return ResultEvidence{}, err
		}
		if err := appendValueEvidence(&evidence, seenValues, "left", *result.Left, fields.left, idx); err != nil {
			return ResultEvidence{}, err
		}
	}
	if result.Right != nil {
		if err := appendRowEvidence(&evidence, seenRows, "right", *result.Right, idx); err != nil {
			return ResultEvidence{}, err
		}
		if err := appendValueEvidence(&evidence, seenValues, "right", *result.Right, fields.right, idx); err != nil {
			return ResultEvidence{}, err
		}
	}
	for _, ref := range result.GroupRowKeys {
		side, err := scope.groupSide(ref)
		if err != nil {
			return ResultEvidence{}, err
		}
		if err := appendRowEvidence(&evidence, seenRows, side, ref, idx); err != nil {
			return ResultEvidence{}, err
		}
		if err := appendValueEvidence(&evidence, seenValues, side, ref, fields.forSide(side), idx); err != nil {
			return ResultEvidence{}, err
		}
	}
	return evidence, nil
}

func appendRowEvidence(evidence *ResultEvidence, seen map[string]struct{}, side string, ref engine.RowRef, idx fileIndex) error {
	file, row, err := lookupRow(ref, idx)
	if err != nil {
		return err
	}
	key := side + "\x00" + ref.FileID + "\x00" + ref.RowKey
	if _, exists := seen[key]; exists {
		return nil
	}
	seen[key] = struct{}{}
	evidence.RowRefs = append(evidence.RowRefs, RowEvidenceRef{
		Side:           side,
		FileID:         ref.FileID,
		RowKey:         ref.RowKey,
		Position:       row.Position,
		CapturedReadID: file.file.CapturedRead.ID,
		SourceHash:     file.file.SourceHash,
	})
	return nil
}

func appendValueEvidence(evidence *ResultEvidence, seen map[string]struct{}, side string, ref engine.RowRef, fields []string, idx fileIndex) error {
	file, row, err := lookupRow(ref, idx)
	if err != nil {
		return err
	}
	for _, field := range fields {
		value, ok := row.Values[field]
		if !ok || strings.TrimSpace(value.LogicalField) == "" || value.LogicalField != field {
			return fmt.Errorf("provenance_bind_failed: file %q row %q missing logical field %q provenance", ref.FileID, ref.RowKey, field)
		}
		if strings.TrimSpace(value.Source) == "" {
			return fmt.Errorf("provenance_bind_failed: file %q row %q field %q missing source provenance", ref.FileID, ref.RowKey, field)
		}
		key := side + "\x00" + ref.FileID + "\x00" + ref.RowKey + "\x00" + field
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		evidence.ValueRefs = append(evidence.ValueRefs, ValueEvidenceRef{
			Side:           side,
			FileID:         ref.FileID,
			RowKey:         ref.RowKey,
			LogicalField:   field,
			SourceField:    value.Source,
			Role:           value.Role,
			Raw:            value.Raw,
			RawExact:       value.RawExact,
			Canonical:      value.Canonical,
			CapturedReadID: file.file.CapturedRead.ID,
			SourceHash:     file.file.SourceHash,
			Selector:       selectorPath(ref.RowKey, field),
		})
	}
	return nil
}

func lookupRow(ref engine.RowRef, idx fileIndex) (fileEvidence, router.NormalizedRow, error) {
	file, ok := idx.files[ref.FileID]
	if !ok {
		return fileEvidence{}, router.NormalizedRow{}, fmt.Errorf("provenance_bind_failed: missing file provenance for %q", ref.FileID)
	}
	row, ok := file.rows[ref.RowKey]
	if !ok {
		return fileEvidence{}, router.NormalizedRow{}, fmt.Errorf("provenance_bind_failed: file %q missing row %q provenance", ref.FileID, ref.RowKey)
	}
	return file, row, nil
}

type resultFields struct {
	left  []string
	right []string
}

func (f resultFields) forSide(side string) []string {
	if strings.HasPrefix(side, "left") {
		return f.left
	}
	return f.right
}

func fieldsForResults(req request.Request) resultFields {
	seen := map[string]struct{}{}
	add := func(fields []string, items []string) []string {
		for _, field := range items {
			if strings.TrimSpace(field) == "" {
				continue
			}
			if _, exists := seen[field]; exists {
				continue
			}
			seen[field] = struct{}{}
			fields = append(fields, field)
		}
		return fields
	}
	left := make([]string, 0, len(req.Reconcile.MatchOn)+len(req.Reconcile.Compare))
	right := make([]string, 0, len(req.Reconcile.MatchOn)+len(req.Reconcile.Compare))
	if req.Reconcile.Group != nil {
		left = add(left, req.Reconcile.Group.LeftKey)
		seen = map[string]struct{}{}
		right = add(right, req.Reconcile.Group.RightKey)
	} else {
		left = add(left, req.Reconcile.MatchOn)
		right = append([]string(nil), left...)
	}
	seen = make(map[string]struct{}, len(left))
	for _, field := range left {
		seen[field] = struct{}{}
	}
	left = add(left, req.Reconcile.Compare)
	seen = make(map[string]struct{}, len(right))
	for _, field := range right {
		seen[field] = struct{}{}
	}
	right = add(right, req.Reconcile.Compare)
	return resultFields{left: left, right: right}
}

func selectorPath(rowKey, field string) string {
	return "/rows/" + escapePath(rowKey) + "/values/" + escapePath(field)
}

func escapePath(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

func copyResultSet(in engine.ResultSet) engine.ResultSet {
	out := in
	out.MatchOn = append([]string(nil), in.MatchOn...)
	out.Compare = append([]string(nil), in.Compare...)
	out.Results = append([]engine.Result(nil), in.Results...)
	for i := range out.Results {
		out.Results[i].Key = append([]engine.KeyValue(nil), in.Results[i].Key...)
		out.Results[i].GroupRowKeys = append([]engine.RowRef(nil), in.Results[i].GroupRowKeys...)
		out.Results[i].Differences = append([]engine.Difference(nil), in.Results[i].Differences...)
		if in.Results[i].Left != nil {
			left := *in.Results[i].Left
			out.Results[i].Left = &left
		}
		if in.Results[i].Right != nil {
			right := *in.Results[i].Right
			out.Results[i].Right = &right
		}
	}
	return out
}
