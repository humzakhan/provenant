# Phase 02 — Op Catalog Core (`calc.add`, `calc.subtract`, `calc.sum`)

## Objective

Implement the pure kernel derivation engine and the three additive ops: `calc.add`, `calc.subtract`, `calc.sum`. The engine is a pure function over input tokens; it returns a new derived `Token` plus an `AuditEntry` of kind `token_derive` and performs no I/O. Replay of any bundle containing these ops must yield byte-identical output.

## Scope (in)

- New package `kernel/public/derive` exposing `Derive` and op registration.
- Internal package `kernel/public/internal/ops` housing each op's pure implementation.
- Decimal arithmetic via `github.com/shopspring/decimal` (already a dependency — see `go.mod` line 5).
- Unit-safety helper shared across additive ops.
- `memio.Binding.Derive` convenience that wires the run, clock, audit sink, and token store to the kernel engine.
- Replay byte-equality test that re-derives every derived token from a bundle and asserts equality with the recorded `Evaluated`.

## Scope (out)

- No multiplicative or filtering ops (Phase 03).
- No CLI surface (Phase 05).
- No run lifecycle changes (Phase 04).
- No named-source layer.

## Technical Design

### Engine surface

```go
// kernel/public/derive/derive.go
package derive

import (
    "context"
    kernel "github.com/provenant/provenant/kernel/public"
)

// Request is the input to a derivation. Op identifies the op catalog entry.
// Inputs is ordered; ordering is part of the deterministic identity.
// Rounding is honored only by ops that round.
type Request struct {
    Run      kernel.Run
    Op       string
    Inputs   []kernel.Token
    Rounding kernel.RoundingPolicy
}

// Result is the derived token plus the audit entry kernel will append.
type Result struct {
    Token      kernel.Token
    AuditEntry kernel.AuditEntry
}

// Derive evaluates Op over Inputs and returns the derived Token plus the
// audit entry the caller must append. It does no I/O, calls no clock other
// than the one provided through Dependencies, and uses no randomness.
func Derive(ctx context.Context, deps Dependencies, req Request) (Result, error)
```

`Dependencies` matches the existing kernel injection style:

```go
type Dependencies struct {
    Clock     kernel.Clock
    Store     kernel.TokenStore
    AuditSink kernel.AuditSink
    Scope     kernel.TransactionalScope
}
```

### Op registry

```go
// kernel/public/derive/registry.go
type Op interface {
    Name() string
    Evaluate(inputs []kernel.Token, rounding kernel.RoundingPolicy) (Evaluation, error)
}

type Evaluation struct {
    Value decimal.Decimal // unrounded
    Unit  string
}
```

`registry` is package-private and seeded by `init()`. Phase 02 registers `calc.add`, `calc.subtract`, `calc.sum`. Phase 03 will register the rest.

### Token ID derivation

Derived token IDs must be deterministic. Use:

```go
tokenID = "tok_" + hexFirst16(sha256(
    runID || tenantID || op || joinByZero(inputTokenIDs) || valueCanonical || unit
))
```

`hexFirst16` matches the existing leaf-token ID convention in `kernel/public/internal/phaseone` (verify before implementing). Add `kernel/public/internal/derivation.DeriveTokenID` and unit-test against fixed vectors.

### Decimal handling

- Parse `Token.ValueCanonical` with `decimal.NewFromString`. Existing leaf tokens use a 9-digit canonical scale (e.g. `"123.450000000"`).
- Compute in full precision. Apply `RoundingPolicy` after the op if `scale > 0` and `mode != ""`. Default `half_even`, default scale 9 (preserves canonical form).
- Re-serialize via `decimal.String()` with explicit padding to 9 decimal places to keep canonical-string stability across platforms.

### Unit safety (additive)

```go
// kernel/public/internal/ops/units.go
func RequireSameUnit(inputs []kernel.Token) (string, error) {
    if len(inputs) == 0 {
        return "", ErrNoInputs
    }
    unit := inputs[0].Unit
    for _, t := range inputs[1:] {
        if t.Unit != unit {
            return "", fmt.Errorf("%w: %s vs %s", ErrUnitMismatch, unit, t.Unit)
        }
    }
    return unit, nil
}
```

`calc.add`, `calc.subtract`, and `calc.sum` all call `RequireSameUnit`.

### Per-op semantics

- `calc.add`: `len(inputs) == 2`, returns `inputs[0] + inputs[1]`. Commutative.
- `calc.subtract`: `len(inputs) == 2`, returns `inputs[0] - inputs[1]` (minuend, subtrahend). Order is significant; the audit payload preserves it.
- `calc.sum`: `len(inputs) >= 1`, returns `sum(inputs)`. Order is significant only for hash stability; the value is order-independent but the audit entry encodes the supplied order.

### Errors

```go
// kernel/public/derive/errors.go
var (
    ErrUnknownOp       = errors.New("derive: unknown op")
    ErrUnitMismatch    = errors.New("derive: unit mismatch")
    ErrNoInputs        = errors.New("derive: no inputs")
    ErrArity           = errors.New("derive: wrong arity")
    ErrInputNotInStore = errors.New("derive: input token not in store")
    ErrDecimalParse    = errors.New("derive: cannot parse canonical decimal")
)
```

All errors are fail-closed: no token is added to the store and no audit entry is appended on error.

## Implementation Steps

1. Create `kernel/public/derive/` with `derive.go`, `registry.go`, `errors.go`.
2. Create `kernel/public/internal/ops/` with `add.go`, `subtract.go`, `sum.go`, `units.go`, and a shared `canonical.go` for decimal parse/format helpers.
3. Wire `Derive`:
   - Look up op by name (`ErrUnknownOp` if missing).
   - Load each `Input.ID` from `deps.Store` (`ErrInputNotInStore` if missing); verify run/tenant equality with `req.Run`.
   - Call `op.Evaluate`.
   - Apply `RoundingPolicy` post-evaluation.
   - Compute deterministic `tokenID`.
   - Build `kernel.Token` with `Derivation` populated and `Source` zero-valued.
   - Build `kernel.AuditEntry` with kind `EntryKindTokenDerive` and deterministic payload encoded via `internal/derivation.Encode`.
   - Append via `deps.AuditSink.AppendAuditEntry` inside `deps.Scope.Transactional`.
   - Persist via `deps.Store`.
4. Extend `kernel/public/memio/memio.go` with a thin wrapper that constructs `Dependencies` from a `Binding`.
5. Add property/replay tests as below.

## Tests Required

| Test | Asserts |
|---|---|
| `calc.add` happy path | Result token value, unit, derivation operation, inputs ordering |
| `calc.subtract` order | `100 - 30 = 70`, swap fails the assertion |
| `calc.sum` over 1, 2, N inputs | Correct totals; empty input list returns `ErrNoInputs` |
| Unit mismatch (USD + EUR) | Returns `ErrUnitMismatch`, no audit entry appended |
| Arity violations | `calc.add` with 1 or 3 inputs returns `ErrArity` |
| Replay byte-equality | Build bundle with 3 leaf mints + 2 `calc.subtract` derives, snapshot bytes, replay, snapshot again — bytes equal |
| Derived input | `calc.add(a, calc.sum(b, c))` — derived token can be input to another derive; audit seq forward-ref rejection holds |
| Deterministic ID | Two runs with identical inputs produce identical derived token IDs |
| Fuzz: `calc.sum` associativity | `sum(a,b,c) == add(a, sum(b,c)) == add(sum(a,b), c)` for random decimals (golden, not approximate) |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` clean.
- [ ] Public API surface limited to `derive.Derive`, `derive.Request`, `derive.Result`, `derive.Dependencies`, and the sentinel errors.
- [ ] `shopspring/decimal` is the only new transitive dependency.
- [ ] Old bundles still verify (regression test from Phase 01 still green).
- [ ] Replay byte-equality test passes on Linux, macOS amd64, and macOS arm64.

## Risks / Open Questions

- Canonical decimal scale: leaf tokens use 9 fractional digits today (`123.450000000`). We adopt the same scale for derived tokens. If a future op needs more, we widen at that point — never narrow.
- `decimal.Decimal.RoundBank` is the banker's-rounding API; confirm semantics on .5 boundary cases under both negative and positive values.
- Should `calc.sum` reject zero inputs? Yes — `ErrNoInputs`. Empty totals must come from `select.passthrough` of a literal zero token (Phase 03), so the zero is always grounded.

## Dependencies

- [Phase 01](./phase-01-token-dag-schema.md) must be merged. The audit chain and bundle replay must already accept `token_derive` entries.
