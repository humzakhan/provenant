package kernel

import (
	"context"
	"time"
)

// Tx is an opaque transaction handle owned by an injected substrate.
type Tx interface{}

// TransactionalScope runs Store and AuditSink work in one substrate transaction.
type TransactionalScope interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
	TransactionalSubstrate() string
}

// SourceResolver resolves logical source paths to injected in-memory bytes.
type SourceResolver interface {
	ResolveSource(ctx context.Context, path string) (SourceBytes, error)
}

// SourceReader reads one deterministic slice from injected source bytes.
type SourceReader interface {
	Read(ctx context.Context, run Run, source SourceBytes, selector SourceSelector) (ReadResult, error)
}

// Store records kernel-owned token state for a run.
type Store interface {
	TransactionalSubstrate() string
	PutToken(ctx context.Context, tx Tx, token Token) error
	Tokens(ctx context.Context, tx Tx, runID string) ([]Token, error)
}

// CapturedReadStore records and reads captured source-read evidence for a run.
type CapturedReadStore interface {
	PutCapturedRead(ctx context.Context, tx Tx, capturedRead CapturedRead) error
	CapturedReads(ctx context.Context, tx Tx, runID string) ([]CapturedRead, error)
}

// AuditSink appends and reads the tenant-scoped audit chain for a run.
type AuditSink interface {
	TransactionalSubstrate() string
	AppendAuditEntry(ctx context.Context, tx Tx, entry AuditEntry) (AuditEntry, error)
	AuditEntries(ctx context.Context, tx Tx, tenantID string, runID string) ([]AuditEntry, error)
}

// Clock supplies non-deterministic wall-clock timestamps to kernel code.
type Clock interface {
	Now() time.Time
}
