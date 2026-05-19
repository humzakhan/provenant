package extractjsonl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
)

const (
	readerName    = "extract.jsonl"
	readerVersion = "v1"
)

var (
	ErrMissingSourceBytes  = errors.New("extractjsonl: missing source bytes")
	ErrMissingSourceSHA256 = errors.New("extractjsonl: missing source sha256")
	ErrSourceHashMismatch  = errors.New("extractjsonl: source hash mismatch")
	ErrUnsupportedSelector = errors.New("extractjsonl: unsupported selector metadata")
	ErrInvalidLineWindow   = errors.New("extractjsonl: invalid line window")
	ErrInvalidCursor       = errors.New("extractjsonl: invalid cursor")
	ErrInvalidJSONPointer  = errors.New("extractjsonl: invalid json pointer")
	ErrInvalidJSONPath     = errors.New("extractjsonl: invalid jsonpath")
	ErrNonUniqueJSONPath   = errors.New("extractjsonl: non-unique jsonpath")
	ErrInvalidByteSpan     = errors.New("extractjsonl: invalid byte span")
	ErrSelectorMismatch    = errors.New("extractjsonl: selector mismatch")
	ErrByteSpanMismatch    = errors.New("extractjsonl: byte span mismatch")
	ErrInvalidJSONL        = errors.New("extractjsonl: invalid jsonl")
	ErrSelectorNotFound    = errors.New("extractjsonl: selector not found")
	ErrAmbiguousByteSpan   = errors.New("extractjsonl: byte span requires exactly one selected record")
)

type Reader struct{}

type lineWindow struct {
	DeclaredStart  int
	DeclaredEnd    int
	EffectiveStart int
	EffectiveEnd   int
	PageSize       int
	InputCursor    string
	NextCursor     string
}

type lineRecord struct {
	Line          int
	WindowIndex   int
	RecordStart   int
	RecordEnd     int
	SelectedStart int
	SelectedEnd   int
	JSONPointer   string
	JSONPath      string
	RawValue      string
	Value         value
}

type recordEnvelope struct {
	ByteEnd         int    `json:"byte_end"`
	ByteStart       int    `json:"byte_start"`
	JSONPointer     string `json:"json_pointer,omitempty"`
	JSONPath        string `json:"jsonpath,omitempty"`
	Line            int    `json:"line"`
	NormalizedValue value  `json:"normalized_value"`
	RawValue        string `json:"raw_value"`
	Value           value  `json:"value"`
	WindowIndex     int    `json:"window_index"`
}

type outputEnvelope struct {
	Cursor             string           `json:"cursor,omitempty"`
	EffectiveLineEnd   int              `json:"effective_line_end"`
	EffectiveLineStart int              `json:"effective_line_start"`
	LineWindow         string           `json:"line_window"`
	NextCursor         string           `json:"next_cursor,omitempty"`
	NormalizedValue    *value           `json:"normalized_value,omitempty"`
	RawValue           string           `json:"raw_value,omitempty"`
	RecordCount        int              `json:"record_count"`
	RecordOrder        string           `json:"record_order"`
	Records            []recordEnvelope `json:"records"`
	SourceHash         string           `json:"source_hash"`
	Value              *value           `json:"value,omitempty"`
}

type locatorEnvelope struct {
	Cursor             string          `json:"cursor,omitempty"`
	EffectiveLineEnd   int             `json:"effective_line_end"`
	EffectiveLineStart int             `json:"effective_line_start"`
	JSONPointer        string          `json:"json_pointer,omitempty"`
	JSONPath           string          `json:"jsonpath,omitempty"`
	LineWindow         string          `json:"line_window"`
	NextCursor         string          `json:"next_cursor,omitempty"`
	RecordCount        int             `json:"record_count"`
	RecordOrder        string          `json:"record_order"`
	Records            []locatorRecord `json:"records"`
}

type locatorRecord struct {
	ByteEnd        int `json:"byte_end"`
	ByteStart      int `json:"byte_start"`
	Line           int `json:"line"`
	ValueByteEnd   int `json:"value_byte_end"`
	ValueByteStart int `json:"value_byte_start"`
	WindowIndex    int `json:"window_index"`
}

type readerOptionsEnvelope struct {
	ByteSpanValidated bool   `json:"byte_span_validated,omitempty"`
	CursorApplied     bool   `json:"cursor_applied,omitempty"`
	PageSize          int    `json:"page_size"`
	ProjectionMode    string `json:"projection_mode"`
	RecordOrder       string `json:"record_order"`
	SelectorMode      string `json:"selector_mode"`
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

	window, err := parseLineWindow(selector.LineWindow, selector.Cursor)
	if err != nil {
		return kernel.ReadResult{}, err
	}

	lines, totalLines, err := selectLines(source.Bytes, window)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	if len(lines) == 0 {
		return kernel.ReadResult{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, selector.LineWindow)
	}
	window.NextCursor = nextCursor(lines[len(lines)-1].Line, totalLines)

	records, err := projectRecords(lines, selector)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	if selector.ByteSpan != "" {
		if len(records) != 1 {
			return kernel.ReadResult{}, ErrAmbiguousByteSpan
		}
		start, end, err := parseByteSpan(selector.ByteSpan)
		if err != nil {
			return kernel.ReadResult{}, err
		}
		if start != records[0].SelectedStart || end != records[0].SelectedEnd {
			return kernel.ReadResult{}, fmt.Errorf("%w: declared %s resolved %d-%d", ErrByteSpanMismatch, selector.ByteSpan, records[0].SelectedStart, records[0].SelectedEnd)
		}
	}

	output, err := marshalOutput(source.Source.SHA256, window, records)
	if err != nil {
		return kernel.ReadResult{}, fmt.Errorf("extractjsonl: marshal output: %w", err)
	}
	outputHash := sha256.Sum256(output)
	outputSHA256 := hex.EncodeToString(outputHash[:])

	captured := kernel.CapturedRead{
		ID:            readID(run.ID, source.Source.Path, selector, source.Source.SHA256, outputSHA256),
		RunID:         run.ID,
		SourcePath:    source.Source.Path,
		Origin:        origin(source.Source.Path, selector, records),
		Selector:      selector,
		ReaderName:    readerName,
		ReaderVersion: readerVersion,
		ReaderOptions: readerOptions(selector, window),
		Locator:       locator(selector, window, records),
		SourceSHA256:  source.Source.SHA256,
		OutputSHA256:  outputSHA256,
		Output:        output,
	}

	return kernel.ReadResult{
		Output:       output,
		CapturedRead: captured,
	}, nil
}

type sourceLine struct {
	Line  int
	Start int
	End   int
	Bytes []byte
}

func validateSelector(selector kernel.SourceSelector) error {
	if err := selector.ValidatePhaseTwoSelector(); err != nil {
		return ErrUnsupportedSelector
	}
	if selector.LineWindow == "" {
		return ErrInvalidLineWindow
	}
	if selector.RowWindow != "" ||
		selector.ColumnProjection != "" ||
		selector.Sheet != "" ||
		selector.Range != "" ||
		selector.TableName != "" ||
		selector.HeaderRow != 0 ||
		selector.Dialect != "" ||
		selector.Encoding != "" ||
		selector.FormulaPolicy != "" {
		return ErrUnsupportedSelector
	}
	return nil
}

func parseLineWindow(raw, cursor string) (lineWindow, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidLineWindow, raw)
	}
	start, err := parseIndex(parts[0])
	if err != nil || start == 0 {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidLineWindow, raw)
	}
	end, err := parseIndex(parts[1])
	if err != nil || end < start {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidLineWindow, raw)
	}
	window := lineWindow{
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
	if !strings.HasPrefix(cursor, "line:") {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidCursor, cursor)
	}
	lastLine, err := parseIndex(strings.TrimPrefix(cursor, "line:"))
	if err != nil {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidCursor, cursor)
	}
	if lastLine <= 0 {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidCursor, cursor)
	}
	window.EffectiveStart = lastLine + 1
	window.EffectiveEnd = lastLine + window.PageSize
	if window.EffectiveStart <= 0 || window.EffectiveEnd < window.EffectiveStart {
		return lineWindow{}, fmt.Errorf("%w: %s", ErrInvalidCursor, cursor)
	}
	return window, nil
}

func selectLines(data []byte, window lineWindow) ([]sourceLine, int, error) {
	lines := make([]sourceLine, 0, window.PageSize)
	lineNumber := 1
	lineStart := 0

	for index := 0; index <= len(data); index++ {
		atEnd := index == len(data)
		if !atEnd && data[index] != '\n' {
			continue
		}
		if atEnd && lineStart == len(data) {
			return lines, lineNumber - 1, nil
		}

		lineEnd := index
		if lineEnd > lineStart && data[lineEnd-1] == '\r' {
			lineEnd--
		}
		lineBytes := data[lineStart:lineEnd]
		if len(lineBytes) == 0 || len(lineBytes) != len(bytesTrimSpaceASCII(lineBytes)) {
			return nil, 0, fmt.Errorf("%w: line %d", ErrInvalidJSONL, lineNumber)
		}

		if lineNumber >= window.EffectiveStart && lineNumber <= window.EffectiveEnd {
			lines = append(lines, sourceLine{
				Line:  lineNumber,
				Start: lineStart,
				End:   lineEnd,
				Bytes: lineBytes,
			})
		}
		if atEnd {
			return lines, lineNumber, nil
		}
		lineNumber++
		lineStart = index + 1
	}
	return lines, lineNumber, nil
}

func bytesTrimSpaceASCII(data []byte) []byte {
	start := 0
	for start < len(data) {
		switch data[start] {
		case ' ', '\t', '\n', '\r':
			start++
		default:
			goto trailing
		}
	}
	return nil

trailing:
	end := len(data)
	for end > start {
		switch data[end-1] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			return data[start:end]
		}
	}
	return nil
}

func projectRecords(lines []sourceLine, selector kernel.SourceSelector) ([]lineRecord, error) {
	records := make([]lineRecord, 0, len(lines))
	for index, line := range lines {
		root, err := decodeValue(line.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d", err, line.Line)
		}

		selected, err := resolveProjection(root, selector)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d", err, line.Line)
		}

		records = append(records, lineRecord{
			Line:          line.Line,
			WindowIndex:   index,
			RecordStart:   line.Start,
			RecordEnd:     line.End,
			SelectedStart: line.Start + selected.start,
			SelectedEnd:   line.Start + selected.end,
			JSONPointer:   recordJSONPointer(selector, selected.pointer),
			JSONPath:      recordJSONPath(selector),
			RawValue:      string(line.Bytes[selected.start:selected.end]),
			Value:         selected.value,
		})
	}
	return records, nil
}

func resolveProjection(root value, selector kernel.SourceSelector) (selectedValue, error) {
	selected, err := resolvePointer(root, selector.JSONPointer)
	if err != nil {
		return selectedValue{}, err
	}
	if selector.JSONPath == "" {
		return selected, nil
	}
	pathSelected, err := resolveJSONPath(root, selector.JSONPath)
	if err != nil {
		return selectedValue{}, err
	}
	if selector.JSONPointer != "" && pathSelected.pointer != selected.pointer {
		return selectedValue{}, fmt.Errorf("%w: json_pointer=%s jsonpath=%s", ErrSelectorMismatch, selector.JSONPointer, selector.JSONPath)
	}
	return pathSelected, nil
}

func marshalOutput(sourceHash string, window lineWindow, records []lineRecord) ([]byte, error) {
	payloadRecords := make([]recordEnvelope, 0, len(records))
	for _, record := range records {
		payloadRecords = append(payloadRecords, recordEnvelope{
			ByteEnd:         record.SelectedEnd,
			ByteStart:       record.SelectedStart,
			JSONPointer:     record.JSONPointer,
			JSONPath:        record.JSONPath,
			Line:            record.Line,
			NormalizedValue: record.Value,
			RawValue:        record.RawValue,
			Value:           record.Value,
			WindowIndex:     record.WindowIndex,
		})
	}

	payload := outputEnvelope{
		Cursor:             window.InputCursor,
		EffectiveLineEnd:   records[len(records)-1].Line,
		EffectiveLineStart: records[0].Line,
		LineWindow:         fmt.Sprintf("%d-%d", window.DeclaredStart, window.DeclaredEnd),
		NextCursor:         window.NextCursor,
		RecordCount:        len(payloadRecords),
		RecordOrder:        "source_line_asc",
		Records:            payloadRecords,
		SourceHash:         sourceHash,
	}
	if len(records) == 1 {
		payload.NormalizedValue = &records[0].Value
		payload.RawValue = records[0].RawValue
		payload.Value = &records[0].Value
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}

func locator(selector kernel.SourceSelector, window lineWindow, records []lineRecord) json.RawMessage {
	locatorRecords := make([]locatorRecord, 0, len(records))
	for _, record := range records {
		locatorRecords = append(locatorRecords, locatorRecord{
			ByteEnd:        record.RecordEnd,
			ByteStart:      record.RecordStart,
			Line:           record.Line,
			ValueByteEnd:   record.SelectedEnd,
			ValueByteStart: record.SelectedStart,
			WindowIndex:    record.WindowIndex,
		})
	}
	raw, _ := json.Marshal(locatorEnvelope{
		Cursor:             window.InputCursor,
		EffectiveLineEnd:   records[len(records)-1].Line,
		EffectiveLineStart: records[0].Line,
		JSONPointer:        selector.JSONPointer,
		JSONPath:           selector.JSONPath,
		LineWindow:         fmt.Sprintf("%d-%d", window.DeclaredStart, window.DeclaredEnd),
		NextCursor:         window.NextCursor,
		RecordCount:        len(records),
		RecordOrder:        "source_line_asc",
		Records:            locatorRecords,
	})
	return canonicalRawMessage(raw)
}

func readerOptions(selector kernel.SourceSelector, window lineWindow) json.RawMessage {
	raw, _ := json.Marshal(readerOptionsEnvelope{
		ByteSpanValidated: selector.ByteSpan != "",
		CursorApplied:     selector.Cursor != "",
		PageSize:          window.PageSize,
		ProjectionMode:    projectionMode(selector),
		RecordOrder:       "source_line_asc",
		SelectorMode:      "line_window",
	})
	return canonicalRawMessage(raw)
}

func projectionMode(selector kernel.SourceSelector) string {
	switch {
	case selector.JSONPointer != "" && selector.JSONPath != "":
		return "json_pointer+jsonpath"
	case selector.JSONPointer != "":
		return "json_pointer"
	case selector.JSONPath != "":
		return "jsonpath"
	default:
		return "record"
	}
}

func recordJSONPointer(selector kernel.SourceSelector, resolvedPointer string) string {
	if selector.JSONPointer == "" {
		return ""
	}
	return resolvedPointer
}

func recordJSONPath(selector kernel.SourceSelector) string {
	return selector.JSONPath
}

func nextCursor(lastLine, totalLines int) string {
	if lastLine >= totalLines {
		return ""
	}
	return fmt.Sprintf("line:%d", lastLine)
}

func canonicalRawMessage(raw []byte) json.RawMessage {
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return json.RawMessage(raw)
	}
	return json.RawMessage(canonical)
}

func origin(path string, selector kernel.SourceSelector, records []lineRecord) string {
	parts := []string{fmt.Sprintf("line=%d-%d", records[0].Line, records[len(records)-1].Line)}
	if selector.JSONPointer != "" {
		parts = append(parts, "ptr="+selector.JSONPointer)
	}
	if selector.JSONPath != "" {
		parts = append(parts, "jsonpath="+selector.JSONPath)
	}
	if len(records) == 1 {
		parts = append(parts, fmt.Sprintf("byte=%d-%d", records[0].SelectedStart, records[0].SelectedEnd))
	}
	return path + "#" + strings.Join(parts, "&")
}

func readID(runID, sourcePath string, selector kernel.SourceSelector, sourceSHA256, outputSHA256 string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		readerName,
		readerVersion,
		runID,
		sourcePath,
		selector.LineWindow,
		selector.Cursor,
		selector.JSONPointer,
		selector.JSONPath,
		selector.ByteSpan,
		sourceSHA256,
		outputSHA256,
	}, "\n")))
	return "read_" + hex.EncodeToString(sum[:16])
}
