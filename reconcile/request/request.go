package request

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	FormatCSV   = "csv"
	FormatXLSX  = "xlsx"
	FormatJSON  = "json"
	FormatJSONL = "jsonl"
)

const (
	ReaderExtractCSVRows     = "extract.csv_rows"
	ReaderExtractSpreadsheet = "extract.spreadsheet"
	ReaderExtractJSON        = "extract.json"
	ReaderExtractJSONL       = "extract.jsonl"
)

const (
	RoleKey         = "key"
	RoleAmount      = "amount"
	RoleCurrency    = "currency"
	RoleDate        = "date"
	RoleDescription = "description"
	RoleEntity      = "entity"
	RoleStatus      = "status"
	RoleReference   = "reference"
	RoleIgnored     = "ignored"
)

const (
	ModeExactMatch       = "exact_match"
	ModeAmountTolerance  = "amount_tolerance"
	ModeExistence        = "existence"
	ModeOneToMany        = "one_to_many"
	ModeSchemaValidation = "schema_validation"
)

type Request struct {
	Run       RunConfig       `json:"run"`
	Files     []FileConfig    `json:"files"`
	Reconcile ReconcileConfig `json:"reconcile"`
}

type RunConfig struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
}

type FileConfig struct {
	ID       string         `json:"id"`
	Purpose  string         `json:"purpose"`
	Format   string         `json:"format"`
	Path     string         `json:"path"`
	Reader   string         `json:"reader"`
	Selector SelectorConfig `json:"selector"`
	Columns  Columns        `json:"columns"`
}

type SelectorConfig struct {
	JSONPointer string `json:"json_pointer,omitempty"`
	LineWindow  string `json:"line_window,omitempty"`
	RowWindow   string `json:"row_window,omitempty"`
	Sheet       string `json:"sheet,omitempty"`
	Range       string `json:"range,omitempty"`
	Table       string `json:"table,omitempty"`
	HeaderRow   int    `json:"header_row,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Dialect     string `json:"dialect,omitempty"`
}

type Columns map[string]ColumnConfig

type ColumnConfig struct {
	Source   string         `json:"source"`
	Role     string         `json:"role"`
	Required bool           `json:"required,omitempty"`
	Unique   bool           `json:"unique,omitempty"`
	Format   string         `json:"format,omitempty"`
	Numeric  *NumericConfig `json:"numeric,omitempty"`
}

type NumericConfig struct {
	Scale         int    `json:"scale"`
	CurrencyField string `json:"currency_field"`
}

type ReconcileConfig struct {
	Mode      string           `json:"mode"`
	Left      string           `json:"left"`
	Right     string           `json:"right"`
	MatchOn   []string         `json:"match_on,omitempty"`
	Compare   []string         `json:"compare,omitempty"`
	Tolerance *ToleranceConfig `json:"tolerance,omitempty"`
	Group     *GroupConfig     `json:"group,omitempty"`
}

type ToleranceConfig struct {
	Amount string `json:"amount"`
}

type GroupConfig struct {
	LeftKey  []string `json:"left_key"`
	RightKey []string `json:"right_key"`
}

func (c *Columns) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("columns must be a JSON object")
	}

	result := make(Columns)
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("columns key must be a string")
		}
		if _, exists := result[key]; exists {
			return fmt.Errorf("duplicate columns key %q", key)
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}

		var column ColumnConfig
		if err := decodeStrictJSON(raw, &column); err != nil {
			return fmt.Errorf("column %q: %w", key, err)
		}
		result[key] = column
	}

	if _, err := dec.Token(); err != nil {
		return err
	}
	if err := ensureDecoderEOF(dec); err != nil {
		return err
	}

	*c = result
	return nil
}
