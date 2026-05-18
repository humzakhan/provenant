package memio

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sync"

	kernel "github.com/provenant/provenant/kernel/public"
)

const substrate = "provenant.substrate/kernel.public.memio/v1"

var (
	ErrUnknownSourcePath       = errors.New("memio: unknown source path")
	ErrUnexpectedTx            = errors.New("memio: unexpected transaction type")
	ErrDuplicateTokenID        = errors.New("memio: duplicate token id")
	ErrDuplicateAuditSeq       = errors.New("memio: duplicate audit seq")
	ErrInvalidAuditChain       = errors.New("memio: invalid audit chain")
	ErrTenantRunMismatch       = errors.New("memio: tenant/run mismatch")
	ErrConcurrentWriteConflict = errors.New("memio: concurrent write conflict")
)

// Binding is the Phase 1 in-memory substrate for sources, tokens, and audit entries.
type Binding struct {
	mu       sync.Mutex
	sources  map[string]kernel.SourceBytes
	tokens   map[string][]kernel.Token
	audits   map[string][]kernel.AuditEntry
	revision uint64
}

// NewBinding builds an in-memory binding seeded with the provided sources.
func NewBinding(sources ...kernel.SourceBytes) *Binding {
	b := &Binding{
		sources: make(map[string]kernel.SourceBytes, len(sources)),
		tokens:  make(map[string][]kernel.Token),
		audits:  make(map[string][]kernel.AuditEntry),
	}
	for _, source := range sources {
		b.sources[source.Source.Path] = cloneSource(source)
	}
	return b
}

func (b *Binding) TransactionalSubstrate() string {
	return substrate
}

func (b *Binding) WithTx(ctx context.Context, fn func(ctx context.Context, tx kernel.Tx) error) error {
	b.mu.Lock()
	tx := &transaction{
		baseRevision: b.revision,
		tokens:       cloneTokenMap(b.tokens),
		audits:       cloneAuditMap(b.audits),
	}
	b.mu.Unlock()

	if err := fn(ctx, tx); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if !tx.dirty {
		return nil
	}
	if b.revision != tx.baseRevision {
		return ErrConcurrentWriteConflict
	}
	b.tokens = tx.tokens
	b.audits = tx.audits
	b.revision++
	return nil
}

func (b *Binding) ResolveSource(_ context.Context, path string) (kernel.SourceBytes, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	source, ok := b.sources[path]
	if !ok {
		return kernel.SourceBytes{}, fmt.Errorf("%w: %s", ErrUnknownSourcePath, path)
	}
	return cloneSource(source), nil
}

func (b *Binding) PutToken(_ context.Context, tx kernel.Tx, token kernel.Token) error {
	memTx, ok := tx.(*transaction)
	if !ok {
		return ErrUnexpectedTx
	}

	runTokens := memTx.tokens[token.RunID]
	for _, existing := range runTokens {
		if existing.ID == token.ID {
			return fmt.Errorf("%w: %s", ErrDuplicateTokenID, token.ID)
		}
	}

	memTx.tokens[token.RunID] = append(runTokens, cloneToken(token))
	memTx.dirty = true
	return nil
}

func (b *Binding) Tokens(_ context.Context, tx kernel.Tx, runID string) ([]kernel.Token, error) {
	memTx, ok := tx.(*transaction)
	if !ok {
		return nil, ErrUnexpectedTx
	}
	return cloneTokens(memTx.tokens[runID]), nil
}

func (b *Binding) AppendAuditEntry(_ context.Context, tx kernel.Tx, entry kernel.AuditEntry) (kernel.AuditEntry, error) {
	memTx, ok := tx.(*transaction)
	if !ok {
		return kernel.AuditEntry{}, ErrUnexpectedTx
	}

	key := auditKey(entry.TenantID, entry.RunID)
	runEntries := memTx.audits[key]
	if len(runEntries) > 0 {
		last := runEntries[len(runEntries)-1]
		if entry.Seq != last.Seq+1 {
			return kernel.AuditEntry{}, fmt.Errorf("%w: seq=%d want=%d", ErrDuplicateAuditSeq, entry.Seq, last.Seq+1)
		}
		if entry.TenantID != last.TenantID || entry.RunID != last.RunID {
			return kernel.AuditEntry{}, ErrTenantRunMismatch
		}
		if !slices.Equal(entry.PrevHash, last.Hash) {
			return kernel.AuditEntry{}, ErrInvalidAuditChain
		}
	} else if entry.Seq != 1 {
		return kernel.AuditEntry{}, fmt.Errorf("%w: first seq=%d", ErrDuplicateAuditSeq, entry.Seq)
	} else if !slices.Equal(entry.PrevHash, genesisHash(entry.TenantID)) {
		return kernel.AuditEntry{}, ErrInvalidAuditChain
	}

	cloned := cloneAuditEntry(entry)
	memTx.audits[key] = append(runEntries, cloned)
	memTx.dirty = true
	return cloned, nil
}

func (b *Binding) AuditEntries(_ context.Context, tx kernel.Tx, tenantID string, runID string) ([]kernel.AuditEntry, error) {
	memTx, ok := tx.(*transaction)
	if !ok {
		return nil, ErrUnexpectedTx
	}
	return cloneAuditEntries(memTx.audits[auditKey(tenantID, runID)]), nil
}

type transaction struct {
	baseRevision uint64
	dirty        bool
	tokens       map[string][]kernel.Token
	audits       map[string][]kernel.AuditEntry
}

func auditKey(tenantID, runID string) string {
	return tenantID + "\n" + runID
}

func cloneSource(source kernel.SourceBytes) kernel.SourceBytes {
	return kernel.SourceBytes{
		Source: source.Source,
		Bytes:  append([]byte(nil), source.Bytes...),
	}
}

func cloneTokenMap(in map[string][]kernel.Token) map[string][]kernel.Token {
	out := make(map[string][]kernel.Token, len(in))
	for runID, tokens := range in {
		out[runID] = cloneTokens(tokens)
	}
	return out
}

func cloneTokens(tokens []kernel.Token) []kernel.Token {
	if tokens == nil {
		return nil
	}

	out := make([]kernel.Token, len(tokens))
	for i, token := range tokens {
		out[i] = cloneToken(token)
	}
	return out
}

func cloneToken(token kernel.Token) kernel.Token {
	cloned := token
	if token.Derivation != nil {
		derivation := *token.Derivation
		derivation.Inputs = append([]string(nil), token.Derivation.Inputs...)
		cloned.Derivation = &derivation
	}
	return cloned
}

func cloneAuditMap(in map[string][]kernel.AuditEntry) map[string][]kernel.AuditEntry {
	out := make(map[string][]kernel.AuditEntry, len(in))
	for key, entries := range in {
		out[key] = cloneAuditEntries(entries)
	}
	return out
}

func cloneAuditEntries(entries []kernel.AuditEntry) []kernel.AuditEntry {
	if entries == nil {
		return nil
	}

	out := make([]kernel.AuditEntry, len(entries))
	for i, entry := range entries {
		out[i] = cloneAuditEntry(entry)
	}
	return out
}

func cloneAuditEntry(entry kernel.AuditEntry) kernel.AuditEntry {
	cloned := entry
	cloned.PrevHash = append([]byte(nil), entry.PrevHash...)
	cloned.Hash = append([]byte(nil), entry.Hash...)
	cloned.Payload.Deterministic = append([]byte(nil), entry.Payload.Deterministic...)
	cloned.Payload.NonDeterministic = append([]byte(nil), entry.Payload.NonDeterministic...)
	cloned.Payload.HashInputCanonical = append([]byte(nil), entry.Payload.HashInputCanonical...)
	return cloned
}

func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}
