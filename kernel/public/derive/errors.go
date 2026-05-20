// Package derive exposes the kernel derivation engine. Derive evaluates a
// named op over input tokens and returns a new derived Token plus an AuditEntry
// of kind token_derive. It does no I/O and uses no randomness.
package derive

import "errors"

// Sentinel errors returned by Derive.
var (
	ErrUnknownOp             = errors.New("derive: unknown op")
	ErrUnitMismatch          = errors.New("derive: unit mismatch")
	ErrNoInputs              = errors.New("derive: no inputs")
	ErrArity                 = errors.New("derive: wrong arity")
	ErrInputNotInStore       = errors.New("derive: input token not in store")
	ErrDecimalParse          = errors.New("derive: cannot parse canonical decimal")
	ErrDivisionByZero        = errors.New("derive: division by zero")
	ErrRoundingRequired      = errors.New("derive: rounding scale required")
	ErrRoundingMode          = errors.New("derive: unsupported rounding mode")
	ErrAmbiguousLineage      = errors.New("derive: ambiguous lineage")
	ErrPredicateNotRowShaped = errors.New("derive: predicate requires row-shaped source")
	ErrPredicateOp           = errors.New("derive: unsupported predicate op")
	ErrNoMatchingInputs      = errors.New("derive: no inputs matched predicate")
)
