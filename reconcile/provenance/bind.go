package provenance

import (
	"context"
	"fmt"
	"sort"
	"strings"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"
)

type Options struct{}

type ProvenancedResultSet struct {
	Results       engine.ResultSet  `json:"results"`
	CapturedReads []CapturedReadRef `json:"captured_reads"`
	Evidence      []ResultEvidence  `json:"evidence"`
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
}

func Bind(ctx context.Context, req request.Request, files []router.NormalizedFile, results *engine.ResultSet, opts Options) (*ProvenancedResultSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		return nil, fmt.Errorf("provenance_bind_failed: missing result set")
	}

	index, captured, err := newFileIndex(files)
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
		evidence, err := bindResult(result, fields, index)
		if err != nil {
			return nil, err
		}
		bound.Evidence = append(bound.Evidence, evidence)
	}
	return bound, nil
}

type fileIndex struct {
	files map[string]fileEvidence
}

type fileEvidence struct {
	file router.NormalizedFile
	rows map[string]router.NormalizedRow
}

func newFileIndex(files []router.NormalizedFile) (fileIndex, []CapturedReadRef, error) {
	idx := fileIndex{files: make(map[string]fileEvidence, len(files))}
	captured := make([]CapturedReadRef, 0, len(files))
	for _, file := range files {
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
	sort.Slice(captured, func(i, j int) bool { return captured[i].FileID < captured[j].FileID })
	return idx, captured, nil
}

func bindResult(result engine.Result, fields resultFields, idx fileIndex) (ResultEvidence, error) {
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
		side := groupSide(ref, result)
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

func groupSide(ref engine.RowRef, result engine.Result) string {
	if result.Left != nil && ref.FileID == result.Left.FileID {
		return "left_group"
	}
	if result.Right != nil && ref.FileID == result.Right.FileID {
		return "right_group"
	}
	return "group"
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
