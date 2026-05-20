package router

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/reconcile/request"
)

func resolveSelector(file request.FileConfig, source []byte) (kernel.SourceSelector, error) {
	switch file.Format {
	case request.FormatCSV:
		rowWindow, start, err := resolveWindow(file.Selector.RowWindow, countCSVRows(source), true, "row_window")
		if err != nil {
			return kernel.SourceSelector{}, err
		}
		headerRow := file.Selector.HeaderRow
		if headerRow == 0 && start > 1 && columnsUseNamedSources(file.Columns) {
			headerRow = 1
		}
		selector := kernel.SourceSelector{
			RowWindow: rowWindow,
			HeaderRow: headerRow,
			Dialect:   file.Selector.Dialect,
			Encoding:  file.Selector.Encoding,
		}
		if headerRow > 0 && columnsUseOnlyNamedSources(file.Columns) {
			selector.ColumnProjection = joinSorted(uniqueColumnSources(file.Columns))
		}
		return selector, nil
	case request.FormatJSONL:
		lineWindow, _, err := resolveWindow(file.Selector.LineWindow, countJSONLLines(source), true, "line_window")
		if err != nil {
			return kernel.SourceSelector{}, err
		}
		return kernel.SourceSelector{
			LineWindow:  lineWindow,
			JSONPointer: file.Selector.JSONPointer,
		}, nil
	case request.FormatXLSX:
		selector := kernel.SourceSelector{
			Sheet: file.Selector.Sheet,
			Range: file.Selector.Range,
		}
		if file.Selector.Table != "" {
			selector.TableName = file.Selector.Table
		}
		if file.Selector.RowWindow != "" {
			rowWindow, _, err := resolveWindow(file.Selector.RowWindow, 0, false, "row_window")
			if err != nil {
				return kernel.SourceSelector{}, err
			}
			selector.RowWindow = rowWindow
		}
		if projection, ok := spreadsheetProjection(file.Columns); ok {
			selector.ColumnProjection = projection
		}
		return selector, nil
	case request.FormatJSON:
		return kernel.SourceSelector{JSONPointer: file.Selector.JSONPointer}, nil
	default:
		return kernel.SourceSelector{}, fmt.Errorf("selector_invalid: unsupported format %s", file.Format)
	}
}

func resolveWindow(raw string, total int, allowOpen bool, field string) (string, int, error) {
	start, end, openEnded, err := parseWindow(raw)
	if err != nil {
		return "", 0, fmt.Errorf("selector_invalid: %s %q", field, raw)
	}
	if openEnded {
		if !allowOpen {
			return "", 0, fmt.Errorf("selector_invalid: %s %q", field, raw)
		}
		if total < start {
			return "", 0, fmt.Errorf("selector_invalid: %s %q", field, raw)
		}
		end = total
	}
	return fmt.Sprintf("%d-%d", start, end), start, nil
}

func parseWindow(raw string) (start, end int, openEnded bool, err error) {
	if strings.Contains(raw, ":") {
		parts := strings.Split(raw, ":")
		if len(parts) != 2 || parts[1] != "" {
			return 0, 0, false, fmt.Errorf("invalid")
		}
		start, err = strconv.Atoi(parts[0])
		if err != nil || start <= 0 {
			return 0, 0, false, fmt.Errorf("invalid")
		}
		return start, 0, true, nil
	}
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return 0, 0, false, fmt.Errorf("invalid")
	}
	start, err = strconv.Atoi(parts[0])
	if err != nil || start <= 0 {
		return 0, 0, false, fmt.Errorf("invalid")
	}
	end, err = strconv.Atoi(parts[1])
	if err != nil || end < start {
		return 0, 0, false, fmt.Errorf("invalid")
	}
	return start, end, false, nil
}

func countCSVRows(data []byte) int {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	total := 0
	for {
		_, err := r.Read()
		if err != nil {
			break
		}
		total++
	}
	return total
}

func countJSONLLines(data []byte) int {
	lineNumber := 1
	lineStart := 0
	for index := 0; index <= len(data); index++ {
		atEnd := index == len(data)
		if !atEnd && data[index] != '\n' {
			continue
		}
		if atEnd && lineStart == len(data) {
			return lineNumber - 1
		}
		if atEnd {
			return lineNumber
		}
		lineNumber++
		lineStart = index + 1
	}
	return lineNumber
}

func uniqueColumnSources(columns request.Columns) []string {
	values := make([]string, 0, len(columns))
	seen := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		if _, ok := seen[column.Source]; ok {
			continue
		}
		seen[column.Source] = struct{}{}
		values = append(values, column.Source)
	}
	sort.Strings(values)
	return values
}

func columnsUseNamedSources(columns request.Columns) bool {
	for _, column := range columns {
		if _, err := strconv.Atoi(column.Source); err != nil {
			return true
		}
	}
	return false
}

func columnsUseOnlyNamedSources(columns request.Columns) bool {
	for _, column := range columns {
		if _, err := strconv.Atoi(column.Source); err == nil {
			return false
		}
	}
	return true
}

func spreadsheetProjection(columns request.Columns) (string, bool) {
	values := uniqueColumnSources(columns)
	for _, value := range values {
		if !isSpreadsheetColumn(value) {
			return "", false
		}
	}
	return joinSorted(values), true
}

func isSpreadsheetColumn(raw string) bool {
	if raw == "" {
		return false
	}
	for _, ch := range raw {
		if ch < 'A' || ch > 'Z' {
			return false
		}
	}
	return true
}

func joinSorted(values []string) string {
	clone := append([]string(nil), values...)
	sort.Strings(clone)
	return strings.Join(clone, ",")
}
