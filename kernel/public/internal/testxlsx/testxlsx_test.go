package testxlsx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestBuildTableFixtureEmitsValidContentTypesAndTableMetadata(t *testing.T) {
	t.Parallel()

	raw, err := Build(Workbook{
		Sheets: []Sheet{
			{
				Name:      "TrialBalance",
				Dimension: "A1:C4",
				Cells: []Cell{
					{A1: "A1", Value: "account"},
					{A1: "B1", Value: "amount"},
					{A1: "C1", Value: "note"},
					{A1: "A2", Value: "cash"},
					{A1: "B2", Value: "1.10"},
					{A1: "C2", Value: "open"},
					{A1: "A3", Value: "ar"},
					{A1: "B3", Value: "2.20"},
					{A1: "C3", Value: "carry"},
					{A1: "A4", Value: "ap"},
					{A1: "B4", Value: "3.30"},
					{A1: "C4", Value: "close"},
				},
				Tables: []Table{{Name: "Amounts", Range: "A1:C4"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	files := unzipFiles(t, raw)

	contentTypes := files["[Content_Types].xml"]
	if got := strings.Count(contentTypes, `<Default Extension="xml" ContentType="application/xml"/>`); got != 1 {
		t.Fatalf("[Content_Types].xml xml default count = %d, want 1\n%s", got, contentTypes)
	}

	tableXML := files["xl/tables/table11.xml"]
	wantColumns := `<tableColumns count="3"><tableColumn id="1" name="col1"/><tableColumn id="2" name="col2"/><tableColumn id="3" name="col3"/></tableColumns>`
	if !strings.Contains(tableXML, wantColumns) {
		t.Fatalf("xl/tables/table11.xml = %s, want %s", tableXML, wantColumns)
	}
}

func unzipFiles(t *testing.T, raw []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}

	files := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("Open(%s) error = %v", file.Name, err)
		}
		body, err := io.ReadAll(rc)
		if closeErr := rc.Close(); closeErr != nil {
			t.Fatalf("Close(%s) error = %v", file.Name, closeErr)
		}
		if err != nil {
			t.Fatalf("ReadAll(%s) error = %v", file.Name, err)
		}
		files[file.Name] = string(body)
	}
	return files
}
