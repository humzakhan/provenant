// Package derivation encodes and decodes the deterministic token_derive audit
// payload. It mirrors phaseone.MintEvidence for the leaf-mint path.
package derivation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
)

// Errors returned by Decode and Encode when the payload is malformed.
var (
	ErrMissingTokenID     = errors.New("derivation: missing token_id")
	ErrMissingOperation   = errors.New("derivation: missing operation")
	ErrMissingEvaluated   = errors.New("derivation: missing evaluated")
	ErrEmptyInputs        = errors.New("derivation: inputs must be non-empty")
	ErrInvalidRounding    = errors.New("derivation: invalid rounding")
	ErrMatchedNotSubset   = errors.New("derivation: matched must be a subset of inputs in original order")
	ErrPredicateFieldOnly = errors.New("derivation: predicate and matched fields are only valid for filter.sum_where")
)

// Payload is the deterministic token_derive audit entry payload. It is the
// canonical source of truth for what was computed; the corresponding Token
// fields are derived from it during bundle verification.
//
// Matched and Predicate are set only for filter.sum_where; they must be absent
// for all other ops. Matched is sorted by input-position (not lexicographically)
// so replay produces identical bytes.
type Payload struct {
	TokenID   string                `json:"token_id"`
	Operation string                `json:"operation"`
	Inputs    []string              `json:"inputs"`
	Matched   []string              `json:"matched,omitempty"`
	Predicate *kernel.RowPredicate  `json:"predicate,omitempty"`
	FormulaID string                `json:"formula_id,omitempty"`
	Rounding  kernel.RoundingPolicy `json:"rounding,omitempty"`
	Evaluated string                `json:"evaluated"`
	Unit      string                `json:"unit,omitempty"`
}

// validRoundingModes is the closed set of allowed rounding mode strings.
// The empty string is treated as "half_even" by the kernel but is valid here
// because the payload omits the field when the default applies.
var validRoundingModes = map[string]bool{
	"":          true,
	"half_even": true,
	"half_up":   true,
	"down":      true,
}

// Encode returns the JCS-canonicalized JSON encoding of p, suitable for use
// as the deterministic payload in an audit entry.
func Encode(p Payload) ([]byte, error) {
	if err := validate(p); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("derivation: encode payload: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("derivation: canonicalize payload: %w", err)
	}
	return canonical, nil
}

// Decode parses and validates a JCS-canonical token_derive deterministic
// payload. It uses DisallowUnknownFields so that any schema drift is caught
// immediately.
func Decode(data []byte) (Payload, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var p Payload
	if err := dec.Decode(&p); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return Payload{}, fmt.Errorf("derivation: truncated payload: %w", err)
		}
		return Payload{}, fmt.Errorf("derivation: decode payload: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Payload{}, errors.New("derivation: trailing payload bytes")
	}
	if err := validate(p); err != nil {
		return Payload{}, err
	}
	return p, nil
}

func validate(p Payload) error {
	switch {
	case p.TokenID == "":
		return ErrMissingTokenID
	case p.Operation == "":
		return ErrMissingOperation
	case p.Evaluated == "":
		return ErrMissingEvaluated
	case len(p.Inputs) == 0:
		return ErrEmptyInputs
	case !validRoundingModes[p.Rounding.Mode]:
		return fmt.Errorf("%w: mode %q not in {half_even, half_up, down}", ErrInvalidRounding, p.Rounding.Mode)
	case p.Rounding.Scale < 0 || p.Rounding.Scale > 18:
		return fmt.Errorf("%w: scale %d out of range [0, 18]", ErrInvalidRounding, p.Rounding.Scale)
	}

	// filter.sum_where-specific validation.
	isFilter := p.Operation == "filter.sum_where"
	if !isFilter && (len(p.Matched) > 0 || p.Predicate != nil) {
		return fmt.Errorf("%w: operation is %q", ErrPredicateFieldOnly, p.Operation)
	}
	if isFilter && len(p.Matched) > 0 {
		if err := validateMatchedSubset(p.Inputs, p.Matched); err != nil {
			return err
		}
	}
	return nil
}

// validateMatchedSubset checks that matched is a subsequence of inputs
// (every element in matched appears in inputs, and the relative order is
// preserved). This mirrors the spec requirement: "matched is canonical: sorted
// by input position so replay produces identical bytes."
func validateMatchedSubset(inputs, matched []string) error {
	inputIdx := make(map[string]int, len(inputs))
	for i, id := range inputs {
		inputIdx[id] = i
	}
	prev := -1
	for _, id := range matched {
		pos, ok := inputIdx[id]
		if !ok {
			return fmt.Errorf("%w: %q not in inputs", ErrMatchedNotSubset, id)
		}
		if pos <= prev {
			return fmt.Errorf("%w: order violation at %q", ErrMatchedNotSubset, id)
		}
		prev = pos
	}
	return nil
}
