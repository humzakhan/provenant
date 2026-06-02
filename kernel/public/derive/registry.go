package derive

import (
	"github.com/shopspring/decimal"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/internal/ops"
)

// op is a named pure arithmetic function over input tokens. Package-private.
type op interface {
	Name() string
	Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (evaluation, error)
}

// evaluation is the result of a successful op evaluation. Package-private.
type evaluation struct {
	Value decimal.Decimal
	Unit  string
}

// registry maps op names to their implementations. Seeded by init; read-only
// after init completes.
var registry = map[string]op{}

func init() {
	for _, o := range ops.All() {
		adapted := &opAdapter{inner: o}
		registry[adapted.Name()] = adapted
	}
}

// opAdapter bridges the internal ops.Op interface to the package-level op
// interface so the two packages stay decoupled.
type opAdapter struct {
	inner ops.Op
}

func (a *opAdapter) Name() string { return a.inner.Name() }

func (a *opAdapter) Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (evaluation, error) {
	ev, err := a.inner.Evaluate(inputs, rounding)
	if err != nil {
		return evaluation{}, mapOpsErr(err)
	}
	return evaluation{Value: ev.Value, Unit: ev.Unit}, nil
}

func lookupOp(name string) (op, bool) {
	o, ok := registry[name]
	return o, ok
}
