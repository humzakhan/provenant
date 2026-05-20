// Package derive exposes the kernel derivation engine. Derive evaluates a
// named op over input tokens and returns a new derived Token plus an AuditEntry
// of kind token_derive. It does no I/O and uses no randomness.
package derive

import "errors"

// Sentinel errors returned by Derive.
var (
	ErrUnknownOp       = errors.New("derive: unknown op")
	ErrUnitMismatch    = errors.New("derive: unit mismatch")
	ErrNoInputs        = errors.New("derive: no inputs")
	ErrArity           = errors.New("derive: wrong arity")
	ErrInputNotInStore = errors.New("derive: input token not in store")
	ErrDecimalParse    = errors.New("derive: cannot parse canonical decimal")
)
