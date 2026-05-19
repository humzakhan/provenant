package extractcsvrows

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	kernel "github.com/provenant/provenant/kernel/public"
)

func TestReaderReadRowWindowHeaderProjectionAndPagination(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("account,amount,note\ncash,1.10,a\nar,2.20,b\nap,3.30,c\nrev,4.40,d")
	source := sourceWithHash("fixture://amounts.csv", sourceBytes)
	selector := kernel.SourceSelector{
		RowWindow:        "1-2",
		Cursor:           "row:2",
		ColumnProjection: "amount,note",
		HeaderRow:        1,
		Dialect:          "excel",
		Encoding:         "utf-8",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	var payload struct {
		Cursor            string `json:"cursor"`
		ColumnCount       int    `json:"column_count"`
		ColumnOrder       string `json:"column_order"`
		ColumnProjection  string `json:"column_projection"`
		EffectiveRowEnd   int    `json:"effective_row_end"`
		EffectiveRowStart int    `json:"effective_row_start"`
		Header            struct {
			Cells []struct {
				Column          int    `json:"column"`
				NormalizedValue string `json:"normalized_value"`
				RawValue        string `json:"raw_value"`
				Value           string `json:"value"`
			} `json:"cells"`
			Row int `json:"row"`
		} `json:"header"`
		HeaderRow  int    `json:"header_row"`
		NextCursor string `json:"next_cursor"`
		RowCount   int    `json:"row_count"`
		RowOrder   string `json:"row_order"`
		RowWindow  string `json:"row_window"`
		Rows       []struct {
			Cells []struct {
				Column          int    `json:"column"`
				Header          string `json:"header"`
				NormalizedValue string `json:"normalized_value"`
				RawValue        string `json:"raw_value"`
				Row             int    `json:"row"`
				Value           string `json:"value"`
			} `json:"cells"`
			Row         int `json:"row"`
			WindowIndex int `json:"window_index"`
		} `json:"rows"`
		SourceHash string `json:"source_hash"`
	}
	if err := json.Unmarshal(result.Output, &payload); err != nil {
		t.Fatalf("Unmarshal(output) error = %v", err)
	}

	if payload.Cursor != "row:2" {
		t.Fatalf("output.cursor = %q, want %q", payload.Cursor, "row:2")
	}
	if payload.ColumnCount != 2 || payload.ColumnOrder != "source_column_asc" {
		t.Fatalf("output column metadata = (%d, %q)", payload.ColumnCount, payload.ColumnOrder)
	}
	if payload.ColumnProjection != "amount,note" {
		t.Fatalf("output.column_projection = %q", payload.ColumnProjection)
	}
	if payload.EffectiveRowStart != 3 || payload.EffectiveRowEnd != 4 {
		t.Fatalf("effective rows = %d-%d, want 3-4", payload.EffectiveRowStart, payload.EffectiveRowEnd)
	}
	if payload.HeaderRow != 1 || payload.Header.Row != 1 {
		t.Fatalf("header row = (%d, %d), want (1, 1)", payload.HeaderRow, payload.Header.Row)
	}
	if payload.NextCursor != "row:4" {
		t.Fatalf("output.next_cursor = %q, want %q", payload.NextCursor, "row:4")
	}
	if payload.RowCount != 2 || payload.RowOrder != "source_row_asc" || payload.RowWindow != "1-2" {
		t.Fatalf("row metadata = (%d, %q, %q)", payload.RowCount, payload.RowOrder, payload.RowWindow)
	}
	if payload.SourceHash != source.Source.SHA256 {
		t.Fatalf("output.source_hash = %q, want %q", payload.SourceHash, source.Source.SHA256)
	}
	if len(payload.Header.Cells) != 2 || payload.Header.Cells[0].Value != "amount" || payload.Header.Cells[1].Value != "note" {
		t.Fatalf("header cells = %#v", payload.Header.Cells)
	}
	if len(payload.Rows) != 2 {
		t.Fatalf("len(output.rows) = %d, want 2", len(payload.Rows))
	}
	if payload.Rows[0].Row != 3 || payload.Rows[0].WindowIndex != 0 {
		t.Fatalf("first row metadata = (%d, %d)", payload.Rows[0].Row, payload.Rows[0].WindowIndex)
	}
	if payload.Rows[0].Cells[0].Value != "2.20" || payload.Rows[0].Cells[0].Header != "amount" {
		t.Fatalf("first row first cell = %#v", payload.Rows[0].Cells[0])
	}
	if payload.Rows[1].Cells[1].Value != "c" || payload.Rows[1].Cells[1].Header != "note" {
		t.Fatalf("second row second cell = %#v", payload.Rows[1].Cells[1])
	}
	if string(result.CapturedRead.ReaderOptions) != `{"column_order":"source_column_asc","dialect":"excel","encoding":"utf-8","header_echoed":true,"page_size":2,"projection_mode":"header_names","row_order":"source_row_asc","selector_mode":"row_window"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
	}
	if string(result.CapturedRead.Locator) != `{"column_count":2,"column_order":"source_column_asc","column_projection":"amount,note","cursor":"row:2","effective_row_end":4,"effective_row_start":3,"header_row":1,"next_cursor":"row:4","row_count":2,"row_order":"source_row_asc","row_window":"1-2","rows":[{"byte_end":41,"byte_start":32,"cells":[{"column":2,"header":"amount","raw_byte_end":39,"raw_byte_start":35},{"column":3,"header":"note","raw_byte_end":41,"raw_byte_start":40}],"row":3,"window_index":0},{"byte_end":51,"byte_start":42,"cells":[{"column":2,"header":"amount","raw_byte_end":49,"raw_byte_start":45},{"column":3,"header":"note","raw_byte_end":51,"raw_byte_start":50}],"row":4,"window_index":1}]}` {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != "fixture://amounts.csv#row=3-4&col=2,3&header_row=1&headers=amount,note" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
}

func TestReaderReadSingleCellProjectionExposesValue(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("account,amount,note\ncash,\"1.10\",a\nar,2.20,b")
	source := sourceWithHash("fixture://amounts.csv", sourceBytes)
	selector := kernel.SourceSelector{
		RowWindow:        "2-2",
		ColumnProjection: "amount",
		HeaderRow:        1,
		Dialect:          "rfc4180",
		Encoding:         "utf-8",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"column_count":1,"column_order":"source_column_asc","column_projection":"amount","effective_row_end":2,"effective_row_start":2,"header":{"cells":[{"column":2,"normalized_value":"amount","raw_value":"amount","value":"amount"}],"row":1},"header_row":1,"next_cursor":"row:2","normalized_value":"1.10","raw_value":"\"1.10\"","row_count":1,"row_order":"source_row_asc","row_window":"2-2","rows":[{"cells":[{"column":2,"header":"amount","normalized_value":"1.10","raw_value":"\"1.10\"","row":2,"value":"1.10"}],"row":2,"window_index":0}],"source_hash":"` + source.Source.SHA256 + `","value":"1.10"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if result.CapturedRead.Origin != "fixture://amounts.csv#row=2&col=2&header_row=1&header=amount" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
}

func TestReaderReadColumnProjectionByIndexWithoutHeader(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte("cash,1.10,a\nar,2.20,b")
	source := sourceWithHash("fixture://amounts.csv", sourceBytes)
	selector := kernel.SourceSelector{
		RowWindow:        "1-2",
		ColumnProjection: "2",
		Dialect:          "excel",
		Encoding:         "utf-8",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	var payload struct {
		Rows []struct {
			Cells []struct {
				Column int    `json:"column"`
				Value  string `json:"value"`
			} `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(result.Output, &payload); err != nil {
		t.Fatalf("Unmarshal(output) error = %v", err)
	}
	if len(payload.Rows) != 2 || len(payload.Rows[0].Cells) != 1 {
		t.Fatalf("rows = %#v", payload.Rows)
	}
	if payload.Rows[0].Cells[0].Column != 2 || payload.Rows[0].Cells[0].Value != "1.10" {
		t.Fatalf("first projected cell = %#v", payload.Rows[0].Cells[0])
	}
	if string(result.CapturedRead.ReaderOptions) != `{"column_order":"source_column_asc","dialect":"excel","encoding":"utf-8","page_size":2,"projection_mode":"column_indexes","row_order":"source_row_asc","selector_mode":"row_window"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
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
			name:     "missing bytes",
			source:   kernel.SourceBytes{Source: kernel.Source{Path: "fixture://amounts.csv", SHA256: "abc"}},
			selector: kernel.SourceSelector{RowWindow: "1-1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrMissingSourceBytes,
		},
		{
			name:     "missing source hash",
			source:   kernel.SourceBytes{Source: kernel.Source{Path: "fixture://amounts.csv"}, Bytes: []byte("cash,1.10")},
			selector: kernel.SourceSelector{RowWindow: "1-1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrMissingSourceSHA256,
		},
		{
			name:     "invalid row window",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "0-1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrInvalidRowWindow,
		},
		{
			name:     "invalid cursor",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "1-1", Cursor: "line:1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrInvalidCursor,
		},
		{
			name:     "unsupported dialect",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "1-1", Dialect: "infer", Encoding: "utf-8"},
			wantErr:  ErrUnsupportedDialect,
		},
		{
			name:     "unsupported encoding",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "1-1", Dialect: "excel", Encoding: "utf-16le"},
			wantErr:  ErrUnsupportedEncoding,
		},
		{
			name:     "invalid utf8 source",
			source:   sourceWithHash("fixture://amounts.csv", []byte{0xff, 0xfe, 0x00}),
			selector: kernel.SourceSelector{RowWindow: "1-1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrInvalidEncodedSource,
		},
		{
			name:     "utf8 bom rejected",
			source:   sourceWithHash("fixture://amounts.csv", append([]byte{0xef, 0xbb, 0xbf}, []byte("cash,1.10")...)),
			selector: kernel.SourceSelector{RowWindow: "1-1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrInvalidEncodedSource,
		},
		{
			name:     "malformed quotes",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,\"1.10\nar,2.20")),
			selector: kernel.SourceSelector{RowWindow: "1-2", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrMalformedCSV,
		},
		{
			name:     "ragged row rejected",
			source:   sourceWithHash("fixture://amounts.csv", []byte("account,amount\ncash,1.10\nar,2.20,b")),
			selector: kernel.SourceSelector{RowWindow: "2-3", HeaderRow: 1, Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrMalformedCSV,
		},
		{
			name:     "header row overlaps selected rows",
			source:   sourceWithHash("fixture://amounts.csv", []byte("account,amount\ncash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "1-1", HeaderRow: 1, Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrSelectorWindowMismatch,
		},
		{
			name:     "header projection not found",
			source:   sourceWithHash("fixture://amounts.csv", []byte("account,amount\ncash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "2-2", HeaderRow: 1, ColumnProjection: "missing", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name:     "index projection not found",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "1-1", ColumnProjection: "3", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name:     "invalid projection whitespace",
			source:   sourceWithHash("fixture://amounts.csv", []byte("account,amount\ncash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "2-2", HeaderRow: 1, ColumnProjection: "amount, note", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrInvalidColumnProjection,
		},
		{
			name:     "duplicate projected column",
			source:   sourceWithHash("fixture://amounts.csv", []byte("account,amount\ncash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "2-2", HeaderRow: 1, ColumnProjection: "amount,amount", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrDuplicateProjectedColumn,
		},
		{
			name:     "duplicate header name",
			source:   sourceWithHash("fixture://amounts.csv", []byte("amount,amount\n1.10,2.20")),
			selector: kernel.SourceSelector{RowWindow: "2-2", HeaderRow: 1, Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrDuplicateHeaderName,
		},
		{
			name:     "ambiguous selector families",
			source:   sourceWithHash("fixture://amounts.csv", []byte("cash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "1-1", LineWindow: "1-1", Dialect: "excel", Encoding: "utf-8"},
			wantErr:  ErrUnsupportedSelector,
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
	sum := sha256.Sum256(data)
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:   path,
			SHA256: hex.EncodeToString(sum[:]),
		},
		Bytes: data,
	}
}
