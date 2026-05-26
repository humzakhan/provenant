package derive

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/internal/derivation"
	"github.com/humzakhan/provenant/kernel/public/internal/ops"
)

// Request is the input to a derivation. Op identifies the op catalog entry.
// Inputs is ordered; ordering is part of the deterministic identity.
// Rounding is honoured only by ops that round (all Phase 02 ops do).
// Predicate is honoured only by filter.sum_where; it is ignored by all other ops.
type Request struct {
	Run       kernel.Run
	Op        string
	Inputs    []kernel.Token
	Rounding  kernel.RoundingPolicy
	Predicate *kernel.RowPredicate
}

// Result is the derived token plus the audit entry the caller must append.
type Result struct {
	Token      kernel.Token
	AuditEntry kernel.AuditEntry
}

// Dependencies carries the injected dependencies for the derivation engine.
// It mirrors the existing kernel injection style used by ReadAndMint.
// CapturedReadStore is required only by filter.sum_where; it may be nil for
// all other ops.
type Dependencies struct {
	Clock             kernel.Clock
	Store             kernel.Store
	AuditSink         kernel.AuditSink
	Scope             kernel.TransactionalScope
	CapturedReadStore kernel.CapturedReadStore
}

// Derive evaluates Op over Inputs and returns the derived Token plus the
// audit entry the caller must append to the chain. It does no I/O, calls no
// clock other than deps.Clock, and uses no randomness.
//
// All errors are fail-closed: no token is added to the store and no audit
// entry is appended on error.
func Derive(ctx context.Context, deps Dependencies, req Request) (Result, error) {
	if deps.Scope == nil {
		return Result{}, errors.New("derive: transactional scope is required")
	}
	if deps.Store == nil {
		return Result{}, errors.New("derive: store is required")
	}
	if deps.AuditSink == nil {
		return Result{}, errors.New("derive: audit sink is required")
	}
	if deps.Clock == nil {
		return Result{}, errors.New("derive: clock is required")
	}
	if req.Run.ID == "" {
		return Result{}, errors.New("derive: run id is required")
	}
	if req.Run.TenantID == "" {
		return Result{}, errors.New("derive: tenant id is required")
	}

	// Look up op by name.
	op, ok := lookupOp(req.Op)
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownOp, req.Op)
	}

	// Validate rounding mode upfront so we fail fast before any I/O.
	if err := validateRounding(req.Rounding); err != nil {
		return Result{}, err
	}

	var result Result
	if err := deps.Scope.WithTx(ctx, func(ctx context.Context, tx kernel.Tx) error {
		// Load all tokens for this run (used both for input resolution and for
		// lineage traversal by filter.sum_where).
		allTokens, err := deps.Store.Tokens(ctx, tx, req.Run.ID)
		if err != nil {
			return fmt.Errorf("derive: list tokens: %w", err)
		}
		tokensByID := make(map[string]kernel.Token, len(allTokens))
		for _, tok := range allTokens {
			tokensByID[tok.ID] = tok
		}

		// Validate and resolve the input tokens.
		resolvedInputs := make([]kernel.Token, len(req.Inputs))
		for i, reqTok := range req.Inputs {
			tok, ok := tokensByID[reqTok.ID]
			if !ok {
				return fmt.Errorf("%w: %s", ErrInputNotInStore, reqTok.ID)
			}
			resolvedInputs[i] = tok
		}

		var (
			evalValue evaluation
			matched   []string // non-nil only for filter.sum_where
		)

		if req.Op == "filter.sum_where" {
			// filter.sum_where requires a predicate and captured-read access.
			if req.Predicate == nil {
				return fmt.Errorf("derive: filter.sum_where requires a non-nil Predicate")
			}
			if deps.CapturedReadStore == nil {
				return fmt.Errorf("derive: filter.sum_where requires CapturedReadStore in Dependencies")
			}
			allReads, err := deps.CapturedReadStore.CapturedReads(ctx, tx, req.Run.ID)
			if err != nil {
				return fmt.Errorf("derive: list captured reads: %w", err)
			}
			readsByID := make(map[string]kernel.CapturedRead, len(allReads))
			for _, r := range allReads {
				readsByID[r.ID] = r
			}
			fev, ferr := ops.FilterSumWhere{}.EvaluateFilter(
				resolvedInputs,
				req.Rounding,
				*req.Predicate,
				tokensByID,
				readsByID,
			)
			if ferr != nil {
				return mapOpsErr(ferr)
			}
			evalValue = evaluation{Value: fev.Value, Unit: fev.Unit}
			matched = fev.Matched
		} else {
			// Standard op evaluation path.
			ev, err := op.Evaluate(resolvedInputs, req.Rounding)
			if err != nil {
				return mapOpsErr(err)
			}
			evalValue = ev
		}

		valueCanonical := ops.FormatCanonical(evalValue.Value)

		// Compute deterministic token ID.
		inputIDs := make([]string, len(req.Inputs))
		for i, tok := range req.Inputs {
			inputIDs[i] = tok.ID
		}
		tokenID := derivation.DeriveTokenID(
			req.Run.ID,
			req.Run.TenantID,
			req.Op,
			inputIDs,
			valueCanonical,
			evalValue.Unit,
		)

		now := deps.Clock.Now().UTC()

		// Build the derived token.
		token := kernel.Token{
			ID:             tokenID,
			RunID:          req.Run.ID,
			TenantID:       req.Run.TenantID,
			ValueCanonical: valueCanonical,
			Unit:           evalValue.Unit,
			Derivation: &kernel.Derivation{
				Operation: req.Op,
				Inputs:    append([]string(nil), inputIDs...),
				FormulaID: req.Op,
				Rounding:  req.Rounding,
				Evaluated: valueCanonical,
			},
			CreatedAt: now,
		}

		// Build the audit payload.
		auditPayload := derivation.Payload{
			TokenID:   tokenID,
			Operation: req.Op,
			Inputs:    append([]string(nil), inputIDs...),
			FormulaID: req.Op,
			Rounding:  req.Rounding,
			Evaluated: valueCanonical,
			Unit:      evalValue.Unit,
		}
		if req.Op == "filter.sum_where" {
			auditPayload.Matched = append([]string(nil), matched...)
			auditPayload.Predicate = req.Predicate
		}
		det, err := derivation.Encode(auditPayload)
		if err != nil {
			return fmt.Errorf("derive: encode payload: %w", err)
		}

		// Look up the current audit chain tail to compute seq and prevHash.
		entries, err := deps.AuditSink.AuditEntries(ctx, tx, req.Run.TenantID, req.Run.ID)
		if err != nil {
			return fmt.Errorf("derive: read audit chain: %w", err)
		}
		prevHash := deriveGenesisHash(req.Run.TenantID)
		seq := uint64(1)
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			prevHash = append([]byte(nil), last.Hash...)
			seq = last.Seq + 1
		}

		entry := kernel.AuditEntry{
			Seq:      seq,
			RunID:    req.Run.ID,
			TenantID: req.Run.TenantID,
			Kind:     kernel.EntryKindTokenDerive,
			At:       now,
			PrevHash: prevHash,
			Hash:     deriveAuditHash(prevHash, seq, req.Run.ID, req.Run.TenantID, kernel.EntryKindTokenDerive, det),
			Payload: kernel.PayloadEnvelope{
				Deterministic:      det,
				HashInputCanonical: det,
			},
		}

		// Persist token first, then append audit entry (fail-closed: both are
		// inside the same transaction).
		if err := deps.Store.PutToken(ctx, tx, token); err != nil {
			return fmt.Errorf("derive: persist token: %w", err)
		}
		appended, err := deps.AuditSink.AppendAuditEntry(ctx, tx, entry)
		if err != nil {
			return fmt.Errorf("derive: append audit entry: %w", err)
		}

		result = Result{Token: token, AuditEntry: appended}
		return nil
	}); err != nil {
		return Result{}, err
	}

	return result, nil
}

// mapOpsErr rewraps internal ops-package errors to the public derive error sentinels.
func mapOpsErr(err error) error {
	switch {
	case errors.Is(err, ops.ErrUnitMismatch):
		return fmt.Errorf("%w: %v", ErrUnitMismatch, err)
	case errors.Is(err, ops.ErrNoInputs):
		return ErrNoInputs
	case errors.Is(err, ops.ErrArity):
		return fmt.Errorf("%w: %v", ErrArity, err)
	case errors.Is(err, ops.ErrDecimalParse):
		return fmt.Errorf("%w: %v", ErrDecimalParse, err)
	case errors.Is(err, ops.ErrDivisionByZero):
		return ErrDivisionByZero
	case errors.Is(err, ops.ErrRoundingRequired):
		return fmt.Errorf("%w: %v", ErrRoundingRequired, err)
	case errors.Is(err, ops.ErrRoundingMode):
		return fmt.Errorf("%w: %v", ErrRoundingMode, err)
	case errors.Is(err, ops.ErrAmbiguousLineage):
		return fmt.Errorf("%w: %v", ErrAmbiguousLineage, err)
	case errors.Is(err, ops.ErrPredicateNotRowShaped):
		return fmt.Errorf("%w: %v", ErrPredicateNotRowShaped, err)
	case errors.Is(err, ops.ErrPredicateOp):
		return fmt.Errorf("%w: %v", ErrPredicateOp, err)
	case errors.Is(err, ops.ErrNoMatchingInputs):
		return ErrNoMatchingInputs
	default:
		return err
	}
}

func validateRounding(r kernel.RoundingPolicy) error {
	switch r.Mode {
	case "", "half_even", "half_up", "down":
	default:
		return fmt.Errorf("derive: invalid rounding mode %q", r.Mode)
	}
	if r.Scale < 0 || r.Scale > 18 {
		return fmt.Errorf("derive: rounding scale %d out of range [0, 18]", r.Scale)
	}
	return nil
}

// deriveGenesisHash mirrors the unexported kernel.genesisHash so we can
// compute the genesis hash for audit-chain chaining without importing
// non-public symbols.
func deriveGenesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

// deriveAuditHash mirrors the unexported kernel.auditHash.
func deriveAuditHash(prevHash []byte, seq uint64, runID, tenantID string, kind kernel.EntryKind, hashInput []byte) []byte {
	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, seq)

	h := sha256.New()
	h.Write(prevHash)
	h.Write(seqBytes)
	h.Write([]byte(runID))
	h.Write([]byte(tenantID))
	h.Write([]byte(kind))
	h.Write(hashInput)
	return h.Sum(nil)
}
