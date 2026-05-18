package kernel

import (
	"context"
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
