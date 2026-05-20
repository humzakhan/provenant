# Phase 01 — Token DAG Schema and `token_derive` Audit Kind

## Objective

Codify the leaf-vs-derived token distinction in the kernel types and audit chain. Introduce `EntryKindTokenDerive`, extend `bundle.Replay` to accept derivation entries, and tighten the existing invariants so leaf tokens still behave exactly as they do today. Do not implement any ops in this phase — the kernel must refuse to *produce* derived tokens until Phase 02 lands.

## Scope (in)

- `kernel/public/types.go`: tighten `Derivation`, document leaf-vs-derived invariants on `Token`.
- `kernel/public/types.go`: add `EntryKindTokenDerive` constant.
- `kernel/public/bundle/bundle.go`: extend `normalizeState`, `auditLine`, and `decodeTokenMintPayload` callers to recognize a new `token_derive` payload shape and validate derivation tokens.
- `kernel/public/bundle/bundle.go`: relax the `len(AuditEntries) == len(Tokens)` constraint to `one audit entry per minted-or-derived token`.
- New file `kernel/public/internal/derivation/payload.go` for the canonical `token_derive` payload encoder/decoder, mirroring `phaseone.MintEvidence`.
- Golden tests for the bundle backward-compat path: existing `examples/reconciliation` ZIP fixtures must still verify and replay.

## Scope (out)

- No op execution. No `Derive` function.
- No CLI changes.
- No run lifecycle changes.
- No named-source layer.
- No new pack format.

## Technical Design

### Token shape

```go
// kernel/public/types.go
type Token struct {
    ID             string      `json:"id"`
    RunID          string      `json:"run_id"`
    TenantID       string      `json:"tenant_id"`
    ValueCanonical string      `json:"value_canonical"`
    Unit           string      `json:"unit,omitempty"`
    Source         TokenSource `json:"source"`           // zero value on derived tokens
    Derivation     *Derivation `json:"derivation,omitempty"`
    CreatedAt      time.Time   `json:"created_at"`
}

// Exactly one of (Source.CapturedReadID != "") or (Derivation != nil) must hold.
// A token with both, or neither, is invalid and fails Verify/Replay.
```

### Derivation shape

```go
// kernel/public/types.go
type Derivation struct {
    Operation string         `json:"operation"`           // e.g. "calc.add"
    Inputs    []string       `json:"inputs"`              // ordered token IDs
    FormulaID string         `json:"formula_id,omitempty"`
    Rounding  RoundingPolicy `json:"rounding,omitempty"`
    Evaluated string         `json:"evaluated"`           // canonical decimal string, equals Token.ValueCanonical
}

type RoundingPolicy struct {
    Mode  string `json:"mode,omitempty"`  // "half_even" (default), "half_up", "down"
    Scale int    `json:"scale,omitempty"` // decimal places; 0 for unset
}
```

Adding `Evaluated` to `Derivation` (instead of relying on `Token.ValueCanonical`) means the audit hash binds the computed result to the op identity, not just to the token identity.

### Audit kind

```go
// kernel/public/types.go
const EntryKindTokenDerive EntryKind = "token_derive"
```

### `token_derive` payload

```json
{
  "token_id": "tok_<derived>",
  "operation": "calc.add",
  "inputs": ["tok_a", "tok_b"],
  "formula_id": "calc.add",
  "rounding": {"mode": "half_even", "scale": 2},
  "evaluated": "300.000000000",
  "unit": "USD"
}
```

The deterministic payload is the JCS-canonicalized form of this object. The audit hash is computed exactly as for `token_mint`: `auditHash(prevHash, seq, runID, tenantID, "token_derive", deterministic)`.

### Bundle validation changes

In `kernel/public/bundle/bundle.go`, `normalizeState` currently asserts:

```go
if len(state.AuditEntries) != len(state.Tokens) || len(state.CapturedReads) == 0 || len(state.CapturedReads) > len(state.Tokens) {
    return State{}, fmt.Errorf("%w: expected one audit per token and captured reads referenced by tokens", ErrTamperDetected)
}
```

That stays — `audit count == token count` is still correct — but the per-entry switch must dispatch on `EntryKind`:

```go
switch entry.Kind {
case kernel.EntryKindTokenMint:
    // existing leaf path
case kernel.EntryKindTokenDerive:
    // new derived path: decode payload, look up token, verify all input token IDs
    // exist earlier in the chain, verify Token.Derivation == payload, verify
    // Token.ValueCanonical == payload.Evaluated, verify Token.Source is zero.
default:
    return State{}, fmt.Errorf("%w: unsupported audit kind %s", ErrTamperDetected, entry.Kind)
}
```

A derived token must not contribute to `referencedReadIDs`. The "every captured read is referenced" check stays but only the mint branch fills the set.

### `decodeTokenMintPayload` peer

Add `decodeTokenDerivePayload` in `kernel/public/bundle/bundle.go` returning a `tokenDerivePayload` struct. Validate:

- `token_id`, `operation`, `evaluated` are non-empty
- `inputs` is non-empty, all IDs appear in `tokensByID`, and each input's audit seq is strictly less than the current entry's seq (DAG, no cycles, no forward refs)
- `rounding.mode` is in {"", "half_even", "half_up", "down"}; "" implies "half_even"
- `rounding.scale` is in [0, 18]

### Token-bundle integrity

Add `validateDerivedTokenAgainstPayload` analogous to `validateTokenAgainstPayload`. It checks token ID equality, run/tenant, unit equals payload unit, value canonical equals payload evaluated, derivation operation/inputs/rounding/evaluated equal payload, and `Source == TokenSource{}`.

## Implementation Steps

1. Edit `kernel/public/types.go`:
   - Add `RoundingPolicy` type.
   - Replace existing `Derivation` with the shape above; keep `omitempty` semantics so unmarshaling old bundles still works.
   - Add `EntryKindTokenDerive` constant.
   - Document the leaf-vs-derived invariant in a doc comment on `Token`.
2. Create `kernel/public/internal/derivation/payload.go` with `Payload`, `Encode`, `Decode`, and exported validation errors.
3. Edit `kernel/public/bundle/bundle.go`:
   - Add `tokenDerivePayload` struct mirroring `tokenMintPayload`.
   - Add `decodeTokenDerivePayload` with strict field validation.
   - Extend `normalizeState`'s audit-entry loop to dispatch on `EntryKind`.
   - Add `validateDerivedTokenAgainstPayload`.
   - Preserve the order of `normalizedTokens` (driven by audit seq).
4. Extend `cloneToken` (already clones `Derivation.Inputs`) to also deep-copy `Derivation.Rounding`.
5. Update `kernel/public/types_test.go` and add `kernel/public/bundle/bundle_token_derive_test.go` with table-driven cases.

## Tests Required

| Test | Location | Asserts |
|---|---|---|
| Leaf-only bundle replay (golden) | `kernel/public/bundle/bundle_test.go` | Existing fixtures still verify; bundle bytes unchanged |
| Derived-token bundle replay | `kernel/public/bundle/bundle_token_derive_test.go` | Hand-crafted bundle with one mint + one derive verifies and replays |
| Forward-ref rejection | same | A derive entry whose input has a higher seq is rejected with `ErrTamperDetected` |
| Mixed `Source` and `Derivation` | same | Token with both fields populated rejected |
| Empty `inputs[]` | same | Rejected at decode time |
| `Evaluated` ≠ `ValueCanonical` | same | Rejected with a clear error |
| Unknown `Rounding.Mode` | same | Rejected at decode time |
| Backward compat (Phase 5 ZIP) | `access/cli/cli_test.go` | `examples/reconciliation/report.zip` still verifies and replays unchanged |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` clean.
- [ ] No new public API beyond `EntryKindTokenDerive`, `RoundingPolicy`, and `Derivation` field additions. No new exported functions outside `internal/derivation`.
- [ ] Existing pre-Phase-01 bundles in `examples/reconciliation` continue to verify and replay byte-for-byte.
- [ ] `kernel/public/bundle/bundle.go` rejects every malformed derivation shape from the test table above with a typed `ErrTamperDetected`.

## Risks / Open Questions

- The current `Derivation.Inputs` field is already published in `Token`; adding `Evaluated` and `Rounding` is additive but means callers that deeply unmarshal `Token` with `DisallowUnknownFields` against pre-Phase-01 schemas may need a transitional fixture refresh. We accept that — there are no external callers yet.
- `Evaluated` duplicates `Token.ValueCanonical`. We keep both because the audit hash should bind the op output, not just the token identity. Document this in the type comment.
- Should derived tokens have `CreatedAt`? Yes — kernel injects clock at derive time, same as mint. The clock value goes into `payload_non_deterministic`, not the deterministic payload.
- Do we permit a derive entry whose inputs are themselves derived (DAG depth > 2)? Yes; the seq-ordering check is sufficient to forbid cycles.

## Dependencies

None. This is the foundation phase.
