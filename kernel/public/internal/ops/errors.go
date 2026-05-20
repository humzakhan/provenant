package ops

import "errors"

// Sentinel errors returned by op implementations.
var (
	ErrUnitMismatch  = errors.New("derive: unit mismatch")
	ErrNoInputs      = errors.New("derive: no inputs")
	ErrArity         = errors.New("derive: wrong arity")
	ErrDecimalParse  = errors.New("derive: cannot parse canonical decimal")
)
