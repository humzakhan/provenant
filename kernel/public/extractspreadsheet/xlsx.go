package extractspreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const (
	maxWorkbookFiles     = 128
	maxWorkbookFileBytes = 4 << 20
	maxWorkbookTotalSize = 16 << 20

	workbookPath      = "xl/workbook.xml"
	workbookRelsPath  = "xl/_rels/workbook.xml.rels"
	sharedStringsPath = "xl/sharedStrings.xml"
	relationshipsNS   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	worksheetRelType  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"
	tableRelType      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/table"
)

type workbookXML struct {
	Sheets []workbookSheetXML `xml:"sheets>sheet"`
}

type workbookSheetXML struct {
	Name  string `xml:"name,attr"`
	RelID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type relationshipsXML struct {
	Relationships []relationshipXML `xml:"Relationship"`
}

type relationshipXML struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

type worksheetXML struct {
	Dimension  worksheetDimensionXML `xml:"dimension"`
	SheetData  worksheetSheetDataXML `xml:"sheetData"`
	TableParts []worksheetTableXML   `xml:"tableParts>tablePart"`
}

type worksheetDimensionXML struct {
	Ref string `xml:"ref,attr"`
}

type worksheetSheetDataXML struct {
	Rows []worksheetRowXML `xml:"row"`
}

type worksheetRowXML struct {
	Cells []worksheetCellXML `xml:"c"`
}

type worksheetCellXML struct {
	Ref     string `xml:"r,attr"`
	Type    string `xml:"t,attr"`
	Formula string `xml:"f"`
	Value   string `xml:"v"`
}

type worksheetTableXML struct {
	RelID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type sharedStringsXML struct {
	Items []sharedStringItemXML `xml:"si"`
}

type sharedStringItemXML struct {
	Text string               `xml:"t"`
	Runs []sharedStringRunXML `xml:"r"`
}

type sharedStringRunXML struct {
	Text string `xml:"t"`
}

type tableXML struct {
	Name        string `xml:"name,attr"`
	DisplayName string `xml:"displayName,attr"`
	Ref         string `xml:"ref,attr"`
}

func parseWorkbook(source []byte) (*workbook, error) {
	files, err := readWorkbookFiles(source)
	if err != nil {
		return nil, err
	}

	workbookData, ok := files[workbookPath]
	if !ok {
		return nil, fmt.Errorf("%w: missing %s", ErrInvalidWorkbookFixture, workbookPath)
	}
	workbookRelsData, ok := files[workbookRelsPath]
	if !ok {
		return nil, fmt.Errorf("%w: missing %s", ErrInvalidWorkbookFixture, workbookRelsPath)
	}

	sheetTargets, err := parseWorkbookRelationships(workbookRelsData)
	if err != nil {
		return nil, err
	}
	sharedStrings, err := parseSharedStrings(files)
	if err != nil {
		return nil, err
	}

	var workbookDoc workbookXML
	if err := decodeXMLStrict(workbookData, &workbookDoc); err != nil {
		return nil, fmt.Errorf("%w: workbook.xml: %v", ErrInvalidWorkbookFixture, err)
	}
	if len(workbookDoc.Sheets) == 0 {
		return nil, fmt.Errorf("%w: missing workbook sheets", ErrInvalidWorkbookFixture)
	}

	book := &workbook{sheets: make([]*sheet, 0, len(workbookDoc.Sheets))}
	seenSheets := make(map[string]struct{}, len(workbookDoc.Sheets))
	for _, rawSheet := range workbookDoc.Sheets {
		if rawSheet.Name == "" {
			return nil, fmt.Errorf("%w: sheet name is required", ErrInvalidWorkbookFixture)
		}
		if rawSheet.RelID == "" {
			return nil, fmt.Errorf("%w: sheet %q missing relationship id", ErrInvalidWorkbookFixture, rawSheet.Name)
		}
		if _, exists := seenSheets[rawSheet.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate sheet %q", ErrInvalidWorkbookFixture, rawSheet.Name)
		}
		seenSheets[rawSheet.Name] = struct{}{}

		worksheetPath, ok := sheetTargets[rawSheet.RelID]
		if !ok {
			return nil, fmt.Errorf("%w: sheet %q missing worksheet target", ErrInvalidWorkbookFixture, rawSheet.Name)
		}
		worksheetData, ok := files[worksheetPath]
		if !ok {
			return nil, fmt.Errorf("%w: missing worksheet %s", ErrInvalidWorkbookFixture, worksheetPath)
		}
		parsedSheet, err := parseWorksheet(rawSheet.Name, worksheetPath, worksheetData, files, sharedStrings)
		if err != nil {
			return nil, err
		}
		book.sheets = append(book.sheets, parsedSheet)
	}

	return book, nil
}

func readWorkbookFiles(source []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		return nil, fmt.Errorf("%w: open zip: %v", ErrInvalidWorkbookFixture, err)
	}
	if len(reader.File) == 0 {
		return nil, fmt.Errorf("%w: empty zip archive", ErrInvalidWorkbookFixture)
	}
	if len(reader.File) > maxWorkbookFiles {
		return nil, fmt.Errorf("%w: too many files", ErrInvalidWorkbookFixture)
	}

	files := make(map[string][]byte, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		cleanName := path.Clean(file.Name)
		if cleanName == "." || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "../") {
			return nil, fmt.Errorf("%w: invalid zip entry %q", ErrInvalidWorkbookFixture, file.Name)
		}
		if _, exists := files[cleanName]; exists {
			return nil, fmt.Errorf("%w: duplicate zip entry %q", ErrInvalidWorkbookFixture, cleanName)
		}
		if file.UncompressedSize64 > maxWorkbookFileBytes {
			return nil, fmt.Errorf("%w: oversized zip entry %q", ErrInvalidWorkbookFixture, cleanName)
		}
		total += file.UncompressedSize64
		if total > maxWorkbookTotalSize {
			return nil, fmt.Errorf("%w: workbook exceeds size limit", ErrInvalidWorkbookFixture)
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: open %s: %v", ErrInvalidWorkbookFixture, cleanName, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxWorkbookFileBytes+1))
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%w: read %s: %v", ErrInvalidWorkbookFixture, cleanName, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("%w: close %s: %v", ErrInvalidWorkbookFixture, cleanName, closeErr)
		}
		if uint64(len(data)) != file.UncompressedSize64 {
			return nil, fmt.Errorf("%w: truncated zip entry %q", ErrInvalidWorkbookFixture, cleanName)
		}
		files[cleanName] = data
	}
	return files, nil
}

func parseWorkbookRelationships(raw []byte) (map[string]string, error) {
	var rels relationshipsXML
	if err := decodeXMLStrict(raw, &rels); err != nil {
		return nil, fmt.Errorf("%w: workbook relationships: %v", ErrInvalidWorkbookFixture, err)
	}
	targets := make(map[string]string, len(rels.Relationships))
	seen := make(map[string]struct{}, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		if rel.ID == "" || rel.Target == "" {
			return nil, fmt.Errorf("%w: worksheet relationship is incomplete", ErrInvalidWorkbookFixture)
		}
		if _, exists := seen[rel.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate workbook relationship id %q", ErrInvalidWorkbookFixture, rel.ID)
		}
		seen[rel.ID] = struct{}{}
		if rel.Type != worksheetRelType {
			continue
		}
		targets[rel.ID] = resolveZipTarget("xl/workbook.xml", rel.Target)
	}
	return targets, nil
}

func parseSharedStrings(files map[string][]byte) ([]string, error) {
	raw, ok := files[sharedStringsPath]
	if !ok {
		return nil, nil
	}
	var doc sharedStringsXML
	if err := decodeXMLStrict(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: shared strings: %v", ErrInvalidWorkbookFixture, err)
	}
	values := make([]string, 0, len(doc.Items))
	for _, item := range doc.Items {
		switch {
		case item.Text != "":
			values = append(values, item.Text)
		case len(item.Runs) > 0:
			var b strings.Builder
			for _, run := range item.Runs {
				b.WriteString(run.Text)
			}
			values = append(values, b.String())
		default:
			values = append(values, "")
		}
	}
	return values, nil
}

func parseWorksheet(sheetName, worksheetPath string, raw []byte, files map[string][]byte, sharedStrings []string) (*sheet, error) {
	var doc worksheetXML
	if err := decodeXMLStrict(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: worksheet %q: %v", ErrInvalidWorkbookFixture, sheetName, err)
	}

	parsed := &sheet{
		name:  sheetName,
		cells: make(map[cellKey]cell),
	}

	haveCells := false
	derivedRange := gridRange{}
	for _, row := range doc.SheetData.Rows {
		for _, rawCell := range row.Cells {
			if rawCell.Ref == "" {
				return nil, fmt.Errorf("%w: worksheet %q cell missing reference", ErrInvalidWorkbookFixture, sheetName)
			}
			rowNum, colNum, err := parseA1(rawCell.Ref)
			if err != nil {
				return nil, fmt.Errorf("%w: worksheet %q cell %q: %v", ErrInvalidWorkbookFixture, sheetName, rawCell.Ref, err)
			}
			key := cellKey{row: rowNum, column: colNum}
			if _, exists := parsed.cells[key]; exists {
				return nil, fmt.Errorf("%w: worksheet %q duplicate cell %s", ErrInvalidWorkbookFixture, sheetName, rawCell.Ref)
			}
			cellValue, formula, cachedValue, err := resolveWorksheetCellValue(sheetName, rawCell, sharedStrings)
			if err != nil {
				return nil, err
			}
			parsed.cells[key] = cell{
				a1:          formatCellRef(rowNum, colNum),
				row:         rowNum,
				column:      colNum,
				value:       cellValue,
				formula:     formula,
				cachedValue: cachedValue,
			}
			if !haveCells {
				derivedRange = gridRange{startRow: rowNum, endRow: rowNum, startCol: colNum, endCol: colNum}
				haveCells = true
			} else {
				derivedRange.startRow = min(derivedRange.startRow, rowNum)
				derivedRange.endRow = max(derivedRange.endRow, rowNum)
				derivedRange.startCol = min(derivedRange.startCol, colNum)
				derivedRange.endCol = max(derivedRange.endCol, colNum)
			}
		}
	}
	if !haveCells {
		return nil, fmt.Errorf("%w: sheet %q has no cells", ErrInvalidWorkbookFixture, sheetName)
	}

	if doc.Dimension.Ref != "" {
		dimensionRange, _, err := parseRange(doc.Dimension.Ref)
		if err != nil {
			return nil, fmt.Errorf("%w: worksheet %q invalid dimension %q", ErrInvalidWorkbookFixture, sheetName, doc.Dimension.Ref)
		}
		if !rangeContains(dimensionRange, derivedRange) {
			return nil, fmt.Errorf("%w: worksheet %q dimension %q does not cover cells", ErrInvalidWorkbookFixture, sheetName, doc.Dimension.Ref)
		}
		parsed.usedRange = dimensionRange
	} else {
		parsed.usedRange = derivedRange
	}

	tables, err := parseWorksheetTables(sheetName, worksheetPath, doc.TableParts, files)
	if err != nil {
		return nil, err
	}
	parsed.tables = tables

	return parsed, nil
}

func resolveWorksheetCellValue(sheetName string, raw worksheetCellXML, sharedStrings []string) (string, string, string, error) {
	formula := strings.TrimSpace(raw.Formula)
	value := raw.Value
	switch raw.Type {
	case "", "n", "str":
		if formula != "" {
			return "", "=" + formula, value, nil
		}
		return value, "", "", nil
	case "s":
		if formula != "" {
			return "", "", "", fmt.Errorf("%w: worksheet %q cell %s shared-string formula is unsupported", ErrInvalidWorkbookFixture, sheetName, raw.Ref)
		}
		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 0 || index >= len(sharedStrings) {
			return "", "", "", fmt.Errorf("%w: worksheet %q cell %s shared string index", ErrInvalidWorkbookFixture, sheetName, raw.Ref)
		}
		return sharedStrings[index], "", "", nil
	default:
		return "", "", "", fmt.Errorf("%w: worksheet %q cell %s type %q", ErrInvalidWorkbookFixture, sheetName, raw.Ref, raw.Type)
	}
}

func parseWorksheetTables(sheetName, worksheetPath string, tableParts []worksheetTableXML, files map[string][]byte) ([]table, error) {
	if len(tableParts) == 0 {
		return nil, nil
	}

	relsPath := worksheetRelationshipsPath(worksheetPath)
	relsData, ok := files[relsPath]
	if !ok {
		return nil, fmt.Errorf("%w: worksheet %q missing relationships", ErrInvalidWorkbookFixture, sheetName)
	}

	var rels relationshipsXML
	if err := decodeXMLStrict(relsData, &rels); err != nil {
		return nil, fmt.Errorf("%w: worksheet %q relationships: %v", ErrInvalidWorkbookFixture, sheetName, err)
	}
	relTargets := make(map[string]string, len(rels.Relationships))
	seen := make(map[string]struct{}, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		if rel.ID == "" || rel.Target == "" {
			return nil, fmt.Errorf("%w: worksheet %q table relationship is incomplete", ErrInvalidWorkbookFixture, sheetName)
		}
		if _, exists := seen[rel.ID]; exists {
			return nil, fmt.Errorf("%w: worksheet %q duplicate table relationship id %q", ErrInvalidWorkbookFixture, sheetName, rel.ID)
		}
		seen[rel.ID] = struct{}{}
		if rel.Type != tableRelType {
			continue
		}
		relTargets[rel.ID] = resolveZipTarget(worksheetPath, rel.Target)
	}

	tables := make([]table, 0, len(tableParts))
	seenNames := make(map[string]struct{}, len(tableParts))
	for _, part := range tableParts {
		if part.RelID == "" {
			return nil, fmt.Errorf("%w: worksheet %q table part missing relationship id", ErrInvalidWorkbookFixture, sheetName)
		}
		tablePath, ok := relTargets[part.RelID]
		if !ok {
			return nil, fmt.Errorf("%w: worksheet %q table part missing target", ErrInvalidWorkbookFixture, sheetName)
		}
		tableData, ok := files[tablePath]
		if !ok {
			return nil, fmt.Errorf("%w: worksheet %q missing table %s", ErrInvalidWorkbookFixture, sheetName, tablePath)
		}
		var doc tableXML
		if err := decodeXMLStrict(tableData, &doc); err != nil {
			return nil, fmt.Errorf("%w: worksheet %q table %s: %v", ErrInvalidWorkbookFixture, sheetName, tablePath, err)
		}
		name := doc.DisplayName
		if name == "" {
			name = doc.Name
		}
		if name == "" {
			return nil, fmt.Errorf("%w: worksheet %q table %s missing name", ErrInvalidWorkbookFixture, sheetName, tablePath)
		}
		if _, exists := seenNames[name]; exists {
			return nil, fmt.Errorf("%w: worksheet %q duplicate table %q", ErrInvalidWorkbookFixture, sheetName, name)
		}
		seenNames[name] = struct{}{}
		rng, rangeSheet, err := parseRange(doc.Ref)
		if err != nil {
			return nil, fmt.Errorf("%w: worksheet %q table %q range: %v", ErrInvalidWorkbookFixture, sheetName, name, err)
		}
		if rangeSheet != "" {
			return nil, fmt.Errorf("%w: worksheet %q table %q uses qualified ref", ErrInvalidWorkbookFixture, sheetName, name)
		}
		tables = append(tables, table{name: name, sheet: sheetName, rng: rng})
	}

	return tables, nil
}

func decodeXMLStrict(raw []byte, dst any) error {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.Strict = true
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing XML content")
		}
		return err
	}
	return nil
}

func worksheetRelationshipsPath(worksheetPath string) string {
	dir := path.Dir(worksheetPath)
	base := path.Base(worksheetPath)
	return path.Join(dir, "_rels", base+".rels")
}

func resolveZipTarget(basePath, target string) string {
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/"))
	}
	return path.Clean(path.Join(path.Dir(basePath), target))
}

func rangeContains(outer, inner gridRange) bool {
	return outer.startRow <= inner.startRow &&
		outer.endRow >= inner.endRow &&
		outer.startCol <= inner.startCol &&
		outer.endCol >= inner.endCol
}
