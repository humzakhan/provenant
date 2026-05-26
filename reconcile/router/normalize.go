package router

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
	"github.com/humzakhan/provenant/reconcile/request"
)

func normalizeRows(file request.FileConfig, selector any, output []byte) ([]NormalizedRow, error) {
	switch file.Format {
	case request.FormatCSV:
		return normalizeCSVRows(file, output)
	case request.FormatXLSX:
		return normalizeSpreadsheetRows(file, output)
	case request.FormatJSON:
		return normalizeJSONRows(file, output)
	case request.FormatJSONL:
		return normalizeJSONLRows(file, output)
	default:
		return nil, fmt.Errorf("normalization_failed: unsupported format %s", file.Format)
	}
}

type csvOutput struct {
	Rows []struct {
		Row         int `json:"row"`
		WindowIndex int `json:"window_index"`
		Cells       []struct {
			Column   int    `json:"column"`
			Header   string `json:"header"`
			RawValue string `json:"raw_value"`
			Value    string `json:"value"`
		} `json:"cells"`
	} `json:"rows"`
}

func normalizeCSVRows(file request.FileConfig, output []byte) ([]NormalizedRow, error) {
	var payload csvOutput
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("normalization_failed: csv output decode: %w", err)
	}
	rows := make([]NormalizedRow, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		byHeader := make(map[string]struct {
			Raw string
			Val string
		}, len(row.Cells))
		byIndex := make(map[int]struct {
			Raw string
			Val string
		}, len(row.Cells))
		for _, cell := range row.Cells {
			byIndex[cell.Column] = struct {
				Raw string
				Val string
			}{Raw: cell.RawValue, Val: cell.Value}
			if cell.Header != "" {
				byHeader[cell.Header] = struct {
					Raw string
					Val string
				}{Raw: cell.RawValue, Val: cell.Value}
			}
		}
		values := make(map[string]NormalizedValue, len(file.Columns))
		for logical, column := range file.Columns {
			item, ok := byHeader[column.Source]
			if !ok {
				if index, err := strconv.Atoi(column.Source); err == nil {
					item, ok = byIndex[index]
				}
			}
			if !ok {
				if column.Required {
					return nil, fmt.Errorf("required_value_missing: row %d source %q", row.Row, column.Source)
				}
				values[logical] = NormalizedValue{LogicalField: logical, Role: column.Role, Source: column.Source, Empty: true}
				continue
			}
			if column.Required && item.Val == "" {
				return nil, fmt.Errorf("required_value_missing: row %d source %q", row.Row, column.Source)
			}
			values[logical] = NormalizedValue{
				LogicalField: logical,
				Role:         column.Role,
				Source:       column.Source,
				Raw:          item.Raw,
				RawExact:     true,
				Canonical:    item.Val,
				Empty:        item.Val == "",
				Value:        rawJSON(item.Val),
			}
		}
		rows = append(rows, NormalizedRow{
			RowKey:   fmt.Sprintf("row:%d", row.Row),
			Position: RowPosition{Index: row.WindowIndex, Row: row.Row},
			Values:   values,
		})
	}
	return rows, nil
}

type spreadsheetOutput struct {
	Sheet string `json:"sheet"`
	Rows  []struct {
		Row         int    `json:"row"`
		WindowIndex int    `json:"window_index"`
		Sheet       string `json:"sheet"`
		Cells       []struct {
			A1       string `json:"a1"`
			Column   int    `json:"column"`
			RawValue string `json:"raw_value"`
			Value    string `json:"value"`
		} `json:"cells"`
	} `json:"rows"`
}

func normalizeSpreadsheetRows(file request.FileConfig, output []byte) ([]NormalizedRow, error) {
	var payload spreadsheetOutput
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("normalization_failed: spreadsheet output decode: %w", err)
	}
	rows := make([]NormalizedRow, 0, len(payload.Rows))
	for _, row := range payload.Rows {
		byColumn := make(map[string]struct {
			Raw string
			Val string
		}, len(row.Cells))
		for _, cell := range row.Cells {
			letters := spreadsheetCellColumn(cell.A1)
			byColumn[letters] = struct {
				Raw string
				Val string
			}{Raw: cell.RawValue, Val: cell.Value}
		}
		values := make(map[string]NormalizedValue, len(file.Columns))
		for logical, column := range file.Columns {
			item, ok := byColumn[column.Source]
			if !ok {
				if column.Required {
					return nil, fmt.Errorf("required_value_missing: row %d source %q", row.Row, column.Source)
				}
				values[logical] = NormalizedValue{LogicalField: logical, Role: column.Role, Source: column.Source, Empty: true}
				continue
			}
			if column.Required && item.Val == "" {
				return nil, fmt.Errorf("required_value_missing: row %d source %q", row.Row, column.Source)
			}
			values[logical] = NormalizedValue{
				LogicalField: logical,
				Role:         column.Role,
				Source:       column.Source,
				Raw:          item.Raw,
				RawExact:     true,
				Canonical:    item.Val,
				Empty:        item.Val == "",
				Value:        rawJSON(item.Val),
			}
		}
		rows = append(rows, NormalizedRow{
			RowKey:   fmt.Sprintf("row:%d", row.Row),
			Position: RowPosition{Index: row.WindowIndex, Row: row.Row, Sheet: row.Sheet},
			Values:   values,
		})
	}
	return rows, nil
}

type jsonOutput struct {
	RawValue string          `json:"raw_value"`
	Value    json.RawMessage `json:"value"`
}

func normalizeJSONRows(file request.FileConfig, output []byte) ([]NormalizedRow, error) {
	var payload jsonOutput
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("normalization_failed: json output decode: %w", err)
	}
	return normalizeJSONValueRows(file, payload.Value, payload.RawValue)
}

type jsonlOutput struct {
	Records []struct {
		Line        int             `json:"line"`
		WindowIndex int             `json:"window_index"`
		RawValue    string          `json:"raw_value"`
		Value       json.RawMessage `json:"value"`
	} `json:"records"`
}

func normalizeJSONLRows(file request.FileConfig, output []byte) ([]NormalizedRow, error) {
	var payload jsonlOutput
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("normalization_failed: jsonl output decode: %w", err)
	}
	rows := make([]NormalizedRow, 0, len(payload.Records))
	for _, record := range payload.Records {
		values, err := normalizeJSONObject(file, record.Value, record.RawValue)
		if err != nil {
			return nil, err
		}
		rows = append(rows, NormalizedRow{
			RowKey:    fmt.Sprintf("line:%d", record.Line),
			Position:  RowPosition{Index: record.WindowIndex, Line: record.Line},
			RecordRaw: record.RawValue,
			Values:    values,
		})
	}
	return rows, nil
}

func normalizeJSONValueRows(file request.FileConfig, valueRaw json.RawMessage, raw string) ([]NormalizedRow, error) {
	trimmed := strings.TrimSpace(string(valueRaw))
	if trimmed == "" {
		return nil, fmt.Errorf("normalization_failed: empty json value")
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(valueRaw, &items); err != nil {
			return nil, fmt.Errorf("normalization_failed: json array decode: %w", err)
		}
		rows := make([]NormalizedRow, 0, len(items))
		for index, item := range items {
			values, err := normalizeJSONObject(file, item, raw)
			if err != nil {
				return nil, err
			}
			rows = append(rows, NormalizedRow{
				RowKey:    fmt.Sprintf("index:%d", index),
				Position:  RowPosition{Index: index},
				RecordRaw: raw,
				Values:    values,
			})
		}
		return rows, nil
	}
	values, err := normalizeJSONObject(file, valueRaw, raw)
	if err != nil {
		return nil, err
	}
	return []NormalizedRow{{RowKey: "index:0", Position: RowPosition{Index: 0}, RecordRaw: raw, Values: values}}, nil
}

func normalizeJSONObject(file request.FileConfig, valueRaw json.RawMessage, raw string) (map[string]NormalizedValue, error) {
	var anyValue any
	if err := json.Unmarshal(valueRaw, &anyValue); err != nil {
		return nil, fmt.Errorf("normalization_failed: json value decode: %w", err)
	}
	_, isObject := anyValue.(map[string]any)
	if !isObject {
		if len(file.Columns) != 1 {
			return nil, fmt.Errorf("normalization_failed: object field mapping requires json object")
		}
	}
	values := make(map[string]NormalizedValue, len(file.Columns))
	logicalNames := make([]string, 0, len(file.Columns))
	for logical := range file.Columns {
		logicalNames = append(logicalNames, logical)
	}
	sort.Strings(logicalNames)
	for _, logical := range logicalNames {
		column := file.Columns[logical]
		selected, ok, exactRaw, err := jsonSourceValue(anyValue, column.Source, raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			if column.Required {
				return nil, fmt.Errorf("required_value_missing: source %q", column.Source)
			}
			values[logical] = NormalizedValue{LogicalField: logical, Role: column.Role, Source: column.Source, Empty: true}
			continue
		}
		canonical, encoded, err := canonicalJSONValue(selected)
		if err != nil {
			return nil, err
		}
		if column.Required && canonical == "" {
			return nil, fmt.Errorf("required_value_missing: source %q", column.Source)
		}
		rawValue := canonical
		if exactRaw {
			rawValue = raw
		}
		if isObject {
			rawValue = canonical
			exactRaw = false
		}
		values[logical] = NormalizedValue{
			LogicalField: logical,
			Role:         column.Role,
			Source:       column.Source,
			Raw:          rawValue,
			RawExact:     exactRaw,
			Canonical:    canonical,
			Empty:        canonical == "",
			Value:        encoded,
		}
	}
	return values, nil
}

func jsonSourceValue(root any, source, raw string) (any, bool, bool, error) {
	if source == "" {
		return nil, false, false, nil
	}
	if source == "." {
		return root, true, false, nil
	}
	if strings.HasPrefix(source, "/") {
		selected, ok := relativeJSONPointer(root, source)
		return selected, ok, ok && raw != "", nil
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, false, false, nil
	}
	selected, exists := object[source]
	return selected, exists, false, nil
}

func relativeJSONPointer(root any, pointer string) (any, bool) {
	if pointer == "/" {
		return root, true
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	current := root
	for _, part := range parts {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
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

func canonicalJSONValue(value any) (string, json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", nil, fmt.Errorf("normalization_failed: json value encode: %w", err)
	}
	canonicalRaw, err := jcs.Transform(raw)
	if err != nil {
		return "", nil, fmt.Errorf("normalization_failed: json value canonicalize: %w", err)
	}
	var canonical string
	switch typed := value.(type) {
	case nil:
		canonical = "null"
	case string:
		canonical = typed
	case bool:
		if typed {
			canonical = "true"
		} else {
			canonical = "false"
		}
	case float64:
		canonical = strings.TrimSpace(string(canonicalRaw))
	default:
		canonical = string(canonicalRaw)
	}
	return canonical, json.RawMessage(canonicalRaw), nil
}

func spreadsheetCellColumn(a1 string) string {
	for i, ch := range a1 {
		if ch >= '0' && ch <= '9' {
			return a1[:i]
		}
	}
	return a1
}

func rawJSON(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
