package ops

import (
	"github.com/shopspring/decimal"
	kernel "github.com/provenant/provenant/kernel/public"
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

// All returns every registered Phase 02 op in a stable order.
// Phase 03 will extend this list.
func All() []Op {
	return []Op{
		Add{},
		Subtract{},
		Sum{},
	}
}
