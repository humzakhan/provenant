package sources_test

import (
	"strings"
	"testing"

	"github.com/humzakhan/provenant/access/sources"
	kernel "github.com/humzakhan/provenant/kernel/public"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func loadOK(t *testing.T, doc string) *sources.Registry {
	t.Helper()
	reg, err := sources.Load(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return reg
}

func loadErr(t *testing.T, doc string) error {
	t.Helper()
	_, err := sources.Load(strings.NewReader(doc))
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	return err
}

// ---------------------------------------------------------------------------
// valid registry
// ---------------------------------------------------------------------------

const validRegistry = `{
  "schema_version": "provenant.sources/v1",
  "sources": {
    "ledger.get_balance": {
      "reader": "extract.csv_rows",
      "path": "./ledger.csv",
      "selector": {"row_window": "2:", "encoding": "utf-8", "dialect": "excel"},
      "output_path": "$.amount",
      "unit": "USD"
    },
    "payroll.export": {
      "reader": "extract.spreadsheet",
      "path": "./payroll.xlsx",
      "selector": {"sheet": "Q1", "range": "A1:F200"},
      "output_path": "$.gross_pay"
    }
  }
}`

func TestLoadValid(t *testing.T) {
	t.Parallel()
	reg := loadOK(t, validRegistry)

	t.Run("resolve_ledger", func(t *testing.T) {
		t.Parallel()
		e, ok := reg.Resolve("ledger.get_balance")
		if !ok {
			t.Fatal("Resolve(ledger.get_balance) = false, want true")
		}
		if e.Reader != "extract.csv_rows" {
			t.Errorf("Reader = %q, want extract.csv_rows", e.Reader)
		}
		if e.Path != "./ledger.csv" {
			t.Errorf("Path = %q, want ./ledger.csv", e.Path)
		}
		if e.Selector != (kernel.SourceSelector{RowWindow: "2:", Encoding: "utf-8", Dialect: "excel"}) {
			t.Errorf("Selector = %+v, unexpected", e.Selector)
		}
		if e.OutputPath != "$.amount" {
			t.Errorf("OutputPath = %q, want $.amount", e.OutputPath)
		}
		if e.Unit != "USD" {
			t.Errorf("Unit = %q, want USD", e.Unit)
		}
	})

	t.Run("resolve_payroll_no_unit", func(t *testing.T) {
		t.Parallel()
		e, ok := reg.Resolve("payroll.export")
		if !ok {
			t.Fatal("Resolve(payroll.export) = false, want true")
		}
		if e.Unit != "" {
			t.Errorf("Unit = %q, want empty", e.Unit)
		}
	})
}

// ---------------------------------------------------------------------------
// resolve unknown
// ---------------------------------------------------------------------------

func TestResolveUnknown(t *testing.T) {
	t.Parallel()
	reg := loadOK(t, validRegistry)
	_, ok := reg.Resolve("does.not.exist")
	if ok {
		t.Fatal("Resolve(does.not.exist) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// schema version rejection
// ---------------------------------------------------------------------------

func TestLoadWrongSchemaVersion(t *testing.T) {
	t.Parallel()
	doc := `{"schema_version":"provenant.sources/v2","sources":{}}`
	err := loadErr(t, doc)
	if !isInvalidRegistry(err) {
		t.Errorf("want ErrInvalidRegistry, got %v", err)
	}
}

func TestLoadMissingSchemaVersion(t *testing.T) {
	t.Parallel()
	doc := `{"sources":{}}`
	err := loadErr(t, doc)
	if !isInvalidRegistry(err) {
		t.Errorf("want ErrInvalidRegistry, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// malformed documents
// ---------------------------------------------------------------------------

func TestLoadMalformedJSON(t *testing.T) {
	t.Parallel()
	err := loadErr(t, `{not json}`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadTrailingContent(t *testing.T) {
	t.Parallel()
	doc := validRegistry + `{"extra":true}`
	err := loadErr(t, doc)
	if !isInvalidRegistry(err) {
		t.Errorf("want ErrInvalidRegistry, got %v", err)
	}
}

func TestLoadUnknownField(t *testing.T) {
	t.Parallel()
	doc := `{"schema_version":"provenant.sources/v1","sources":{},"extra_field":true}`
	err := loadErr(t, doc)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

// ---------------------------------------------------------------------------
// missing required fields in entries
// ---------------------------------------------------------------------------

var missingFieldCases = []struct {
	name string
	doc  string
}{
	{
		name: "missing_reader",
		doc:  `{"schema_version":"provenant.sources/v1","sources":{"s":{"path":"./f.csv","selector":{"row_window":"1:"},"output_path":"$.v"}}}`,
	},
	{
		name: "missing_path",
		doc:  `{"schema_version":"provenant.sources/v1","sources":{"s":{"reader":"extract.csv_rows","selector":{"row_window":"1:"},"output_path":"$.v"}}}`,
	},
	{
		name: "missing_output_path",
		doc:  `{"schema_version":"provenant.sources/v1","sources":{"s":{"reader":"extract.csv_rows","path":"./f.csv","selector":{"row_window":"1:"}}}}`,
	},
}

func TestLoadMissingRequiredFields(t *testing.T) {
	for _, tc := range missingFieldCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := loadErr(t, tc.doc)
			if !isInvalidRegistry(err) {
				t.Errorf("want ErrInvalidRegistry, got %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// empty sources map is valid
// ---------------------------------------------------------------------------

func TestLoadEmptySources(t *testing.T) {
	t.Parallel()
	reg := loadOK(t, `{"schema_version":"provenant.sources/v1","sources":{}}`)
	_, ok := reg.Resolve("anything")
	if ok {
		t.Fatal("expected resolve to return false on empty registry")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func isInvalidRegistry(err error) bool {
	return err != nil && strings.Contains(err.Error(), "sources: invalid registry")
}
