package extractjson

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
)

const (
	readerName    = "extract.json"
	readerVersion = "v1"
)

var (
	ErrMissingSourceBytes  = errors.New("extractjson: missing source bytes")
	ErrMissingSourceSHA256 = errors.New("extractjson: missing source sha256")
	ErrSourceHashMismatch  = errors.New("extractjson: source hash mismatch")
	ErrMissingJSONPointer  = errors.New("extractjson: missing json pointer")
	ErrUnsupportedSelector = errors.New("extractjson: unsupported selector metadata")
	ErrInvalidJSONPointer  = errors.New("extractjson: invalid json pointer")
	ErrInvalidJSONPath     = errors.New("extractjson: invalid jsonpath")
	ErrNonUniqueJSONPath   = errors.New("extractjson: non-unique jsonpath")
	ErrInvalidByteSpan     = errors.New("extractjson: invalid byte span")
	ErrSelectorMismatch    = errors.New("extractjson: selector mismatch")
	ErrByteSpanMismatch    = errors.New("extractjson: byte span mismatch")
	ErrInvalidJSON         = errors.New("extractjson: invalid json")
	ErrSelectorNotFound    = errors.New("extractjson: selector not found")
)

type Reader struct{}

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

	root, err := decodeValue(source.Bytes)
	if err != nil {
		return kernel.ReadResult{}, err
	}

	selected, err := resolvePointer(root, selector.JSONPointer)
	if err != nil {
		return kernel.ReadResult{}, err
	}
	if selector.JSONPath != "" {
		pathSelected, err := resolveJSONPath(root, selector.JSONPath)
		if err != nil {
			return kernel.ReadResult{}, err
		}
		if pathSelected.pointer != selected.pointer {
			return kernel.ReadResult{}, fmt.Errorf("%w: json_pointer=%s jsonpath=%s", ErrSelectorMismatch, selector.JSONPointer, selector.JSONPath)
		}
	}
	if selector.ByteSpan != "" {
		byteStart, byteEnd, err := parseByteSpan(selector.ByteSpan)
		if err != nil {
			return kernel.ReadResult{}, err
		}
		if byteStart != selected.start || byteEnd != selected.end {
			return kernel.ReadResult{}, fmt.Errorf("%w: declared %s resolved %d-%d", ErrByteSpanMismatch, selector.ByteSpan, selected.start, selected.end)
		}
	}

	output, err := marshalOutput(source.Source.SHA256, selector.JSONPointer, selector.JSONPath, source.Bytes[selected.start:selected.end], selected.value)
	if err != nil {
		return kernel.ReadResult{}, fmt.Errorf("extractjson: marshal output: %w", err)
	}

	outputHash := sha256.Sum256(output)
	outputSHA256 := hex.EncodeToString(outputHash[:])

	captured := kernel.CapturedRead{
		ID:            readID(run.ID, source.Source.Path, selector.JSONPointer, source.Source.SHA256, outputSHA256),
		RunID:         run.ID,
		SourcePath:    source.Source.Path,
		Origin:        origin(source.Source.Path, selector.JSONPointer, selected.start, selected.end),
		Selector:      selector,
		ReaderName:    readerName,
		ReaderVersion: readerVersion,
		ReaderOptions: readerOptions(selector),
		Locator:       locator(selector.JSONPointer, selector.JSONPath, selected.start, selected.end),
		SourceSHA256:  source.Source.SHA256,
		OutputSHA256:  outputSHA256,
		Output:        output,
	}

	return kernel.ReadResult{
		Output:       output,
		CapturedRead: captured,
	}, nil
}

type outputEnvelope struct {
	JSONPointer     string `json:"json_pointer"`
	JSONPath        string `json:"jsonpath,omitempty"`
	ByteStart       int    `json:"byte_start"`
	ByteEnd         int    `json:"byte_end"`
	RawValue        string `json:"raw_value"`
	NormalizedValue value  `json:"normalized_value"`
	SourceHash      string `json:"source_hash"`
	Value           value  `json:"value"`
}

func marshalOutput(sourceHash, pointer, path string, raw []byte, selected value) ([]byte, error) {
	output, err := json.Marshal(outputEnvelope{
		JSONPointer:     pointer,
		JSONPath:        path,
		ByteStart:       selected.start,
		ByteEnd:         selected.end,
		RawValue:        string(raw),
		NormalizedValue: selected,
		SourceHash:      sourceHash,
		Value:           selected,
	})
	if err != nil {
		return nil, err
	}
	return jcs.Transform(output)
}

type locatorEnvelope struct {
	JSONPointer string `json:"json_pointer"`
	JSONPath    string `json:"jsonpath,omitempty"`
	ByteStart   int    `json:"byte_start"`
	ByteEnd     int    `json:"byte_end"`
}

func locator(pointer, path string, byteStart, byteEnd int) json.RawMessage {
	raw, _ := json.Marshal(locatorEnvelope{
		JSONPointer: pointer,
		JSONPath:    path,
		ByteStart:   byteStart,
		ByteEnd:     byteEnd,
	})
	return canonicalRawMessage(raw)
}

type readerOptionsEnvelope struct {
	SelectorMode      string `json:"selector_mode"`
	JSONPathValidated bool   `json:"jsonpath_validated,omitempty"`
	ByteSpanValidated bool   `json:"byte_span_validated,omitempty"`
}

func readerOptions(selector kernel.SourceSelector) json.RawMessage {
	raw, _ := json.Marshal(readerOptionsEnvelope{
		SelectorMode:      "json_pointer",
		JSONPathValidated: selector.JSONPath != "",
		ByteSpanValidated: selector.ByteSpan != "",
	})
	return canonicalRawMessage(raw)
}

func canonicalRawMessage(raw []byte) json.RawMessage {
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return json.RawMessage(raw)
	}
	return json.RawMessage(canonical)
}

func origin(path, pointer string, byteStart, byteEnd int) string {
	if pointer == "" && byteStart == 0 && byteEnd == 0 {
		return path
	}
	return fmt.Sprintf("%s#ptr=%s&byte=%d-%d", path, pointer, byteStart, byteEnd)
}

func readID(runID, sourcePath, pointer, sourceSHA256, outputSHA256 string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		readerName,
		readerVersion,
		runID,
		sourcePath,
		pointer,
		sourceSHA256,
		outputSHA256,
	}, "\n")))
	return "read_" + hex.EncodeToString(sum[:16])
}

func validateSelector(selector kernel.SourceSelector) error {
	if selector.JSONPointer == "" &&
		selector.JSONPath == "" &&
		selector.ByteSpan == "" &&
		selector.LineWindow == "" &&
		selector.Cursor == "" &&
		selector.RowWindow == "" &&
		selector.ColumnProjection == "" &&
		selector.Sheet == "" &&
		selector.Range == "" &&
		selector.TableName == "" &&
		selector.HeaderRow == 0 &&
		selector.Dialect == "" &&
		selector.Encoding == "" &&
		selector.FormulaPolicy == "" {
		return ErrMissingJSONPointer
	}
	if selector.LineWindow != "" ||
		selector.Cursor != "" ||
		selector.RowWindow != "" ||
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
	if selector.JSONPointer == "" {
		return ErrMissingJSONPointer
	}
	return nil
}

type valueKind uint8

const (
	kindNull valueKind = iota
	kindBool
	kindNumber
	kindString
	kindArray
	kindObject
)

type value struct {
	kind    valueKind
	boolean bool
	number  string
	text    string
	array   []value
	object  []field
	start   int
	end     int
}

type field struct {
	key   string
	value value
}

func decodeValue(data []byte) (value, error) {
	index := skipWhitespace(data, 0)
	if index >= len(data) {
		return value{}, ErrInvalidJSON
	}
	v, next, err := parseValue(data, index)
	if err != nil {
		return value{}, err
	}
	next = skipWhitespace(data, next)
	if next != len(data) {
		return value{}, fmt.Errorf("%w: trailing data", ErrInvalidJSON)
	}
	return v, nil
}

func parseValue(data []byte, index int) (value, int, error) {
	if index >= len(data) {
		return value{}, index, ErrInvalidJSON
	}
	switch data[index] {
	case '{':
		return parseObject(data, index)
	case '[':
		return parseArray(data, index)
	case '"':
		text, next, err := parseJSONString(data, index)
		if err != nil {
			return value{}, index, err
		}
		return value{kind: kindString, text: text, start: index, end: next}, next, nil
	case 't':
		return parseLiteral(data, index, "true", value{kind: kindBool, boolean: true})
	case 'f':
		return parseLiteral(data, index, "false", value{kind: kindBool, boolean: false})
	case 'n':
		return parseLiteral(data, index, "null", value{kind: kindNull})
	default:
		if data[index] == '-' || (data[index] >= '0' && data[index] <= '9') {
			number, next, err := parseNumber(data, index)
			if err != nil {
				return value{}, index, err
			}
			return value{kind: kindNumber, number: number, start: index, end: next}, next, nil
		}
		return value{}, index, fmt.Errorf("%w: unexpected byte %q", ErrInvalidJSON, data[index])
	}
}

func parseObject(data []byte, index int) (value, int, error) {
	start := index
	index++
	index = skipWhitespace(data, index)
	seen := make(map[string]struct{})
	fields := make([]field, 0)
	if index < len(data) && data[index] == '}' {
		return value{kind: kindObject, object: fields, start: start, end: index + 1}, index + 1, nil
	}

	for {
		key, next, err := parseJSONString(data, index)
		if err != nil {
			return value{}, index, err
		}
		if _, exists := seen[key]; exists {
			return value{}, index, fmt.Errorf("%w: duplicate object key %q", ErrInvalidJSON, key)
		}
		seen[key] = struct{}{}
		index = skipWhitespace(data, next)
		if index >= len(data) || data[index] != ':' {
			return value{}, index, fmt.Errorf("%w: missing colon", ErrInvalidJSON)
		}
		index = skipWhitespace(data, index+1)
		child, next, err := parseValue(data, index)
		if err != nil {
			return value{}, index, err
		}
		fields = append(fields, field{key: key, value: child})
		index = skipWhitespace(data, next)
		if index >= len(data) {
			return value{}, index, fmt.Errorf("%w: unterminated object", ErrInvalidJSON)
		}
		if data[index] == '}' {
			return value{kind: kindObject, object: fields, start: start, end: index + 1}, index + 1, nil
		}
		if data[index] != ',' {
			return value{}, index, fmt.Errorf("%w: expected comma", ErrInvalidJSON)
		}
		index = skipWhitespace(data, index+1)
	}
}

func parseArray(data []byte, index int) (value, int, error) {
	start := index
	index++
	index = skipWhitespace(data, index)
	items := make([]value, 0)
	if index < len(data) && data[index] == ']' {
		return value{kind: kindArray, array: items, start: start, end: index + 1}, index + 1, nil
	}

	for {
		child, next, err := parseValue(data, index)
		if err != nil {
			return value{}, index, err
		}
		items = append(items, child)
		index = skipWhitespace(data, next)
		if index >= len(data) {
			return value{}, index, fmt.Errorf("%w: unterminated array", ErrInvalidJSON)
		}
		if data[index] == ']' {
			return value{kind: kindArray, array: items, start: start, end: index + 1}, index + 1, nil
		}
		if data[index] != ',' {
			return value{}, index, fmt.Errorf("%w: expected comma", ErrInvalidJSON)
		}
		index = skipWhitespace(data, index+1)
	}
}

func parseLiteral(data []byte, index int, literal string, v value) (value, int, error) {
	if !strings.HasPrefix(string(data[index:]), literal) {
		return value{}, index, fmt.Errorf("%w: invalid literal", ErrInvalidJSON)
	}
	v.start = index
	v.end = index + len(literal)
	return v, v.end, nil
}

func parseJSONString(data []byte, index int) (string, int, error) {
	if index >= len(data) || data[index] != '"' {
		return "", index, fmt.Errorf("%w: expected string", ErrInvalidJSON)
	}
	end := index + 1
	for end < len(data) {
		switch data[end] {
		case '"':
			if !isEscaped(data[index+1 : end]) {
				var text string
				if err := json.Unmarshal(data[index:end+1], &text); err != nil {
					return "", index, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
				}
				return text, end + 1, nil
			}
		case '\n', '\r':
			return "", index, fmt.Errorf("%w: unterminated string", ErrInvalidJSON)
		}
		end++
	}
	return "", index, fmt.Errorf("%w: unterminated string", ErrInvalidJSON)
}

func isEscaped(data []byte) bool {
	count := 0
	for i := len(data) - 1; i >= 0 && data[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func parseNumber(data []byte, index int) (string, int, error) {
	start := index
	if data[index] == '-' {
		index++
		if index >= len(data) {
			return "", start, fmt.Errorf("%w: invalid number", ErrInvalidJSON)
		}
	}
	if data[index] == '0' {
		index++
		if index < len(data) && data[index] >= '0' && data[index] <= '9' {
			return "", start, fmt.Errorf("%w: invalid number", ErrInvalidJSON)
		}
	} else if data[index] >= '1' && data[index] <= '9' {
		for index < len(data) && data[index] >= '0' && data[index] <= '9' {
			index++
		}
	} else {
		return "", start, fmt.Errorf("%w: invalid number", ErrInvalidJSON)
	}
	if index < len(data) && data[index] == '.' {
		index++
		if index >= len(data) || data[index] < '0' || data[index] > '9' {
			return "", start, fmt.Errorf("%w: invalid number", ErrInvalidJSON)
		}
		for index < len(data) && data[index] >= '0' && data[index] <= '9' {
			index++
		}
	}
	if index < len(data) && (data[index] == 'e' || data[index] == 'E') {
		index++
		if index < len(data) && (data[index] == '+' || data[index] == '-') {
			index++
		}
		if index >= len(data) || data[index] < '0' || data[index] > '9' {
			return "", start, fmt.Errorf("%w: invalid number", ErrInvalidJSON)
		}
		for index < len(data) && data[index] >= '0' && data[index] <= '9' {
			index++
		}
	}
	return string(data[start:index]), index, nil
}

func skipWhitespace(data []byte, index int) int {
	for index < len(data) {
		switch data[index] {
		case ' ', '\t', '\n', '\r':
			index++
		default:
			return index
		}
	}
	return index
}

type selectedValue struct {
	value
	pointer string
}

func resolvePointer(root value, pointer string) (selectedValue, error) {
	if pointer == "" {
		return selectedValue{}, fmt.Errorf("%w: empty", ErrInvalidJSONPointer)
	}
	if !strings.HasPrefix(pointer, "/") {
		return selectedValue{}, fmt.Errorf("%w: %q", ErrInvalidJSONPointer, pointer)
	}

	current := root
	for _, rawSegment := range strings.Split(pointer[1:], "/") {
		segment, err := decodeReferenceToken(rawSegment)
		if err != nil {
			return selectedValue{}, err
		}
		switch current.kind {
		case kindObject:
			found := false
			for _, field := range current.object {
				if field.key == segment {
					current = field.value
					found = true
					break
				}
			}
			if !found {
				return selectedValue{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, pointer)
			}
		case kindArray:
			index, err := parseIndex(segment)
			if err != nil || index >= len(current.array) {
				return selectedValue{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, pointer)
			}
			current = current.array[index]
		default:
			return selectedValue{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, pointer)
		}
	}

	return selectedValue{value: current, pointer: pointer}, nil
}

func parseIndex(segment string) (int, error) {
	if segment == "" {
		return 0, errors.New("empty index")
	}
	if len(segment) > 1 && segment[0] == '0' {
		return 0, errors.New("leading zero")
	}
	index := 0
	for _, r := range segment {
		if r < '0' || r > '9' {
			return 0, errors.New("not numeric")
		}
		digit := int(r - '0')
		if index > (math.MaxInt-digit)/10 {
			return 0, errors.New("index overflow")
		}
		index = index*10 + digit
	}
	return index, nil
}

func decodeReferenceToken(segment string) (string, error) {
	if !strings.Contains(segment, "~") {
		return segment, nil
	}

	var b strings.Builder
	b.Grow(len(segment))

	for i := 0; i < len(segment); i++ {
		if segment[i] != '~' {
			b.WriteByte(segment[i])
			continue
		}
		if i+1 >= len(segment) {
			return "", fmt.Errorf("%w: %q", ErrInvalidJSONPointer, segment)
		}
		switch segment[i+1] {
		case '0':
			b.WriteByte('~')
		case '1':
			b.WriteByte('/')
		default:
			return "", fmt.Errorf("%w: %q", ErrInvalidJSONPointer, segment)
		}
		i++
	}

	return b.String(), nil
}

func encodeReferenceToken(segment string) string {
	replacer := strings.NewReplacer("~", "~0", "/", "~1")
	return replacer.Replace(segment)
}

type jsonPathSegmentKind uint8

const (
	jsonPathField jsonPathSegmentKind = iota
	jsonPathIndex
	jsonPathWildcard
)

type jsonPathSegment struct {
	kind  jsonPathSegmentKind
	field string
	index int
}

func resolveJSONPath(root value, path string) (selectedValue, error) {
	segments, err := parseJSONPath(path)
	if err != nil {
		return selectedValue{}, err
	}

	current := []selectedValue{{value: root, pointer: ""}}
	for _, segment := range segments {
		next := make([]selectedValue, 0)
		for _, candidate := range current {
			switch segment.kind {
			case jsonPathField:
				if candidate.kind != kindObject {
					continue
				}
				for _, field := range candidate.object {
					if field.key == segment.field {
						next = append(next, selectedValue{
							value:   field.value,
							pointer: candidate.pointer + "/" + encodeReferenceToken(segment.field),
						})
						break
					}
				}
			case jsonPathIndex:
				if candidate.kind != kindArray || segment.index >= len(candidate.array) {
					continue
				}
				next = append(next, selectedValue{
					value:   candidate.array[segment.index],
					pointer: candidate.pointer + "/" + strconv.Itoa(segment.index),
				})
			case jsonPathWildcard:
				switch candidate.kind {
				case kindObject:
					for _, field := range candidate.object {
						next = append(next, selectedValue{
							value:   field.value,
							pointer: candidate.pointer + "/" + encodeReferenceToken(field.key),
						})
					}
				case kindArray:
					for index, item := range candidate.array {
						next = append(next, selectedValue{
							value:   item,
							pointer: candidate.pointer + "/" + strconv.Itoa(index),
						})
					}
				}
			}
		}
		current = next
	}

	switch len(current) {
	case 0:
		return selectedValue{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, path)
	case 1:
		return current[0], nil
	default:
		return selectedValue{}, fmt.Errorf("%w: %s", ErrNonUniqueJSONPath, path)
	}
}

func parseJSONPath(path string) ([]jsonPathSegment, error) {
	if path == "" || path[0] != '$' {
		return nil, fmt.Errorf("%w: %q", ErrInvalidJSONPath, path)
	}
	if path == "$" {
		return nil, fmt.Errorf("%w: root-only path unsupported", ErrInvalidJSONPath)
	}

	segments := make([]jsonPathSegment, 0)
	for index := 1; index < len(path); {
		switch path[index] {
		case '.':
			index++
			start := index
			for index < len(path) && isJSONPathIdent(path[index]) {
				index++
			}
			if start == index {
				return nil, fmt.Errorf("%w: %q", ErrInvalidJSONPath, path)
			}
			segments = append(segments, jsonPathSegment{
				kind:  jsonPathField,
				field: path[start:index],
			})
		case '[':
			index++
			if index >= len(path) {
				return nil, fmt.Errorf("%w: %q", ErrInvalidJSONPath, path)
			}
			if path[index] == '*' {
				index++
				if index >= len(path) || path[index] != ']' {
					return nil, fmt.Errorf("%w: %q", ErrInvalidJSONPath, path)
				}
				index++
				segments = append(segments, jsonPathSegment{kind: jsonPathWildcard})
				continue
			}
			start := index
			for index < len(path) && path[index] >= '0' && path[index] <= '9' {
				index++
			}
			if start == index || index >= len(path) || path[index] != ']' {
				return nil, fmt.Errorf("%w: unsupported syntax %q", ErrInvalidJSONPath, path)
			}
			value, err := parseIndex(path[start:index])
			if err != nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalidJSONPath, path)
			}
			index++
			segments = append(segments, jsonPathSegment{kind: jsonPathIndex, index: value})
		default:
			return nil, fmt.Errorf("%w: unsupported syntax %q", ErrInvalidJSONPath, path)
		}
	}
	return segments, nil
}

func isJSONPathIdent(ch byte) bool {
	return ch == '_' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}

func parseByteSpan(span string) (int, int, error) {
	parts := strings.Split(span, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("%w: %q", ErrInvalidByteSpan, span)
	}
	start, err := parseIndex(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %q", ErrInvalidByteSpan, span)
	}
	end, err := parseIndex(parts[1])
	if err != nil || end <= start {
		return 0, 0, fmt.Errorf("%w: %q", ErrInvalidByteSpan, span)
	}
	return start, end, nil
}

func (v value) MarshalJSON() ([]byte, error) {
	switch v.kind {
	case kindNull:
		return []byte("null"), nil
	case kindBool:
		if v.boolean {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case kindNumber:
		return []byte(v.number), nil
	case kindString:
		return json.Marshal(v.text)
	case kindArray:
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range v.array {
			if i > 0 {
				b.WriteByte(',')
			}
			data, err := item.MarshalJSON()
			if err != nil {
				return nil, err
			}
			b.Write(data)
		}
		b.WriteByte(']')
		return []byte(b.String()), nil
	case kindObject:
		var b strings.Builder
		b.WriteByte('{')
		for i, item := range v.object {
			if i > 0 {
				b.WriteByte(',')
			}
			key, err := json.Marshal(item.key)
			if err != nil {
				return nil, err
			}
			data, err := item.value.MarshalJSON()
			if err != nil {
				return nil, err
			}
			b.Write(key)
			b.WriteByte(':')
			b.Write(data)
		}
		b.WriteByte('}')
		return []byte(b.String()), nil
	default:
		return nil, fmt.Errorf("extractjson: unknown value kind %d", v.kind)
	}
}
