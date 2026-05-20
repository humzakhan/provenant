package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"
	"github.com/shopspring/decimal"
)

type Outcome string

const (
	OutcomeMatched       Outcome = "matched"
	OutcomeMismatch      Outcome = "mismatch"
	OutcomeMissingLeft   Outcome = "missing_left"
	OutcomeMissingRight  Outcome = "missing_right"
	OutcomeDuplicateKey  Outcome = "duplicate_key"
	OutcomeInvalidRecord Outcome = "invalid_record"
)

type ResultSet struct {
	RunID       string   `json:"run_id"`
	Mode        string   `json:"mode"`
	LeftFileID  string   `json:"left_file_id"`
	RightFileID string   `json:"right_file_id"`
	MatchOn     []string `json:"match_on"`
	Compare     []string `json:"compare,omitempty"`
	Results     []Result `json:"results"`
}

type Result struct {
	ID          string       `json:"id"`
	Outcome     Outcome      `json:"outcome"`
	Key         []KeyValue   `json:"key,omitempty"`
	Left        *RowRef      `json:"left,omitempty"`
	Right       *RowRef      `json:"right,omitempty"`
	Differences []Difference `json:"differences,omitempty"`
	Reason      string       `json:"reason,omitempty"`
}

type KeyValue struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type RowRef struct {
	FileID   string             `json:"file_id"`
	RowKey   string             `json:"row_key"`
	Position router.RowPosition `json:"position"`
}

type Difference struct {
	Field string `json:"field"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

func Reconcile(ctx context.Context, req request.Request, files []router.NormalizedFile) (*ResultSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Reconcile.Mode != request.ModeExactMatch && req.Reconcile.Mode != request.ModeAmountTolerance && req.Reconcile.Mode != request.ModeExistence {
		return nil, fmt.Errorf("unsupported_reconcile_mode: %s", req.Reconcile.Mode)
	}
	plan, err := comparisonPlanFor(req)
	if err != nil {
		return nil, err
	}
	compare := append([]string(nil), req.Reconcile.Compare...)
	if req.Reconcile.Mode == request.ModeExistence {
		compare = nil
	}
	left, right, err := selectedFiles(req, files)
	if err != nil {
		return nil, err
	}

	set := &ResultSet{
		RunID:       req.Run.ID,
		Mode:        req.Reconcile.Mode,
		LeftFileID:  left.FileID,
		RightFileID: right.FileID,
		MatchOn:     append([]string(nil), req.Reconcile.MatchOn...),
		Compare:     compare,
	}

	leftRows, leftInvalid := indexRows(left, req.Reconcile.MatchOn, compare, plan)
	rightRows, rightInvalid := indexRows(right, req.Reconcile.MatchOn, compare, plan)
	set.Results = append(set.Results, leftInvalid...)
	set.Results = append(set.Results, rightInvalid...)

	duplicateKeys := map[string]struct{}{}
	set.Results = append(set.Results, duplicateResults(leftRows, &duplicateKeys, true)...)
	set.Results = append(set.Results, duplicateResults(rightRows, &duplicateKeys, false)...)

	keys := allKeys(leftRows, rightRows)
	for _, key := range keys {
		if _, duplicate := duplicateKeys[key]; duplicate {
			continue
		}
		leftGroup, leftOK := leftRows[key]
		rightGroup, rightOK := rightRows[key]
		switch {
		case !leftOK:
			set.Results = append(set.Results, resultFor(OutcomeMissingLeft, nil, &rightGroup[0], nil, ""))
		case !rightOK:
			set.Results = append(set.Results, resultFor(OutcomeMissingRight, &leftGroup[0], nil, nil, ""))
		case req.Reconcile.Mode == request.ModeExistence:
			set.Results = append(set.Results, resultFor(OutcomeMatched, &leftGroup[0], &rightGroup[0], nil, ""))
		default:
			diffs, reason := compareRows(leftGroup[0].row, rightGroup[0].row, req.Reconcile.Compare, plan)
			outcome := OutcomeMatched
			if len(diffs) > 0 {
				outcome = OutcomeMismatch
			}
			set.Results = append(set.Results, resultFor(outcome, &leftGroup[0], &rightGroup[0], diffs, reason))
		}
	}

	sort.Slice(set.Results, func(i, j int) bool { return resultSortKey(set.Results[i]) < resultSortKey(set.Results[j]) })
	for i := range set.Results {
		set.Results[i].ID = resultID(req.Run.ID, set.Results[i])
	}
	return set, nil
}

func selectedFiles(req request.Request, files []router.NormalizedFile) (router.NormalizedFile, router.NormalizedFile, error) {
	byID := make(map[string]router.NormalizedFile, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.FileID) == "" {
			return router.NormalizedFile{}, router.NormalizedFile{}, fmt.Errorf("invalid_normalized_file: missing file_id")
		}
		if _, exists := byID[file.FileID]; exists {
			return router.NormalizedFile{}, router.NormalizedFile{}, fmt.Errorf("invalid_normalized_file: duplicate file_id %q", file.FileID)
		}
		byID[file.FileID] = file
	}
	left, leftOK := byID[req.Reconcile.Left]
	right, rightOK := byID[req.Reconcile.Right]
	if !leftOK {
		return router.NormalizedFile{}, router.NormalizedFile{}, fmt.Errorf("invalid_normalized_file_reference: left file %q not found", req.Reconcile.Left)
	}
	if !rightOK {
		return router.NormalizedFile{}, router.NormalizedFile{}, fmt.Errorf("invalid_normalized_file_reference: right file %q not found", req.Reconcile.Right)
	}
	return left, right, nil
}

type indexedRow struct {
	fileID string
	row    router.NormalizedRow
	key    []KeyValue
	keyID  string
}

type comparisonPlan struct {
	mode      string
	amounts   map[string]struct{}
	currency  map[string]struct{}
	tolerance decimal.Decimal
}

func comparisonPlanFor(req request.Request) (comparisonPlan, error) {
	cfg := req.Reconcile
	plan := comparisonPlan{mode: cfg.Mode}
	if cfg.Mode != request.ModeAmountTolerance {
		return plan, nil
	}
	if cfg.Tolerance == nil {
		return plan, fmt.Errorf("invalid_amount_tolerance_config: missing tolerance")
	}
	tolerance, err := decimal.NewFromString(cfg.Tolerance.Amount)
	if err != nil {
		return plan, fmt.Errorf("invalid_amount_tolerance_config: invalid tolerance amount")
	}
	plan.tolerance = tolerance
	plan.amounts = make(map[string]struct{})
	plan.currency = make(map[string]struct{})
	columns := columnsForFile(req, cfg.Left)
	for _, field := range cfg.Compare {
		column, ok := columns[field]
		if !ok {
			continue
		}
		switch column.Role {
		case request.RoleAmount:
			plan.amounts[field] = struct{}{}
			if column.Numeric != nil && column.Numeric.CurrencyField != "" {
				plan.currency[column.Numeric.CurrencyField] = struct{}{}
			}
		case request.RoleCurrency:
			plan.currency[field] = struct{}{}
		}
	}
	return plan, nil
}

func columnsForFile(req request.Request, fileID string) request.Columns {
	for _, file := range req.Files {
		if file.ID == fileID {
			return file.Columns
		}
	}
	return nil
}

func indexRows(file router.NormalizedFile, matchOn, compare []string, plan comparisonPlan) (map[string][]indexedRow, []Result) {
	rows := make(map[string][]indexedRow, len(file.Rows))
	invalid := make([]Result, 0)
	for _, row := range file.Rows {
		key, keyID, reason, ok := rowKey(row, matchOn)
		if !ok {
			indexed := indexedRow{fileID: file.FileID, row: row, key: key, keyID: keyID}
			invalid = append(invalid, resultFor(OutcomeInvalidRecord, sideRow(file.FileID, indexed), nil, nil, reason))
			continue
		}
		if reason := validateCompareValues(row, compare, plan); reason != "" {
			indexed := indexedRow{fileID: file.FileID, row: row, key: key, keyID: keyID}
			invalid = append(invalid, resultFor(OutcomeInvalidRecord, sideRow(file.FileID, indexed), nil, nil, reason))
			continue
		}
		rows[keyID] = append(rows[keyID], indexedRow{fileID: file.FileID, row: row, key: key, keyID: keyID})
	}
	return rows, invalid
}

func rowKey(row router.NormalizedRow, fields []string) ([]KeyValue, string, string, bool) {
	key := make([]KeyValue, 0, len(fields))
	for _, field := range fields {
		value, ok := row.Values[field]
		if !ok || value.LogicalField == "" {
			return key, canonicalKey(key), fmt.Sprintf("missing logical field %q", field), false
		}
		if value.Empty {
			return key, canonicalKey(key), fmt.Sprintf("empty match field %q", field), false
		}
		key = append(key, KeyValue{Field: field, Value: value.Canonical})
	}
	return key, canonicalKey(key), "", true
}

func validateCompareValues(row router.NormalizedRow, fields []string, plan comparisonPlan) string {
	for _, field := range fields {
		value, ok := row.Values[field]
		if !ok || value.LogicalField == "" {
			return fmt.Sprintf("missing logical field %q", field)
		}
		if plan.mode != request.ModeAmountTolerance {
			continue
		}
		if _, ok := plan.amounts[field]; ok {
			if value.Empty || strings.TrimSpace(value.Canonical) == "" {
				return fmt.Sprintf("missing amount field %q", field)
			}
			if _, err := decimal.NewFromString(value.Canonical); err != nil {
				return fmt.Sprintf("malformed amount field %q", field)
			}
		}
		if _, ok := plan.currency[field]; ok {
			if value.Empty || strings.TrimSpace(value.Canonical) == "" {
				return fmt.Sprintf("missing currency field %q", field)
			}
		}
	}
	return ""
}

func duplicateResults(rows map[string][]indexedRow, duplicateKeys *map[string]struct{}, leftSide bool) []Result {
	results := make([]Result, 0)
	keys := sortedRowKeys(rows)
	for _, key := range keys {
		group := rows[key]
		if len(group) < 2 {
			continue
		}
		(*duplicateKeys)[key] = struct{}{}
		for _, row := range group {
			if leftSide {
				results = append(results, resultFor(OutcomeDuplicateKey, &row, nil, nil, "duplicate match key"))
				continue
			}
			results = append(results, resultFor(OutcomeDuplicateKey, nil, &row, nil, "duplicate match key"))
		}
	}
	return results
}

func allKeys(left, right map[string][]indexedRow) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		seen[key] = struct{}{}
	}
	for key := range right {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedRowKeys(rows map[string][]indexedRow) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func compareRows(left, right router.NormalizedRow, fields []string, plan comparisonPlan) ([]Difference, string) {
	diffs := make([]Difference, 0)
	reasons := make([]string, 0)
	for _, field := range fields {
		leftValue := left.Values[field].Canonical
		rightValue := right.Values[field].Canonical
		if plan.mode == request.ModeAmountTolerance {
			if _, ok := plan.amounts[field]; ok {
				leftAmount, _ := decimal.NewFromString(leftValue)
				rightAmount, _ := decimal.NewFromString(rightValue)
				delta := leftAmount.Sub(rightAmount).Abs()
				if delta.GreaterThan(plan.tolerance) {
					scale := decimalScale(leftValue, rightValue, plan.tolerance.String())
					deltaText := formatDecimal(delta, scale)
					diffs = append(diffs, Difference{Field: field, Left: leftValue, Right: rightValue + " (delta " + deltaText + ")"})
					reasons = append(reasons, fmt.Sprintf("%s delta %s exceeds tolerance %s", field, deltaText, formatDecimal(plan.tolerance, scale)))
				}
				continue
			}
		}
		if leftValue != rightValue {
			diffs = append(diffs, Difference{Field: field, Left: leftValue, Right: rightValue})
		}
	}
	return diffs, strings.Join(reasons, "; ")
}

func formatDecimal(value decimal.Decimal, scale int32) string {
	return value.StringFixed(scale)
}

func decimalScale(values ...string) int32 {
	var scale int32
	for _, value := range values {
		if dot := strings.IndexByte(value, '.'); dot >= 0 {
			places := int32(len(value) - dot - 1)
			if places > scale {
				scale = places
			}
		}
	}
	return scale
}

func resultFor(outcome Outcome, left, right *indexedRow, diffs []Difference, reason string) Result {
	result := Result{Outcome: outcome, Differences: diffs, Reason: reason}
	if left != nil {
		result.Left = &RowRef{FileID: left.fileID, RowKey: left.row.RowKey, Position: left.row.Position}
		result.Key = append([]KeyValue(nil), left.key...)
	}
	if right != nil {
		result.Right = &RowRef{FileID: right.fileID, RowKey: right.row.RowKey, Position: right.row.Position}
		if len(result.Key) == 0 {
			result.Key = append([]KeyValue(nil), right.key...)
		}
	}
	return result
}

func sideRow(fileID string, row indexedRow) *indexedRow {
	row.fileID = fileID
	return &row
}

func canonicalKey(key []KeyValue) string {
	raw, _ := json.Marshal(key)
	return string(raw)
}

func resultSortKey(result Result) string {
	left := ""
	if result.Left != nil {
		left = result.Left.FileID + ":" + result.Left.RowKey
	}
	right := ""
	if result.Right != nil {
		right = result.Right.FileID + ":" + result.Right.RowKey
	}
	return canonicalKey(result.Key) + "\x00" + string(result.Outcome) + "\x00" + left + "\x00" + right
}

func resultID(runID string, result Result) string {
	payload := struct {
		RunID   string     `json:"run_id"`
		Outcome Outcome    `json:"outcome"`
		Key     []KeyValue `json:"key,omitempty"`
		Left    *RowRef    `json:"left,omitempty"`
		Right   *RowRef    `json:"right,omitempty"`
	}{RunID: runID, Outcome: result.Outcome, Key: result.Key, Left: result.Left, Right: result.Right}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return "res_" + hex.EncodeToString(sum[:16])
}
