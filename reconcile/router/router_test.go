package router

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/reconcile/request"
)

func TestRouteSupportedFormats(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "bank.csv", []byte("Transaction ID,Amount,Currency\nA1,10.25,USD\nA2,20.50,USD\n"))
	writeFile(t, dir, "events.jsonl", []byte("{\"transaction_id\":\"A1\",\"amount\":\"10.25\",\"currency\":\"USD\"}\n{\"transaction_id\":\"A2\",\"amount\":\"20.50\",\"currency\":\"USD\"}\n"))
	writeFile(t, dir, "invoice.json", []byte("{\"items\":[{\"transaction_id\":\"A1\",\"amount\":\"10.25\",\"currency\":\"USD\"},{\"transaction_id\":\"A2\",\"amount\":\"20.50\",\"currency\":\"USD\"}]}"))
	writeFile(t, dir, "ledger.xlsx", workbookBytes(t))

	req := request.Request{
		Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
		Files: []request.FileConfig{
			{
				ID:      "csv",
				Purpose: "bank",
				Format:  request.FormatCSV,
				Path:    "bank.csv",
				Reader:  request.ReaderExtractCSVRows,
				Selector: request.SelectorConfig{
					RowWindow: "2:",
					Encoding:  "utf-8",
					Dialect:   "excel",
				},
				Columns: request.Columns{
					"transaction_id": {Source: "Transaction ID", Role: request.RoleKey, Required: true},
					"amount":         {Source: "Amount", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
					"currency":       {Source: "Currency", Role: request.RoleCurrency, Required: true},
				},
			},
			{
				ID:      "jsonl",
				Purpose: "stream",
				Format:  request.FormatJSONL,
				Path:    "events.jsonl",
				Reader:  request.ReaderExtractJSONL,
				Selector: request.SelectorConfig{
					LineWindow: "1:",
				},
				Columns: request.Columns{
					"transaction_id": {Source: "transaction_id", Role: request.RoleKey, Required: true},
					"amount":         {Source: "amount", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
					"currency":       {Source: "currency", Role: request.RoleCurrency, Required: true},
				},
			},
			{
				ID:      "json",
				Purpose: "document",
				Format:  request.FormatJSON,
				Path:    "invoice.json",
				Reader:  request.ReaderExtractJSON,
				Selector: request.SelectorConfig{
					JSONPointer: "/items",
				},
				Columns: request.Columns{
					"transaction_id": {Source: "transaction_id", Role: request.RoleKey, Required: true},
					"amount":         {Source: "amount", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
					"currency":       {Source: "currency", Role: request.RoleCurrency, Required: true},
				},
			},
			{
				ID:      "xlsx",
				Purpose: "ledger",
				Format:  request.FormatXLSX,
				Path:    "ledger.xlsx",
				Reader:  request.ReaderExtractSpreadsheet,
				Selector: request.SelectorConfig{
					Table:     "Amounts",
					RowWindow: "2-3",
				},
				Columns: request.Columns{
					"transaction_id": {Source: "A", Role: request.RoleKey, Required: true},
					"amount":         {Source: "B", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
					"currency":       {Source: "C", Role: request.RoleCurrency, Required: true},
				},
			},
		},
		Reconcile: request.ReconcileConfig{Mode: request.ModeExactMatch, Left: "csv", Right: "jsonl", MatchOn: []string{"transaction_id"}, Compare: []string{"amount", "currency"}},
	}

	files, err := Route(context.Background(), req, Options{BaseDir: dir})
	if err != nil {
		returnError(t, err)
	}
	if len(files) != 4 {
		t.Fatalf("len(files) = %d, want 4", len(files))
	}
	if files[0].EffectiveSelector.RowWindow != "2-3" || files[0].EffectiveSelector.HeaderRow != 1 {
		t.Fatalf("csv effective selector = %#v", files[0].EffectiveSelector)
	}
	if files[1].DeclaredSelector.LineWindow != "1:" || files[1].EffectiveSelector.LineWindow != "1-2" {
		t.Fatalf("jsonl selectors = %#v / %#v", files[1].DeclaredSelector, files[1].EffectiveSelector)
	}
	if files[2].Rows[1].Values["amount"].Canonical != "20.50" {
		t.Fatalf("json amount = %#v", files[2].Rows[1].Values["amount"])
	}
	if files[2].Rows[0].Values["amount"].RawExact {
		t.Fatalf("json field raw_exact = true, want false")
	}
	if files[3].EffectiveSelector.TableName != "Amounts" || files[3].EffectiveSelector.RowWindow != "2-3" {
		t.Fatalf("xlsx effective selector = %#v", files[3].EffectiveSelector)
	}
	if files[3].Rows[0].Values["amount"].Canonical != "10.25" {
		t.Fatalf("xlsx amount = %#v", files[3].Rows[0].Values["amount"])
	}

	again, err := Route(context.Background(), req, Options{BaseDir: dir})
	if err != nil {
		returnError(t, err)
	}
	if files[0].SourceHash != again[0].SourceHash || files[0].OutputHash != again[0].OutputHash {
		t.Fatalf("hashes changed across runs: %#v %#v", files[0], again[0])
	}
}

func TestRouteCSVInferredHeaderAllowsMixedHeaderAndIndexSources(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "bank.csv", []byte("Transaction ID,Amount,Currency\nA1,10.25,USD\nA2,20.50,USD\n"))

	req := request.Request{
		Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
		Files: []request.FileConfig{
			mixedCSVFile("left", "bank.csv"),
			mixedCSVFile("right", "bank.csv"),
		},
		Reconcile: request.ReconcileConfig{Mode: request.ModeExactMatch, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}, Compare: []string{"amount", "currency"}},
	}

	files, err := Route(context.Background(), req, Options{BaseDir: dir})
	if err != nil {
		returnError(t, err)
	}
	if files[0].EffectiveSelector.HeaderRow != 1 {
		t.Fatalf("HeaderRow = %d, want inferred header row 1", files[0].EffectiveSelector.HeaderRow)
	}
	if files[0].EffectiveSelector.ColumnProjection != "" {
		t.Fatalf("ColumnProjection = %q, want empty projection for mixed header/index sources", files[0].EffectiveSelector.ColumnProjection)
	}
	if got := files[0].Rows[0].Values["transaction_id"].Canonical; got != "A1" {
		t.Fatalf("transaction_id canonical = %q, want A1", got)
	}
	if got := files[0].Rows[0].Values["amount"].Canonical; got != "10.25" {
		t.Fatalf("amount canonical = %q, want 10.25", got)
	}
}

func TestRouteFailures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "bank.csv", []byte("Transaction ID,Amount,Currency\nA1,10.25,USD\n"))

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", "missing.csv"),
				validCSVFile("right", "bank.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: dir})
		if err == nil {
			t.Fatalf("Route() error = nil, want missing file")
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		t.Parallel()
		_, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", "bank.csv"),
				validCSVFile("right", "bank.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: dir, MaxFileBytes: 8})
		if err == nil || !strings.Contains(err.Error(), "route left: input_too_large: bank.csv exceeds 8 bytes") {
			t.Fatalf("Route() error = %v, want oversized bounded-read rejection", err)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		t.Parallel()

		absPath := filepath.Join(dir, "bank.csv")
		_, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", absPath),
				validCSVFile("right", "bank.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: dir})
		if err == nil || !strings.Contains(err.Error(), "route left: path_invalid: absolute path rejected") {
			t.Fatalf("Route() error = %v, want absolute path rejection", err)
		}
	})

	t.Run("directory rejected", func(t *testing.T) {
		t.Parallel()

		if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}

		_, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", "nested"),
				validCSVFile("right", "bank.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: dir})
		if err == nil || !strings.Contains(err.Error(), "route left: input_invalid: nested is not a regular file") {
			t.Fatalf("Route() error = %v, want directory rejection", err)
		}
	})

	t.Run("base dir escape rejected", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		baseDir := filepath.Join(root, "base")
		if err := os.Mkdir(baseDir, 0o755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		writeFile(t, root, "outside.csv", []byte("Transaction ID,Amount,Currency\nA1,10.25,USD\n"))

		_, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", "../outside.csv"),
				validCSVFile("right", "../outside.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: baseDir})
		if err == nil || !strings.Contains(err.Error(), "route left: path_invalid: path escapes base_dir") {
			t.Fatalf("Route() error = %v, want base-dir escape rejection", err)
		}
	})

	t.Run("symlink inside base to target inside base allowed", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		baseDir := filepath.Join(root, "base")
		if err := os.MkdirAll(baseDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		writeFile(t, baseDir, "bank.csv", []byte("Transaction ID,Amount,Currency\nA1,10.25,USD\n"))
		linkPath := filepath.Join(baseDir, "linked.csv")
		if err := os.Symlink(filepath.Join(baseDir, "bank.csv"), linkPath); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		files, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", "linked.csv"),
				validCSVFile("right", "bank.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: baseDir})
		if err != nil {
			t.Fatalf("Route() error = %v", err)
		}
		wantPath, err := filepath.EvalSymlinks(filepath.Join(baseDir, "bank.csv"))
		if err != nil {
			t.Fatalf("EvalSymlinks() error = %v", err)
		}
		if files[0].SourcePath != wantPath || files[0].CapturedRead.SourcePath != wantPath {
			t.Fatalf("resolved symlink path = %q / %q, want %q", files[0].SourcePath, files[0].CapturedRead.SourcePath, wantPath)
		}
	})

	t.Run("symlink inside base to target outside base rejected", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		baseDir := filepath.Join(root, "base")
		outsideDir := filepath.Join(root, "outside")
		if err := os.MkdirAll(baseDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(base) error = %v", err)
		}
		if err := os.MkdirAll(outsideDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(outside) error = %v", err)
		}
		writeFile(t, outsideDir, "bank.csv", []byte("Transaction ID,Amount,Currency\nA1,10.25,USD\n"))
		if err := os.Symlink(filepath.Join(outsideDir, "bank.csv"), filepath.Join(baseDir, "escape.csv")); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		_, err := Route(context.Background(), request.Request{
			Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
			Files: []request.FileConfig{
				validCSVFile("left", "escape.csv"),
				validCSVFile("right", "escape.csv"),
			},
			Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
		}, Options{BaseDir: baseDir})
		if err == nil || !strings.Contains(err.Error(), "route left: path_invalid: path escapes base_dir") {
			t.Fatalf("Route() error = %v, want symlink escape rejection", err)
		}
	})

	t.Run("captured read invariants", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name    string
			mutate  func(kernel.ReadResult, kernel.Run, kernel.SourceBytes, kernel.SourceSelector) kernel.ReadResult
			wantErr string
		}{
			{
				name: "mismatched run id",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.RunID = "run_other"
					return result
				},
				wantErr: "route left: captured_read_invalid: run_id",
			},
			{
				name: "mismatched source path",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.SourcePath = result.CapturedRead.SourcePath + ".other"
					return result
				},
				wantErr: "route left: captured_read_invalid: source_path",
			},
			{
				name: "mismatched selector",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.Selector = kernel.SourceSelector{RowWindow: "99-99", HeaderRow: 1, Dialect: "excel", Encoding: "utf-8"}
					return result
				},
				wantErr: "route left: captured_read_invalid: selector",
			},
			{
				name: "mismatched source hash",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.SourceSHA256 = strings.Repeat("0", 64)
					return result
				},
				wantErr: "route left: captured_read_invalid: source_hash",
			},
			{
				name: "mismatched output hash",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.OutputSHA256 = strings.Repeat("0", 64)
					return result
				},
				wantErr: "route left: captured_read_invalid: output_hash",
			},
			{
				name: "mismatched output bytes",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.Output = json.RawMessage(`{"rows":[]}`)
					return result
				},
				wantErr: "route left: captured_read_invalid: output",
			},
			{
				name: "mismatched reader name",
				mutate: func(result kernel.ReadResult, _ kernel.Run, _ kernel.SourceBytes, _ kernel.SourceSelector) kernel.ReadResult {
					result.CapturedRead.ReaderName = "extract.other"
					return result
				},
				wantErr: "route left: captured_read_invalid: reader_name",
			},
		}

		for _, tt := range cases {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				_, err := Route(context.Background(), request.Request{
					Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
					Files: []request.FileConfig{
						validCSVFile("left", "bank.csv"),
						validCSVFile("right", "bank.csv"),
					},
					Reconcile: request.ReconcileConfig{Mode: request.ModeExistence, Left: "left", Right: "right", MatchOn: []string{"transaction_id"}},
				}, Options{BaseDir: dir, Readers: Readers{CSV: stubCSVReader{mutate: tt.mutate}}})
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Route() error = %v, want %q", err, tt.wantErr)
				}
			})
		}
	})
}

func TestNormalizeJSONRowsWholeValueSourceUsesCanonicalValue(t *testing.T) {
	t.Parallel()

	t.Run("object root", func(t *testing.T) {
		t.Parallel()

		rows, err := normalizeJSONRows(request.FileConfig{
			Format: request.FormatJSON,
			Columns: request.Columns{
				"document": {Source: ".", Role: request.RoleKey, Required: true},
			},
		}, []byte(`{"raw_value":" {\n  \"b\": 2, \"a\": 1\n } ","value":{"b":2,"a":1}}`))
		if err != nil {
			t.Fatalf("normalizeJSONRows() error = %v", err)
		}
		got := rows[0].Values["document"]
		if got.RawExact {
			t.Fatalf("object whole-value raw_exact = true, want false")
		}
		if got.Raw != `{"a":1,"b":2}` {
			t.Fatalf("object whole-value raw = %q, want canonical object", got.Raw)
		}
		if got.Canonical != `{"a":1,"b":2}` {
			t.Fatalf("object whole-value canonical = %q, want canonical object", got.Canonical)
		}
	})

	t.Run("scalar root", func(t *testing.T) {
		t.Parallel()

		rows, err := normalizeJSONRows(request.FileConfig{
			Format: request.FormatJSON,
			Columns: request.Columns{
				"value": {Source: ".", Role: request.RoleKey, Required: true},
			},
		}, []byte(`{"raw_value":"\"USD\"","value":"USD"}`))
		if err != nil {
			t.Fatalf("normalizeJSONRows() error = %v", err)
		}
		got := rows[0].Values["value"]
		if got.RawExact {
			t.Fatalf("scalar whole-value raw_exact = true, want false")
		}
		if got.Raw != "USD" {
			t.Fatalf("scalar whole-value raw = %q, want canonical scalar", got.Raw)
		}
		if got.Canonical != "USD" {
			t.Fatalf("scalar whole-value canonical = %q, want canonical scalar", got.Canonical)
		}
	})
}

func validCSVFile(id, path string) request.FileConfig {
	return request.FileConfig{
		ID:      id,
		Purpose: "bank",
		Format:  request.FormatCSV,
		Path:    path,
		Reader:  request.ReaderExtractCSVRows,
		Selector: request.SelectorConfig{
			RowWindow: "2:",
			Encoding:  "utf-8",
			Dialect:   "excel",
		},
		Columns: request.Columns{
			"transaction_id": {Source: "Transaction ID", Role: request.RoleKey, Required: true},
			"amount":         {Source: "Amount", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
			"currency":       {Source: "Currency", Role: request.RoleCurrency, Required: true},
		},
	}
}

func mixedCSVFile(id, path string) request.FileConfig {
	file := validCSVFile(id, path)
	file.Columns["amount"] = request.ColumnConfig{Source: "2", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}}
	return file
}

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}

func workbookBytes(t *testing.T) []byte {
	t.Helper()

	files := map[string]string{
		"[Content_Types].xml":                 xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/tables/table1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/></Types>`,
		"_rels/.rels":                         xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/_rels/workbook.xml.rels":          xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/workbook.xml":                     xmlHeader + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="TrialBalance" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/worksheets/_rels/sheet1.xml.rels": xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":            xmlHeader + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><dimension ref="A1:C3"/><sheetData><row r="1"><c r="A1" t="str"><v>transaction_id</v></c><c r="B1" t="str"><v>amount</v></c><c r="C1" t="str"><v>currency</v></c></row><row r="2"><c r="A2" t="str"><v>A1</v></c><c r="B2"><v>10.25</v></c><c r="C2" t="str"><v>USD</v></c></row><row r="3"><c r="A3" t="str"><v>A2</v></c><c r="B3"><v>20.50</v></c><c r="C3" t="str"><v>USD</v></c></row></sheetData><tableParts count="1"><tablePart r:id="rId1"/></tableParts></worksheet>`,
		"xl/tables/table1.xml":                xmlHeader + `<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" id="1" name="Amounts" displayName="Amounts" ref="A1:C3"><autoFilter ref="A1:C3"/><tableColumns count="3"><tableColumn id="1" name="transaction_id"/><tableColumn id="2" name="amount"/><tableColumn id="3" name="currency"/></tableColumns><tableStyleInfo name="TableStyleMedium2" showFirstColumn="0" showLastColumn="0" showRowStripes="1" showColumnStripes="0"/></table>`,
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/_rels/workbook.xml.rels", "xl/workbook.xml", "xl/worksheets/_rels/sheet1.xml.rels", "xl/worksheets/sheet1.xml", "xl/tables/table1.xml"} {
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

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

func returnError(t *testing.T, err error) {
	t.Helper()
	t.Fatalf("Route() error = %v", err)
}

type stubCSVReader struct {
	mutate func(kernel.ReadResult, kernel.Run, kernel.SourceBytes, kernel.SourceSelector) kernel.ReadResult
}

func (r stubCSVReader) Read(_ context.Context, run kernel.Run, source kernel.SourceBytes, selector kernel.SourceSelector) (kernel.ReadResult, error) {
	output := json.RawMessage(`{"rows":[{"row":2,"window_index":0,"cells":[{"column":0,"header":"Transaction ID","raw_value":"A1","value":"A1"},{"column":1,"header":"Amount","raw_value":"10.25","value":"10.25"},{"column":2,"header":"Currency","raw_value":"USD","value":"USD"}]}]}`)
	result := kernel.ReadResult{
		Output: output,
		CapturedRead: kernel.CapturedRead{
			RunID:         run.ID,
			SourcePath:    source.Source.Path,
			Selector:      selector,
			ReaderName:    request.ReaderExtractCSVRows,
			ReaderVersion: "test",
			SourceSHA256:  source.Source.SHA256,
			OutputSHA256:  sha256Hex([]byte(output)),
			Output:        output,
		},
	}
	if r.mutate != nil {
		result = r.mutate(result, run, source, selector)
	}
	return result, nil
}
