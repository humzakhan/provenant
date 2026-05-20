# Derivation Graph

## Thesis

Provenant today proves where a number was *read from*. The Derivation Graph promotes the token store from a flat list of leaf reads into a directed acyclic graph in which every internal node is a named, deterministic, audited arithmetic op. With it, Provenant proves how a number was *computed* — every derived value carries a verifiable chain back to the bytes that produced it, and every arithmetic step is replayed offline against a fixed op catalog rather than re-executed by an LLM. This is what turns Provenant from a read-provenance system into a reasoning-provenance system, and it is the precondition for the "0 ungrounded numbers" guarantee.

## Mental Model

- **Leaf token**: minted from a `CapturedRead`. `Derivation == nil`, `Source.CapturedReadID` set. This is what exists today. Every leaf carries the reader name, version, source SHA-256, output SHA-256, selector, and the raw byte span the value was extracted from.
- **Derived token**: produced by the kernel derivation engine. `Derivation != nil`, `Source` is the zero value. Its `Evaluated` field is determined entirely by `(operation, inputs[], rounding)`, and replay must reproduce it byte-for-byte. A derived token's ID is a deterministic function of its op, ordered input IDs, and value — so identical computations always produce identical IDs across runs.
- **Op catalog**: a closed, named set of pure functions. No expression evaluator, no user-supplied formulas, no eval. Phase 02 lands the additive core (`calc.add`, `calc.subtract`, `calc.sum`); Phase 03 lands `calc.multiply`, `calc.divide`, `filter.sum_where`, `select.passthrough`, `round.half_even`. Every op enforces unit safety and fails closed on mismatch.
- **Run lifecycle**: a long-lived `Run` owns one append-only bundle file on disk. `read-mint`, `calc <op>`, and `submit-answer` all append audit entries to that same chain. Closing the run finalizes the bundle. Two appenders against the same run file is refused with an OS-level file lock.
- **Ungrounded-number gate**: `submit-answer` accepts only a token ID. A literal number cannot be submitted — the request struct has no `value` field. The guarantee is structural, not a post-hoc check. Any number reaching a downstream consumer must therefore have a verifiable chain back to bytes.
- **Trace-back**: given any token ID in a finished bundle, the trace tool walks the derivation graph to its leaves and shows, per leaf, which file, which row/byte range, which SHA-256, and what amount that leaf contributed. For additive ops the per-leaf contributions sum exactly to the queried token's value.

## Op Catalog Summary

The eight ops below are the entire arithmetic surface. No others are admitted to the kernel without a separate plan.

| Op                  | Arity   | Unit rule                            | Rounding              | Phase |
|---------------------|---------|--------------------------------------|-----------------------|-------|
| `calc.add`          | 2       | same unit                            | half_even, scale 9    | 02    |
| `calc.subtract`     | 2       | same unit                            | half_even, scale 9    | 02    |
| `calc.sum`          | 1..N    | same unit                            | half_even, scale 9    | 02    |
| `calc.multiply`     | 2       | `u × "" → u`; `u × u → error`        | half_even, scale 9    | 03    |
| `calc.divide`       | 2       | `u / u → ""`; `u / "" → u`           | required, caller sets | 03    |
| `filter.sum_where`  | 1..N    | same unit across matched inputs      | half_even, scale 9    | 03    |
| `select.passthrough`| 1       | preserve                             | preserve              | 03    |
| `round.half_even`   | 1       | preserve                             | half_even, required   | 03    |

## Working Model

A `Run` is a sequence of audit entries on a single file. Phase 04 makes that file long-lived: `run start` creates it, every subsequent verb appends one entry, `run finish` finalizes it. The audit chain runs:

```
run_started → token_mint × M → token_derive × N → answer_submitted × K → run_completed
```

Any subset of these is valid as long as `run_started` is first and a `run_completed` or `run_failed` is last. The same hash linkage rules from Phase 1 of the kernel still apply: each entry's `prev_hash` equals the previous entry's `hash`; the genesis hash is derived from the tenant ID.

## Phase Sequence

1. [Phase 01 — Token DAG schema & `token_derive` audit kind](./phase-01-token-dag-schema.md). Codify leaf vs derived shape, extend bundle replay to accept derivation entries, no ops yet.
2. [Phase 02 — Op catalog core (`calc.add`, `calc.subtract`, `calc.sum`)](./phase-02-op-catalog-core.md). Pure kernel `Derive` engine, decimal arithmetic via `shopspring/decimal`, unit safety, replay byte-equality tests.
3. [Phase 03 — Op catalog rest (`calc.multiply`, `calc.divide`, `filter.sum_where`, `select.passthrough`, `round.half_even`)](./phase-03-op-catalog-rest.md). Unit algebra, explicit rounding modes, predicate-driven aggregation over leaf-source metadata.
4. [Phase 04 — Run lifecycle: long-lived bundle file with atomic append](./phase-04-run-lifecycle.md). `runfile` package, lock-based concurrent-append refusal, durability semantics.
5. [Phase 05 — CLI verbs and ungrounded-number gate](./phase-05-cli-surface.md). `run start|finish`, `read-mint`, `calc`, `submit-answer`, `verify`, `replay`. `submit-answer` rejects literals.
6. [Phase 06 — Named-source mapping & flat JSON audit pack](./phase-06-named-sources-and-pack.md). Logical source registry, `audit.pack.json` single-file artifact alongside the existing ZIP.
7. [Phase 07 — Trace-back tool](./phase-07-traceback.md). Given a token ID, return the leaf contribution graph with per-leaf evidence and amounts.
8. [Phase 08 — End-to-end fauxbooks demo and golden replay test](./phase-08-e2e-fauxbooks.md). Synthetic three-file dataset, Operating Income derivation, bit-identical replay assertion.

## Cross-cutting Invariants

- **Determinism**. No clock, randomness, or I/O inside the derivation engine. The kernel's existing injected `Clock` is the only source of `CreatedAt`, and `CreatedAt` lives in the audit entry's non-deterministic payload. The audit hash chain is the only source of order; identical inputs always yield identical token IDs, identical audit hashes, and bit-identical bundle bytes.
- **Fail-closed**. Every op rejects mismatched units, unknown ops, missing inputs, divisions by zero, ambiguous lineage, malformed predicates, and arity violations with a typed error and zero side effects on the audit chain. A failed `Derive` call appends nothing.
- **Unit safety**. `add`, `subtract`, `sum`, `filter.sum_where` require identical units. `multiply` follows the rule `unit × ""` → `unit` and `unit_a × unit_b` → error. `divide` follows `unit / unit` → `""` and `unit / ""` → `unit`; `"" / unit` is rejected. `select.passthrough` and `round.half_even` are unit-preserving.
- **Append-only chain**. Audit entries are written under a POSIX advisory file lock and an atomic rename. Mid-run crashes never corrupt the chain; partial writes are discarded by the recovery path. The hash chain links every entry to its predecessor; truncation, reordering, or substitution all break verification.
- **Ungrounded-number gate**. `submit-answer` cannot encode a literal value. There is no flag, field, or alternative path. The CLI struct has only `run_file`, `token_id`, and `label`; `DisallowUnknownFields` rejects everything else.
- **Backward compatibility**. Old bundles (only `token_mint` entries) continue to `Verify` and `Replay` unchanged. Every new audit kind, payload field, and token field is `omitempty` and additive. Phase 01 explicitly regresses the existing reconciliation fixture; every subsequent phase must keep that regression green.

## Out of Scope

- No HTTP, MCP, gRPC, or other network surfaces. CLI only.
- No general expression language. Ops are fixed and named.
- No fuzzy matching, OCR, PDF extraction, or LLM-driven anything inside the kernel or CLI.
- No persistence layer beyond the single bundle file per run. No DB, no object store.
- No multi-tenant sharing of a run. One run, one file, one writer.
- No retroactive editing of audit entries. The chain is append-only.

## Definition of Done (Whole Feature)

- The end-to-end fauxbooks demo in Phase 08 passes under `go test ./...` and `go vet ./...`.
- Given any derived token ID in the demo's finished bundle, the Phase 07 trace tool reconstructs the full contribution graph back to leaves, with file path, byte/row span, source sha256, and per-leaf amount.
- The Phase 08 golden test re-runs the demo, replays the resulting bundle, and asserts the bundle bytes are bit-identical to the recorded golden.
- Pre-existing bundles minted before Phase 01 still pass `bundle.Verify` and `bundle.Replay` without modification.
- `provenant submit-answer` cannot accept a literal number; only a registered token ID is allowed. This is enforced by the CLI surface and exercised by a failure-fixture test.
- The op catalog matches the eight ops listed above. No new ops have been added without an explicit follow-up plan.
