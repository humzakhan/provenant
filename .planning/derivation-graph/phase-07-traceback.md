# Phase 07 — Trace-Back Tool

## Objective

Given a finalized bundle and a token ID, reconstruct the full contribution graph from that token down to its leaves. For each leaf, return: the captured read's file path, the byte/row span (selector), the source SHA-256, the canonical leaf value, and the numeric contribution the leaf made to the queried token's value. Expose this as a Go API in a new `trace` package and as a CLI verb `provenant trace`.

## Scope (in)

- New package `kernel/public/trace` exposing `Trace(bundle []byte, tokenID string) (*Graph, error)`.
- Graph data model with nodes (one per token in the lineage) and edges (one per derivation input).
- Per-leaf contribution calculation, derived analytically from the op catalog (not by re-running ops).
- CLI verb `trace` reading a bundle path and token ID, returning JSON.

## Scope (out)

- No new ops.
- No partial-run tracing — operate on finalized bundles only. (Mid-run inspection is `replay` on the partfile.)
- No HTML / visual rendering. JSON output only.
- No persistence layer.

## Technical Design

### Graph model

```go
// kernel/public/trace/trace.go
type Graph struct {
    Root          string            `json:"root"`            // queried token ID
    RootValue     string            `json:"root_value"`      // canonical decimal
    RootUnit      string            `json:"root_unit"`
    Nodes         []Node            `json:"nodes"`           // topological order, leaves last
    Edges         []Edge            `json:"edges"`           // parent -> child (input)
    Leaves        []LeafContribution `json:"leaves"`         // one per distinct leaf token in the lineage
}

type Node struct {
    TokenID        string           `json:"token_id"`
    ValueCanonical string           `json:"value_canonical"`
    Unit           string           `json:"unit"`
    Kind           string           `json:"kind"`            // "leaf" or "derived"
    Operation      string           `json:"operation,omitempty"`
    FormulaID      string           `json:"formula_id,omitempty"`
}

type Edge struct {
    From string `json:"from"`   // parent (derived) token ID
    To   string `json:"to"`     // child (input) token ID
    Position int  `json:"position"` // ordinal position in parent's Derivation.Inputs
}

type LeafContribution struct {
    TokenID        string `json:"token_id"`
    CapturedReadID string `json:"captured_read_id"`
    SourceName     string `json:"source_name,omitempty"`
    SourcePath     string `json:"source_path"`
    SourceSHA256   string `json:"source_sha256"`
    Selector       kernel.SourceSelector `json:"selector"`
    LeafValue      string `json:"leaf_value"`              // canonical decimal
    Unit           string `json:"unit"`
    Contribution   string `json:"contribution"`            // signed canonical decimal
}
```

### Contribution calculation

A leaf's contribution is the signed amount it adds to the root token. For the supported ops, contribution is well-defined and exactly computable:

- `calc.add(a, b)`: contribution of `a`'s leaves are themselves; same for `b`.
- `calc.subtract(a, b)`: `a`'s leaf contributions are positive; `b`'s leaf contributions are negated.
- `calc.sum(a, b, ...)`: each input contributes its own leaf values.
- `select.passthrough(a)`: contributions pass through unchanged.
- `round.half_even(a)`: contributions pass through; the rounding residual is recorded as a synthetic `LeafContribution` with `TokenID = ""` and `SourceName = "<rounding>"`. The sum of contributions + rounding residual equals the root value.
- `calc.multiply(a, b)`: when one input is dimensionless, contributions of the currency-unit input are multiplied by the dimensionless input's canonical value; the dimensionless input contributes 0 (it is a scaling factor, not an additive amount).
- `calc.divide(a, b)`: contributions of `a` are divided by `b`'s canonical value. `b`'s leaves contribute 0. A `<rounding>` synthetic residual captures any rounding loss.
- `filter.sum_where(inputs, predicate)`: only `matched` inputs contribute; unmatched inputs are listed in the graph as `Node.Kind = "leaf"` with `Contribution = "0"` for visibility.

Where contribution is well-defined (`add`, `subtract`, `sum`, `passthrough`, `filter.sum_where`, `round.half_even` modulo residual), the sum of `LeafContribution.Contribution` plus rounding residuals equals `RootValue` exactly. The trace asserts this invariant and returns `ErrContributionMismatch` if it ever doesn't — this is a kernel-integrity tripwire.

Where contribution requires multiplicative scaling (`multiply`, `divide`), the leaves of the scaled side carry the scaled contribution; the scalar side reports `Contribution = "0"`. This is a deliberate modeling choice — multiplication is not an additive decomposition, but we still want a reportable answer for "which file did this number come from."

### Algorithm

1. Load bundle via `bundle.Replay`. Build maps `tokensByID`, `readsByID`, `auditBySeq`.
2. DFS from `tokenID` following `Derivation.Inputs` for derived nodes. Stop at leaves (tokens with `Source.CapturedReadID != ""`).
3. Record nodes in topological order (parents before children).
4. Walk each leaf path, accumulating a sign/scale multiplier per edge:
   - `calc.add` and `calc.sum`: multiplier passes unchanged.
   - `calc.subtract` position 0: unchanged. Position 1: negate.
   - `select.passthrough`, `round.half_even`: unchanged.
   - `filter.sum_where`: unchanged for matched inputs, multiplier 0 for unmatched.
   - `calc.multiply`: if the *other* operand is dimensionless, multiply by its canonical value; if both have units it's a kernel-integrity violation (Phase 03 would have rejected the derive).
   - `calc.divide` position 0: divide by the position-1 input's canonical value. Position 1: multiplier 0.
5. `Contribution = leafValue * multiplier`. Use exact decimal arithmetic via `shopspring/decimal`.
6. Add synthetic `<rounding>` contributions wherever a derived node's `Evaluated` differs from the unrounded composition of its inputs.
7. Assert `sum(contributions) + sum(roundingResiduals) == rootValue`.

### CLI verb

```bash
provenant trace --bundle ./run.bundle.json --token tok_x
```

JSON output is the `Graph` struct above, JCS-canonicalized for determinism.

## Implementation Steps

1. Create `kernel/public/trace/trace.go` with `Graph`, `Node`, `Edge`, `LeafContribution`, `Trace`.
2. Create `kernel/public/trace/contributions.go` with the per-op multiplier table.
3. Create `kernel/public/trace/trace_test.go` with table-driven cases covering each op.
4. Add CLI verb `trace` in `access/cli/traceverb.go`.
5. Document the contribution semantics in a doc comment on `Trace` — especially the "multiplicative ops zero-out the scalar side" choice.
6. Extend the contribution invariant check to be a defensive `panic` only inside tests; the production path returns `ErrContributionMismatch`.

## Tests Required

| Test | Asserts |
|---|---|
| Trace a leaf | Returns a graph with one node, one leaf, contribution equals leaf value |
| Trace `calc.subtract` | Two leaves, one with positive contribution, one negative; sum equals root |
| Trace `calc.sum` of 5 | Five leaves, contributions sum exactly to root |
| Trace nested derive | `calc.add(a, calc.subtract(b, c))` — three leaves with correct signs |
| Trace `filter.sum_where` | Unmatched inputs appear with contribution `"0"`; matched inputs sum to root |
| Trace `round.half_even` | Synthetic `<rounding>` contribution captures residual; sum invariant holds |
| Trace `calc.multiply` USD × dim | Currency-side leaves scaled; dimensionless leaves contribute `"0"` |
| Trace `calc.divide` | Numerator leaves scaled by `1/denominator`; denominator leaves contribute `"0"`; rounding residual recorded |
| Trace unknown token | Returns `ErrTokenNotFound` |
| Trace on tampered bundle | `bundle.Verify` fails first; trace never runs |
| Contribution determinism | Two `Trace` calls on the same bundle and token return byte-identical JSON |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] Public API limited to `trace.Trace`, the `Graph`/`Node`/`Edge`/`LeafContribution` types, and the sentinel errors.
- [ ] Contribution invariant `sum(contributions) + rounding == root` holds in every test case where the op set permits exact decomposition.
- [ ] CLI verb produces byte-deterministic JSON output.
- [ ] Documentation in `kernel/public/trace/doc.go` explains the multiplicative scaling choice and why dimensionless-side leaves are reported as zero.

## Risks / Open Questions

- Multiplicative decomposition is genuinely ambiguous in general — multiple decomposition conventions exist. We pick the "scalar contributes zero, currency side carries scaled amount" convention because it matches the audit narrative most reviewers expect ("the FX rate didn't add money; it scaled the EUR balance into USD"). Document this clearly.
- For `calc.divide` with non-decimal-terminating quotients, rounding residual must be computed at the same scale as the derived token's rounding policy. Otherwise the invariant breaks. Test explicitly.
- Should the trace graph include audit-entry seq numbers? Yes — add `Node.AuditSeq` so consumers can correlate trace nodes to audit lines. Add this in the initial design above (not retrofitted).
- Should there be a textual "explain" rendering? Out of scope. Consumers can render `Graph` however they like.

## Dependencies

- [Phase 01](./phase-01-token-dag-schema.md) — bundle replay produces derivation tokens.
- [Phase 02](./phase-02-op-catalog-core.md), [Phase 03](./phase-03-op-catalog-rest.md) — full op catalog.
- Phase 04 and 05 are not strict requirements but are typically already merged before Phase 07 because trace is most useful against runs that actually contain derivations.
