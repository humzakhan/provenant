package extractcsvrows

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
)

const (
	readerName    = "extract.csv_rows"
	readerVersion = "v1"
)

var (
	ErrMissingSourceBytes       = errors.New("extractcsvrows: missing source bytes")
	ErrMissingSourceSHA256      = errors.New("extractcsvrows: missing source sha256")
	ErrSourceHashMismatch       = errors.New("extractcsvrows: source hash mismatch")
	ErrUnsupportedSelector      = errors.New("extractcsvrows: unsupported selector metadata")
	ErrInvalidRowWindow         = errors.New("extractcsvrows: invalid row window")
	ErrInvalidCursor            = errors.New("extractcsvrows: invalid cursor")
	ErrInvalidHeaderRow         = errors.New("extractcsvrows: invalid header row")
	ErrMissingDialect           = errors.New("extractcsvrows: dialect is required")
	ErrUnsupportedDialect       = errors.New("extractcsvrows: unsupported dialect")
	ErrMissingEncoding          = errors.New("extractcsvrows: encoding is required")
	ErrUnsupportedEncoding      = errors.New("extractcsvrows: unsupported encoding")
	ErrInvalidEncodedSource     = errors.New("extractcsvrows: invalid encoded source")
	ErrInvalidColumnProjection  = errors.New("extractcsvrows: invalid column projection")
	ErrMalformedCSV             = errors.New("extractcsvrows: malformed csv")
	ErrSelectorNotFound         = errors.New("extractcsvrows: selector not found")
	ErrSelectorWindowMismatch   = errors.New("extractcsvrows: selector/window mismatch")
	ErrDuplicateHeaderName      = errors.New("extractcsvrows: duplicate header name")
	ErrDuplicateProjectedColumn = errors.New("extractcsvrows: duplicate projected column")
)

type Reader struct{}

type rowWindow struct {
	DeclaredStart  int
	DeclaredEnd    int
	EffectiveStart int
	EffectiveEnd   int
	PageSize       int
	InputCursor    string
	NextCursor     string
}

type sourceCell struct {
	Column     int
	RawStart   int
	RawEnd     int
	RawValue   string
	Value      string
	Quoted     bool
	HeaderName string
}

type sourceRow struct {
	Row         int
	RecordStart int
	RecordEnd   int
	Cells       []sourceCell
}

type columnSpec struct {
	Column int
	Header string
}

type headerEnvelope struct {
	Cells []headerCellEnvelope `json:"cells"`
	Row   int                  `json:"row"`
}

type headerCellEnvelope struct {
	Column          int    `json:"column"`
	NormalizedValue string `json:"normalized_value"`
	RawValue        string `json:"raw_value"`
	Value           string `json:"value"`
}

type cellEnvelope struct {
	Column          int    `json:"column"`
	Header          string `json:"header,omitempty"`
	NormalizedValue string `json:"normalized_value"`
	RawValue        string `json:"raw_value"`
	Row             int    `json:"row"`
	Value           string `json:"value"`
}

type rowEnvelope struct {
	Cells       []cellEnvelope `json:"cells"`
	Row         int            `json:"row"`
	WindowIndex int            `json:"window_index"`
}

type outputEnvelope struct {
	Cursor            string          `json:"cursor,omitempty"`
	ColumnCount       int             `json:"column_count"`
	ColumnOrder       string          `json:"column_order"`
	ColumnProjection  string          `json:"column_projection,omitempty"`
	EffectiveRowEnd   int             `json:"effective_row_end"`
	EffectiveRowStart int             `json:"effective_row_start"`
	Header            *headerEnvelope `json:"header,omitempty"`
	HeaderRow         int             `json:"header_row,omitempty"`
	NextCursor        string          `json:"next_cursor,omitempty"`
	NormalizedValue   string          `json:"normalized_value,omitempty"`
	RawValue          string          `json:"raw_value,omitempty"`
	RowCount          int             `json:"row_count"`
	RowOrder          string          `json:"row_order"`
	RowWindow         string          `json:"row_window"`
	Rows              []rowEnvelope   `json:"rows"`
	SourceHash        string          `json:"source_hash"`
	Value             string          `json:"value,omitempty"`
}

type locatorEnvelope struct {
	ColumnCount       int          `json:"column_count"`
	ColumnOrder       string       `json:"column_order"`
	ColumnProjection  string       `json:"column_projection,omitempty"`
	Cursor            string       `json:"cursor,omitempty"`
	EffectiveRowEnd   int          `json:"effective_row_end"`
	EffectiveRowStart int          `json:"effective_row_start"`
	HeaderRow         int          `json:"header_row,omitempty"`
	NextCursor        string       `json:"next_cursor,omitempty"`
	RowCount          int          `json:"row_count"`
	RowOrder          string       `json:"row_order"`
	RowWindow         string       `json:"row_window"`
	Rows              []locatorRow `json:"rows"`
}

type locatorRow struct {
	ByteEnd     int           `json:"byte_end"`
	ByteStart   int           `json:"byte_start"`
	Cells       []locatorCell `json:"cells"`
	Row         int           `json:"row"`
	WindowIndex int           `json:"window_index"`
}

type locatorCell struct {
	Column       int    `json:"column"`
	Header       string `json:"header,omitempty"`
	RawByteEnd   int    `json:"raw_byte_end"`
	RawByteStart int    `json:"raw_byte_start"`
}

type readerOptionsEnvelope struct {
	ColumnOrder    string `json:"column_order"`
	Encoding       string `json:"encoding"`
	HeaderEchoed   bool   `json:"header_echoed,omitempty"`
	PageSize       int    `json:"page_size"`
	ProjectionMode string `json:"projection_mode"`
	RowOrder       string `json:"row_order"`
	SelectorMode   string `json:"selector_mode"`
	Dialect        string `json:"dialect"`
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
	if err := validateEncoding(source.Bytes, selector.Encoding); err != nil {
		return kernel.ReadResult{}, err
	}

	window, err := parseRowWindow(selector.RowWindow, selector.Cursor)
	if err != nil {
		return kernel.ReadResult{}, err
	}

	rows, err := parseCSV(source.Bytes)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	if selector.HeaderRow > len(rows) {
		return kernel.ReadResult{}, fmt.Errorf("%w: header row %d exceeds row count %d", ErrSelectorNotFound, selector.HeaderRow, len(rows))
	}
	if selector.HeaderRow != 0 && window.EffectiveStart <= selector.HeaderRow && selector.HeaderRow <= window.EffectiveEnd {
		return kernel.ReadResult{}, fmt.Errorf("%w: header row %d overlaps selected rows", ErrSelectorWindowMismatch, selector.HeaderRow)
	}

	selectedRows := selectRows(rows, window)
	if len(selectedRows) == 0 {
		return kernel.ReadResult{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, selector.RowWindow)
	}
	window.NextCursor = nextCursor(selectedRows[len(selectedRows)-1].Row, len(rows))

	header, columns, err := resolveColumns(rows, selectedRows, selector)
	if err != nil {
		return kernel.ReadResult{}, err
	}

	output, err := marshalOutput(source.Source.SHA256, selector, window, header, columns, selectedRows)
	if err != nil {
		return kernel.ReadResult{}, fmt.Errorf("extractcsvrows: marshal output: %w", err)
	}
	outputHash := sha256.Sum256(output)
	outputSHA256 := hex.EncodeToString(outputHash[:])

	captured := kernel.CapturedRead{
		ID:            readID(run.ID, source.Source.Path, selector, source.Source.SHA256, outputSHA256),
		RunID:         run.ID,
		SourcePath:    source.Source.Path,
		Origin:        origin(source.Source.Path, selector, columns, selectedRows),
		Selector:      selector,
		ReaderName:    readerName,
		ReaderVersion: readerVersion,
		ReaderOptions: readerOptions(selector, window, header != nil),
		Locator:       locator(selector, window, columns, selectedRows),
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
	if selector.RowWindow == "" {
		return ErrInvalidRowWindow
	}
	if selector.JSONPointer != "" ||
		selector.JSONPath != "" ||
		selector.ByteSpan != "" ||
		selector.LineWindow != "" ||
		selector.Sheet != "" ||
		selector.Range != "" ||
		selector.TableName != "" ||
		selector.FormulaPolicy != "" {
		return ErrUnsupportedSelector
	}
	if selector.HeaderRow < 0 {
		return ErrInvalidHeaderRow
	}
	if selector.Dialect == "" {
		return ErrMissingDialect
	}
	switch selector.Dialect {
	case "excel", "rfc4180":
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedDialect, selector.Dialect)
	}
	if selector.Encoding == "" {
		return ErrMissingEncoding
	}
	if selector.Encoding != "utf-8" {
		return fmt.Errorf("%w: %s", ErrUnsupportedEncoding, selector.Encoding)
	}
	return nil
}

func validateEncoding(data []byte, encoding string) error {
	if encoding != "utf-8" {
		return fmt.Errorf("%w: %s", ErrUnsupportedEncoding, encoding)
	}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		return fmt.Errorf("%w: utf-8 bom is not supported", ErrInvalidEncodedSource)
	}
	if !utf8.Valid(data) {
		return ErrInvalidEncodedSource
	}
	return nil
}

func parseRowWindow(raw, cursor string) (rowWindow, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidRowWindow, raw)
	}
	start, err := parseIndex(parts[0])
	if err != nil || start == 0 {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidRowWindow, raw)
	}
	end, err := parseIndex(parts[1])
	if err != nil || end < start {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidRowWindow, raw)
	}
	window := rowWindow{
		DeclaredStart:  start,
		DeclaredEnd:    end,
		EffectiveStart: start,
		EffectiveEnd:   end,
		PageSize:       end - start + 1,
		InputCursor:    cursor,
	}
	if cursor == "" {
		return window, nil
	}
	if !strings.HasPrefix(cursor, "row:") {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidCursor, cursor)
	}
	lastRow, err := parseIndex(strings.TrimPrefix(cursor, "row:"))
	if err != nil || lastRow <= 0 {
		return rowWindow{}, fmt.Errorf("%w: %s", ErrInvalidCursor, cursor)
	}
	window.EffectiveStart = lastRow + 1
	window.EffectiveEnd = lastRow + window.PageSize
	return window, nil
}

func parseIndex(raw string) (int, error) {
	if raw == "" {
		return 0, ErrInvalidRowWindow
	}
	value := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, ErrInvalidRowWindow
		}
		value = value*10 + int(ch-'0')
	}
	return value, nil
}

func parseCSV(data []byte) ([]sourceRow, error) {
	rows := make([]sourceRow, 0, 16)
	index := 0
	expectedColumns := -1
	for rowNumber := 1; index <= len(data); rowNumber++ {
		recordStart := index
		cells := make([]sourceCell, 0, 8)
		column := 1

		for {
			if index > len(data) {
				return nil, ErrMalformedCSV
			}
			cell, next, recordDone, err := parseCell(data, index, rowNumber, column)
			if err != nil {
				return nil, err
			}
			cells = append(cells, cell)
			index = next
			if recordDone {
				break
			}
			column++
		}

		recordEnd := index
		if recordEnd > recordStart {
			last := data[recordEnd-1]
			if last == '\n' {
				recordEnd--
				if recordEnd > recordStart && data[recordEnd-1] == '\r' {
					recordEnd--
				}
			}
		}

		if expectedColumns == -1 {
			expectedColumns = len(cells)
		} else if len(cells) != expectedColumns {
			return nil, fmt.Errorf("%w: row %d column count %d, want %d", ErrMalformedCSV, rowNumber, len(cells), expectedColumns)
		}
		rows = append(rows, sourceRow{
			Row:         rowNumber,
			RecordStart: recordStart,
			RecordEnd:   recordEnd,
			Cells:       cells,
		})

		if index == len(data) {
			break
		}
	}
	return rows, nil
}

func parseCell(data []byte, start, row, column int) (sourceCell, int, bool, error) {
	if start > len(data) {
		return sourceCell{}, 0, false, ErrMalformedCSV
	}
	if start == len(data) {
		return sourceCell{
			Column:   column,
			RawStart: start,
			RawEnd:   start,
			RawValue: "",
			Value:    "",
		}, start, true, nil
	}

	cell := sourceCell{Column: column, RawStart: start}
	if data[start] == '"' {
		cell.Quoted = true
		var builder strings.Builder
		index := start + 1
		for index < len(data) {
			switch data[index] {
			case '"':
				if index+1 < len(data) && data[index+1] == '"' {
					builder.WriteByte('"')
					index += 2
					continue
				}
				cell.RawEnd = index + 1
				cell.RawValue = string(data[start:cell.RawEnd])
				cell.Value = builder.String()
				index++
				switch {
				case index == len(data):
					return cell, index, true, nil
				case data[index] == ',':
					return cell, index + 1, false, nil
				case data[index] == '\n':
					return cell, index + 1, true, nil
				case data[index] == '\r':
					if index+1 >= len(data) || data[index+1] != '\n' {
						return sourceCell{}, 0, false, fmt.Errorf("%w: row %d column %d", ErrMalformedCSV, row, column)
					}
					return cell, index + 2, true, nil
				default:
					return sourceCell{}, 0, false, fmt.Errorf("%w: row %d column %d", ErrMalformedCSV, row, column)
				}
			default:
				builder.WriteByte(data[index])
				index++
			}
		}
		return sourceCell{}, 0, false, fmt.Errorf("%w: row %d column %d", ErrMalformedCSV, row, column)
	}

	index := start
	for index < len(data) {
		switch data[index] {
		case '"':
			return sourceCell{}, 0, false, fmt.Errorf("%w: row %d column %d", ErrMalformedCSV, row, column)
		case ',':
			cell.RawEnd = index
			cell.RawValue = string(data[start:index])
			cell.Value = cell.RawValue
			return cell, index + 1, false, nil
		case '\n':
			cell.RawEnd = index
			cell.RawValue = string(data[start:index])
			cell.Value = cell.RawValue
			return cell, index + 1, true, nil
		case '\r':
			if index+1 >= len(data) || data[index+1] != '\n' {
				return sourceCell{}, 0, false, fmt.Errorf("%w: row %d column %d", ErrMalformedCSV, row, column)
			}
			cell.RawEnd = index
			cell.RawValue = string(data[start:index])
			cell.Value = cell.RawValue
			return cell, index + 2, true, nil
		default:
			index++
		}
	}
	cell.RawEnd = len(data)
	cell.RawValue = string(data[start:])
	cell.Value = cell.RawValue
	return cell, len(data), true, nil
}

func selectRows(rows []sourceRow, window rowWindow) []sourceRow {
	selected := make([]sourceRow, 0, window.PageSize)
	for _, row := range rows {
		if row.Row < window.EffectiveStart {
			continue
		}
		if row.Row > window.EffectiveEnd {
			break
		}
		selected = append(selected, row)
	}
	return selected
}

func resolveColumns(rows, selectedRows []sourceRow, selector kernel.SourceSelector) (*sourceRow, []columnSpec, error) {
	if selector.HeaderRow == 0 {
		columns, err := projectionByIndex(selectedRows[0].Cells, selector.ColumnProjection)
		return nil, columns, err
	}

	header := rows[selector.HeaderRow-1]
	seenNames := make(map[string]struct{}, len(header.Cells))
	for _, cell := range header.Cells {
		if _, exists := seenNames[cell.Value]; exists {
			return nil, nil, fmt.Errorf("%w: %q", ErrDuplicateHeaderName, cell.Value)
		}
		seenNames[cell.Value] = struct{}{}
	}
	columns, err := projectionByHeader(header.Cells, selector.ColumnProjection)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range selectedRows {
		if len(row.Cells) < len(header.Cells) {
			return nil, nil, fmt.Errorf("%w: row %d has %d columns, want at least %d", ErrSelectorWindowMismatch, row.Row, len(row.Cells), len(header.Cells))
		}
	}
	return &header, columns, nil
}

func projectionByIndex(cells []sourceCell, projection string) ([]columnSpec, error) {
	if projection == "" {
		columns := make([]columnSpec, 0, len(cells))
		for _, cell := range cells {
			columns = append(columns, columnSpec{Column: cell.Column})
		}
		return columns, nil
	}
	tokens, err := parseProjectionTokens(projection)
	if err != nil {
		return nil, err
	}
	columns := make([]columnSpec, 0, len(tokens))
	seen := make(map[int]struct{}, len(tokens))
	for _, token := range tokens {
		column, err := parseIndex(token)
		if err != nil || column <= 0 {
			return nil, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, projection)
		}
		if column > len(cells) {
			return nil, fmt.Errorf("%w: column %d", ErrSelectorNotFound, column)
		}
		if _, exists := seen[column]; exists {
			return nil, fmt.Errorf("%w: column %d", ErrDuplicateProjectedColumn, column)
		}
		seen[column] = struct{}{}
		columns = append(columns, columnSpec{Column: column})
	}
	slices.SortFunc(columns, func(left, right columnSpec) int {
		return left.Column - right.Column
	})
	return columns, nil
}

func projectionByHeader(cells []sourceCell, projection string) ([]columnSpec, error) {
	indexByHeader := make(map[string]int, len(cells))
	for _, cell := range cells {
		indexByHeader[cell.Value] = cell.Column
	}
	if projection == "" {
		columns := make([]columnSpec, 0, len(cells))
		for _, cell := range cells {
			columns = append(columns, columnSpec{Column: cell.Column, Header: cell.Value})
		}
		return columns, nil
	}
	tokens, err := parseProjectionTokens(projection)
	if err != nil {
		return nil, err
	}
	columns := make([]columnSpec, 0, len(tokens))
	seen := make(map[int]struct{}, len(tokens))
	for _, token := range tokens {
		column, ok := indexByHeader[token]
		if !ok {
			return nil, fmt.Errorf("%w: header %q", ErrSelectorNotFound, token)
		}
		if _, exists := seen[column]; exists {
			return nil, fmt.Errorf("%w: header %q", ErrDuplicateProjectedColumn, token)
		}
		seen[column] = struct{}{}
		columns = append(columns, columnSpec{Column: column, Header: token})
	}
	slices.SortFunc(columns, func(left, right columnSpec) int {
		return left.Column - right.Column
	})
	return columns, nil
}

func parseProjectionTokens(projection string) ([]string, error) {
	parts := strings.Split(projection, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, projection)
	}
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return nil, fmt.Errorf("%w: %s", ErrInvalidColumnProjection, projection)
		}
		tokens = append(tokens, part)
	}
	return tokens, nil
}

func marshalOutput(sourceHash string, selector kernel.SourceSelector, window rowWindow, header *sourceRow, columns []columnSpec, rows []sourceRow) ([]byte, error) {
	payloadRows := make([]rowEnvelope, 0, len(rows))
	for index, row := range rows {
		cells := make([]cellEnvelope, 0, len(columns))
		for _, column := range columns {
			cell := row.Cells[column.Column-1]
			cells = append(cells, cellEnvelope{
				Column:          column.Column,
				Header:          column.Header,
				NormalizedValue: cell.Value,
				RawValue:        cell.RawValue,
				Row:             row.Row,
				Value:           cell.Value,
			})
		}
		payloadRows = append(payloadRows, rowEnvelope{
			Cells:       cells,
			Row:         row.Row,
			WindowIndex: index,
		})
	}

	payload := outputEnvelope{
		Cursor:            window.InputCursor,
		ColumnCount:       len(columns),
		ColumnOrder:       "source_column_asc",
		ColumnProjection:  selector.ColumnProjection,
		EffectiveRowEnd:   rows[len(rows)-1].Row,
		EffectiveRowStart: rows[0].Row,
		HeaderRow:         selector.HeaderRow,
		NextCursor:        window.NextCursor,
		RowCount:          len(rows),
		RowOrder:          "source_row_asc",
		RowWindow:         selector.RowWindow,
		Rows:              payloadRows,
		SourceHash:        sourceHash,
	}
	if header != nil {
		headerCells := make([]headerCellEnvelope, 0, len(columns))
		for _, column := range columns {
			cell := header.Cells[column.Column-1]
			headerCells = append(headerCells, headerCellEnvelope{
				Column:          column.Column,
				NormalizedValue: cell.Value,
				RawValue:        cell.RawValue,
				Value:           cell.Value,
			})
		}
		payload.Header = &headerEnvelope{Cells: headerCells, Row: header.Row}
	}
	if len(rows) == 1 && len(columns) == 1 {
		cell := rows[0].Cells[columns[0].Column-1]
		payload.NormalizedValue = cell.Value
		payload.RawValue = cell.RawValue
		payload.Value = cell.Value
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}

func locator(selector kernel.SourceSelector, window rowWindow, columns []columnSpec, rows []sourceRow) json.RawMessage {
	locatorRows := make([]locatorRow, 0, len(rows))
	for index, row := range rows {
		cells := make([]locatorCell, 0, len(columns))
		for _, column := range columns {
			cell := row.Cells[column.Column-1]
			cells = append(cells, locatorCell{
				Column:       column.Column,
				Header:       column.Header,
				RawByteEnd:   cell.RawEnd,
				RawByteStart: cell.RawStart,
			})
		}
		locatorRows = append(locatorRows, locatorRow{
			ByteEnd:     row.RecordEnd,
			ByteStart:   row.RecordStart,
			Cells:       cells,
			Row:         row.Row,
			WindowIndex: index,
		})
	}

	raw, _ := json.Marshal(locatorEnvelope{
		ColumnCount:       len(columns),
		ColumnOrder:       "source_column_asc",
		ColumnProjection:  selector.ColumnProjection,
		Cursor:            window.InputCursor,
		EffectiveRowEnd:   rows[len(rows)-1].Row,
		EffectiveRowStart: rows[0].Row,
		HeaderRow:         selector.HeaderRow,
		NextCursor:        window.NextCursor,
		RowCount:          len(rows),
		RowOrder:          "source_row_asc",
		RowWindow:         selector.RowWindow,
		Rows:              locatorRows,
	})
	return canonicalRawMessage(raw)
}

func readerOptions(selector kernel.SourceSelector, window rowWindow, headerEchoed bool) json.RawMessage {
	raw, _ := json.Marshal(readerOptionsEnvelope{
		ColumnOrder:    "source_column_asc",
		Dialect:        selector.Dialect,
		Encoding:       selector.Encoding,
		HeaderEchoed:   headerEchoed,
		PageSize:       window.PageSize,
		ProjectionMode: projectionMode(selector),
		RowOrder:       "source_row_asc",
		SelectorMode:   "row_window",
	})
	return canonicalRawMessage(raw)
}

func projectionMode(selector kernel.SourceSelector) string {
	switch {
	case selector.ColumnProjection != "" && selector.HeaderRow != 0:
		return "header_names"
	case selector.ColumnProjection != "":
		return "column_indexes"
	case selector.HeaderRow != 0:
		return "header_row"
	default:
		return "all_columns"
	}
}

func canonicalRawMessage(raw []byte) json.RawMessage {
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return json.RawMessage(raw)
	}
	return json.RawMessage(canonical)
}

func origin(path string, selector kernel.SourceSelector, columns []columnSpec, rows []sourceRow) string {
	rowPart := fmt.Sprintf("row=%d-%d", rows[0].Row, rows[len(rows)-1].Row)
	if len(rows) == 1 {
		rowPart = fmt.Sprintf("row=%d", rows[0].Row)
	}
	columnIDs := make([]string, 0, len(columns))
	headerNames := make([]string, 0, len(columns))
	for _, column := range columns {
		columnIDs = append(columnIDs, fmt.Sprintf("%d", column.Column))
		if column.Header != "" {
			headerNames = append(headerNames, column.Header)
		}
	}
	parts := []string{rowPart, "col=" + strings.Join(columnIDs, ",")}
	if selector.HeaderRow != 0 {
		parts = append(parts, fmt.Sprintf("header_row=%d", selector.HeaderRow))
	}
	if len(rows) == 1 && len(columns) == 1 && columns[0].Header != "" {
		parts = append(parts, "header="+columns[0].Header)
	} else if len(headerNames) > 0 {
		parts = append(parts, "headers="+strings.Join(headerNames, ","))
	}
	return path + "#" + strings.Join(parts, "&")
}

func readID(runID, sourcePath string, selector kernel.SourceSelector, sourceSHA256, outputSHA256 string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		readerName,
		readerVersion,
		runID,
		sourcePath,
		selector.RowWindow,
		selector.Cursor,
		selector.ColumnProjection,
		fmt.Sprintf("%d", selector.HeaderRow),
		selector.Dialect,
		selector.Encoding,
		sourceSHA256,
		outputSHA256,
	}, "\n")))
	return "read_" + hex.EncodeToString(sum[:16])
}

func nextCursor(lastRow, totalRows int) string {
	if lastRow >= totalRows {
		return ""
	}
	return fmt.Sprintf("row:%d", lastRow)
}
