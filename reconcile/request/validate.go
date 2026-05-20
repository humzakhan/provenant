package request

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

type fileIndex struct {
	byID map[string]int
	info []fileInfo
}

type fileInfo struct {
	config       *FileConfig
	rolesByField map[string]string
	amountFields map[string]ColumnConfig
	requiredKeys map[string]struct{}
}

func Validate(req *Request) error {
	if req == nil {
		return &ValidationErrors{Errors: []ValidationError{{Path: "request", Code: CodeMissingRequired, Message: "request is required"}}}
	}

	var errs ValidationErrors
	validateRun(req.Run, &errs)
	index := validateFiles(req.Files, &errs)
	validateReconcile(req.Reconcile, index, &errs)
	return errs.errOrNil()
}

func validateRun(run RunConfig, errs *ValidationErrors) {
	if strings.TrimSpace(run.ID) == "" {
		errs.add("run.id", CodeMissingRequired, "run id is required")
	}
	if strings.TrimSpace(run.TenantID) == "" {
		errs.add("run.tenant_id", CodeMissingRequired, "run tenant_id is required")
	}
}

func validateFiles(files []FileConfig, errs *ValidationErrors) fileIndex {
	index := fileIndex{byID: make(map[string]int), info: make([]fileInfo, len(files))}
	if len(files) < 2 {
		errs.add("files", CodeMissingRequired, "at least two files are required")
	}

	for i := range files {
		file := &files[i]
		path := fmt.Sprintf("files[%d]", i)
		id := strings.TrimSpace(file.ID)
		if id == "" {
			errs.add(path+".id", CodeMissingRequired, "file id is required")
		} else if prev, exists := index.byID[id]; exists {
			errs.add(path+".id", CodeDuplicateID, fmt.Sprintf("duplicates files[%d].id", prev))
		} else {
			index.byID[id] = i
		}
		if strings.TrimSpace(file.Purpose) == "" {
			errs.add(path+".purpose", CodeMissingRequired, "file purpose is required")
		}
		if strings.TrimSpace(file.Format) == "" {
			errs.add(path+".format", CodeMissingRequired, "file format is required")
		} else if !isSupportedFormat(file.Format) {
			errs.add(path+".format", CodeUnsupportedEnum, fmt.Sprintf("unsupported format %q", file.Format))
		}
		if strings.TrimSpace(file.Path) == "" {
			errs.add(path+".path", CodeMissingRequired, "file path is required")
		}
		if strings.TrimSpace(file.Reader) == "" {
			errs.add(path+".reader", CodeMissingRequired, "file reader is required")
		} else if !isSupportedReader(file.Reader) {
			errs.add(path+".reader", CodeUnsupportedEnum, fmt.Sprintf("unsupported reader %q", file.Reader))
		} else if file.Format != "" && expectedReaderForFormat(file.Format) != file.Reader {
			errs.add(path+".reader", CodeIncompatibleReader, fmt.Sprintf("reader %q is not valid for format %q", file.Reader, file.Format))
		}

		validateSelector(path+".selector", file.Format, file.Selector, errs)
		info := validateColumns(path+".columns", file.Columns, errs)
		index.info[i] = fileInfo{
			config:       file,
			rolesByField: info.rolesByField,
			amountFields: info.amountFields,
			requiredKeys: info.requiredKeys,
		}
	}

	return index
}

type columnInfo struct {
	rolesByField map[string]string
	amountFields map[string]ColumnConfig
	requiredKeys map[string]struct{}
}

func validateSelector(path, format string, selector SelectorConfig, errs *ValidationErrors) {
	switch format {
	case FormatCSV:
		if strings.TrimSpace(selector.RowWindow) == "" {
			errs.add(path+".row_window", CodeMissingRequired, "csv selector row_window is required")
		}
		if selector.JSONPointer != "" || selector.Sheet != "" || selector.Range != "" || selector.Table != "" {
			errs.add(path, CodeInvalidModeConfig, "csv selector contains unsupported fields")
		}
	case FormatJSONL:
		if strings.TrimSpace(selector.RowWindow) == "" {
			errs.add(path+".row_window", CodeMissingRequired, "jsonl selector row_window is required")
		}
		if selector.JSONPointer != "" || selector.Sheet != "" || selector.Range != "" || selector.Table != "" || selector.Dialect != "" || selector.HeaderRow != 0 {
			errs.add(path, CodeInvalidModeConfig, "jsonl selector contains unsupported fields")
		}
	case FormatXLSX:
		if strings.TrimSpace(selector.Sheet) == "" && strings.TrimSpace(selector.Range) == "" && strings.TrimSpace(selector.Table) == "" {
			errs.add(path, CodeMissingRequired, "xlsx selector requires sheet, range, or table")
		}
		if selector.JSONPointer != "" || selector.RowWindow != "" || selector.Encoding != "" || selector.Dialect != "" {
			errs.add(path, CodeInvalidModeConfig, "xlsx selector contains unsupported fields")
		}
	case FormatJSON:
		if strings.TrimSpace(selector.JSONPointer) == "" {
			errs.add(path+".json_pointer", CodeMissingRequired, "json selector json_pointer is required")
		}
		if selector.RowWindow != "" || selector.Sheet != "" || selector.Range != "" || selector.Table != "" || selector.HeaderRow != 0 || selector.Encoding != "" || selector.Dialect != "" {
			errs.add(path, CodeInvalidModeConfig, "json selector contains unsupported fields")
		}
	}
}

func validateColumns(path string, columns Columns, errs *ValidationErrors) columnInfo {
	info := columnInfo{
		rolesByField: make(map[string]string, len(columns)),
		amountFields: make(map[string]ColumnConfig),
		requiredKeys: make(map[string]struct{}),
	}
	if len(columns) == 0 {
		errs.add(path, CodeMissingRequired, "at least one column declaration is required")
		return info
	}

	logicalNames := make([]string, 0, len(columns))
	for name := range columns {
		logicalNames = append(logicalNames, name)
	}
	sort.Strings(logicalNames)

	sourceToField := make(map[string]string, len(logicalNames))
	singletonRoleOwner := make(map[string]string, 3)
	for _, name := range logicalNames {
		column := columns[name]
		fieldPath := path + "." + name
		if strings.TrimSpace(name) == "" {
			errs.add(fieldPath, CodeMissingRequired, "logical field name is required")
		}
		if strings.TrimSpace(column.Source) == "" {
			errs.add(fieldPath+".source", CodeMissingRequired, "column source is required")
		}
		if strings.TrimSpace(column.Role) == "" {
			errs.add(fieldPath+".role", CodeMissingRequired, "column role is required")
		} else if !isSupportedRole(column.Role) {
			errs.add(fieldPath+".role", CodeUnsupportedEnum, fmt.Sprintf("unsupported role %q", column.Role))
		} else {
			info.rolesByField[name] = column.Role
			if previous, exists := sourceToField[column.Source]; exists {
				errs.add(fieldPath+".source", CodeDuplicateDeclaration, fmt.Sprintf("duplicates source mapping for %q", previous))
			} else {
				sourceToField[column.Source] = name
			}
			if isSingletonRole(column.Role) {
				if previous, exists := singletonRoleOwner[column.Role]; exists {
					errs.add(fieldPath+".role", CodeAmbiguousRole, fmt.Sprintf("role %q already declared by %q", column.Role, previous))
				} else {
					singletonRoleOwner[column.Role] = name
				}
			}
		}
		if column.Role == RoleIgnored && column.Required {
			errs.add(fieldPath+".required", CodeInvalidModeConfig, "ignored columns cannot be required")
		}
		if column.Role == RoleIgnored && column.Unique {
			errs.add(fieldPath+".unique", CodeInvalidModeConfig, "ignored columns cannot be unique")
		}
		if column.Role == RoleAmount && column.Numeric == nil {
			errs.add(fieldPath+".numeric", CodeMissingRequired, "amount columns require numeric config")
		}
		if column.Numeric != nil {
			if column.Role != RoleAmount {
				errs.add(fieldPath+".numeric", CodeInvalidModeConfig, "numeric config is only allowed for amount columns")
			} else {
				if column.Numeric.Scale < 0 {
					errs.add(fieldPath+".numeric.scale", CodeInvalidModeConfig, "numeric scale must be zero or greater")
				}
				if strings.TrimSpace(column.Numeric.CurrencyField) == "" {
					errs.add(fieldPath+".numeric.currency_field", CodeMissingRequired, "amount columns require numeric.currency_field")
				}
				info.amountFields[name] = column
			}
		}
		if column.Format != "" && column.Role != RoleDate {
			errs.add(fieldPath+".format", CodeInvalidModeConfig, "format is only allowed for date columns")
		}
		if column.Role == RoleKey && column.Required {
			info.requiredKeys[name] = struct{}{}
		}
	}

	for _, name := range logicalNames {
		column := columns[name]
		if column.Role != RoleAmount || column.Numeric == nil {
			continue
		}
		currencyField := column.Numeric.CurrencyField
		currency, ok := columns[currencyField]
		if !ok {
			errs.add(path+"."+name+".numeric.currency_field", CodeInvalidReference, fmt.Sprintf("currency field %q does not exist", currencyField))
			continue
		}
		if currency.Role != RoleCurrency {
			errs.add(path+"."+name+".numeric.currency_field", CodeInvalidReference, fmt.Sprintf("currency field %q must have role %q", currencyField, RoleCurrency))
		}
	}

	return info
}

func validateTolerance(path string, tolerance *ToleranceConfig, errs *ValidationErrors) {
	if tolerance == nil {
		return
	}
	if strings.TrimSpace(tolerance.Amount) == "" {
		errs.add(path+".amount", CodeMissingRequired, "tolerance amount is required")
		return
	}
	value, err := decimal.NewFromString(tolerance.Amount)
	if err != nil {
		errs.add(path+".amount", CodeInvalidModeConfig, "tolerance amount must be a decimal string")
		return
	}
	if value.IsNegative() {
		errs.add(path+".amount", CodeInvalidModeConfig, "tolerance amount must be zero or greater")
	}
}

func isSupportedFormat(format string) bool {
	switch format {
	case FormatCSV, FormatXLSX, FormatJSON, FormatJSONL:
		return true
	default:
		return false
	}
}

func isSupportedReader(reader string) bool {
	switch reader {
	case ReaderExtractCSVRows, ReaderExtractSpreadsheet, ReaderExtractJSON, ReaderExtractJSONL:
		return true
	default:
		return false
	}
}

func expectedReaderForFormat(format string) string {
	switch format {
	case FormatCSV:
		return ReaderExtractCSVRows
	case FormatXLSX:
		return ReaderExtractSpreadsheet
	case FormatJSON:
		return ReaderExtractJSON
	case FormatJSONL:
		return ReaderExtractJSONL
	default:
		return ""
	}
}

func isSupportedRole(role string) bool {
	switch role {
	case RoleKey, RoleAmount, RoleCurrency, RoleDate, RoleDescription, RoleEntity, RoleStatus, RoleReference, RoleIgnored:
		return true
	default:
		return false
	}
}

func isSingletonRole(role string) bool {
	switch role {
	case RoleAmount, RoleCurrency, RoleDate:
		return true
	default:
		return false
	}
}
