package extractspreadsheet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
)

const (
	readerName    = "extract.spreadsheet"
	readerVersion = "v1"
)

var (
	ErrMissingSourceBytes           = errors.New("extractspreadsheet: missing source bytes")
	ErrMissingSourceSHA256          = errors.New("extractspreadsheet: missing source sha256")
	ErrSourceHashMismatch           = errors.New("extractspreadsheet: source hash mismatch")
	ErrUnsupportedSelector          = errors.New("extractspreadsheet: unsupported selector metadata")
	ErrInvalidWorkbookFixture       = errors.New("extractspreadsheet: invalid workbook fixture")
	ErrInvalidRange                 = errors.New("extractspreadsheet: invalid range")
	ErrInvalidRowWindow             = errors.New("extractspreadsheet: invalid row window")
	ErrInvalidColumnProjection      = errors.New("extractspreadsheet: invalid column projection")
	ErrUnsupportedFormulaPolicy     = errors.New("extractspreadsheet: unsupported formula policy")
	ErrFormulaEvaluationUnsupported = errors.New("extractspreadsheet: non-deterministic formula evaluation")
	ErrSelectorNotFound             = errors.New("extractspreadsheet: selector not found")
	ErrAmbiguousSelector            = errors.New("extractspreadsheet: ambiguous selector resolution")
	ErrSelectorMismatch             = errors.New("extractspreadsheet: selector mismatch")
)

var a1Pattern = regexp.MustCompile(`^([A-Z]+)([1-9][0-9]*)$`)

type Reader struct{}

type workbook struct {
	sheets []*sheet
}

type sheet struct {
	name      string
	cells     map[cellKey]cell
	usedRange gridRange
	tables    []table
}

type table struct {
	name  string
	sheet string
	rng   gridRange
}

type cell struct {
	a1          string
	row         int
	column      int
	value       string
	formula     string
	cachedValue string
}

type cellKey struct {
	row    int
	column int
}

type gridRange struct {
	startRow int
	endRow   int
	startCol int
	endCol   int
}

type rowWindow struct {
	DeclaredStart  int
	DeclaredEnd    int
	EffectiveStart int
	EffectiveEnd   int
	PageSize       int
}

type resolvedSelection struct {
	sheet         *sheet
	rng           gridRange
	rangeText     string
	tableName     string
	selectorMode  string
	rangeMode     string
	formulaPolicy string
}

type selectedCell struct {
	cell
}

type outputEnvelope struct {
	Sheet             string        `json:"sheet"`
	Range             string        `json:"range"`
	TableName         string        `json:"table_name,omitempty"`
	FormulaPolicy     string        `json:"formula_policy,omitempty"`
	ColumnCount       int           `json:"column_count"`
	ColumnOrder       string        `json:"column_order"`
	ColumnProjection  string        `json:"column_projection,omitempty"`
	EffectiveRowEnd   int           `json:"effective_row_end"`
	EffectiveRowStart int           `json:"effective_row_start"`
	NormalizedValue   string        `json:"normalized_value,omitempty"`
	RawValue          string        `json:"raw_value,omitempty"`
	RowCount          int           `json:"row_count"`
	RowOrder          string        `json:"row_order"`
	RowWindow         string        `json:"row_window,omitempty"`
	Rows              []rowEnvelope `json:"rows"`
	SourceHash        string        `json:"source_hash"`
	Value             string        `json:"value,omitempty"`
}

type rowEnvelope struct {
	Sheet       string         `json:"sheet"`
	Row         int            `json:"row"`
	WindowIndex int            `json:"window_index"`
	Cells       []cellEnvelope `json:"cells"`
}

type cellEnvelope struct {
	Sheet              string `json:"sheet"`
	Row                int    `json:"row"`
	Column             int    `json:"column"`
	A1                 string `json:"a1"`
	RawValue           string `json:"raw_value"`
	NormalizedValue    string `json:"normalized_value"`
	Value              string `json:"value"`
	Formula            string `json:"formula,omitempty"`
	FormulaCachedValue string `json:"formula_cached_value,omitempty"`
}

type locatorEnvelope struct {
	Sheet             string       `json:"sheet"`
	Range             string       `json:"range"`
	TableName         string       `json:"table_name,omitempty"`
	ColumnCount       int          `json:"column_count"`
	ColumnOrder       string       `json:"column_order"`
	ColumnProjection  string       `json:"column_projection,omitempty"`
	EffectiveRowEnd   int          `json:"effective_row_end"`
	EffectiveRowStart int          `json:"effective_row_start"`
	RowCount          int          `json:"row_count"`
	RowOrder          string       `json:"row_order"`
	RowWindow         string       `json:"row_window,omitempty"`
	Rows              []locatorRow `json:"rows"`
}

type locatorRow struct {
	Sheet       string        `json:"sheet"`
	Row         int           `json:"row"`
	WindowIndex int           `json:"window_index"`
	Cells       []locatorCell `json:"cells"`
}

type locatorCell struct {
	Column int    `json:"column"`
	A1     string `json:"a1"`
}

type readerOptionsEnvelope struct {
	ColumnOrder    string `json:"column_order"`
	FormulaPolicy  string `json:"formula_policy"`
	ProjectionMode string `json:"projection_mode"`
	RangeMode      string `json:"range_mode"`
	RowOrder       string `json:"row_order"`
	SelectorMode   string `json:"selector_mode"`
}

func (Reader) Read(_ context.Context, run kernel.Run, source kernel.SourceBytes, selector kernel.SourceSelector) (kernel.ReadResult, error) {
	if len(source.Bytes) == 0 {
		return kernel.ReadResult{}, ErrMissingSourceBytes
	}
	if source.Source.SHA256 == "" {
		return kernel.ReadResult{}, ErrMissingSourceSHA256
	}
	if err := validateSelector(selector); err != nil {
		return kernel.ReadResult{}, err
	}

	sourceHash := sha256.Sum256(source.Bytes)
	sourceSHA256 := hex.EncodeToString(sourceHash[:])
	if sourceSHA256 != source.Source.SHA256 {
		return kernel.ReadResult{}, fmt.Errorf("%w: expected %s got %s", ErrSourceHashMismatch, source.Source.SHA256, sourceSHA256)
	}

	book, err := parseWorkbook(source.Bytes)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	selection, err := resolveSelection(book, selector)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	window, err := resolveRowWindow(selection.rng, selector.RowWindow)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	columns, err := resolveColumns(selection.rng, selector.ColumnProjection)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	rows, err := collectRows(selection, window, columns)
	if err != nil {
		return kernel.ReadResult{}, err
	}

	output, err := marshalOutput(source.Source.SHA256, selection, selector, window, rows, columns)
	if err != nil {
		return kernel.ReadResult{}, fmt.Errorf("extractspreadsheet: marshal output: %w", err)
	}
	outputHash := sha256.Sum256(output)
	outputSHA256 := hex.EncodeToString(outputHash[:])

	captured := kernel.CapturedRead{
		ID:            readID(run.ID, source.Source.Path, selector, source.Source.SHA256, outputSHA256),
		RunID:         run.ID,
		SourcePath:    source.Source.Path,
		Origin:        origin(source.Source.Path, selection, rows, columns),
		Selector:      selector,
		ReaderName:    readerName,
		ReaderVersion: readerVersion,
		ReaderOptions: readerOptions(selection, selector),
		Locator:       locator(selection, selector, window, rows, columns),
		SourceSHA256:  source.Source.SHA256,
		OutputSHA256:  outputSHA256,
		Output:        output,
	}

	return kernel.ReadResult{Output: output, CapturedRead: captured}, nil
}

func validateSelector(selector kernel.SourceSelector) error {
	if err := selector.ValidatePhaseTwoSelector(); err != nil {
		return ErrUnsupportedSelector
	}
	if selector.JSONPointer != "" ||
		selector.JSONPath != "" ||
		selector.ByteSpan != "" ||
		selector.LineWindow != "" ||
		selector.Cursor != "" ||
		selector.HeaderRow != 0 ||
		selector.Dialect != "" ||
		selector.Encoding != "" {
		return ErrUnsupportedSelector
	}
	if selector.FormulaPolicy != "" && selector.FormulaPolicy != "cached_only" {
		return fmt.Errorf("%w: %s", ErrUnsupportedFormulaPolicy, selector.FormulaPolicy)
	}
	return nil
}

func resolveSelection(book *workbook, selector kernel.SourceSelector) (resolvedSelection, error) {
	selection := resolvedSelection{
		formulaPolicy: "cached_only",
		selectorMode:  "sheet",
		rangeMode:     "used_range",
	}
	if selector.FormulaPolicy != "" {
		selection.formulaPolicy = selector.FormulaPolicy
	}

	var rangeSheetName string
	var declaredRange gridRange
	if selector.Range != "" {
		var err error
		declaredRange, rangeSheetName, err = parseRange(selector.Range)
		if err != nil {
			return resolvedSelection{}, err
		}
		selection.rangeMode = "explicit_range"
		if selector.Sheet == "" && selector.TableName == "" {
			selection.selectorMode = "range"
		}
	}

	var tableMatch *table
	if selector.TableName != "" {
		nameMatches := make([]table, 0, 2)
		filteredMatches := make([]table, 0, 2)
		for _, candidateSheet := range book.sheets {
			for _, candidate := range candidateSheet.tables {
				if candidate.name != selector.TableName {
					continue
				}
				nameMatches = append(nameMatches, candidate)
				if selector.Sheet != "" && candidate.sheet != selector.Sheet {
					continue
				}
				if rangeSheetName != "" && candidate.sheet != rangeSheetName {
					continue
				}
				if selector.Range != "" && candidate.rng != declaredRange {
					continue
				}
				filteredMatches = append(filteredMatches, candidate)
			}
		}
		switch len(filteredMatches) {
		case 0:
			if len(nameMatches) == 0 {
				return resolvedSelection{}, fmt.Errorf("%w: table_name=%s", ErrSelectorNotFound, selector.TableName)
			}
			return resolvedSelection{}, selectorConstraintMismatch(selector)
		case 1:
			tableMatch = &filteredMatches[0]
		default:
			return resolvedSelection{}, fmt.Errorf("%w: table_name=%s", ErrAmbiguousSelector, selector.TableName)
		}
		selection.tableName = tableMatch.name
		selection.selectorMode = "table_name"
		selection.rangeMode = "table_range"
	}

	var sheetMatch *sheet
	if selector.Sheet != "" {
		for _, candidate := range book.sheets {
			if candidate.name == selector.Sheet {
				sheetMatch = candidate
				break
			}
		}
		if sheetMatch == nil {
			return resolvedSelection{}, fmt.Errorf("%w: sheet=%s", ErrSelectorNotFound, selector.Sheet)
		}
	}
	if rangeSheetName != "" {
		if sheetMatch != nil && sheetMatch.name != rangeSheetName {
			return resolvedSelection{}, fmt.Errorf("%w: sheet=%s range=%s", ErrSelectorMismatch, selector.Sheet, selector.Range)
		}
		for _, candidate := range book.sheets {
			if candidate.name == rangeSheetName {
				sheetMatch = candidate
				break
			}
		}
		if sheetMatch == nil {
			return resolvedSelection{}, fmt.Errorf("%w: range sheet=%s", ErrSelectorNotFound, rangeSheetName)
		}
	}
	if tableMatch != nil {
		if sheetMatch != nil && sheetMatch.name != tableMatch.sheet {
			return resolvedSelection{}, fmt.Errorf("%w: sheet=%s table_name=%s", ErrSelectorMismatch, sheetMatch.name, tableMatch.name)
		}
		for _, candidate := range book.sheets {
			if candidate.name == tableMatch.sheet {
				sheetMatch = candidate
				break
			}
		}
		if selector.Range != "" && declaredRange != tableMatch.rng {
			return resolvedSelection{}, fmt.Errorf("%w: range=%s table_name=%s", ErrSelectorMismatch, selector.Range, selector.TableName)
		}
	}

	if sheetMatch == nil {
		if len(book.sheets) == 1 {
			sheetMatch = book.sheets[0]
		} else {
			return resolvedSelection{}, fmt.Errorf("%w: sheet must be pinned when multiple sheets exist", ErrAmbiguousSelector)
		}
	}

	selection.sheet = sheetMatch
	switch {
	case tableMatch != nil:
		selection.rng = tableMatch.rng
	case selector.Range != "":
		selection.rng = declaredRange
	default:
		selection.rng = sheetMatch.usedRange
	}
	selection.rangeText = formatRange(selection.rng)

	if selector.Sheet != "" && selector.Range == "" && selector.TableName == "" {
		selection.selectorMode = "sheet"
	}
	if selector.Sheet != "" && (selector.Range != "" || selector.TableName != "") {
		selection.selectorMode = "intersection"
	}
	if selector.Range != "" && selector.TableName != "" {
		selection.selectorMode = "intersection"
	}

	return selection, nil
}

func resolveRowWindow(rng gridRange, raw string) (rowWindow, error) {
	totalRows := rng.endRow - rng.startRow + 1
	if raw == "" {
		return rowWindow{
			DeclaredStart:  1,
			DeclaredEnd:    totalRows,
			EffectiveStart: rng.startRow,
			EffectiveEnd:   rng.endRow,
			PageSize:       totalRows,
		}, nil
	}
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidRowWindow, raw)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil || start <= 0 {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidRowWindow, raw)
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil || end < start {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidRowWindow, raw)
	}
	if end > totalRows {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, raw)
	}
	return rowWindow{
		DeclaredStart:  start,
		DeclaredEnd:    end,
		EffectiveStart: rng.startRow + start - 1,
		EffectiveEnd:   rng.startRow + end - 1,
		PageSize:       end - start + 1,
	}, nil
}

func resolveColumns(rng gridRange, projection string) ([]int, error) {
	if projection == "" {
		columns := make([]int, 0, rng.endCol-rng.startCol+1)
		for column := rng.startCol; column <= rng.endCol; column++ {
			columns = append(columns, column)
		}
		return columns, nil
	}
	parts := strings.Split(projection, ",")
	columns := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return nil, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, projection)
		}
		column, err := parseColumnLetters(part)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, projection)
		}
		if column < rng.startCol || column > rng.endCol {
			return nil, fmt.Errorf("%w: %s", ErrSelectorNotFound, projection)
		}
		if _, ok := seen[column]; ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, projection)
		}
		seen[column] = struct{}{}
		columns = append(columns, column)
	}
	slices.Sort(columns)
	return columns, nil
}

func collectRows(selection resolvedSelection, window rowWindow, columns []int) ([]rowEnvelope, error) {
	rows := make([]rowEnvelope, 0, window.PageSize)
	for row := window.EffectiveStart; row <= window.EffectiveEnd; row++ {
		envelope := rowEnvelope{
			Sheet:       selection.sheet.name,
			Row:         row,
			WindowIndex: row - window.EffectiveStart,
			Cells:       make([]cellEnvelope, 0, len(columns)),
		}
		for _, column := range columns {
			key := cellKey{row: row, column: column}
			selected, ok := selection.sheet.cells[key]
			if !ok {
				selected = cell{
					a1:     formatCellRef(row, column),
					row:    row,
					column: column,
				}
			}
			if selected.formula != "" && selected.cachedValue == "" {
				return nil, fmt.Errorf("%w: %s!%s", ErrFormulaEvaluationUnsupported, selection.sheet.name, selected.a1)
			}
			rawValue := selected.value
			normalizedValue := selected.value
			if selected.formula != "" {
				rawValue = selected.formula
				normalizedValue = selected.cachedValue
			}
			envelope.Cells = append(envelope.Cells, cellEnvelope{
				Sheet:              selection.sheet.name,
				Row:                row,
				Column:             column,
				A1:                 selected.a1,
				RawValue:           rawValue,
				NormalizedValue:    normalizedValue,
				Value:              normalizedValue,
				Formula:            selected.formula,
				FormulaCachedValue: selected.cachedValue,
			})
		}
		rows = append(rows, envelope)
	}
	return rows, nil
}

func selectorConstraintMismatch(selector kernel.SourceSelector) error {
	switch {
	case selector.Sheet != "" && selector.Range != "":
		return fmt.Errorf("%w: sheet=%s range=%s table_name=%s", ErrSelectorMismatch, selector.Sheet, selector.Range, selector.TableName)
	case selector.Sheet != "":
		return fmt.Errorf("%w: sheet=%s table_name=%s", ErrSelectorMismatch, selector.Sheet, selector.TableName)
	case selector.Range != "":
		return fmt.Errorf("%w: range=%s table_name=%s", ErrSelectorMismatch, selector.Range, selector.TableName)
	default:
		return fmt.Errorf("%w: table_name=%s", ErrSelectorMismatch, selector.TableName)
	}
}

func marshalOutput(sourceHash string, selection resolvedSelection, selector kernel.SourceSelector, window rowWindow, rows []rowEnvelope, columns []int) ([]byte, error) {
	output := outputEnvelope{
		Sheet:             selection.sheet.name,
		Range:             selection.rangeText,
		TableName:         selection.tableName,
		FormulaPolicy:     selection.formulaPolicy,
		ColumnCount:       len(columns),
		ColumnOrder:       "source_column_asc",
		ColumnProjection:  selector.ColumnProjection,
		EffectiveRowEnd:   window.EffectiveEnd,
		EffectiveRowStart: window.EffectiveStart,
		RowCount:          len(rows),
		RowOrder:          "source_row_asc",
		RowWindow:         selector.RowWindow,
		Rows:              rows,
		SourceHash:        sourceHash,
	}
	if len(rows) == 1 && len(rows[0].Cells) == 1 {
		output.RawValue = rows[0].Cells[0].RawValue
		output.NormalizedValue = rows[0].Cells[0].NormalizedValue
		output.Value = rows[0].Cells[0].Value
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}

func locator(selection resolvedSelection, selector kernel.SourceSelector, window rowWindow, rows []rowEnvelope, columns []int) json.RawMessage {
	locatorRows := make([]locatorRow, 0, len(rows))
	for _, row := range rows {
		cells := make([]locatorCell, 0, len(row.Cells))
		for _, cell := range row.Cells {
			cells = append(cells, locatorCell{Column: cell.Column, A1: cell.A1})
		}
		locatorRows = append(locatorRows, locatorRow{
			Sheet:       row.Sheet,
			Row:         row.Row,
			WindowIndex: row.WindowIndex,
			Cells:       cells,
		})
	}
	raw, _ := json.Marshal(locatorEnvelope{
		Sheet:             selection.sheet.name,
		Range:             selection.rangeText,
		TableName:         selection.tableName,
		ColumnCount:       len(columns),
		ColumnOrder:       "source_column_asc",
		ColumnProjection:  selector.ColumnProjection,
		EffectiveRowEnd:   window.EffectiveEnd,
		EffectiveRowStart: window.EffectiveStart,
		RowCount:          len(rows),
		RowOrder:          "source_row_asc",
		RowWindow:         selector.RowWindow,
		Rows:              locatorRows,
	})
	return canonicalRawMessage(raw)
}

func readerOptions(selection resolvedSelection, selector kernel.SourceSelector) json.RawMessage {
	projectionMode := "all_columns"
	if selector.ColumnProjection != "" {
		projectionMode = "absolute_columns"
	}
	raw, _ := json.Marshal(readerOptionsEnvelope{
		ColumnOrder:    "source_column_asc",
		FormulaPolicy:  selection.formulaPolicy,
		ProjectionMode: projectionMode,
		RangeMode:      selection.rangeMode,
		RowOrder:       "source_row_asc",
		SelectorMode:   selection.selectorMode,
	})
	return canonicalRawMessage(raw)
}

func origin(path string, selection resolvedSelection, rows []rowEnvelope, columns []int) string {
	if len(rows) == 1 && len(columns) == 1 {
		return fmt.Sprintf("%s#sheet=%s&row=%d&col=%d&a1=%s", path, selection.sheet.name, rows[0].Row, columns[0], rows[0].Cells[0].A1)
	}
	columnRefs := make([]string, 0, len(columns))
	for _, column := range columns {
		columnRefs = append(columnRefs, formatColumnLetters(column))
	}
	base := fmt.Sprintf("%s#sheet=%s&range=%s&row=%d-%d&col=%s", path, selection.sheet.name, selection.rangeText, rows[0].Row, rows[len(rows)-1].Row, strings.Join(columnRefs, ","))
	if selection.tableName != "" {
		base += "&table=" + selection.tableName
	}
	return base
}

func readID(runID, sourcePath string, selector kernel.SourceSelector, sourceSHA256, outputSHA256 string) string {
	parts := []string{
		readerName,
		readerVersion,
		runID,
		sourcePath,
		selector.Sheet,
		selector.Range,
		selector.TableName,
		selector.RowWindow,
		selector.ColumnProjection,
		selector.FormulaPolicy,
		sourceSHA256,
		outputSHA256,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "read_" + hex.EncodeToString(sum[:16])
}

func canonicalRawMessage(raw []byte) json.RawMessage {
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return json.RawMessage(raw)
	}
	return json.RawMessage(canonical)
}

func parseRange(raw string) (gridRange, string, error) {
	sheetName := ""
	rangeText := raw
	if bang := strings.Index(raw, "!"); bang >= 0 {
		sheetName = raw[:bang]
		rangeText = raw[bang+1:]
		if sheetName == "" {
			return gridRange{}, "", fmt.Errorf("%w: %s", ErrInvalidRange, raw)
		}
	}
	parts := strings.Split(rangeText, ":")
	switch len(parts) {
	case 1:
		row, col, err := parseA1(parts[0])
		if err != nil {
			return gridRange{}, "", fmt.Errorf("%w: %s", ErrInvalidRange, raw)
		}
		return gridRange{startRow: row, endRow: row, startCol: col, endCol: col}, sheetName, nil
	case 2:
		startRow, startCol, err := parseA1(parts[0])
		if err != nil {
			return gridRange{}, "", fmt.Errorf("%w: %s", ErrInvalidRange, raw)
		}
		endRow, endCol, err := parseA1(parts[1])
		if err != nil {
			return gridRange{}, "", fmt.Errorf("%w: %s", ErrInvalidRange, raw)
		}
		if endRow < startRow || endCol < startCol {
			return gridRange{}, "", fmt.Errorf("%w: %s", ErrInvalidRange, raw)
		}
		return gridRange{startRow: startRow, endRow: endRow, startCol: startCol, endCol: endCol}, sheetName, nil
	default:
		return gridRange{}, "", fmt.Errorf("%w: %s", ErrInvalidRange, raw)
	}
}

func parseA1(raw string) (int, int, error) {
	matches := a1Pattern.FindStringSubmatch(strings.ToUpper(raw))
	if matches == nil {
		return 0, 0, fmt.Errorf("%w: %s", ErrInvalidRange, raw)
	}
	column, err := parseColumnLetters(matches[1])
	if err != nil {
		return 0, 0, err
	}
	row, err := strconv.Atoi(matches[2])
	if err != nil || row <= 0 {
		return 0, 0, fmt.Errorf("%w: %s", ErrInvalidRange, raw)
	}
	return row, column, nil
}

func parseColumnLetters(raw string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("%w: empty column", ErrInvalidColumnProjection)
	}
	column := 0
	for _, r := range raw {
		if r < 'A' || r > 'Z' {
			return 0, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, raw)
		}
		column = column*26 + int(r-'A'+1)
	}
	return column, nil
}

func normalizeA1(raw string) string {
	row, col, err := parseA1(raw)
	if err != nil {
		return raw
	}
	return formatCellRef(row, col)
}

func formatRange(rng gridRange) string {
	start := formatCellRef(rng.startRow, rng.startCol)
	end := formatCellRef(rng.endRow, rng.endCol)
	if start == end {
		return start
	}
	return start + ":" + end
}

func formatCellRef(row, column int) string {
	return formatColumnLetters(column) + strconv.Itoa(row)
}

func formatColumnLetters(column int) string {
	if column <= 0 {
		return ""
	}
	buf := make([]byte, 0, 4)
	for column > 0 {
		column--
		buf = append(buf, byte('A'+column%26))
		column /= 26
	}
	slices.Reverse(buf)
	return string(buf)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
