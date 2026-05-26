package extractspreadsheet

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/internal/testxlsx"
)

func TestReaderReadRejectsDuplicateWorkbookRelationshipIDs(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://dup-workbook-rels.xlsx", duplicateWorkbookRelsWorkbookBytes(t))

	_, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{Sheet: "TrialBalance"})
	if err == nil {
		t.Fatalf("Read() error = nil, want duplicate workbook relationship rejection")
	}
	if !errors.Is(err, ErrInvalidWorkbookFixture) || !strings.Contains(err.Error(), "duplicate workbook relationship id") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestReaderReadRejectsDuplicateWorksheetTableRelationshipIDs(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://dup-table-rels.xlsx", duplicateWorksheetTableRelsWorkbookBytes(t))

	_, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{Sheet: "TrialBalance"})
	if err == nil {
		t.Fatalf("Read() error = nil, want duplicate worksheet table relationship rejection")
	}
	if !errors.Is(err, ErrInvalidWorkbookFixture) || !strings.Contains(err.Error(), "duplicate table relationship id") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestReaderReadSheetRangeWindowProjectionAndCachedFormula(t *testing.T) {
	t.Parallel()

	sourceBytes := workbookBytes(t, testxlsx.Workbook{
		Sheets: []testxlsx.Sheet{
			{
				Name:      "TrialBalance",
				Dimension: "A1:C4",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "C1", Value: "note"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "1.10"},
					{A1: "C2", Value: "open"},
					{A1: "A3", Value: "ar"},
					{A1: "B3", Formula: "=B2", CachedValue: "1.10"},
					{A1: "C3", Value: "carry"},
					{A1: "A4", Value: "ap"},
					{A1: "B4", Value: "3.30"},
					{A1: "C4", Value: "close"},
				},
				Tables: []testxlsx.Table{{Name: "Amounts", Range: "A1:C4"}},
			},
			{
				Name:      "Summary",
				Dimension: "A1:B1",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "total"},
					{A1: "B1", Value: "4.40"},
				},
			},
		},
	})
	source := sourceWithHash("fixture://amounts.xlsx", sourceBytes)
	selector := kernel.SourceSelector{
		Sheet:            "TrialBalance",
		Range:            "B2:C4",
		RowWindow:        "1-2",
		ColumnProjection: "B,C",
		FormulaPolicy:    "cached_only",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	var payload struct {
		Sheet             string `json:"sheet"`
		Range             string `json:"range"`
		FormulaPolicy     string `json:"formula_policy"`
		ColumnCount       int    `json:"column_count"`
		ColumnOrder       string `json:"column_order"`
		ColumnProjection  string `json:"column_projection"`
		EffectiveRowStart int    `json:"effective_row_start"`
		EffectiveRowEnd   int    `json:"effective_row_end"`
		RowCount          int    `json:"row_count"`
		RowOrder          string `json:"row_order"`
		RowWindow         string `json:"row_window"`
		Rows              []struct {
			Sheet       string `json:"sheet"`
			Row         int    `json:"row"`
			WindowIndex int    `json:"window_index"`
			Cells       []struct {
				Sheet              string `json:"sheet"`
				Row                int    `json:"row"`
				Column             int    `json:"column"`
				A1                 string `json:"a1"`
				RawValue           string `json:"raw_value"`
				NormalizedValue    string `json:"normalized_value"`
				Value              string `json:"value"`
				Formula            string `json:"formula"`
				FormulaCachedValue string `json:"formula_cached_value"`
			} `json:"cells"`
		} `json:"rows"`
		SourceHash string `json:"source_hash"`
	}
	if err := json.Unmarshal(result.Output, &payload); err != nil {
		t.Fatalf("Unmarshal(output) error = %v", err)
	}

	if payload.Sheet != "TrialBalance" || payload.Range != "B2:C4" {
		t.Fatalf("sheet/range = (%q, %q)", payload.Sheet, payload.Range)
	}
	if payload.FormulaPolicy != "cached_only" {
		t.Fatalf("formula_policy = %q", payload.FormulaPolicy)
	}
	if payload.ColumnCount != 2 || payload.ColumnOrder != "source_column_asc" || payload.ColumnProjection != "B,C" {
		t.Fatalf("column metadata = (%d, %q, %q)", payload.ColumnCount, payload.ColumnOrder, payload.ColumnProjection)
	}
	if payload.EffectiveRowStart != 2 || payload.EffectiveRowEnd != 3 || payload.RowCount != 2 {
		t.Fatalf("row metadata = (%d, %d, %d)", payload.EffectiveRowStart, payload.EffectiveRowEnd, payload.RowCount)
	}
	if payload.RowOrder != "source_row_asc" || payload.RowWindow != "1-2" {
		t.Fatalf("row order/window = (%q, %q)", payload.RowOrder, payload.RowWindow)
	}
	if payload.SourceHash != source.Source.SHA256 {
		t.Fatalf("source_hash = %q, want %q", payload.SourceHash, source.Source.SHA256)
	}
	if len(payload.Rows) != 2 || len(payload.Rows[1].Cells) != 2 {
		t.Fatalf("rows = %#v", payload.Rows)
	}
	if payload.Rows[1].Cells[0].A1 != "B3" || payload.Rows[1].Cells[0].RawValue != "=B2" || payload.Rows[1].Cells[0].Value != "1.10" {
		t.Fatalf("formula cell = %#v", payload.Rows[1].Cells[0])
	}
	if payload.Rows[1].Cells[0].Formula != "=B2" || payload.Rows[1].Cells[0].FormulaCachedValue != "1.10" {
		t.Fatalf("formula metadata = %#v", payload.Rows[1].Cells[0])
	}
	if string(result.CapturedRead.ReaderOptions) != `{"column_order":"source_column_asc","formula_policy":"cached_only","projection_mode":"absolute_columns","range_mode":"explicit_range","row_order":"source_row_asc","selector_mode":"intersection"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
	}
	if string(result.CapturedRead.Locator) != `{"column_count":2,"column_order":"source_column_asc","column_projection":"B,C","effective_row_end":3,"effective_row_start":2,"range":"B2:C4","row_count":2,"row_order":"source_row_asc","row_window":"1-2","rows":[{"cells":[{"a1":"B2","column":2},{"a1":"C2","column":3}],"row":2,"sheet":"TrialBalance","window_index":0},{"cells":[{"a1":"B3","column":2},{"a1":"C3","column":3}],"row":3,"sheet":"TrialBalance","window_index":1}],"sheet":"TrialBalance"}` {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != "fixture://amounts.xlsx#sheet=TrialBalance&range=B2:C4&row=2-3&col=B,C" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
}

func TestReaderReadTableSelectionSingleCellValue(t *testing.T) {
	t.Parallel()

	sourceBytes := workbookBytes(t, testxlsx.Workbook{
		Sheets: []testxlsx.Sheet{
			{
				Name:      "TrialBalance",
				Dimension: "A1:B3",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "1.10"},
					{A1: "A3", Value: "ar"},
					{A1: "B3", Value: "2.20"},
				},
				Tables: []testxlsx.Table{{Name: "Amounts", Range: "A1:B3"}},
			},
		},
	})
	source := sourceWithHash("fixture://amounts.xlsx", sourceBytes)
	selector := kernel.SourceSelector{
		TableName:        "Amounts",
		RowWindow:        "3-3",
		ColumnProjection: "B",
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, selector)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"column_count":1,"column_order":"source_column_asc","column_projection":"B","effective_row_end":3,"effective_row_start":3,"formula_policy":"cached_only","normalized_value":"2.20","range":"A1:B3","raw_value":"2.20","row_count":1,"row_order":"source_row_asc","row_window":"3-3","rows":[{"cells":[{"a1":"B3","column":2,"normalized_value":"2.20","raw_value":"2.20","row":3,"sheet":"TrialBalance","value":"2.20"}],"row":3,"sheet":"TrialBalance","window_index":0}],"sheet":"TrialBalance","source_hash":"` + source.Source.SHA256 + `","table_name":"Amounts","value":"2.20"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if result.CapturedRead.Origin != "fixture://amounts.xlsx#sheet=TrialBalance&row=3&col=2&a1=B3" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
}

func TestReaderReadDisambiguatesTableBySheet(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://dup.xlsx", workbookBytes(t, testxlsx.Workbook{
		Sheets: []testxlsx.Sheet{
			{
				Name:      "TrialBalance",
				Dimension: "A1:B2",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "1.10"},
				},
				Tables: []testxlsx.Table{{Name: "Dup", Range: "A1:B2"}},
			},
			{
				Name:      "Summary",
				Dimension: "A1:B2",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "label"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "total"},
					{A1: "B2", Value: "9.90"},
				},
				Tables: []testxlsx.Table{{Name: "Dup", Range: "A1:B2"}},
			},
		},
	}))

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{
		Sheet:            "Summary",
		TableName:        "Dup",
		ColumnProjection: "B",
		RowWindow:        "2-2",
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got := string(result.Output); !bytes.Contains([]byte(got), []byte(`"sheet":"Summary"`)) || !bytes.Contains([]byte(got), []byte(`"value":"9.90"`)) {
		t.Fatalf("Output = %s", got)
	}
}

func TestReaderReadFailures(t *testing.T) {
	t.Parallel()

	workbook := workbookBytes(t, testxlsx.Workbook{
		Sheets: []testxlsx.Sheet{
			{
				Name:      "TrialBalance",
				Dimension: "A1:B2",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Formula: "=1+1"},
				},
				Tables: []testxlsx.Table{{Name: "Dup", Range: "A1:B2"}},
			},
			{
				Name:      "Summary",
				Dimension: "A1:B2",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "2.20"},
				},
				Tables: []testxlsx.Table{{Name: "Dup", Range: "A1:B2"}},
			},
			{
				Name:      "Other",
				Dimension: "A1:B2",
				Cells: []testxlsx.Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "3.30"},
				},
				Tables: []testxlsx.Table{{Name: "Dup", Range: "A1:B2"}},
			},
		},
	})

	tests := []struct {
		name     string
		source   kernel.SourceBytes
		selector kernel.SourceSelector
		wantErr  error
	}{
		{
			name:     "missing bytes",
			source:   kernel.SourceBytes{Source: kernel.Source{Path: "fixture://book.xlsx", SHA256: "abc"}},
			selector: kernel.SourceSelector{Sheet: "TrialBalance"},
			wantErr:  ErrMissingSourceBytes,
		},
		{
			name:     "missing source hash",
			source:   kernel.SourceBytes{Source: kernel.Source{Path: "fixture://book.xlsx"}, Bytes: workbook},
			selector: kernel.SourceSelector{Sheet: "TrialBalance"},
			wantErr:  ErrMissingSourceSHA256,
		},
		{
			name:     "unsupported formula policy",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{Sheet: "TrialBalance", FormulaPolicy: "fail_closed"},
			wantErr:  ErrUnsupportedFormulaPolicy,
		},
		{
			name:     "ambiguous unqualified range",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{Range: "A1:B1"},
			wantErr:  ErrAmbiguousSelector,
		},
		{
			name:     "ambiguous table name after full selector set",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{TableName: "Dup", Range: "A1:B2"},
			wantErr:  ErrAmbiguousSelector,
		},
		{
			name:     "sheet table mismatch",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{Sheet: "Missing", TableName: "Dup"},
			wantErr:  ErrSelectorMismatch,
		},
		{
			name:     "formula without cached value",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{Sheet: "TrialBalance", Range: "B2"},
			wantErr:  ErrFormulaEvaluationUnsupported,
		},
		{
			name:     "invalid projection spacing",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{Sheet: "Summary", ColumnProjection: "A, B"},
			wantErr:  ErrInvalidColumnProjection,
		},
		{
			name:     "row window outside selection",
			source:   sourceWithHash("fixture://book.xlsx", workbook),
			selector: kernel.SourceSelector{Sheet: "Summary", RowWindow: "3-4"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name:     "malformed workbook",
			source:   sourceWithHash("fixture://bad.xlsx", []byte("not-a-zip")),
			selector: kernel.SourceSelector{Sheet: "Summary"},
			wantErr:  ErrInvalidWorkbookFixture,
		},
		{
			name:   "unsupported workbook cell type",
			source: sourceWithHash("fixture://unsupported.xlsx", unsupportedWorkbookBytes(t)),
			selector: kernel.SourceSelector{
				Sheet: "Unsupported",
			},
			wantErr: ErrInvalidWorkbookFixture,
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

func workbookBytes(t *testing.T, workbook testxlsx.Workbook) []byte {
	t.Helper()

	raw, err := testxlsx.Build(workbook)
	if err != nil {
		t.Fatalf("testxlsx.Build() error = %v", err)
	}
	return raw
}

func duplicateWorkbookRelsWorkbookBytes(t *testing.T) []byte {
	t.Helper()

	return workbookZipBytes(t, map[string]string{
		"[Content_Types].xml":        xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/_rels/workbook.xml.rels": xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/workbook.xml":            xmlHeader + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="TrialBalance" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml":   xmlHeader + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><dimension ref="A1"/><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>value</t></is></c></row></sheetData></worksheet>`,
	})
}

func duplicateWorksheetTableRelsWorkbookBytes(t *testing.T) []byte {
	t.Helper()

	return workbookZipBytes(t, map[string]string{
		"[Content_Types].xml":                 xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/tables/table11.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/></Types>`,
		"_rels/.rels":                         xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/_rels/workbook.xml.rels":          xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/workbook.xml":                     xmlHeader + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="TrialBalance" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/tables/table11.xml":               xmlHeader + `<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" name="Amounts" ref="A1:A2"/>`,
		"xl/worksheets/_rels/sheet1.xml.rels": xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table11.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table11.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":            xmlHeader + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><dimension ref="A1:A2"/><sheetData><row r="1"><c r="A1"><v>1</v></c></row><row r="2"><c r="A2"><v>2</v></c></row></sheetData><tableParts count="1"><tablePart r:id="rId1"/></tableParts></worksheet>`,
	})
}

func unsupportedWorkbookBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipText(t, zw, "[Content_Types].xml", xmlHeader+`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`)
	writeZipText(t, zw, "_rels/.rels", xmlHeader+`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	writeZipText(t, zw, "xl/workbook.xml", xmlHeader+`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Unsupported" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	writeZipText(t, zw, "xl/_rels/workbook.xml.rels", xmlHeader+`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	writeZipText(t, zw, "xl/worksheets/sheet1.xml", xmlHeader+`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><dimension ref="A1"/><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>bad</t></is></c></row></sheetData></worksheet>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func workbookZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		writeZipText(t, zw, name, files[name])
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func writeZipText(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()

	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", name, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("Write(%s) error = %v", name, err)
	}
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>`
