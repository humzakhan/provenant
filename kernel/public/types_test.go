package kernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestContractSpineCompiles(t *testing.T) {
	run := Run{ID: "run_1", TenantID: "tenant_1", Status: RunRunning}
	source := SourceBytes{
		Source: Source{
			Path:      "fixture://invoice.json",
			MediaType: "application/json",
			SHA256:    "abc123",
		},
		Bytes: []byte(`{"total":"12.34"}`),
	}
	selector := SourceSelector{JSONPointer: "/total"}
	read := ReadResult{
		Output: []byte(`{"value":"12.34"}`),
		CapturedRead: CapturedRead{
			ID:            "read_1",
			RunID:         run.ID,
			SourcePath:    source.Source.Path,
			Origin:        "fixture://invoice.json#ptr=/total",
			Selector:      selector,
			ReaderName:    "extract.json",
			ReaderVersion: "v1",
			ReaderOptions: []byte(`{"selector_mode":"json_pointer"}`),
			Locator:       []byte(`{"json_pointer":"/total","byte_start":9,"byte_end":16}`),
			SourceSHA256:  source.Source.SHA256,
			OutputSHA256:  "def456",
			Output:        []byte(`{"value":"12.34"}`),
		},
	}
	token := Token{
		ID:             "tok_1",
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: "12.34",
		Source: TokenSource{
			CapturedReadID: read.CapturedRead.ID,
			SourcePath:     source.Source.Path,
			SourceSelector: selector,
			OutputPath:     "$.value",
		},
	}
	entry := AuditEntry{
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     EntryKindTokenMint,
		Payload: PayloadEnvelope{
			Deterministic: []byte(`{"token_id":"tok_1"}`),
		},
	}

	var _ Store = storeFunc{}
	var _ CapturedReadStore = storeFunc{}
	var _ AuditSink = auditSinkFunc{}
	var _ TransactionalScope = transactionalScopeFunc{}
	var _ SourceResolver = sourceResolverFunc{}
	var _ SourceReader = sourceReaderFunc{}
	var _ Clock = clockFunc{}

	if token.Source.CapturedReadID != read.CapturedRead.ID {
		t.Fatalf("token source does not reference captured read")
	}
	if entry.Kind != EntryKindTokenMint {
		t.Fatalf("unexpected entry kind: %s", entry.Kind)
	}
}

func TestSourceSelectorLegacyJSONShapeStaysStable(t *testing.T) {
	selector := SourceSelector{JSONPointer: "/total"}

	raw, err := json.Marshal(selector)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(raw) != `{"json_pointer":"/total"}` {
		t.Fatalf("Marshal() = %s", raw)
	}
}

func TestSourceSelectorPhaseTwoFieldsRoundTripAndRemainComparable(t *testing.T) {
	selector := SourceSelector{
		JSONPointer:      "/records/0/amount",
		JSONPath:         "$.records[0].amount",
		ByteSpan:         "0-24",
		LineWindow:       "1-100",
		Cursor:           "line:100",
		RowWindow:        "2-51",
		ColumnProjection: "amount,total",
		Sheet:            "Sheet1",
		Range:            "A1:D50",
		TableName:        "Amounts",
		HeaderRow:        1,
		Dialect:          "excel",
		Encoding:         "utf-8",
		FormulaPolicy:    "cached_only",
	}

	index := map[SourceSelector]string{
		selector: "ok",
	}
	if index[selector] != "ok" {
		t.Fatal("SourceSelector is not usable as a comparable map key")
	}

	raw, err := json.Marshal(selector)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded SourceSelector
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded != selector {
		t.Fatalf("round trip selector = %#v, want %#v", decoded, selector)
	}
}

func TestSourceSelectorValidatePhaseTwoSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		selector SourceSelector
		wantErr  string
	}{
		{
			name:     "json selector",
			selector: SourceSelector{JSONPointer: "/invoice/total", JSONPath: "$.invoice.total", ByteSpan: "10-24"},
		},
		{
			name:     "jsonl selector",
			selector: SourceSelector{LineWindow: "1-100", Cursor: "line:100", JSONPath: "$.invoice.total", ByteSpan: "10-24"},
		},
		{
			name:     "csv selector",
			selector: SourceSelector{RowWindow: "2-51", Cursor: "row:51", ColumnProjection: "amount,total", HeaderRow: 1, Dialect: "excel", Encoding: "utf-8"},
		},
		{
			name:     "spreadsheet selector",
			selector: SourceSelector{Sheet: "TrialBalance", Range: "A1:D50", TableName: "Amounts", RowWindow: "2-51", ColumnProjection: "A,C", FormulaPolicy: "cached_only"},
		},
		{
			name:     "missing selector",
			selector: SourceSelector{},
			wantErr:  "no supported selector family",
		},
		{
			name:     "json selector without pointer",
			selector: SourceSelector{JSONPath: "$.invoice.total"},
			wantErr:  "json selector requires json_pointer",
		},
		{
			name:     "cursor without window",
			selector: SourceSelector{Cursor: "line:100"},
			wantErr:  "cursor requires line_window or row_window",
		},
		{
			name:     "ambiguous jsonl and csv",
			selector: SourceSelector{LineWindow: "1-10", RowWindow: "2-5"},
			wantErr:  "ambiguous selector families",
		},
		{
			name:     "csv projection without row window",
			selector: SourceSelector{ColumnProjection: "amount"},
			wantErr:  "csv selector requires row_window",
		},
		{
			name:     "spreadsheet metadata without primary selector",
			selector: SourceSelector{FormulaPolicy: "cached_only"},
			wantErr:  "spreadsheet selector requires sheet, range, or table_name",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.selector.ValidatePhaseTwoSelector()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidatePhaseTwoSelector() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ValidatePhaseTwoSelector() error = nil")
			}
			if err != nil && err.Error() == "" {
				t.Fatal("ValidatePhaseTwoSelector() error message is empty")
			}
			if got := err.Error(); got == "" || !strings.Contains(got, tt.wantErr) {
				t.Fatalf("ValidatePhaseTwoSelector() error = %q, want substring %q", got, tt.wantErr)
			}
		})
	}
}

type storeFunc struct{}

func (storeFunc) TransactionalSubstrate() string            { return "provenant.substrate/test/memory/v1" }
func (storeFunc) PutToken(context.Context, Tx, Token) error { return nil }
func (storeFunc) PutCapturedRead(context.Context, Tx, CapturedRead) error {
	return nil
}
func (storeFunc) CapturedReads(context.Context, Tx, string) ([]CapturedRead, error) {
	return nil, nil
}
func (storeFunc) Tokens(context.Context, Tx, string) ([]Token, error) { return nil, nil }

type auditSinkFunc struct{}

func (auditSinkFunc) TransactionalSubstrate() string { return "provenant.substrate/test/memory/v1" }
func (auditSinkFunc) AppendAuditEntry(context.Context, Tx, AuditEntry) (AuditEntry, error) {
	return AuditEntry{}, nil
}
func (auditSinkFunc) AuditEntries(context.Context, Tx, string, string) ([]AuditEntry, error) {
	return nil, nil
}

type transactionalScopeFunc struct{}

func (transactionalScopeFunc) WithTx(ctx context.Context, fn func(context.Context, Tx) error) error {
	return fn(ctx, nil)
}
func (transactionalScopeFunc) TransactionalSubstrate() string {
	return "provenant.substrate/test/memory/v1"
}

type sourceResolverFunc struct{}

func (sourceResolverFunc) ResolveSource(context.Context, string) (SourceBytes, error) {
	return SourceBytes{}, nil
}

type sourceReaderFunc struct{}

func (sourceReaderFunc) Read(context.Context, Run, SourceBytes, SourceSelector) (ReadResult, error) {
	return ReadResult{}, nil
}

type clockFunc struct{}

func (clockFunc) Now() time.Time {
	return time.Time{}
}
