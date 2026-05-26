package extractjson

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

func TestReaderReadScalarWithJSONPathAndByteSpan(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}],"total":"69.12"}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)
	rawValue := `"56.78"`
	byteStart, byteEnd := mustUniqueSpan(t, sourceBytes, rawValue)
	selector := kernel.SourceSelector{
		JSONPointer: "/invoice/items/1/amount",
		JSONPath:    "$.invoice.items[1].amount",
		ByteSpan:    fmt.Sprintf("%d-%d", byteStart, byteEnd),
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/invoice/items/1/amount","jsonpath":"$.invoice.items[1].amount","normalized_value":"56.78","raw_value":"\"56.78\"","source_hash":"%s","value":"56.78"}`, byteEnd, byteStart, source.Source.SHA256)
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if string(result.CapturedRead.Output) != wantOutput {
		t.Fatalf("CapturedRead.Output = %s, want %s", result.CapturedRead.Output, wantOutput)
	}
	if result.CapturedRead.ReaderName != readerName {
		t.Fatalf("ReaderName = %s, want %s", result.CapturedRead.ReaderName, readerName)
	}
	if result.CapturedRead.ReaderVersion != readerVersion {
		t.Fatalf("ReaderVersion = %s, want %s", result.CapturedRead.ReaderVersion, readerVersion)
	}
	if result.CapturedRead.SourcePath != source.Source.Path {
		t.Fatalf("SourcePath = %s, want %s", result.CapturedRead.SourcePath, source.Source.Path)
	}
	if result.CapturedRead.SourceSHA256 != source.Source.SHA256 {
		t.Fatalf("SourceSHA256 = %s, want %s", result.CapturedRead.SourceSHA256, source.Source.SHA256)
	}
	if result.CapturedRead.OutputSHA256 != sha256Hex([]byte(wantOutput)) {
		t.Fatalf("OutputSHA256 = %s, want %s", result.CapturedRead.OutputSHA256, sha256Hex([]byte(wantOutput)))
	}
	if string(result.CapturedRead.ReaderOptions) != `{"byte_span_validated":true,"jsonpath_validated":true,"selector_mode":"json_pointer"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
	}
	if string(result.CapturedRead.Locator) != fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/invoice/items/1/amount","jsonpath":"$.invoice.items[1].amount"}`, byteEnd, byteStart) {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != fmt.Sprintf("fixture://invoice.json#ptr=/invoice/items/1/amount&byte=%d-%d", byteStart, byteEnd) {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
	if result.CapturedRead.ID == "" {
		t.Fatal("CapturedRead.ID is empty")
	}

	resultAgain, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() second call error = %v", err)
	}
	if string(resultAgain.Output) != string(result.Output) {
		t.Fatalf("second Output = %s, want %s", resultAgain.Output, result.Output)
	}
	if resultAgain.CapturedRead.ID != result.CapturedRead.ID {
		t.Fatalf("second ID = %s, want %s", resultAgain.CapturedRead.ID, result.CapturedRead.ID)
	}
}

func TestReaderReadNumericValuePreservesJSONNumberAtValuePath(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"totals":[1e3,1.2300]}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)
	rawValue := `1e3`
	byteStart, byteEnd := mustUniqueSpan(t, sourceBytes, rawValue)
	selector := kernel.SourceSelector{
		JSONPointer: "/invoice/totals/0",
		JSONPath:    "$.invoice.totals[0]",
		ByteSpan:    fmt.Sprintf("%d-%d", byteStart, byteEnd),
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/invoice/totals/0","jsonpath":"$.invoice.totals[0]","normalized_value":1000,"raw_value":"1e3","source_hash":"%s","value":1000}`, byteEnd, byteStart, source.Source.SHA256)
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if string(result.CapturedRead.Output) != wantOutput {
		t.Fatalf("CapturedRead.Output = %s, want %s", result.CapturedRead.Output, wantOutput)
	}
	if string(result.CapturedRead.Locator) != fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/invoice/totals/0","jsonpath":"$.invoice.totals[0]"}`, byteEnd, byteStart) {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
}

func TestReaderReadObjectPreservesDocumentOrder(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"b":2,"a":1}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)
	rawValue := `{"b":2,"a":1}`
	byteStart, byteEnd := mustUniqueSpan(t, sourceBytes, rawValue)

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{JSONPointer: "/invoice"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/invoice","normalized_value":{"a":1,"b":2},"raw_value":"{\"b\":2,\"a\":1}","source_hash":"%s","value":{"a":1,"b":2}}`, byteEnd, byteStart, source.Source.SHA256)
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
}

func TestReaderReadArrayPreservesOrder(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"pairs":[{"z":2,"a":1},{"y":4,"b":3}]}`)
	source := sourceWithHash("fixture://pairs.json", sourceBytes)
	rawValue := `[{"z":2,"a":1},{"y":4,"b":3}]`
	byteStart, byteEnd := mustUniqueSpan(t, sourceBytes, rawValue)

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{JSONPointer: "/pairs"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := fmt.Sprintf(`{"byte_end":%d,"byte_start":%d,"json_pointer":"/pairs","normalized_value":[{"a":1,"z":2},{"b":3,"y":4}],"raw_value":"[{\"z\":2,\"a\":1},{\"y\":4,\"b\":3}]","source_hash":"%s","value":[{"a":1,"z":2},{"b":3,"y":4}]}`, byteEnd, byteStart, source.Source.SHA256)
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
}

func TestReaderReadFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   kernel.SourceBytes
		selector kernel.SourceSelector
		wantErr  error
	}{
		{
			name: "missing bytes",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: "abc"},
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrMissingSourceBytes,
		},
		{
			name: "missing source hash",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json"},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrMissingSourceSHA256,
		},
		{
			name:     "missing selector",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":1}`)),
			selector: kernel.SourceSelector{},
			wantErr:  ErrMissingJSONPointer,
		},
		{
			name:     "jsonpath without pointer",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":1}`)),
			selector: kernel.SourceSelector{JSONPath: "$.invoice"},
			wantErr:  ErrMissingJSONPointer,
		},
		{
			name: "source hash mismatch",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: "deadbeef"},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrSourceHashMismatch,
		},
		{
			name:     "malformed json",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrInvalidJSON,
		},
		{
			name:     "duplicate keys",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":1,"total":2}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/total"},
			wantErr:  ErrInvalidJSON,
		},
		{
			name:     "jsonl selector metadata rejected",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":1}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice", LineWindow: "1-10"},
			wantErr:  ErrUnsupportedSelector,
		},
		{
			name:     "invalid pointer",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":1}`)),
			selector: kernel.SourceSelector{JSONPointer: "invoice"},
			wantErr:  ErrInvalidJSONPointer,
		},
		{
			name:     "invalid pointer bare tilde escape",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"~":1}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/~"},
			wantErr:  ErrInvalidJSONPointer,
		},
		{
			name:     "selector not found",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":1}`)),
			selector: kernel.SourceSelector{JSONPointer: "/missing"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name:     "jsonpath not found",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":1}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice", JSONPath: "$.missing"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name:     "jsonpath unsupported syntax",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":1}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/total", JSONPath: `$.invoice["total"]`},
			wantErr:  ErrInvalidJSONPath,
		},
		{
			name:     "jsonpath non unique",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount", JSONPath: "$.invoice.items[*].amount"},
			wantErr:  ErrNonUniqueJSONPath,
		},
		{
			name:     "pointer path mismatch",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount", JSONPath: "$.invoice.items[1].amount"},
			wantErr:  ErrSelectorMismatch,
		},
		{
			name:     "invalid byte span syntax",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":"69.12"}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/total", ByteSpan: "0:"},
			wantErr:  ErrInvalidByteSpan,
		},
		{
			name:     "byte span mismatch",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":"69.12"}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/total", ByteSpan: "0-4"},
			wantErr:  ErrByteSpanMismatch,
		},
		{
			name:     "oversized array index",
			source:   sourceWithHash("fixture://invoice.json", []byte(`{"invoice":[1,2,3]}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/999999999999999999999999999999999999"},
			wantErr:  ErrSelectorNotFound,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, tt.source, tt.selector)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Read() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func sourceWithHash(path string, data []byte) kernel.SourceBytes {
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:      path,
			MediaType: "application/json",
			SHA256:    sha256Hex(data),
		},
		Bytes: data,
	}
}

func mustUniqueSpan(t *testing.T, source []byte, raw string) (int, int) {
	t.Helper()

	start := strings.Index(string(source), raw)
	if start < 0 {
		t.Fatalf("raw value %q not found in %s", raw, source)
	}
	if strings.Count(string(source), raw) != 1 {
		t.Fatalf("raw value %q appears multiple times in %s", raw, source)
	}
	return start, start + len(raw)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
