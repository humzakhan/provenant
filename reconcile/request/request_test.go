package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestDecodeRequestValidModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  Request
	}{
		{
			name: "exact_match",
			req:  validRequest(ModeExactMatch),
		},
		{
			name: "amount_tolerance",
			req:  validRequest(ModeAmountTolerance),
		},
		{
			name: "existence",
			req:  validRequest(ModeExistence),
		},
		{
			name: "one_to_many",
			req:  validRequest(ModeOneToMany),
		},
		{
			name: "schema_validation",
			req:  validRequest(ModeSchemaValidation),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DecodeRequest(bytes.NewReader(mustJSON(t, tt.req)))
			if err != nil {
				t.Fatalf("DecodeRequest() error = %v", err)
			}
			if got.Reconcile.Mode != tt.req.Reconcile.Mode {
				t.Fatalf("Mode = %q, want %q", got.Reconcile.Mode, tt.req.Reconcile.Mode)
			}
		})
	}
}

func TestDecodeRequestFailures(t *testing.T) {
	t.Parallel()

	base := validRequest(ModeExactMatch)
	amountTolerance := validRequest(ModeAmountTolerance)
	oneToMany := validRequest(ModeOneToMany)

	tests := []struct {
		name        string
		payload     []byte
		wantDecode  ErrorCode
		wantField   string
		wantValCode ErrorCode
	}{
		{
			name:       "trailing json",
			payload:    append(mustJSON(t, base), []byte("\n{}")...),
			wantDecode: CodeTrailingData,
		},
		{
			name:       "oversized request",
			payload:    append(mustJSON(t, base), bytes.Repeat([]byte(" "), maxRequestBytes)...),
			wantDecode: CodeRequestTooLarge,
		},
		{
			name: "oversized request mid token",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Run.ID = string(bytes.Repeat([]byte("a"), maxRequestBytes))
			}),
			wantDecode: CodeRequestTooLarge,
		},
		{
			name:       "unknown top level field",
			payload:    []byte(`{"run":{"id":"run_1","tenant_id":"tenant_1"},"files":[],"reconcile":{},"extra":true}`),
			wantDecode: CodeUnknownField,
		},
		{
			name:       "unknown nested field",
			payload:    []byte(`{"run":{"id":"run_1","tenant_id":"tenant_1"},"files":[{"id":"left","purpose":"bank","format":"csv","path":"left.csv","reader":"extract.csv_rows","selector":{"row_window":"2:","unexpected":true},"columns":{"transaction_id":{"source":"Transaction ID","role":"key"},"amount":{"source":"Amount","role":"amount","numeric":{"scale":2,"currency_field":"currency"}},"currency":{"source":"Currency","role":"currency"}}},{"id":"right","purpose":"ledger","format":"csv","path":"right.csv","reader":"extract.csv_rows","selector":{"row_window":"2:"},"columns":{"transaction_id":{"source":"Transaction ID","role":"key"},"amount":{"source":"Amount","role":"amount","numeric":{"scale":2,"currency_field":"currency"}},"currency":{"source":"Currency","role":"currency"}}}],"reconcile":{"mode":"exact_match","left":"left","right":"right","match_on":["transaction_id"]}}`),
			wantDecode: CodeUnknownField,
		},
		{
			name:        "duplicate file id",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[1].ID = req.Files[0].ID }),
			wantField:   "files[1].id",
			wantValCode: CodeDuplicateID,
		},
		{
			name:       "duplicate logical field declaration",
			payload:    []byte(`{"run":{"id":"run_1","tenant_id":"tenant_1"},"files":[{"id":"left","purpose":"bank","format":"csv","path":"left.csv","reader":"extract.csv_rows","selector":{"row_window":"2:"},"columns":{"transaction_id":{"source":"Transaction ID","role":"key"},"transaction_id":{"source":"Transaction ID","role":"key"},"amount":{"source":"Amount","role":"amount","numeric":{"scale":2,"currency_field":"currency"}},"currency":{"source":"Currency","role":"currency"}}},{"id":"right","purpose":"ledger","format":"csv","path":"right.csv","reader":"extract.csv_rows","selector":{"row_window":"2:"},"columns":{"transaction_id":{"source":"Transaction ID","role":"key"},"amount":{"source":"Amount","role":"amount","numeric":{"scale":2,"currency_field":"currency"}},"currency":{"source":"Currency","role":"currency"}}}],"reconcile":{"mode":"exact_match","left":"left","right":"right","match_on":["transaction_id"]}}`),
			wantDecode: CodeInvalidJSON,
		},
		{
			name:        "missing run",
			payload:     []byte(`{"files":[],"reconcile":{}}`),
			wantField:   "run.id",
			wantValCode: CodeMissingRequired,
		},
		{
			name:        "fewer than two files",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files = req.Files[:1] }),
			wantField:   "files",
			wantValCode: CodeMissingRequired,
		},
		{
			name:        "missing file path",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[0].Path = "" }),
			wantField:   "files[0].path",
			wantValCode: CodeMissingRequired,
		},
		{
			name:        "unsupported format",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[0].Format = "pdf" }),
			wantField:   "files[0].format",
			wantValCode: CodeUnsupportedEnum,
		},
		{
			name:        "unsupported reader",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[0].Reader = "extract.xml" }),
			wantField:   "files[0].reader",
			wantValCode: CodeUnsupportedEnum,
		},
		{
			name:        "mismatched format reader",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[0].Reader = ReaderExtractJSON }),
			wantField:   "files[0].reader",
			wantValCode: CodeIncompatibleReader,
		},
		{
			name:        "missing selector for csv",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[0].Selector = SelectorConfig{} }),
			wantField:   "files[0].selector.row_window",
			wantValCode: CodeMissingRequired,
		},
		{
			name: "missing selector for xlsx",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Format = FormatXLSX
				req.Files[0].Reader = ReaderExtractSpreadsheet
				req.Files[0].Selector = SelectorConfig{}
			}),
			wantField:   "files[0].selector",
			wantValCode: CodeMissingRequired,
		},
		{
			name:        "unsupported column role",
			payload:     mutateJSON(t, base, func(req *Request) { req.Files[0].Columns["amount"] = ColumnConfig{Source: "Amount", Role: "mystery"} }),
			wantField:   "files[0].columns.amount.role",
			wantValCode: CodeUnsupportedEnum,
		},
		{
			name: "duplicate singleton amount role",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["amount_2"] = ColumnConfig{Source: "Net Amount", Role: RoleAmount, Numeric: &NumericConfig{Scale: 2, CurrencyField: "currency"}}
			}),
			wantField:   "files[0].columns.amount_2.role",
			wantValCode: CodeAmbiguousRole,
		},
		{
			name: "numeric on non amount",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["transaction_id"] = ColumnConfig{Source: "Transaction ID", Role: RoleKey, Numeric: &NumericConfig{Scale: 0, CurrencyField: "currency"}}
			}),
			wantField:   "files[0].columns.transaction_id.numeric",
			wantValCode: CodeInvalidModeConfig,
		},
		{
			name: "date format on non date",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["currency"] = ColumnConfig{Source: "Currency", Role: RoleCurrency, Format: "YYYY-MM-DD"}
			}),
			wantField:   "files[0].columns.currency.format",
			wantValCode: CodeInvalidModeConfig,
		},
		{
			name: "amount missing numeric config",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["amount"] = ColumnConfig{Source: "Amount", Role: RoleAmount}
			}),
			wantField:   "files[0].columns.amount.numeric",
			wantValCode: CodeMissingRequired,
		},
		{
			name: "amount missing currency field",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["amount"] = ColumnConfig{Source: "Amount", Role: RoleAmount, Numeric: &NumericConfig{Scale: 2}}
			}),
			wantField:   "files[0].columns.amount.numeric.currency_field",
			wantValCode: CodeMissingRequired,
		},
		{
			name: "amount with non currency reference",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["amount"] = ColumnConfig{Source: "Amount", Role: RoleAmount, Numeric: &NumericConfig{Scale: 2, CurrencyField: "transaction_id"}}
			}),
			wantField:   "files[0].columns.amount.numeric.currency_field",
			wantValCode: CodeInvalidReference,
		},
		{
			name:        "unsupported reconciliation mode",
			payload:     mutateJSON(t, base, func(req *Request) { req.Reconcile.Mode = "fuzzy" }),
			wantField:   "reconcile.mode",
			wantValCode: CodeUnsupportedEnum,
		},
		{
			name:        "missing left file reference",
			payload:     mutateJSON(t, base, func(req *Request) { req.Reconcile.Left = "missing" }),
			wantField:   "reconcile.left",
			wantValCode: CodeInvalidReference,
		},
		{
			name:        "match_on references missing field",
			payload:     mutateJSON(t, base, func(req *Request) { req.Reconcile.MatchOn = []string{"missing_key"} }),
			wantField:   "reconcile.match_on[0]",
			wantValCode: CodeInvalidReference,
		},
		{
			name:        "compare references missing field",
			payload:     mutateJSON(t, amountTolerance, func(req *Request) { req.Reconcile.Compare = []string{"missing_amount"} }),
			wantField:   "reconcile.compare[0]",
			wantValCode: CodeInvalidReference,
		},
		{
			name:        "grouped mode missing group keys",
			payload:     mutateJSON(t, oneToMany, func(req *Request) { req.Reconcile.Group = &GroupConfig{} }),
			wantField:   "reconcile.group.left_key",
			wantValCode: CodeMissingRequired,
		},
		{
			name:        "grouped mode missing compare fields",
			payload:     mutateJSON(t, oneToMany, func(req *Request) { req.Reconcile.Compare = nil }),
			wantField:   "reconcile.compare",
			wantValCode: CodeMissingRequired,
		},
		{
			name:        "tolerance on unsupported mode",
			payload:     mutateJSON(t, base, func(req *Request) { req.Reconcile.Tolerance = &ToleranceConfig{Amount: "0.01"} }),
			wantField:   "reconcile.tolerance",
			wantValCode: CodeInvalidModeConfig,
		},
		{
			name:        "invalid tolerance",
			payload:     mutateJSON(t, amountTolerance, func(req *Request) { req.Reconcile.Tolerance = &ToleranceConfig{Amount: "-0.01"} }),
			wantField:   "reconcile.tolerance.amount",
			wantValCode: CodeInvalidModeConfig,
		},
		{
			name: "required key missing from match_on",
			payload: mutateJSON(t, base, func(req *Request) {
				req.Files[0].Columns["reference_id"] = ColumnConfig{Source: "Reference ID", Role: RoleKey, Required: true}
				req.Files[1].Columns["reference_id"] = ColumnConfig{Source: "Reference ID", Role: RoleKey, Required: true}
			}),
			wantField:   "reconcile.match_on",
			wantValCode: CodeInvalidModeConfig,
		},
		{
			name:        "amount compare missing currency compare",
			payload:     mutateJSON(t, amountTolerance, func(req *Request) { req.Reconcile.Compare = []string{"amount"} }),
			wantField:   "reconcile.compare",
			wantValCode: CodeInvalidModeConfig,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeRequest(bytes.NewReader(tt.payload))
			if tt.wantDecode != "" {
				var decErr *DecodeError
				if !errors.As(err, &decErr) {
					t.Fatalf("DecodeRequest() error = %v, want decode error", err)
				}
				if decErr.Code != tt.wantDecode {
					t.Fatalf("DecodeError.Code = %q, want %q", decErr.Code, tt.wantDecode)
				}
				return
			}

			var valErr *ValidationErrors
			if !errors.As(err, &valErr) {
				t.Fatalf("DecodeRequest() error = %v, want validation errors", err)
			}
			assertValidationError(t, valErr, tt.wantField, tt.wantValCode)
		})
	}
}

func validRequest(mode string) Request {
	left := FileConfig{
		ID:      "left",
		Purpose: "bank_statement",
		Format:  FormatCSV,
		Path:    "left.csv",
		Reader:  ReaderExtractCSVRows,
		Selector: SelectorConfig{
			RowWindow: "2:",
			Encoding:  "utf-8",
			Dialect:   "excel",
		},
		Columns: Columns{
			"transaction_id": {Source: "Transaction ID", Role: RoleKey, Required: true, Unique: true},
			"amount":         {Source: "Amount", Role: RoleAmount, Required: true, Numeric: &NumericConfig{Scale: 2, CurrencyField: "currency"}},
			"currency":       {Source: "Currency", Role: RoleCurrency, Required: true},
			"posted_date":    {Source: "Posted Date", Role: RoleDate, Format: "YYYY-MM-DD"},
		},
	}
	right := FileConfig{
		ID:      "right",
		Purpose: "ledger",
		Format:  FormatCSV,
		Path:    "right.csv",
		Reader:  ReaderExtractCSVRows,
		Selector: SelectorConfig{
			RowWindow: "2:",
			Encoding:  "utf-8",
			Dialect:   "excel",
		},
		Columns: Columns{
			"transaction_id": {Source: "Entry ID", Role: RoleKey, Required: true, Unique: true},
			"amount":         {Source: "Net Amount", Role: RoleAmount, Required: true, Numeric: &NumericConfig{Scale: 2, CurrencyField: "currency"}},
			"currency":       {Source: "Currency", Role: RoleCurrency, Required: true},
			"posted_date":    {Source: "Posting Date", Role: RoleDate, Format: "YYYY-MM-DD"},
		},
	}

	reconcile := ReconcileConfig{
		Mode:    mode,
		Left:    "left",
		Right:   "right",
		MatchOn: []string{"transaction_id"},
	}

	switch mode {
	case ModeExactMatch:
		reconcile.Compare = []string{"amount", "currency"}
	case ModeAmountTolerance:
		reconcile.Compare = []string{"amount", "currency"}
		reconcile.Tolerance = &ToleranceConfig{Amount: "0.01"}
	case ModeExistence:
	case ModeOneToMany:
		reconcile.Compare = []string{"amount", "currency"}
		reconcile.Group = &GroupConfig{LeftKey: []string{"transaction_id"}, RightKey: []string{"transaction_id"}}
	case ModeSchemaValidation:
		reconcile.MatchOn = nil
	}

	return Request{
		Run: RunConfig{ID: "run_1", TenantID: "tenant_1"},
		Files: []FileConfig{
			left,
			right,
		},
		Reconcile: reconcile,
	}
}

func assertValidationError(t *testing.T, err *ValidationErrors, path string, code ErrorCode) {
	t.Helper()
	for _, item := range err.Errors {
		if item.Path == path && item.Code == code {
			return
		}
	}
	t.Fatalf("validation errors %v do not contain %s/%s", err.Errors, path, code)
}

func mutateJSON(t *testing.T, base Request, mutate func(*Request)) []byte {
	t.Helper()
	clone := cloneRequest(t, base)
	mutate(&clone)
	return mustJSON(t, clone)
}

func cloneRequest(t *testing.T, req Request) Request {
	t.Helper()
	var clone Request
	if err := json.Unmarshal(mustJSON(t, req), &clone); err != nil {
		t.Fatalf("Unmarshal(clone) error = %v", err)
	}
	return clone
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func TestValidationErrorsSorted(t *testing.T) {
	t.Parallel()

	req := validRequest(ModeAmountTolerance)
	req.Run.ID = ""
	req.Run.TenantID = ""
	req.Reconcile.Compare = []string{"missing"}

	_, err := DecodeRequest(bytes.NewReader(mustJSON(t, req)))
	var valErr *ValidationErrors
	if !errors.As(err, &valErr) {
		t.Fatalf("DecodeRequest() error = %v, want validation errors", err)
	}
	if len(valErr.Errors) < 2 {
		t.Fatalf("len(Errors) = %d, want at least 2", len(valErr.Errors))
	}
	for i := 1; i < len(valErr.Errors); i++ {
		prev := valErr.Errors[i-1]
		curr := valErr.Errors[i]
		if lessValidationError(curr, prev) {
			t.Fatalf("errors not sorted: %#v before %#v", prev, curr)
		}
	}
}
