# Phase 03 — Op Catalog Rest

## Objective

Land the remaining five ops on top of the Phase 02 engine: `calc.multiply`, `calc.divide`, `filter.sum_where`, `select.passthrough`, `round.half_even`. This phase introduces unit algebra for multiplicative ops, explicit rounding modes, and a tightly-scoped predicate evaluator for `filter.sum_where` that operates only over captured-read row metadata — never over arbitrary expressions.

## Scope (in)

- `kernel/public/internal/ops/multiply.go`, `divide.go`, `passthrough.go`, `round.go`, `filter_sum_where.go`.
- Unit-algebra helpers in `kernel/public/internal/ops/units.go`.
- Predicate type `kernel.RowPredicate` and decoder, supporting a small fixed set of comparisons.
- Registration in `kernel/public/derive`.

## Scope (out)

- No new ops beyond the five above.
- No expression language. `filter.sum_where` predicates are fixed-shape JSON, not strings.
- No CLI surface (Phase 05).
- No new audit entry kinds — every op continues to emit `token_derive`.

## Technical Design

### `calc.multiply`

```go
// inputs[0] * inputs[1]
// Unit algebra:
//   USD × ""  -> USD       (currency × dimensionless)
//   ""  × USD -> USD
//   ""  × ""  -> ""        (dimensionless × dimensionless)
//   USD × USD -> ErrUnitMismatch  (squared currency is meaningless here)
//   USD × EUR -> ErrUnitMismatch
```

Arity 2; commutative on value but the audit entry preserves supplied order. Default rounding `half_even` at scale 9.

### `calc.divide`

```go
// inputs[0] / inputs[1]
// Unit algebra:
//   USD / USD -> ""        (ratio)
//   USD / ""  -> USD
//   ""  / ""  -> ""
//   ""  / USD -> ErrUnitMismatch  (1/USD is not modeled)
//   USD / EUR -> ErrUnitMismatch
// Division by zero -> ErrDivisionByZero (new sentinel)
```

Rounding is mandatory: divide rejects `RoundingPolicy.Scale == 0` with `ErrRoundingRequired` to force callers to make precision explicit. The kernel does not pick a default scale.

### `select.passthrough`

```go
// Arity 1. Returns a derived token whose value and unit equal the input.
// Used to (a) re-anchor a leaf token under a logical name, or (b) introduce
// a deterministic literal-zero / literal-one ground token.
```

The kernel still requires the input to exist as a token (leaf or derived). A literal-zero token must be introduced upstream — typically by minting from a constant captured read like a config-blob source reader, or by a future `mint.literal` reader. Phase 03 does not add a literal-mint reader; if callers need a zero ground they pre-mint one via the existing leaf-mint path.

### `round.half_even`

```go
// Arity 1. Rounds the input to RoundingPolicy.Scale fractional digits using
// banker's rounding. Unit-preserving. RoundingPolicy.Mode must be either "" or
// "half_even"; other modes return ErrRoundingMode.
```

This is the only op whose entire purpose is rounding. Other ops apply rounding implicitly per their default policy.

### `filter.sum_where`

This is the most constrained op. It sums a subset of input tokens selected by a row-metadata predicate.

```go
// kernel/public/types.go (extend)
type RowPredicate struct {
    Field    string         `json:"field"`     // e.g. "row.account", "row.region"
    Op       string         `json:"op"`        // "eq", "in"
    Value    string         `json:"value,omitempty"`
    Values   []string       `json:"values,omitempty"`
}
```

```go
// kernel/public/derive/derive.go (extend Request)
type Request struct {
    // ...existing fields...
    Predicate *kernel.RowPredicate // honored only by filter.sum_where; nil otherwise
}
```

`filter.sum_where` evaluates the predicate against each input token's *leaf-source row metadata*. Concretely, for each input it walks back through `Derivation.Inputs` to find the underlying leaf `CapturedRead`, parses the row JSON via the reader's recorded `OutputPath`, and tests the predicate against the row's structured fields. Tokens whose leaves do not match are excluded from the sum.

Constraints, all fail-closed:

- Every input must trace to exactly one leaf captured read. Mixed-lineage inputs (more than one leaf) are rejected with `ErrAmbiguousLineage`.
- All leaves must originate from row-shaped readers (`extract.csv_rows`, `extract.spreadsheet`, `extract.jsonl`). `extract.json` is rejected with `ErrPredicateNotRowShaped`.
- Predicate `Op` is in {`eq`, `in`}. Anything else returns `ErrPredicateOp`.
- Field paths are limited to top-level row fields (no nested traversal in Phase 03).
- Unit safety is `RequireSameUnit` across the *selected* inputs after filtering. If no inputs match, the op returns `ErrNoMatchingInputs` rather than producing a zero — the zero must be grounded.

The deterministic audit payload for `filter.sum_where` includes the predicate verbatim, the ordered subset of input token IDs that passed the predicate, and the canonical sum.

```json
{
  "token_id": "tok_<derived>",
  "operation": "filter.sum_where",
  "inputs": ["tok_a", "tok_b", "tok_c", "tok_d"],
  "predicate": {"field": "row.account", "op": "eq", "value": "4000"},
  "matched": ["tok_a", "tok_c"],
  "evaluated": "123.456789000",
  "unit": "USD",
  "formula_id": "filter.sum_where"
}
```

`matched` is canonical: sorted by the input position so replay produces identical bytes.

## Implementation Steps

1. Extend `kernel/public/types.go` with `RowPredicate`.
2. Add `internal/ops/multiply.go`, `divide.go`, `passthrough.go`, `round.go`, `filter_sum_where.go`.
3. Extend `kernel/public/internal/ops/units.go` with `MultiplyUnit(a, b string)` and `DivideUnit(a, b string)`.
4. Extend `kernel/public/derive.Request` with `Predicate *kernel.RowPredicate`. Document that it is honored only by `filter.sum_where`.
5. Extend `kernel/public/internal/derivation/payload.go` to emit `matched` and `predicate` fields for `filter.sum_where` and to enforce field absence for other ops.
6. Extend bundle decoder in `kernel/public/bundle/bundle.go` to accept the new payload fields and validate `matched` is a subset of `inputs`, in original order.
7. Add `ErrDivisionByZero`, `ErrRoundingRequired`, `ErrRoundingMode`, `ErrAmbiguousLineage`, `ErrPredicateNotRowShaped`, `ErrPredicateOp`, `ErrNoMatchingInputs`.
8. Add `ResolveLeafCapturedRead(token kernel.Token, store kernel.TokenStore, reads kernel.CapturedReadStore) (kernel.CapturedRead, error)` in `kernel/public/internal/derivation/lineage.go`.

## Tests Required

| Test | Asserts |
|---|---|
| `calc.multiply` USD × dim | Returns USD; reverse order also returns USD |
| `calc.multiply` USD × USD | Returns `ErrUnitMismatch` |
| `calc.divide` USD / USD | Returns dimensionless; banker's-rounded to requested scale |
| `calc.divide` by zero | Returns `ErrDivisionByZero`, no audit entry appended |
| `calc.divide` missing scale | Returns `ErrRoundingRequired` |
| `round.half_even` boundary | `2.5` rounded to scale 0 returns `2`; `3.5` returns `4`; `-2.5` returns `-2` |
| `select.passthrough` | Output value and unit equal input; audit entry has 1 input |
| `filter.sum_where` happy | Three CSV-row leaves over `account in ["4000","4010"]`; sum equals matched subset |
| `filter.sum_where` mixed lineage | Input that is itself a derived token with two leaf ancestors returns `ErrAmbiguousLineage` |
| `filter.sum_where` no matches | Returns `ErrNoMatchingInputs` |
| `filter.sum_where` JSON leaf | Returns `ErrPredicateNotRowShaped` |
| Replay byte-equality across all ops | Phase 02 replay test extended to include one of each new op |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] No new public API beyond the op names registered in `derive`, `kernel.RowPredicate`, `derive.Request.Predicate`, and the sentinel errors.
- [ ] Bundle backward-compat regression test green.
- [ ] `filter.sum_where` predicate cannot evaluate arbitrary expressions — confirmed by code review and a fuzz target on `RowPredicate.Op` that asserts only `eq` and `in` are accepted.

## Risks / Open Questions

- Lineage resolution: do we walk `Derivation.Inputs` recursively at derive time, or precompute lineage on each token? Phase 03 walks at derive time, accepting O(depth) cost. Phase 07 may cache.
- `select.passthrough` is structurally redundant with the input token. We keep it because it gives the audit chain an explicit "re-anchor" event that consumers (and Phase 07's trace tool) can present as an intentional step.
- Banker's rounding on exact-half negatives: `decimal.NewFromString("-2.5").RoundBank(0)` returns `-2`. Confirm with a vector test before relying on it.
- We are not adding `calc.ratio`, `calc.percent_change`, `calc.variance`, or `calc.weighted_average` from the architecture doc. Those compose from the eight ops we ship. If a future skill pack demands them as primitives, a separate phase plan will be drafted.

## Dependencies

- [Phase 01](./phase-01-token-dag-schema.md) (bundle accepts `token_derive`).
- [Phase 02](./phase-02-op-catalog-core.md) (engine and additive ops).
