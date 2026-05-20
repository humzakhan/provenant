package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/derive"
	"github.com/provenant/provenant/kernel/public/runfile"
)

const maxCalcInputs = 1000

// calcRequest is the JSON body for `calc`.
type calcRequest struct {
	RunFile   string                `json:"run_file"`
	Op        string                `json:"op"`
	Inputs    []string              `json:"inputs"`
	Rounding  kernel.RoundingPolicy `json:"rounding,omitempty"`
	Predicate *kernel.RowPredicate  `json:"predicate,omitempty"`
}

// calcResponse is written to stdout on `calc` success.
type calcResponse struct {
	Token      kernel.Token      `json:"token"`
	AuditEntry kernel.AuditEntry `json:"audit_entry"`
	Seq        uint64            `json:"seq"`
}

func (r Runner) runCalc(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[calcRequest](args, stdin)
	if err != nil {
		return err
	}
	if err := validateCalcRequest(req); err != nil {
		return err
	}

	clock := fixedClock{t: r.now()}

	// Open the run partfile (replays existing state).
	run, err := runfile.Recover(req.RunFile)
	if err != nil {
		return normalizeRunfileError(err)
	}
	run.SetClock(clock)

	kernRun := run.RunIdentity()

	// Validate predicate usage before looking up tokens.
	if req.Op != "filter.sum_where" && req.Predicate != nil {
		_ = run.Abort()
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("predicate is only valid for filter.sum_where, got op %q", req.Op))
	}

	// Resolve input tokens from the run's current token set.
	tokensByID := make(map[string]kernel.Token, len(run.Tokens()))
	for _, tok := range run.Tokens() {
		tokensByID[tok.ID] = tok
	}

	inputs := make([]kernel.Token, len(req.Inputs))
	for i, id := range req.Inputs {
		tok, ok := tokensByID[id]
		if !ok {
			_ = run.Abort()
			return newPacketError(packetErrorTokenNotFound, fmt.Errorf("token %q not found in run %s", id, kernRun.ID))
		}
		inputs[i] = tok
	}

	// Build a chain-aware sink so derive.Derive can compute the correct seq/prevHash.
	sink := newRunfileChainSink(run, kernRun)

	result, err := derive.Derive(ctx, derive.Dependencies{
		Clock:             clock,
		Store:             sink,
		AuditSink:         sink,
		Scope:             sink,
		CapturedReadStore: sink,
	}, derive.Request{
		Run:       kernRun,
		Op:        req.Op,
		Inputs:    inputs,
		Rounding:  req.Rounding,
		Predicate: req.Predicate,
	})
	if err != nil {
		_ = run.Abort()
		return normalizeCalcError(err)
	}

	// Append the derive result to the runfile.
	if err := run.AppendEntry(result.AuditEntry, &result.Token, nil); err != nil {
		_ = run.Abort()
		return normalizeRunfileError(err)
	}

	seq := run.LastSeq()
	if err := run.Close(); err != nil {
		return fmt.Errorf("runfile: close after calc: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(calcResponse{
		Token:      result.Token,
		AuditEntry: result.AuditEntry,
		Seq:        seq,
	})
}

func validateCalcRequest(req calcRequest) error {
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required"))
	}
	if req.Op == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("op is required"))
	}
	if len(req.Inputs) == 0 {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("inputs must be non-empty"))
	}
	if len(req.Inputs) > maxCalcInputs {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("inputs exceeds maximum of %d", maxCalcInputs))
	}
	if req.Rounding.Scale > 18 {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("rounding.scale %d out of range [0, 18]", req.Rounding.Scale))
	}
	return nil
}

func normalizeCalcError(err error) error {
	switch {
	case errors.Is(err, derive.ErrUnknownOp),
		errors.Is(err, derive.ErrUnitMismatch),
		errors.Is(err, derive.ErrNoInputs),
		errors.Is(err, derive.ErrArity),
		errors.Is(err, derive.ErrDecimalParse),
		errors.Is(err, derive.ErrDivisionByZero),
		errors.Is(err, derive.ErrRoundingRequired),
		errors.Is(err, derive.ErrRoundingMode),
		errors.Is(err, derive.ErrAmbiguousLineage),
		errors.Is(err, derive.ErrPredicateNotRowShaped),
		errors.Is(err, derive.ErrPredicateOp),
		errors.Is(err, derive.ErrNoMatchingInputs),
		errors.Is(err, derive.ErrInputNotInStore):
		return newPacketError(packetErrorRequestInvalid, err)
	default:
		return err
	}
}
