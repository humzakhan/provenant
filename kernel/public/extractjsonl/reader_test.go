package extractjsonl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	kernel "github.com/provenant/provenant/kernel/public"
)

func TestReaderReadLineWindowProjectionAndPagination(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}\n{\"amount\":\"3.30\",\"note\":\"c\"}\n{\"amount\":\"4.40\",\"note\":\"d\"}\n{\"amount\":\"5.50\",\"note\":\"e\"}")
	source := sourceWithHash("fixture://amounts.jsonl", sourceBytes)
	selector := kernel.SourceSelector{
		LineWindow:  "1-2",
		Cursor:      "line:2",
		JSONPointer: "/amount",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"cursor":"line:2","effective_line_end":4,"effective_line_start":3,"line_window":"1-2","next_cursor":"line:4","record_count":2,"record_order":"source_line_asc","records":[{"byte_end":74,"byte_start":68,"json_pointer":"/amount","line":3,"normalized_value":"3.30","raw_value":"\"3.30\"","value":"3.30","window_index":0},{"byte_end":103,"byte_start":97,"json_pointer":"/amount","line":4,"normalized_value":"4.40","raw_value":"\"4.40\"","value":"4.40","window_index":1}],"source_hash":"` + source.Source.SHA256 + `"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if string(result.CapturedRead.Output) != wantOutput {
		t.Fatalf("CapturedRead.Output = %s, want %s", result.CapturedRead.Output, wantOutput)
	}
	if string(result.CapturedRead.ReaderOptions) != `{"cursor_applied":true,"page_size":2,"projection_mode":"json_pointer","record_order":"source_line_asc","selector_mode":"line_window"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
	}
	if string(result.CapturedRead.Locator) != `{"cursor":"line:2","effective_line_end":4,"effective_line_start":3,"json_pointer":"/amount","line_window":"1-2","next_cursor":"line:4","record_count":2,"record_order":"source_line_asc","records":[{"byte_end":86,"byte_start":58,"line":3,"value_byte_end":74,"value_byte_start":68,"window_index":0},{"byte_end":115,"byte_start":87,"line":4,"value_byte_end":103,"value_byte_start":97,"window_index":1}]}` {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != "fixture://amounts.jsonl#line=3-4&ptr=/amount" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
	if result.CapturedRead.ReaderName != readerName {
		t.Fatalf("ReaderName = %s", result.CapturedRead.ReaderName)
	}
	if result.CapturedRead.ReaderVersion != readerVersion {
		t.Fatalf("ReaderVersion = %s", result.CapturedRead.ReaderVersion)
	}
	if result.CapturedRead.OutputSHA256 != sha256Hex([]byte(wantOutput)) {
		t.Fatalf("OutputSHA256 = %s", result.CapturedRead.OutputSHA256)
	}
}

func TestReaderReadSingleRecordJSONPathWithByteSpan(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}")
	source := sourceWithHash("fixture://amounts.jsonl", sourceBytes)
	selector := kernel.SourceSelector{
		LineWindow: "1-1",
		Cursor:     "line:1",
		JSONPath:   "$.amount",
		ByteSpan:   "39-45",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"cursor":"line:1","effective_line_end":2,"effective_line_start":2,"line_window":"1-1","normalized_value":"2.20","raw_value":"\"2.20\"","record_count":1,"record_order":"source_line_asc","records":[{"byte_end":45,"byte_start":39,"jsonpath":"$.amount","line":2,"normalized_value":"2.20","raw_value":"\"2.20\"","value":"2.20","window_index":0}],"source_hash":"` + source.Source.SHA256 + `","value":"2.20"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if string(result.CapturedRead.ReaderOptions) != `{"byte_span_validated":true,"cursor_applied":true,"page_size":1,"projection_mode":"jsonpath","record_order":"source_line_asc","selector_mode":"line_window"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
	}
	if string(result.CapturedRead.Locator) != `{"cursor":"line:1","effective_line_end":2,"effective_line_start":2,"jsonpath":"$.amount","line_window":"1-1","record_count":1,"record_order":"source_line_asc","records":[{"byte_end":57,"byte_start":29,"line":2,"value_byte_end":45,"value_byte_start":39,"window_index":0}]}` {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != "fixture://amounts.jsonl#line=2-2&jsonpath=$.amount&byte=39-45" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
}

func TestReaderReadPartialFinalPageUsesSelectedLineBounds(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}\n{\"amount\":\"3.30\",\"note\":\"c\"}\n{\"amount\":\"4.40\",\"note\":\"d\"}\n{\"amount\":\"5.50\",\"note\":\"e\"}")
	source := sourceWithHash("fixture://amounts.jsonl", sourceBytes)
	selector := kernel.SourceSelector{
		LineWindow:  "1-3",
		Cursor:      "line:3",
		JSONPointer: "/amount",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"cursor":"line:3","effective_line_end":5,"effective_line_start":4,"line_window":"1-3","record_count":2,"record_order":"source_line_asc","records":[{"byte_end":103,"byte_start":97,"json_pointer":"/amount","line":4,"normalized_value":"4.40","raw_value":"\"4.40\"","value":"4.40","window_index":0},{"byte_end":132,"byte_start":126,"json_pointer":"/amount","line":5,"normalized_value":"5.50","raw_value":"\"5.50\"","value":"5.50","window_index":1}],"source_hash":"` + source.Source.SHA256 + `"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if string(result.CapturedRead.Locator) != `{"cursor":"line:3","effective_line_end":5,"effective_line_start":4,"json_pointer":"/amount","line_window":"1-3","record_count":2,"record_order":"source_line_asc","records":[{"byte_end":115,"byte_start":87,"line":4,"value_byte_end":103,"value_byte_start":97,"window_index":0},{"byte_end":144,"byte_start":116,"line":5,"value_byte_end":132,"value_byte_start":126,"window_index":1}]}` {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != "fixture://amounts.jsonl#line=4-5&ptr=/amount" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
}

func TestReaderReadCursorAtEOFReturnsSelectorNotFound(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}\n{\"amount\":2}"))
	selector := kernel.SourceSelector{
		LineWindow: "1-1",
		Cursor:     "line:2",
	}

	_, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if !errors.Is(err, ErrSelectorNotFound) {
		t.Fatalf("Read() error = %v, want %v", err, ErrSelectorNotFound)
	}
}

func TestReaderReadTerminalNewlineDoesNotCreateMalformedEOFRecord(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}\n")
	source := sourceWithHash("fixture://amounts.jsonl", sourceBytes)
	selector := kernel.SourceSelector{
		LineWindow:  "1-2",
		JSONPointer: "/amount",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"effective_line_end":2,"effective_line_start":1,"line_window":"1-2","record_count":2,"record_order":"source_line_asc","records":[{"byte_end":16,"byte_start":10,"json_pointer":"/amount","line":1,"normalized_value":"1.10","raw_value":"\"1.10\"","value":"1.10","window_index":0},{"byte_end":45,"byte_start":39,"json_pointer":"/amount","line":2,"normalized_value":"2.20","raw_value":"\"2.20\"","value":"2.20","window_index":1}],"source_hash":"` + source.Source.SHA256 + `"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if result.CapturedRead.Origin != "fixture://amounts.jsonl#line=1-2&ptr=/amount" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
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
				Source: kernel.Source{Path: "fixture://amounts.jsonl", SHA256: "abc"},
			},
			selector: kernel.SourceSelector{LineWindow: "1-1"},
			wantErr:  ErrMissingSourceBytes,
		},
		{
			name: "missing source hash",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://amounts.jsonl"},
				Bytes:  []byte("{\"amount\":1}\n"),
			},
			selector: kernel.SourceSelector{LineWindow: "1-1"},
			wantErr:  ErrMissingSourceSHA256,
		},
		{
			name:     "invalid line window",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}")),
			selector: kernel.SourceSelector{LineWindow: "0-1"},
			wantErr:  ErrInvalidLineWindow,
		},
		{
			name:     "invalid cursor",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", Cursor: "row:1"},
			wantErr:  ErrInvalidCursor,
		},
		{
			name:     "cursor line zero rejected",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", Cursor: "line:0"},
			wantErr:  ErrInvalidCursor,
		},
		{
			name:     "source hash mismatch",
			source:   kernel.SourceBytes{Source: kernel.Source{Path: "fixture://amounts.jsonl", SHA256: "deadbeef"}, Bytes: []byte("{\"amount\":1}")},
			selector: kernel.SourceSelector{LineWindow: "1-1"},
			wantErr:  ErrSourceHashMismatch,
		},
		{
			name:     "malformed line",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}\n{\"amount\":\n")),
			selector: kernel.SourceSelector{LineWindow: "1-2"},
			wantErr:  ErrInvalidJSONL,
		},
		{
			name:     "blank line rejected",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}\n\n{\"amount\":2}")),
			selector: kernel.SourceSelector{LineWindow: "1-2"},
			wantErr:  ErrInvalidJSONL,
		},
		{
			name:     "ambiguous selector families",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", RowWindow: "1-1"},
			wantErr:  ErrUnsupportedSelector,
		},
		{
			name:     "invalid pointer",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", JSONPointer: "amount"},
			wantErr:  ErrInvalidJSONPointer,
		},
		{
			name:     "path not found",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", JSONPath: "$.missing"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name:     "non unique path",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"items\":[{\"amount\":1},{\"amount\":2}]}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", JSONPath: "$.items[*].amount"},
			wantErr:  ErrNonUniqueJSONPath,
		},
		{
			name:     "pointer path mismatch",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1,\"other\":2}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", JSONPointer: "/amount", JSONPath: "$.other"},
			wantErr:  ErrSelectorMismatch,
		},
		{
			name:     "byte span mismatch on multi record page",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":1}\n{\"amount\":2}")),
			selector: kernel.SourceSelector{LineWindow: "1-2", ByteSpan: "0-1"},
			wantErr:  ErrAmbiguousByteSpan,
		},
		{
			name:     "byte span mismatch single record",
			source:   sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":\"2.20\"}")),
			selector: kernel.SourceSelector{LineWindow: "1-1", JSONPointer: "/amount", ByteSpan: "0-4"},
			wantErr:  ErrByteSpanMismatch,
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
			MediaType: "application/jsonl",
			SHA256:    sha256Hex(data),
		},
		Bytes: data,
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
