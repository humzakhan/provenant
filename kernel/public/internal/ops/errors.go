package ops

import "errors"

// Sentinel errors returned by op implementations.
var (
	ErrUnitMismatch          = errors.New("derive: unit mismatch")
	ErrNoInputs              = errors.New("derive: no inputs")
	ErrArity                 = errors.New("derive: wrong arity")
	ErrDecimalParse          = errors.New("derive: cannot parse canonical decimal")
	ErrDivisionByZero        = errors.New("derive: division by zero")
	ErrRoundingRequired      = errors.New("derive: rounding scale required")
	ErrRoundingMode          = errors.New("derive: unsupported rounding mode")
	ErrAmbiguousLineage      = errors.New("derive: ambiguous lineage")
	ErrPredicateNotRowShaped = errors.New("derive: predicate requires row-shaped source")
	ErrPredicateOp           = errors.New("derive: unsupported predicate op")
	ErrNoMatchingInputs      = errors.New("derive: no inputs matched predicate")
)
