# Provenant Roadmap

This document describes what has been built, what is in progress, and what is planned. It covers the derivation-graph feature set (the core product milestone sequence) and the reconciliation harness extension.

See `docs/thesis.md` for the product context and `architecture/README.md` for the architectural overview.

---

## What Provenant does

Provenant is a Go enforcement library for LLM agents in regulated finance. It turns any agent loop into one whose numeric outputs carry verifiable lineage back to source bytes, whose audit trail is internally tamper-detectable and replayable, and whose deterministic portions produce bit-identical results on replay.

The library is the product. The CLI, MCP server, and HTTP API are optional thin shells over the same kernel. Storage is a deployment concern; the kernel ships an in-memory binding and emits a self-contained audit bundle as bytes.

---

## Current state

The kernel layer is implemented and stable:

- Pure Go kernel with the four non-configurable invariants (non-bypassable, fail-closed, schema-first, replay-deterministic).
- Token minting from captured reads via the `extract.*` tool family.
- Bundle verification and replay: given a finalized bundle, `bundle.Verify` checks the hash chain; `bundle.Replay` reconstructs the token and captured-read state.
- In-memory binding (`kernel/public/memio/`) as the library-mode default.
- Enumeration tables for `EntryKind`, `error_code`, and `failure_subcode`, enforced at write time by `AuditSink.Append`.
- `tools/enumgen/` generator keeping the markdown tables in `architecture/subsystems/audit.md` in sync with the generated Go constants.
- Initial CLI surface (`provenant reconcile`, `provenant read-mint-bundle`).

---

## Derivation graph — the main milestone sequence

The derivation graph promotes Provenant from a read-provenance system into a reasoning-provenance system: every derived value carries a verifiable chain back to the source bytes, and every arithmetic step can be replayed offline against a fixed op catalog.

### Phase 01 — Token DAG schema (complete)

- Codified leaf-vs-derived token distinction in kernel types.
- Introduced `EntryKindTokenDerive` audit kind.
- Extended `bundle.Replay` to accept derivation entries.
- Existing pre-DAG bundles continue to verify and replay unchanged.

### Phase 02 — Op catalog core

Additive arithmetic ops, pure kernel derivation engine, decimal arithmetic via `shopspring/decimal`, unit safety, replay byte-equality tests.

Ops: `calc.add`, `calc.subtract`, `calc.sum`.

Invariants: no clock, randomness, or I/O inside the derivation engine; identical inputs always yield identical token IDs, identical audit hashes, and bit-identical bundle bytes; unit mismatch always fails closed.

### Phase 03 — Op catalog remainder

Remaining ops: `calc.multiply`, `calc.divide`, `filter.sum_where`, `select.passthrough`, `round.half_even`.

Unit algebra: `multiply` follows `unit × "" → unit` and `unit_a × unit_b → error`; `divide` follows `unit / unit → ""` and `unit / "" → unit`.

### Phase 04 — Run lifecycle: long-lived bundle file

Long-lived `Run` backed by a single append-only bundle file on disk. `run start`, sequential and parallel op appends, `run finish`. OS-level file lock refuses two concurrent writers. Atomic append via POSIX advisory lock and atomic rename. Mid-run crashes leave no corrupt chain; recovery path discards partial writes.

### Phase 05 — CLI surface and ungrounded-number gate

CLI verbs: `run start`, `run finish`, `run fail`, `read-mint`, `calc`, `submit-answer`, `verify`, `replay`.

The ungrounded-number gate is structural: `submit-answer` accepts only a token ID registered in the current run. The request struct has no `value` field. `DisallowUnknownFields` rejects anything else. Literal numbers cannot be submitted.

Introduces `EntryKindAnswerSubmitted`.

### Phase 06 — Named-source mapping and flat JSON audit pack

Named-source registry that maps logical source names (e.g. `ledger.get_balance`) to concrete reader, path, and selector tuples. Source name recorded in `CapturedRead` and the `token_mint` audit payload.

Flat single-file `audit.pack.json` format as an alternative to the existing ZIP artifact. CLI verb: `provenant pack`. Existing ZIP path is unchanged.

### Phase 07 — Trace-back tool

Given a finalized bundle and a token ID, reconstructs the full contribution graph back to leaf tokens. Per leaf: file path, byte/row span, source SHA-256, canonical leaf value, numeric contribution.

For additive ops, per-leaf contributions sum exactly to the queried token's value.

New Go package `kernel/public/trace`. CLI verb: `provenant trace`.

### Phase 08 — End-to-end fauxbooks demo and golden replay test

Deterministic synthetic accounting dataset (three CSV files: `gl.csv`, `payroll.csv`, `revenue.csv`). Full sequence: mint leaf tokens, derive Operating Income via `calc.subtract`, submit answer, finalize, verify, replay, trace.

Golden files committed: `golden.bundle.json`, `golden.trace.json`. A Go test in `examples/derivation-graph/e2e_test.go` re-runs the sequence and asserts byte-equality with both goldens. Pre-existing bundles minted before Phase 01 still pass `bundle.Verify` and `bundle.Replay`.

### Phase 09 — Scaled demo and HTML audit report

Scaled dataset (~100 employees, ~20 000–30 000 total rows). A second sibling dataset with deliberate breakages surfaces two distinct reconciliation failures.

New package `audit/report/` exporting `RenderHTML(*pack.Document) ([]byte, error)`: a self-contained, deterministic, golden-testable HTML renderer. No JavaScript; no external assets; opens from disk without network access.

CLI verb: `provenant audit-html`. Consumes an already-produced audit pack JSON file. No new kernel ops or schema changes.

---

## Reconciliation harness extension

A deterministic, config-driven reconciliation harness layer on top of the core kernel surface. The harness accepts multiple structured files with explicit column roles and reconciliation mode configuration, routes each file to the correct kernel reader, mints evidence tokens, runs deterministic reconciliation, and emits a self-contained report package.

Supported reconciliation families:

1. **Exact match** — join records by configured key fields; report matched, missing, duplicate, and extra records.
2. **Amount tolerance** — compare numeric fields with explicit tolerance and currency rules.
3. **Existence and completeness** — prove every required record in one file exists in another.
4. **One-to-many / many-to-one** — compare a parent record to a group of child records, with summed amount checks.
5. **Schema and value validation** — validate required columns, duplicate keys, empty keys, amount/date/currency parsing, and allowed-value constraints before matching.

The reconciliation engine lives outside the kernel. The kernel remains responsible for evidence capture, token minting, audit determinism, and bundle verification.

Output package: `audit-report.zip` containing `report.md`, `report.json`, `mismatches.csv`, `captured_reads.json`, `tokens.json`, `audit.jsonl`, `bundle.json`, `manifest.json`.

Non-goals for this extension: LLM inference, fuzzy matching, automatic column guessing, PDF/OCR input, runtime database or persistent service, kernel expansion beyond reader/mint/bundle calls.

---

## Deferred and out of scope for v0

The following are documented trade-offs or explicit deferrals. They will not be reopened without a new plan or ADR.

- **External notary for audit chain.** Tamper-evidence in v0 is internal hash-chain detection only. No external timestamping authority or signed attestation. See `docs/KNOWN_TRADE_OFFS.md`.
- **Streaming model responses.** The harness forces `stream=false` at the `ModelClient` layer. Streaming and cross-turn sibling merging are deferred. See `docs/KNOWN_TRADE_OFFS.md`.
- **Live progress during parallel blocks.** The audit chain remains buffered during parallel execution. Progress is surfaced via OTLP `parallel_block` trace spans and `provenant run inspect`. See `docs/KNOWN_TRADE_OFFS.md`.
- **Cryptographic host conformance attestation.** The `host_conformance_attestation` field is self-declared in v0. Hosted-service or hardware attestation is deferred to v0.5+.
- **Locale-specific numeric normalization in the kernel.** Forms like `1,5` or `1 234 567,89` surface as `numeric.parse_error`. Per-backend normalization via the `normalizers/` skill-pack slot is the documented mitigation. See `docs/KNOWN_TRADE_OFFS.md`.
- **Cross-deployment fresh-run equivalence.** The determinism contract is captured-deterministic replay, not cross-deployment equivalence when upstream sources change between runs.
- **Custom validators and sandboxed code loading.** v0 ships built-in validators only. Skill authors who need a custom validator must upstream it into the kernel's built-in set. WASM loading is the v0.5+ extension point.
- **Non-Go embeddings.** Go is the v0 kernel language. Non-Go hosts reach the kernel through the runtime/sidecar shape. A non-Go binding (FFI or sub-50 ms loopback sidecar) is a post-v0 deliverable.
- **Multi-tenant SaaS.** v0 ships single-tenant and air-gapped. Multi-tenant hosted service is deferred to v0.5.
- **Cross-run token sharing within a tenant.** v0 rejects cross-run token references.
- **Unsigned local skill packs.** v0 ships unsigned local packs. Pack signing and admin-approval supply chain is deferred to v0.5.

---

## Definition of done (whole derivation-graph feature)

- The Phase 08 fauxbooks demo passes under `go test ./...` and `go vet ./...`.
- Given any derived token ID in the demo's finished bundle, `provenant trace` reconstructs the full contribution graph back to leaves, with file path, byte/row span, source SHA-256, and per-leaf amount.
- The Phase 08 golden test re-runs the demo, replays the resulting bundle, and asserts bit-identical bundle bytes.
- Pre-existing bundles minted before Phase 01 still pass `bundle.Verify` and `bundle.Replay` without modification.
- `provenant submit-answer` cannot accept a literal number; only a registered token ID is allowed, enforced by the CLI struct shape and exercised by a failure-fixture test.
- The op catalog contains exactly the eight ops described above. No new ops are added without an explicit follow-up plan.
