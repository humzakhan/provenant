package ops

import (
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/shopspring/decimal"
)

// Op is a named pure arithmetic function over input tokens.
// Name must match the op catalog entry string (e.g. "calc.add").
// Evaluate is pure: no I/O, no clock, no side effects.
type Op interface {
	Name() string
	Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error)
}

// Evaluation is the result of a successful op evaluation before token
// wrapping. Value is the unrounded intermediate unless the op applies
// rounding internally (which all Phase 02 ops do via ApplyRounding).
type Evaluation struct {
	Value decimal.Decimal
	Unit  string
}

// All returns every registered op in a stable order. Phase 02 ops are first,
// followed by Phase 03 additions.
//
// Note: FilterSumWhere is included so its name is registered, but its
// standard Evaluate method must not be called directly — the derive engine
// dispatches it via the EvaluateFilter path.
func All() []Op {
	return []Op{
		Add{},
		Subtract{},
		Sum{},
		Multiply{},
		Divide{},
		Passthrough{},
		Round{},
		FilterSumWhere{},
	}
}
