package testxlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

const relationshipsNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

type Workbook struct {
	Sheets []Sheet
}

type Sheet struct {
	Name      string
	Dimension string
	Cells     []Cell
	Tables    []Table
}

type Cell struct {
	A1          string
	Type        string
	Value       string
	Formula     string
	CachedValue string
}

type Table struct {
	Name  string
	Range string
}

func Build(workbook Workbook) ([]byte, error) {
	if len(workbook.Sheets) == 0 {
		return nil, fmt.Errorf("workbook must include at least one sheet")
	}

	sharedIndex := map[string]int{}
	sharedStrings := make([]string, 0)
	type filesMap map[string]string
	files := filesMap{}

	workbookRels := make([]relationship, 0, len(workbook.Sheets))
	workbookSheets := make([]workbookSheet, 0, len(workbook.Sheets))

	for i, sheet := range workbook.Sheets {
		sheetID := i + 1
		sheetPath := fmt.Sprintf("xl/worksheets/sheet%d.xml", sheetID)
		sheetRelID := fmt.Sprintf("rId%d", sheetID)
		workbookSheets = append(workbookSheets, workbookSheet{Name: sheet.Name, RelID: sheetRelID})
		workbookRels = append(workbookRels, relationship{ID: sheetRelID, Type: worksheetRelType, Target: fmt.Sprintf("worksheets/sheet%d.xml", sheetID)})

		sheetXML, sheetRels, tableFiles, err := buildSheet(sheet, sheetID, sharedIndex, &sharedStrings)
		if err != nil {
			return nil, err
		}
		files[sheetPath] = sheetXML
		if sheetRels != "" {
			files[fmt.Sprintf("xl/worksheets/_rels/sheet%d.xml.rels", sheetID)] = sheetRels
		}
		for name, body := range tableFiles {
			files[name] = body
		}
	}

	files["[Content_Types].xml"] = contentTypesXML(workbook)
	files["xl/workbook.xml"] = workbookXML(workbookSheets)
	files["xl/_rels/workbook.xml.rels"] = relationshipsXML(workbookRels)
	files["_rels/.rels"] = rootRelationshipsXML()
	if len(sharedStrings) > 0 {
		files["xl/sharedStrings.xml"] = sharedStringsXML(sharedStrings)
	}

	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type relationship struct {
	ID     string
	Type   string
	Target string
}

type workbookSheet struct {
	Name  string
	RelID string
}

const (
	worksheetRelType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"
	tableRelType     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/table"
)

func buildSheet(sheet Sheet, sheetID int, sharedIndex map[string]int, sharedStrings *[]string) (string, string, map[string]string, error) {
	type cellXML struct {
		Ref     string `xml:"r,attr"`
		Type    string `xml:"t,attr,omitempty"`
		Formula string `xml:"f,omitempty"`
		Value   string `xml:"v,omitempty"`
	}
	type rowXML struct {
		Index int       `xml:"r,attr"`
		Cells []cellXML `xml:"c"`
	}
	type sheetDataXML struct {
		Rows []rowXML `xml:"row"`
	}
	type dimensionXML struct {
		Ref string `xml:"ref,attr"`
	}
	type tablePartXML struct {
		RelID string `xml:"r:id,attr"`
	}
	type tableParts struct {
		Count int            `xml:"count,attr"`
		Parts []tablePartXML `xml:"tablePart"`
	}
	type worksheetXML struct {
		XMLName    xml.Name     `xml:"worksheet"`
		Xmlns      string       `xml:"xmlns,attr"`
		XmlnsR     string       `xml:"xmlns:r,attr"`
		Dimension  dimensionXML `xml:"dimension"`
		SheetData  sheetDataXML `xml:"sheetData"`
		TableParts *tableParts  `xml:"tableParts,omitempty"`
	}

	rowMap := map[int][]cellXML{}
	dimension := sheet.Dimension
	if dimension == "" && len(sheet.Cells) > 0 {
		first := sheet.Cells[0].A1
		last := sheet.Cells[len(sheet.Cells)-1].A1
		dimension = first + ":" + last
	}
	for _, cell := range sheet.Cells {
		row, err := parseRow(cell.A1)
		if err != nil {
			return "", "", nil, err
		}
		xcell := cellXML{Ref: cell.A1}
		switch {
		case cell.Formula != "":
			xcell.Formula = strings.TrimPrefix(cell.Formula, "=")
			xcell.Value = cell.CachedValue
		case cell.Type == "s" || (cell.Type == "" && shouldShare(cell.Value)):
			xcell.Type = "s"
			index, ok := sharedIndex[cell.Value]
			if !ok {
				index = len(*sharedStrings)
				sharedIndex[cell.Value] = index
				*sharedStrings = append(*sharedStrings, cell.Value)
			}
			xcell.Value = fmt.Sprintf("%d", index)
		default:
			xcell.Type = cell.Type
			xcell.Value = cell.Value
		}
		rowMap[row] = append(rowMap[row], xcell)
	}

	var rowNumbers []int
	for row := range rowMap {
		rowNumbers = append(rowNumbers, row)
	}
	sort.Ints(rowNumbers)
	rows := make([]rowXML, 0, len(rowNumbers))
	for _, row := range rowNumbers {
		cells := rowMap[row]
		sort.Slice(cells, func(i, j int) bool { return cells[i].Ref < cells[j].Ref })
		rows = append(rows, rowXML{Index: row, Cells: cells})
	}

	var rels []relationship
	tableFiles := map[string]string{}
	var tablePartsValue *tableParts
	if len(sheet.Tables) > 0 {
		parts := make([]tablePartXML, 0, len(sheet.Tables))
		rels = make([]relationship, 0, len(sheet.Tables))
		for i, table := range sheet.Tables {
			tableID := (sheetID * 10) + i + 1
			relID := fmt.Sprintf("rId%d", i+1)
			tablePath := fmt.Sprintf("xl/tables/table%d.xml", tableID)
			parts = append(parts, tablePartXML{RelID: relID})
			rels = append(rels, relationship{ID: relID, Type: tableRelType, Target: fmt.Sprintf("../tables/table%d.xml", tableID)})
			tableBody, err := tableXML(table)
			if err != nil {
				return "", "", nil, err
			}
			tableFiles[tablePath] = tableBody
		}
		tablePartsValue = &tableParts{Count: len(parts), Parts: parts}
	}

	worksheetDoc := worksheetXML{
		Xmlns:     "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
		XmlnsR:    relationshipsNS,
		Dimension: dimensionXML{Ref: dimension},
		SheetData: sheetDataXML{Rows: rows},
	}
	if tablePartsValue != nil {
		worksheetDoc.TableParts = tablePartsValue
	}
	worksheetBody, err := xml.Marshal(worksheetDoc)
	if err != nil {
		return "", "", nil, err
	}
	worksheetXMLText := xml.Header + string(worksheetBody)

	relsXMLText := ""
	if len(rels) > 0 {
		relsXMLText = relationshipsXML(rels)
	}
	return worksheetXMLText, relsXMLText, tableFiles, nil
}

func contentTypesXML(workbook Workbook) string {
	override := []string{
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`,
	}
	hasSharedStrings := false
	for i, sheet := range workbook.Sheets {
		override = append(override, fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i+1))
		if len(sheet.Tables) > 0 {
			for j := range sheet.Tables {
				override = append(override, fmt.Sprintf(`<Override PartName="/xl/tables/table%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/>`, (i+1)*10+j+1))
			}
		}
		for _, cell := range sheet.Cells {
			if cell.Formula == "" && (cell.Type == "s" || (cell.Type == "" && shouldShare(cell.Value))) {
				hasSharedStrings = true
			}
		}
	}
	if hasSharedStrings {
		override = append(override, `<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>`)
	}
	sort.Strings(override)
	return xml.Header + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		strings.Join(override, "") +
		`</Types>`
}

func workbookXML(sheets []workbookSheet) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="`)
	b.WriteString(relationshipsNS)
	b.WriteString(`"><sheets>`)
	for i, sheet := range sheets {
		b.WriteString(fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="%s"/>`, xmlEscape(sheet.Name), i+1, sheet.RelID))
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func relationshipsXML(rels []relationship) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, rel := range rels {
		b.WriteString(fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, rel.ID, rel.Type, xmlEscape(rel.Target)))
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func rootRelationshipsXML() string {
	return xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`
}

func sharedStringsXML(values []string) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="`)
	b.WriteString(fmt.Sprintf("%d", len(values)))
	b.WriteString(`" uniqueCount="`)
	b.WriteString(fmt.Sprintf("%d", len(values)))
	b.WriteString(`">`)
	for _, value := range values {
		b.WriteString(`<si><t>`)
		b.WriteString(xmlEscape(value))
		b.WriteString(`</t></si>`)
	}
	b.WriteString(`</sst>`)
	return b.String()
}

func tableXML(table Table) (string, error) {
	columnCount, err := tableRangeWidth(table.Range)
	if err != nil {
		return "", err
	}

	var columns strings.Builder
	for i := 1; i <= columnCount; i++ {
		columns.WriteString(fmt.Sprintf(`<tableColumn id="%d" name="col%d"/>`, i, i))
	}

	return xml.Header + fmt.Sprintf(
		`<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" id="1" name="%s" displayName="%s" ref="%s" totalsRowShown="0"><autoFilter ref="%s"/><tableColumns count="%d">%s</tableColumns><tableStyleInfo name="TableStyleMedium2" showFirstColumn="0" showLastColumn="0" showRowStripes="1" showColumnStripes="0"/></table>`,
		xmlEscape(table.Name),
		xmlEscape(table.Name),
		xmlEscape(table.Range),
		xmlEscape(table.Range),
		columnCount,
		columns.String(),
	), nil
}

func tableRangeWidth(raw string) (int, error) {
	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 1:
		_, _, err := tableParseA1(parts[0])
		if err != nil {
			return 0, err
		}
		return 1, nil
	case 2:
		_, startCol, err := tableParseA1(parts[0])
		if err != nil {
			return 0, err
		}
		_, endCol, err := tableParseA1(parts[1])
		if err != nil {
			return 0, err
		}
		if endCol < startCol {
			return 0, fmt.Errorf("invalid table range %q", raw)
		}
		return endCol - startCol + 1, nil
	default:
		return 0, fmt.Errorf("invalid table range %q", raw)
	}
}

func tableParseA1(raw string) (int, int, error) {
	if raw == "" {
		return 0, 0, fmt.Errorf("invalid table range %q", raw)
	}
	var columnLetters strings.Builder
	row := 0
	foundRow := false
	for _, r := range strings.ToUpper(raw) {
		switch {
		case r >= 'A' && r <= 'Z' && !foundRow:
			columnLetters.WriteRune(r)
		case r >= '0' && r <= '9':
			foundRow = true
			row = row*10 + int(r-'0')
		default:
			return 0, 0, fmt.Errorf("invalid table range %q", raw)
		}
	}
	if columnLetters.Len() == 0 || !foundRow || row <= 0 {
		return 0, 0, fmt.Errorf("invalid table range %q", raw)
	}

	column := 0
	for _, r := range columnLetters.String() {
		column = column*26 + int(r-'A'+1)
	}
	return row, column, nil
}

func parseRow(a1 string) (int, error) {
	for i := 0; i < len(a1); i++ {
		if a1[i] >= '0' && a1[i] <= '9' {
			var row int
			_, err := fmt.Sscanf(a1[i:], "%d", &row)
			return row, err
		}
	}
	return 0, fmt.Errorf("invalid cell reference %q", a1)
}

func shouldShare(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return true
		}
	}
	return false
}

func xmlEscape(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
