package extractjson

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

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

	root, err := decodeValue(json.NewDecoder(bytes.NewReader(source.Bytes)))
	if err != nil {
		return kernel.ReadResult{}, err
	}

	selected, err := resolvePointer(root, selector.JSONPointer)
	if err != nil {
		return kernel.ReadResult{}, err
	}

	output, err := marshalOutput(selector.JSONPointer, selected)
	if err != nil {
		return kernel.ReadResult{}, fmt.Errorf("extractjson: marshal output: %w", err)
	}

	outputHash := sha256.Sum256(output)
	outputSHA256 := hex.EncodeToString(outputHash[:])

	captured := kernel.CapturedRead{
		ID:            readID(run.ID, source.Source.Path, selector.JSONPointer, source.Source.SHA256, outputSHA256),
		RunID:         run.ID,
		SourcePath:    source.Source.Path,
		Origin:        origin(source.Source.Path, selector.JSONPointer),
		Selector:      selector,
		ReaderName:    readerName,
		ReaderVersion: readerVersion,
		ReaderOptions: json.RawMessage(`{"selector_mode":"json_pointer"}`),
		Locator:       locator(selector.JSONPointer),
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
	JSONPointer string `json:"json_pointer"`
	Value       value  `json:"value"`
}

func marshalOutput(pointer string, selected value) ([]byte, error) {
	return json.Marshal(outputEnvelope{
		JSONPointer: pointer,
		Value:       selected,
	})
}

func locator(pointer string) json.RawMessage {
	return json.RawMessage([]byte(fmt.Sprintf(`{"json_pointer":%q}`, pointer)))
}

func origin(path, pointer string) string {
	if pointer == "" {
		return path
	}
	return path + "#ptr=" + pointer
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
	if selector.JSONPath != "" ||
		selector.ByteSpan != "" ||
		selector.LineWindow != "" ||
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
}

type field struct {
	key   string
	value value
}

func decodeValue(dec *json.Decoder) (value, error) {
	dec.UseNumber()

	token, err := dec.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return value{}, ErrInvalidJSON
		}
		return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	v, err := decodeToken(dec, token)
	if err != nil {
		return value{}, err
	}

	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return value{}, fmt.Errorf("%w: trailing data", ErrInvalidJSON)
		}
		return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	return v, nil
}

func decodeToken(dec *json.Decoder, token json.Token) (value, error) {
	switch tok := token.(type) {
	case json.Delim:
		switch tok {
		case '{':
			return decodeObject(dec)
		case '[':
			return decodeArray(dec)
		default:
			return value{}, fmt.Errorf("%w: unexpected delimiter %q", ErrInvalidJSON, tok)
		}
	case nil:
		return value{kind: kindNull}, nil
	case bool:
		return value{kind: kindBool, boolean: tok}, nil
	case string:
		return value{kind: kindString, text: tok}, nil
	case json.Number:
		return value{kind: kindNumber, number: tok.String()}, nil
	default:
		return value{}, fmt.Errorf("%w: unexpected token %T", ErrInvalidJSON, tok)
	}
}

func decodeObject(dec *json.Decoder) (value, error) {
	seen := make(map[string]struct{})
	fields := make([]field, 0)
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return value{}, fmt.Errorf("%w: object key %T", ErrInvalidJSON, keyToken)
		}
		if _, exists := seen[key]; exists {
			return value{}, fmt.Errorf("%w: duplicate object key %q", ErrInvalidJSON, key)
		}
		seen[key] = struct{}{}

		childToken, err := dec.Token()
		if err != nil {
			return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
		child, err := decodeToken(dec, childToken)
		if err != nil {
			return value{}, err
		}
		fields = append(fields, field{key: key, value: child})
	}

	end, err := dec.Token()
	if err != nil {
		return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if delim, ok := end.(json.Delim); !ok || delim != '}' {
		return value{}, fmt.Errorf("%w: unterminated object", ErrInvalidJSON)
	}

	return value{kind: kindObject, object: fields}, nil
}

func decodeArray(dec *json.Decoder) (value, error) {
	items := make([]value, 0)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
		child, err := decodeToken(dec, token)
		if err != nil {
			return value{}, err
		}
		items = append(items, child)
	}

	end, err := dec.Token()
	if err != nil {
		return value{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if delim, ok := end.(json.Delim); !ok || delim != ']' {
		return value{}, fmt.Errorf("%w: unterminated array", ErrInvalidJSON)
	}

	return value{kind: kindArray, array: items}, nil
}

func resolvePointer(root value, pointer string) (value, error) {
	if pointer == "" {
		return root, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return value{}, fmt.Errorf("%w: %q", ErrInvalidJSONPointer, pointer)
	}

	current := root
	for _, rawSegment := range strings.Split(pointer[1:], "/") {
		segment, err := decodeReferenceToken(rawSegment)
		if err != nil {
			return value{}, err
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
				return value{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, pointer)
			}
		case kindArray:
			index, err := parseIndex(segment)
			if err != nil || index >= len(current.array) {
				return value{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, pointer)
			}
			current = current.array[index]
		default:
			return value{}, fmt.Errorf("%w: %s", ErrSelectorNotFound, pointer)
		}
	}

	return current, nil
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
