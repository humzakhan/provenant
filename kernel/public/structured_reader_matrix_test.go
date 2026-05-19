package kernel_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/extractcsvrows"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/extractjsonl"
	"github.com/provenant/provenant/kernel/public/extractspreadsheet"
)

func TestStructuredReaderPositiveMatrix(t *testing.T) {
	t.Parallel()

	type positiveCase struct {
		name               string
		reader             kernel.SourceReader
		source             kernel.SourceBytes
		selector           kernel.SourceSelector
		wantReaderName     string
		wantOutputContains []string
		wantLocatorKeys    []string
		wantOriginContains []string
	}

	tests := []positiveCase{
		{
			name:   "json pointer jsonpath byte span",
			reader: extractjson.Reader{},
			source: sourceWithMatrixHash("fixture://invoice.json", []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)),
			selector: kernel.SourceSelector{
				JSONPointer: "/invoice/items/1/amount",
				JSONPath:    "$.invoice.items[1].amount",
				ByteSpan:    "50-57",
			},
			wantReaderName:     "extract.json",
			wantOutputContains: []string{`"json_pointer":"/invoice/items/1/amount"`, `"jsonpath":"$.invoice.items[1].amount"`, `"raw_value":"\"56.78\""`, `"value":"56.78"`},
			wantLocatorKeys:    []string{`"byte_start":50`, `"byte_end":57`, `"json_pointer":"/invoice/items/1/amount"`},
			wantOriginContains: []string{`#ptr=/invoice/items/1/amount`, `byte=50-57`},
		},
		{
			name:   "jsonl line window cursor projection",
			reader: extractjsonl.Reader{},
			source: sourceWithMatrixHash("fixture://amounts.jsonl", []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}\n{\"amount\":\"3.30\",\"note\":\"c\"}")),
			selector: kernel.SourceSelector{
				LineWindow:  "1-1",
				Cursor:      "line:1",
				JSONPointer: "/amount",
			},
			wantReaderName:     "extract.jsonl",
			wantOutputContains: []string{`"effective_line_start":2`, `"effective_line_end":2`, `"line_window":"1-1"`, `"json_pointer":"/amount"`, `"value":"2.20"`},
			wantLocatorKeys:    []string{`"line_window":"1-1"`, `"line":2`, `"value_byte_start":39`, `"value_byte_end":45`},
			wantOriginContains: []string{`#line=2-2`, `ptr=/amount`},
		},
		{
			name:   "csv row window header projection",
			reader: extractcsvrows.Reader{},
			source: sourceWithMatrixHash("fixture://amounts.csv", []byte("account,amount,note\ncash,1.10,a\nar,2.20,b")),
			selector: kernel.SourceSelector{
				RowWindow:        "2-2",
				ColumnProjection: "amount",
				HeaderRow:        1,
				Dialect:          "excel",
				Encoding:         "utf-8",
			},
			wantReaderName:     "extract.csv_rows",
			wantOutputContains: []string{`"header_row":1`, `"column_projection":"amount"`, `"row_window":"2-2"`, `"raw_value":"1.10"`, `"value":"1.10"`},
			wantLocatorKeys:    []string{`"header_row":1`, `"row":2`, `"column":2`, `"raw_byte_start":25`, `"raw_byte_end":29`},
			wantOriginContains: []string{`#row=2`, `col=2`, `header=amount`},
		},
		{
			name:   "spreadsheet table selection cached formula",
			reader: extractspreadsheet.Reader{},
			source: sourceWithMatrixHash("fixture://amounts.xlsx", matrixWorkbookBytes(t)),
			selector: kernel.SourceSelector{
				TableName:        "Amounts",
				RowWindow:        "3-3",
				ColumnProjection: "B",
			},
			wantReaderName:     "extract.spreadsheet",
			wantOutputContains: []string{`"sheet":"TrialBalance"`, `"table_name":"Amounts"`, `"formula_policy":"cached_only"`, `"raw_value":"=B2*2"`, `"value":"2.20"`},
			wantLocatorKeys:    []string{`"sheet":"TrialBalance"`, `"row":3`, `"column":2`, `"a1":"B3"`},
			wantOriginContains: []string{`#sheet=TrialBalance`, `row=3`, `a1=B3`},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := tt.reader.Read(context.Background(), kernel.Run{ID: "run_matrix"}, tt.source, tt.selector)
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if result.CapturedRead.ReaderName != tt.wantReaderName {
				t.Fatalf("ReaderName = %q, want %q", result.CapturedRead.ReaderName, tt.wantReaderName)
			}
			if result.CapturedRead.SourceSHA256 != tt.source.Source.SHA256 {
				t.Fatalf("SourceSHA256 = %q, want %q", result.CapturedRead.SourceSHA256, tt.source.Source.SHA256)
			}
			if result.CapturedRead.Selector != tt.selector {
				t.Fatalf("Selector = %#v, want %#v", result.CapturedRead.Selector, tt.selector)
			}
			for _, fragment := range tt.wantOutputContains {
				if !strings.Contains(string(result.Output), fragment) {
					t.Fatalf("Output = %s, want fragment %q", result.Output, fragment)
				}
			}
			for _, fragment := range tt.wantLocatorKeys {
				if !strings.Contains(string(result.CapturedRead.Locator), fragment) {
					t.Fatalf("Locator = %s, want fragment %q", result.CapturedRead.Locator, fragment)
				}
			}
			for _, fragment := range tt.wantOriginContains {
				if !strings.Contains(result.CapturedRead.Origin, fragment) {
					t.Fatalf("Origin = %q, want fragment %q", result.CapturedRead.Origin, fragment)
				}
			}
		})
	}
}

func TestStructuredReaderNegativeMatrix(t *testing.T) {
	t.Parallel()

	type negativeCase struct {
		name     string
		reader   kernel.SourceReader
		source   kernel.SourceBytes
		selector kernel.SourceSelector
		wantErr  error
	}

	tests := []negativeCase{
		{
			name:     "json selector mismatch fails closed",
			reader:   extractjson.Reader{},
			source:   sourceWithMatrixHash("fixture://invoice.json", []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)),
			selector: kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount", JSONPath: "$.invoice.items[1].amount"},
			wantErr:  extractjson.ErrSelectorMismatch,
		},
		{
			name:     "jsonl byte span on multi-record page is ambiguous",
			reader:   extractjsonl.Reader{},
			source:   sourceWithMatrixHash("fixture://amounts.jsonl", []byte("{\"amount\":1}\n{\"amount\":2}")),
			selector: kernel.SourceSelector{LineWindow: "1-2", ByteSpan: "0-1"},
			wantErr:  extractjsonl.ErrAmbiguousByteSpan,
		},
		{
			name:     "csv unsupported encoding fails closed",
			reader:   extractcsvrows.Reader{},
			source:   sourceWithMatrixHash("fixture://amounts.csv", []byte("account,amount\ncash,1.10")),
			selector: kernel.SourceSelector{RowWindow: "2-2", HeaderRow: 1, Dialect: "excel", Encoding: "utf-16le", ColumnProjection: "amount"},
			wantErr:  extractcsvrows.ErrUnsupportedEncoding,
		},
		{
			name:     "spreadsheet unsupported formula policy fails closed",
			reader:   extractspreadsheet.Reader{},
			source:   sourceWithMatrixHash("fixture://amounts.xlsx", matrixWorkbookBytes(t)),
			selector: kernel.SourceSelector{Sheet: "TrialBalance", FormulaPolicy: "fail_closed"},
			wantErr:  extractspreadsheet.ErrUnsupportedFormulaPolicy,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.reader.Read(context.Background(), kernel.Run{ID: "run_matrix"}, tt.source, tt.selector)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Read() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func sourceWithMatrixHash(path string, data []byte) kernel.SourceBytes {
	sum := sha256.Sum256(data)
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:   path,
			SHA256: hex.EncodeToString(sum[:]),
		},
		Bytes: data,
	}
}

func matrixWorkbookBytes(t *testing.T) []byte {
	t.Helper()

	files := map[string]string{
		"[Content_Types].xml":                 xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/tables/table1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/></Types>`,
		"_rels/.rels":                         xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/_rels/workbook.xml.rels":          xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/workbook.xml":                     xmlHeader + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="TrialBalance" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/worksheets/_rels/sheet1.xml.rels": xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":            xmlHeader + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><dimension ref="A1:B3"/><sheetData><row r="1"><c r="A1" t="str"><v>account</v></c><c r="B1" t="str"><v>amount</v></c></row><row r="2"><c r="A2" t="str"><v>cash</v></c><c r="B2"><v>1.10</v></c></row><row r="3"><c r="A3" t="str"><v>ar</v></c><c r="B3"><f>B2*2</f><v>2.20</v></c></row></sheetData><tableParts count="1"><tablePart r:id="rId1"/></tableParts></worksheet>`,
		"xl/tables/table1.xml":                xmlHeader + `<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" id="1" name="Amounts" displayName="Amounts" ref="A1:B3"><autoFilter ref="A1:B3"/><tableColumns count="2"><tableColumn id="1" name="account"/><tableColumn id="2" name="amount"/></tableColumns><tableStyleInfo name="TableStyleMedium2" showFirstColumn="0" showLastColumn="0" showRowStripes="1" showColumnStripes="0"/></table>`,
	}

	var names []string
	for name := range files {
		names = append(names, name)
	}
	sortStrings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>`
