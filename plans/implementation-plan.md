# Implementation Plan

This plan sequences Provenant from empty repository to v0 — the scope defined in [`architecture/README.md`](../architecture/README.md#v0-scope-summary) plus the dual-mode pluggability requirement stated in [`thesis.md`](../docs/thesis.md) and [`architecture/OVERVIEW.md`](../architecture/OVERVIEW.md). Each phase ends with a working, testable artifact and a falsifiable "done when" criterion. Phases are sequential; later phases assume earlier-phase artifacts exist and pass.

Decisions and rationale belong in [ADRs](../architecture/adrs/). This document is a plan, not a justification.

## Phase 0 — Establish the kernel module boundary

**Goal.** Carve out the kernel as a pure-Go library with no I/O at the public API surface, and **pin the load-bearing constants and headers** that downstream phases will hash into the audit chain. This is the mechanical foundation of the dual-mode pluggability promise — "ratify-and-pin" the immovable surface so Phases 1+ can build on stable bytes. Everything downstream depends on the boundary being right.

**Tasks.**

- Create the repository layout: `kernel/` (pure library), `runtime/` (transport + process lifecycle), `adapters/` (host-side shims), `tools/` (tool backends), `internal/` (shared non-public helpers).
- Define kernel-public types: `Token`, `Source`, `Derivation`, `ToolDef`, `ToolCall`, `Claim`, `Payload`, `ValidationError`, `AuditEntry`, `EntryKind`, `Run`, run status enum.
- Define kernel-public interfaces (every I/O dependency): `Store`, `BlobStore`, `ModelClient`, `Clock`, `RandSource`, `AuditSink`, `ToolBackend`, `TransactionalScope` (joint-transaction primitive across `Store` and `AuditSink` — required so the parallel-block atomicity invariant in [`subsystems/audit.md`](../architecture/subsystems/audit.md#parallel-sibling-deterministic-order) survives separately-injected backends).
- Define kernel-public pure functions (skeletons; bodies filled in later phases): `Mint`, `VerifyToken`, `HashEntry` (canonical-JSON SHA-256), schema-validation helpers, numeric extractor.
- **Ship the pinned kernel-internal constants and header.** Phase 0 is the "ratify-and-pin" phase; this task is **not** deferred. Specifically: ship `kernel/internal/promptkernel/header_v1.go` (the kernel-immutable header text whose sha256 is the `kernel_header_hash` recorded on every `EntryKindRunStarted`), the `EMPTY_CJSON_SHA256` pinned constant in the kernel package, and the per-`kernel_version` fixture files under `kernel/testdata/header-fixtures/` that the release-gate test consumes.
- Provide an in-memory `Store`, `BlobStore`, `AuditSink` in `kernel/internal/memstore/` for tests.
- Add a `go vet`-style check (custom or `depguard`) that fails the build if any file under `kernel/...` imports `net/*`, `net/http`, `database/sql`, `github.com/jackc/pgx/...`, `os/exec`, or any MCP/HTTP/CLI package.
- Set up CI: `go test ./kernel/...` with mocked I/O; lint; the import-guard check.

**Done when.**

- `go build ./kernel/...` succeeds.
- `go test ./kernel/...` passes with the in-memory implementations of every injected interface.
- The import-guard check passes; no `net/*` or `database/sql` import leaks through `kernel/`'s public API or any package transitively imported by it.
- **TransactionalScope boot-time enforcement test passes.** A unit test constructs a kernel boot configuration that injects an in-memory `Store` whose `TransactionalSubstrate()` marker is `"memstore-A"` and an in-memory `AuditSink` whose marker is `"memstore-B"` and asserts the kernel refuses to initialize with a typed `kernel_integrity` error. A second test injects matched markers and a `TransactionalScope` that commits the two halves in separate transactions; the kernel must initialize, but the first `kernel.SubmitAnswer` or first parallel-block flush detects the violation via the tx-local canary write and fails closed.
- **`EMPTY_CJSON_SHA256` pinned-constant test passes.** A unit test in the kernel package asserts byte-identity between `sha256(0x7B, 0x7D)` and the pinned hex constant `44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a` (see [`architecture/subsystems/audit.md`](../architecture/subsystems/audit.md#host_conformance_attestation)). A canonicalizer regression that emitted whitespace or a BOM would fail this test at build time rather than silently changing the chain on Mode A runs.
- **Kernel-header release-gate test passes.** A release-gate test under `kernel/testdata/header-fixtures/` enumerates each `kernel_version` and asserts that `sha256(kernel/internal/promptkernel/header_v<N>.go's exported text bytes)` equals the version-pinned hex in the fixture. A drift between the header file and the fixture (e.g., a contributor edits the header text without bumping `kernel_version`) fails the release.
- **Write-time enumeration-gate negative tests pass.** Three negative-tests over fixture entries assert that `AuditSink.Append` rejects entries whose deterministic enumeration fields fall outside their published const blocks, with a typed `kernel_integrity` error and **no row written**:
  1. An `EntryKindRunFailed` whose `failure_subcode = "unknown_subcode"` (a value not in the §"`run_failed.failure_subcode` enum" table of [`subsystems/audit.md`](../architecture/subsystems/audit.md#run_failedfailure_subcode-enum)) is rejected.
  2. An `AuditEntry` whose `kind` is a string outside the published `EntryKind` const block (e.g., `"made_up_kind"`) is rejected — the §"Entry kinds" rule "an entry whose `kind` is not in the enum is rejected at write time, not silently dropped" is exercised by an actual test.
  3. An `EntryKindToolCallFailed` whose `error_code` is outside the canonical `error_code` table in [`subsystems/audit.md`](../architecture/subsystems/audit.md#canonical-error_code-table) (e.g., `"made.up.code"`, or the non-conforming snake_case `"credential_id_manifest_mismatch"`) is rejected.
  All three gates run over the in-memory `AuditSink` from `kernel/internal/memstore/` and assert the rejection is observable via the public kernel surface (typed error returned, no row appended). The kernel boots its enumeration validators from a single generated Go const block (`kernel/public/errorcode/codes.go`, kernel-public `EntryKind` and `FailureSubcode` slices) whose contents are CI-diff-checked against the canonical tables; a Phase 5 binding that hard-codes a different set fails the cross-binding byte-identity test by construction, but the Phase 0 gates catch the same drift inside the Go reference at build time.
- **Markdown-canonical enum generator (`tools/enumgen/`) is hermetic and drift-gated.** `go generate ./kernel/public/...` is hermetic (no internet, no module fetch in this step — the generator reads only `architecture/subsystems/audit.md` and `architecture/subsystems/skills.md` from the repository tree) and `git diff --exit-code` after running it returns 0 on a clean checkout. The generator under `tools/enumgen/` parses four anchor-pinned tables and emits one Go file per anchor, per the cycle-10 enumgen anchor convention pinned in [`architecture/subsystems/audit.md`](../architecture/subsystems/audit.md) §"Drift-prevention mechanism":

  - `architecture/subsystems/audit.md → ### Canonical \`error_code\` table → kernel/public/errorcode/codes.go`
  - `architecture/subsystems/audit.md → ### Per-EntryKind payload schema (deterministic-surface enumeration) → kernel/public/audit/entrykind.go`
  - `architecture/subsystems/audit.md → ### \`run_failed.failure_subcode\` enum → kernel/public/audit/failuresubcode.go`
  - `architecture/subsystems/skills.md → ### v0 built-in normalizer set → kernel/public/normalizers/registry.go` A unit test in `tools/enumgen/enumgen_test.go` parses fixture markdown slices under `tools/enumgen/testdata/` and asserts the generated Go matches a golden fixture under `tools/enumgen/testdata/golden/`. CI runs `go generate ./kernel/public/...` followed by `git diff --exit-code`; any markdown↔Go drift (table edited without regenerating, or generated file hand-edited) fails the build. The markdown tables are the **single source of truth**; the Go const blocks are **derived**.
- **Kernel public-surface presence + projection round-trip test passes.** All kernel public types listed in [`architecture/subsystems/kernel.md`](../architecture/subsystems/kernel.md#kernel-public-types) (`Token`, `Source`, `Derivation`, `ToolDef`, `ToolCall`, `Claim`, `Payload`, `ValidationError`, `AuditEntry`, `PayloadEnvelope`, `EntryKind`, `Run`, run status enum, `CredentialRef`, `Store`, `BlobStore`, `ModelClient`, `Clock`, `RandSource`, `AuditSink`, `ToolBackend`, `TransactionalScope`, `Tx`, `NormalizerRegistry`) are present under `kernel/public/` with godoc comments. A **snapshot test** under `kernel/public/snapshot_test.go` walks the published list (a const-string slice maintained alongside kernel.md) and asserts via reflection that each named type exists, is exported, and has a non-empty godoc; a contributor who adds or removes a public type without updating the published list fails the test at build time. Separately, a **projection round-trip test** loads a fixture set covering every `EntryKind` listed in [`audit.md`](../architecture/subsystems/audit.md#entry-kinds), constructs an `AuditEntry` for each, projects it onto the three-column persistence shape (`payload_deterministic`, `payload_non_deterministic`, `hash_input_canonical`), reads it back, and asserts the reconstructed `AuditEntry` is byte-identical to the original on every field (per [`kernel.md`](../architecture/subsystems/kernel.md#auditentry-in-memory-shape-and-three-column-persistence-projection)). This closes the cycle-7 gap where Phase 0 done-when was only `go build` — silently allowing Phase 1 to discover the public surface or the projection rule was incomplete.
- A short `architecture/adrs/0009-kernel-public-surface.md` (or equivalent) ratifies the import-guard list. (If the open question about Go-as-kernel from [`architecture/OVERVIEW.md`](../architecture/OVERVIEW.md) §2.1 resolves to "switch language", this phase restarts; that is acceptable cost.)

**Deferred from this phase.** Postgres-backed `Store`. S3-backed `BlobStore`. Real `ModelClient`. Tool implementations. (The kernel-internal pinned constants and `header_v1.go` ship in Phase 0 per the "ratify-and-pin" goal above; they are explicitly **not** deferred. The cycle-5 `Deferred` line "Audit chain hash format finalization beyond the SHA-256-over-canonical-JSON contract" is retracted in cycle 6 as it contradicted the Phase 0 done-when items (d) `EMPTY_CJSON_SHA256` test and (e) header release-gate hash test, which require the format to be pinned in Phase 0.)

## Phase 1 — Storage and hash-chained audit log

**Goal.** Persist runs, tokens, tool calls, and audit entries to Postgres. Build the hash chain. Make replay possible.

**Tasks.**

- Implement the Postgres-backed `Store` (`runtime/store/pg/`) using `pgx` + `sqlc`. Implements every kernel `Store` method.
- Implement the S3-backed `BlobStore` (`runtime/blobstore/s3/`) with the in-memory `BlobStore` retained for tests.
- Implement the Postgres-backed `AuditSink` with the per-tenant monotonic sequence and `prev_hash`/`hash` columns. The chain construction logic lives in the kernel as a pure function; the sink writes the rows the kernel produces.
- Run atlas migrations for the v0 schema described in [`subsystems/storage.md`](../architecture/subsystems/storage.md): `runs`, `tokens`, `tool_calls`, `captured_reads`, `guard_runs`, `audit_entries`, `cases`, `case_metrics`, `skills`, `tools_registry`, `hitl_reviews`, `tenants`. The initial migration includes the cycle 3–5 schema deltas: `tokens.source_value_raw`, `tokens.normalizer_applied`, `runs.credential_identity_baseline`, `runs.kernel_header_hash`, and the `audit_entries.payload_deterministic` / `payload_non_deterministic` / `hash_input_canonical` three-column split (see [`subsystems/storage.md`](../architecture/subsystems/storage.md#schema-deltas-required-by-cycles-35-load-bearing-columns)).
- Implement `provenant migrate up` (atlas wrapper) as a CLI subcommand stub.
- Implement audit chain verification: given a `run_id` or `tenant_id`, walk entries and verify each `hash == HashEntry(prev_hash, payload, ...)`.
- Implement audit replay: read all entries for a run in order, reconstruct the run state.

**Done when.**

- An integration test starts Postgres (testcontainers or docker-compose), inserts a synthetic run with 50 tokens, 20 tool calls, and 100 audit entries; the test then verifies the chain and replays the entries to reconstruct the run state byte-identically.
- Killing the test process mid-write and re-running the verifier flags the truncated chain.
- A property test mutates a random audit row and confirms the verifier detects the tamper. The same property test mutates a random field inside `payload_non_deterministic` and confirms the verifier does **not** flag it (the non-deterministic column is excluded from the hash input); a mutation of `payload_deterministic` or `hash_input_canonical` is flagged.
- **Orphan-run detection (unit tests) is implemented.** The detection function — given a run row, a stub `skills` registry, and an injected clock — returns `true` iff the run is in `running` or `pending` status and its last audit entry is older than `skill.bounds.max_wall_clock_seconds × skill.bounds.orphan_multiplier` (default `orphan_multiplier = 2`, **pinned in the skill manifest and not deployment-overridable** — see [`subsystems/harness.md`](../architecture/subsystems/harness.md#orphan-run-threshold-per-skill) and [`subsystems/skills.md`](../architecture/subsystems/skills.md)). Phase 1 ships unit tests against a fake skill registry (test-supplied `skill_id` stub, test-injected `max_wall_clock_seconds`) covering the short-budget-vs-long-budget threshold case and the audit-chain-intact-on-detection property. The kernel-public `EntryKindRunOrphanRecovered` payload schema is in place. The **end-to-end orphan-recovery integration test** — that uses the harness's real `skill_id` / `skill_version` on persisted runs and verifies recovery across a forced mid-write kill — is in Phase 3's done-when (it requires the harness, the skill manifest loader, and persisted `skill_id`/`skill_version` on the run row, all Phase 3 artifacts).

**Deferred from this phase.** Multi-tenant SaaS isolation modes beyond the schema (row-level security wiring is v0; SaaS-grade tenant onboarding is v0.5). Cross-region object-store replication.

## Phase 2 — Tool registry and guard pipeline (pre + post)

**Goal.** A tool can be registered, dispatched with schema validation, mint tokens, and have its output flow through pre-LLM guards. The post-LLM validator pipeline runs end-to-end against payloads without a model in the loop.

**Tasks.**

- Implement the tool registry: typed schema registration, `mintable_paths`, version pinning. Backends register via the kernel `ToolBackend` interface. See [`subsystems/tools.md`](../architecture/subsystems/tools.md).
- Implement audit middleware around every tool call: `tool_call_started`, `tool_call_completed` entries; full input + output captured (large payloads to blob store with pointer in the entry).
- Implement token minting from `mintable_paths`. Unit-incompatible derivations rejected.
- Implement the pre-LLM guard pipeline ([`subsystems/guards.md`](../architecture/subsystems/guards.md#pre-llm-input-guards)): `pii.redact`, `injection.scan`, `injection.heuristic`, `source.authenticate`, `length.cap`. Severity tiers L1/L2/L3 honored. Every execution writes a `guard_runs` row.
- Implement the post-LLM validator pipeline: `schema.output`, `arith.crossfoot`, `arith.tie`, `arith.sign`, `arith.balance`, `policy.coa_membership`, `policy.citation_exists`, `confidence.extraction_min`, `currency.consistent`.
- Implement `kernel.SubmitAnswer`: numeric extraction, per-claim token grounding via `kernel.VerifyToken`, validator pipeline (collect-all and fail-fast modes), persistence on pass.
- Implement at least the pure-Go tool backends end-to-end: `reconcile.match`, `calc.subtract`, `calc.sum`, `calc.percent_change`, `calc.variance`. These need no external systems and can be the first end-to-end tools.
- **Implement the `match` tool category with the pgvector reference backend.** Register `match.pair`, `match.candidates`, `match.cross_set` per [`architecture/tools/match/README.md`](../architecture/tools/match/README.md). Wire the pgvector backend per [`architecture/tools/match/backends/pgvector.md`](../architecture/tools/match/backends/pgvector.md) (operator-configured embedding model — provenant does not bundle one; same precedent as `policy`'s pgvector backend). `sqlite-vss` and `in-process-onnx` backends are listed as v0.5 deferred; no implementation in v0.
- **Implement the `dedupe` tool category on top of `match`.** Register `dedupe.cluster` per [`architecture/tools/dedupe/README.md`](../architecture/tools/dedupe/README.md). Wire the HITL routing path so that `decision = review_required` clusters auto-route through the cycle-5/6 canonical `/v1/hitl/...` surface (the routing surface itself ships in Phase 4; in Phase 2 the dedupe tool emits the routing intent and the `dedupe.review_required` validator fails closed with `dedupe.review_required_no_hitl` when HITL is not yet wired).
- **Register the v0 built-in normalizer set in the kernel's normalizer registry.** Per the cycle-9 amendment in [`architecture/subsystems/skills.md`](../architecture/subsystems/skills.md#pack-level-normalizers-normalizers) and [ADR-0006](../architecture/adrs/0006-skill-pack-format.md), the kernel ships the five normalizers `numeric.locale.eu_comma_decimal`, `numeric.thousands_separator.nbsp`, `currency.iso4217`, `date.iso8601_lenient`, `entity_name.basic` registered at compile time. Each ships with a fixture-input → canonical-output unit test.
- **Register the new validators in the validator pipeline.** `match.confidence_min`, `match.similarity_min`, `match.method_allowed`, `dedupe.cluster_size_max`, `dedupe.review_required` — declared as built-in validators per [`architecture/subsystems/guards.md`](../architecture/subsystems/guards.md) "Built-in validators (v0)" and the `match.*` / `dedupe.*` `error_code` namespaces in the canonical `error_code` table in [`architecture/subsystems/audit.md`](../architecture/subsystems/audit.md#canonical-error_code-table).
- **Implement the pack-load-time `credential_id` ↔ `ToolBackend.Credentials()` reconciliation step** (see [`subsystems/skills.md`](../architecture/subsystems/skills.md#loading-lifecycle) step 3). The loader binds each `tools.yaml` entry to its registered backend and rejects packs whose `credential_id` declaration disagrees with the backend's `Credentials()` accessor: empty `Credentials()` with declared `credential_id` → `skill.credential_id_unexpected`; non-empty `Credentials()` with missing `credential_id` → `skill.credential_id_missing`; mismatched ID → `credential.id_manifest_mismatch`. The check is part of the Phase 2 pack-loader code path (alongside the `skill.normalizer_id_unknown` rejection that already ships here).

**Done when.**

- An integration test feeds a recorded reconciliation tool-call sequence (no model) into the kernel; every tool call mints tokens correctly; every guard runs; a final synthetic `submit_answer` payload is verified end-to-end. The resulting trace is fully typed and schema-validated.
- Injecting an ungrounded number into the synthetic answer is rejected by token grounding.
- Injecting a prompt-injection pattern into a synthetic tool output triggers L2 quarantine.
- All validators have unit tests; the pipeline has integration tests.
- **`credential_id` reconciliation fixture-pack rejection tests pass.** Two fixture packs exercise the load-time iff-rule: (a) a pack whose `tools.yaml` omits `credential_id` for a tool whose registered backend's `Credentials()` returns non-empty — must be rejected at load with `skill.credential_id_missing`; (b) a pack whose `tools.yaml` declares `credential_id` for a pure-Go tool whose backend's `Credentials()` returns empty — must be rejected with `skill.credential_id_unexpected`. A third fixture (c) declares `credential_id: ledger.WRONG` against a backend whose `Credentials()` returns `ledger.sap_eu` — must be rejected with `credential.id_manifest_mismatch`. All three error codes resolve through the canonical `error_code` table in [`subsystems/audit.md`](../architecture/subsystems/audit.md#canonical-error_code-table); the loader emits no entry on the chain (the run never starts).
- **Phase 2 dedupe HITL fail-closed negative test.** A `review_required` cluster routed through `dedupe` with no HITL surface available produces the typed `dedupe.review_required_no_hitl` error at submit-answer time; the validator fails closed and the run is rejected via the existing `submit_rejected` path. Phase 2 does **not** ship a synthetic in-process HITL stub; the end-to-end positive HITL path is Phase 4.
- **The five built-in normalizers each have a fixture-input → canonical-output unit test.** The fixtures cover at minimum: `numeric.locale.eu_comma_decimal` (`1.234,56` → `1234.56`); `numeric.thousands_separator.nbsp` (a string with NBSP, narrow NBSP, thin space, and regular space separators all collapsing); `currency.iso4217` (symbol, country-code-prefixed variant, and ISO code all converging on the same code); `date.iso8601_lenient` (an ISO 8601 input, a `DD/MM/YYYY` input with `date_order: dmy`, and an undated/no-timezone input that defaults to UTC with `tz_defaulted = true` in the non-deterministic sub-object); `entity_name.basic` (`ACME Corp.` → `acme`, including the legal-form-suffix-list strip and the Unicode NFC + case-fold + whitespace-collapse path).
- **The `tools/enumgen/`-generated `kernel/public/normalizers/registry.go` matches the markdown listing.** `go generate ./kernel/public/...` re-runs the generator over the cycle-9 v0 built-in normalizer set table in [`architecture/subsystems/skills.md`](../architecture/subsystems/skills.md#pack-level-normalizers-normalizers); CI's `git diff --exit-code` after `go generate` returns 0. A drift (a contributor adds a normalizer to the markdown without regenerating, or hand-edits the generated file) fails the build.
- **A pure-kernel micro-benchmark** measures `kernel.SubmitAnswer` p95 overhead on a representative recon payload with mocked I/O. The §7.3 budget is "50 ms on top of validator costs" — a budget on kernel-orchestration overhead, not on the whole pipeline. The benchmark therefore reports two numbers and compares each to the correctly-scoped budget:
  - `t_kernel_orchestration` = total `SubmitAnswer` wall time **minus** the summed wall time of the individual validator and per-claim token-grounding calls (each validator and `VerifyToken` is instrumented to emit its own duration; orchestration is the residual: numeric extraction, dispatch glue, claim/token mapping, audit append, pipeline mode handling). This is the figure compared against the 50 ms §7.3 budget.
  - `t_total_with_validators` = total `SubmitAnswer` wall time including validator and token-grounding costs. This is recorded as the kernel's intrinsic floor for Phase 4's transport-overhead measurement to subtract from end-to-end latency.
  Both numbers are persisted to a bench-results JSON file that Phase 4 reads. If `t_kernel_orchestration` exceeds 50 ms, Phase 3 does not start until the kernel orchestration path is optimized or the budget is renegotiated in an ADR; validator cost overruns are a separate ADR concern (per-validator budgets live in [`subsystems/guards.md`](../architecture/subsystems/guards.md)). This separation prevents the cycle-2 critic #8 failure mode where Phase 2 reports "under budget" by conflating orchestration with validator costs, leaving Phase 4 no slack for MCP overhead.

**Deferred from this phase.** External-system tool backends (Textract, OFAC, OpenFX) wire-up; that's Phase 3's responsibility. BoN-MAV mode (validator pipeline is wired for it but the harness re-prompt logic ships in v0.5). Custom per-skill numeric extractors.

## Phase 3 — Harness (Mode A) with one model class

**Goal.** The CLI runs a reconciliation end-to-end against a real model (Claude), with one signed skill pack and the v0 tool backend set. Full audit + tokens + guard outcomes persisted.

**Tasks.**

- Implement the kernel harness (Mode A) per [`subsystems/harness.md`](../architecture/subsystems/harness.md): prompt assembly with immutable kernel header, model call loop, parallel tool dispatch, bound enforcement (`max_steps`, `max_wall_clock`, `max_cost`).
- Implement the model abstraction interface and the Claude implementation (`runtime/model/anthropic/`).
- Implement the v0 skill pack format ([`subsystems/skills.md`](../architecture/subsystems/skills.md), [ADR-0006](../architecture/adrs/0006-skill-pack-format.md)) and ship the reconciliation skill pack as a local directory.
- Wire up the external tool backends required by the recon skill: SQL ledger, Textract extract, keyword policy, OpenFX external. Each follows the conformance template in [`architecture/tools/README.md`](../architecture/tools/README.md).
- Implement `provenant run`, `provenant replay`, `provenant cases`, `provenant audit show`, `provenant audit verify`, `provenant audit pack`, `provenant skill list/show/install`, `provenant tools list/show`.
- Implement cost and latency tracking; update `runs.cost_usd`, `runs.tokens_in`, `runs.tokens_out`.

**Done when.**

- `provenant run recon --input ./fixtures/may-2026-recon.json --model claude-opus-4-7 --tenant acme` completes end-to-end with a passing `submit_answer` for a known-good fixture.
- `provenant audit verify <run_id>` returns OK; `provenant audit pack <run_id>` produces a bundle containing the audit chain, the token graph, the guard outcomes, and every prompt+response.
- `provenant replay <run_id>` reconstructs the deterministic portion (tool calls, validator outcomes, audit chain) byte-identically across two consecutive replays.
- **ModelClient stream-refusal test passes.** A unit test injects a `ModelClient` that advertises streaming capability and attempts to deliver tool_use blocks progressively on a compliance-path run. The harness must either coerce `stream=false` at the abstraction layer (the documented mitigation in KNOWN_TRADE_OFFS.md) or reject the client at run start with a typed `harness.streaming_required` error; the test asserts that no run path produces a streamed tool_use response. A second test injects a `ModelClient` that emits parallel tool_use blocks split across two consecutive response turns and asserts that each turn forms an **independent** `sibling_group_id` (no cross-turn merging), the resulting audit chain is byte-identical across two replays of the same run, and each turn's `parent_step_id` derives from its own `step_index` per [`subsystems/audit.md`](../architecture/subsystems/audit.md#parent_step_id-construction). These two tests close the cycle-4 critic #4 gap where the streaming-disabled and split-turn-independence claims existed only on paper.
- **Silent-credential-rotation replay byte-identity test.** A fixture run injects a deterministic silent credential rotation at a fixed `step_index` (a test-only secret-store stub flips the credential's stable-identifier hash on the Nth tool call without surfacing an authentication failure). The harness must emit exactly one `EntryKindCredentialIdentityChanged` entry at that step and abort the run with `failure_class = aborted`, `failure_subcode = credential_identity_changed` (per [`cross-cutting/security.md`](../architecture/cross-cutting/security.md#silent-credential-rotation-detection-per-run-identity-baseline)). The test replays the same fixture (same input bytes, same deterministic secret-store stub) twice and asserts that the `EntryKindCredentialIdentityChanged` entry is **byte-identical** across the two replays — including `credential_identifier_hash_baseline`, `credential_identifier_hash_observed`, `at_step_index_be8`, and the entry's `hash` and `prev_hash`. This closes the cycle-5 outstanding gap where the entry's determinism was specified but no test in the plan asserted it.
- **Orphan-run recovery end-to-end test passes.** A test forcibly kills the `provenant-server` process mid-write across two concurrent runs (one short-budget skill, one long-budget skill — both real Phase-3 skill packs with manifests loaded by the harness and `skill_id` / `skill_version` persisted on the run row). On restart, the orphan sweep — using the persisted `(skill_id, skill_version)` to look up `bounds.max_wall_clock_seconds × bounds.orphan_multiplier` from the `skills` table per the Phase 1 detection rule — marks only the short-budget run past its threshold as `failed: orphaned` and emits exactly one `EntryKindRunOrphanRecovered` with `threshold_seconds_used` recorded. The long-budget run remains `running` until its own threshold elapses. Both audit chains pass `audit verify` after recovery.
- **Harness-overhead isolation benchmark.** The CLI harness records, per run, the time spent in: (a) model API calls, (b) tool backends, (c) kernel pipeline (validators + token grounding + audit append), (d) harness orchestration (prompt assembly + scheduling). The Phase 3 done criterion publishes (c) + (d) — the Provenant-attributable overhead — as a baseline number. Phase 4's MCP-overhead measurement subtracts this baseline so transport cost is not conflated with harness cost.

**Deferred from this phase.** Other model classes (OpenAI, Azure, local). Other skill packs. The runtime layer's MCP and HTTP surfacing (Phase 4). Streaming model responses.

## Phase 4 — Runtime layer: MCP server, HTTP API, and CLI parity

**Goal.** The same case from Phase 3 runs identically via MCP and via HTTP. The runtime layer is a thin shell around kernel calls.

**Tasks.**

- Implement the MCP server ([`adapters/mcp.md`](../architecture/adapters/mcp.md)) using the official Go MCP SDK. Expose every Provenant tool as an MCP tool; every skill prompt as an MCP prompt; audit and case data as MCP resources. Bearer-token auth, tenant scoping, skill scoping.
- Implement the HTTP API ([`adapters/http.md`](../architecture/adapters/http.md)) covering the same surface as the MCP server: tool invocation, prompt retrieval, run inspection, audit retrieval. RESTful; same auth shape as MCP.
- Wire up the runtime process: configuration loading, signal handling, graceful shutdown, observability (Prometheus metrics, OTel traces, JSON logs per [`cross-cutting/observability.md`](../architecture/cross-cutting/observability.md)).
- Ship the `provenant-server` binary (the runtime daemon) alongside the existing `provenant` CLI.
- Add the `provenant-mcp-client` stdio-to-HTTP wrapper used by Claude Code et al.
- **Ship the v0 HITL routing surface** (the web UI is deferred to v0.5; the routing itself is v0 scope per [`subsystems/hitl.md`](../architecture/subsystems/hitl.md)). This includes: the `hitl_reviews` state machine (`pending` → `approved` | `edited` | `rejected` | `escalated` | `expired`), the canonical REST surface under `/v1/hitl/...` (`POST /v1/hitl/reviews/{review_id}/decision`, `POST /v1/runs/{run_id}/hitl/decision` (convenience alias), `GET /v1/hitl/pending`, `GET /v1/hitl/reviews/{review_id}`, `POST /v1/hitl/reviews/{review_id}/claim`, `POST /v1/hitl/bulk/approve`, `GET /v1/hitl/reviews/{review_id}/audit` — see [`subsystems/hitl.md`](../architecture/subsystems/hitl.md#rest-api-v0)), the corresponding MCP tools, the kernel-side state transition logic with audit-log entries (`EntryKindHITLDecision.decision` covers all five enum values, with `edits_canonical` populated when `decision = edited`), and the harness path that moves a run from `awaiting_review` back to `running` (on approval / edited / escalated as configured) or `failed` (on rejection or expiry as configured) when a decision arrives.

**Done when.**

- A Claude Code instance configured against the Provenant MCP server completes the same recon fixture from Phase 3. The audit chain's deterministic portion is byte-identical to the Phase 3 CLI run on the same input. (LLM-portion variance is allowed and measured.)
- The same fixture run via HTTP API directly produces the same byte-identical deterministic-portion audit chain.
- The same fixture run via MCP and via HTTP — neither compared against the CLI — produces byte-identical deterministic-portion chains. ADR-0010 requires all three pairs (MCP-vs-CLI, HTTP-vs-CLI, MCP-vs-HTTP) to be byte-identical; transitivity through CLI is not accepted as a substitute, because a bug that affects MCP and HTTP in the same way (e.g., a shared runtime-layer canonicalization shim that differs from the CLI's direct kernel path) would pass the two CLI-pairwise tests and silently fail this third one.
- Cross-tenant access attempts are rejected. Invalid bearer tokens are rejected. MCP latency overhead is below the 50ms p95 budget on top of tool execution.
- An integration test escalates a run to `awaiting_review`, posts a HITL decision via the REST endpoint, and verifies the run resumes (or fails) deterministically; an audit-chain entry records the decision.
- **Messy-ledger end-to-end fixture pack runs through dedupe with the full HITL positive path.** A fixture skill pack with a two-ledger input (same vendor master appearing under multiple name / locale variants on the two sides) is run through `dedupe.cluster` and produces at least one `review_required` cluster; that cluster is routed via the canonical `/v1/hitl/...` REST endpoints; a reviewer decision is recorded as `EntryKindHITLDecision`; and the run completes with the cluster's representative token consumed downstream by reconcile. Cluster tokens reference back to source tokens via the `dedupe.cluster` output's `representative_token_id` / `member_token_ids` / `pairwise_evidence_token_ids` fields, and each cluster's `min_pairwise_confidence` token verifies against `kernel.VerifyToken`.

**Deferred from this phase.** LangChain and OpenAI Agents adapters (v0.5). Per-tenant Grafana dashboards. Per-tenant rate limits beyond the global cap. Web HITL UI.

## Phase 5 — Code-level embedding path (non-Go binding)

**Goal.** Demonstrate the code-level pluggability promise. A host application written in a language other than Go embeds Provenant — either through FFI or through a strict-latency sidecar protocol — and produces an audit chain byte-identical to a pure-Go run of the same case.

**Tasks.**

- **Phase 5a — decision gate (must complete before 5b or 5c).** Resolve between FFI-binding and sidecar-only embedding. The decision rests on Phase 0-4 ergonomics learnings, the Go-as-kernel-language open question ([`architecture/OVERVIEW.md`](../architecture/OVERVIEW.md) §2.1.1) resolution, and prototype-driven latency measurements. Document the decision in an ADR before implementation. Prerequisites for each path:
  - **Sidecar path prerequisites** (must hold at Phase 4 exit, verified explicitly): Phase 4's measured MCP/HTTP overhead on the recon fixture is at or below the 50 ms p95 budget set in [`architecture/OVERVIEW.md`](../architecture/OVERVIEW.md) §7.3, *after* subtracting the Phase 3 harness-overhead baseline. If the budget is missed, the team produces an explicit overhead-reduction plan or renegotiates the budget in an ADR before entering Phase 5b.
  - **FFI path prerequisites** (must hold at Phase 0 / Phase 2 exit, verified explicitly): kernel public types (`Token`, `decimal.Decimal`, `AuditEntry`, error shapes) have a documented C-ABI-projection strategy. If Phase 0's kernel-public-surface ADR does not include this projection, Phase 5b cannot begin without a return trip to Phase 0 to refactor those types.
- **Phase 5b (FFI) or 5c (sidecar)** — implementation of the chosen path:
- If FFI: design and freeze a C-ABI subset of the kernel's public API. Implement a Python binding (`provenant-py`) that wraps it. Marshal tokens, decimal values, and audit entries across the boundary.
- If sidecar: harden the HTTP API (or a minimal MCP profile) for sub-50ms-overhead loopback use. Implement a Python client (`provenant-py`) that speaks it. The client must inject the same `tenant_id`, `run_id`, and tool-call sequence as a Go host would.
- Build a Python test driver that:
  - Loads the same recon fixture used in Phase 3 and Phase 4.
  - Drives an agent loop using the embedded/sidecar Provenant.
  - Captures the resulting audit chain.
- Compare the captured chain against the Phase 3 pure-Go run on the deterministic portion.

**Done when.**

- The Python test driver completes the recon fixture end-to-end.
- The deterministic portion of the resulting audit chain (tool call sequence, tokens minted, validator outcomes, hash chain) is byte-identical to the pure-Go Phase 3 run on the same input.
- The latency overhead vs. pure-Go embedding is documented; if sidecar, the overhead is below the budget set in §7.3 of [`architecture/OVERVIEW.md`](../architecture/OVERVIEW.md).
- The binding choice has an ADR.

**Deferred from this phase.** TypeScript binding. Rust binding. A second non-Go binding to validate that the chosen approach generalizes. (One binding establishes the pattern; the second proves it.)

## Phase 6 — Eval harness and case bank

**Goal.** The case bank captures runs, the eval harness reports the four DFAH-style metrics, and the metric set is the evidence finance buyers and model risk reviewers need.

**Tasks.**

- Implement the case bank capture path: every completed run optionally captured to the `cases` table with full input, full audit chain reference, and labels (correctness, HITL decision, etc.). See [`subsystems/eval.md`](../architecture/subsystems/eval.md).
- Implement the four metrics:
  - **Trajectory determinism** — fraction of tool-call sequences identical across N replays of the same case.
  - **Decision determinism** — fraction of final-answer numeric values identical across N replays.
  - **Faithfulness** — fraction of numeric claims in released answers that map to real tokens with verified lineage. (At enforcement time this is 100% by construction; the metric measures the agent's tendency to attempt ungrounded claims and be rejected.)
  - **Accuracy** — fraction of released answers that match the labeled correct answer for cases with ground truth.
- Implement `provenant eval --skill recon --against ./fixtures/case-bank.jsonl`: runs every case, produces the four-metric report, exits non-zero if any metric drops below the skill's configured threshold.
- Document the recommended CI integration pattern.
- **Implement the `submit_answer_rejection_rate` operational alarm.** A skill that produces faithfulness=1.0 because every answer is rejected pre-enforcement is a useless deployed system; the four metrics alone do not flag this. The eval harness and the runtime emit a `submit_attempts_total` and `submit_releases_total` metric pair (per-skill, per-tenant); the derived `release_rate = releases / attempts` is alarmed in the skill's `eval/thresholds.yaml` with a `release_rate_min` threshold (default 0.5; configurable per skill). A skill that falls below the threshold in CI eval or in production is release-blocked / paged. This ensures "working substrate, useless agent" cannot pass eval.

**Done when.**

- A four-metric report is produced from the recon case bank.
- The report is reproducible: two consecutive `provenant eval` invocations against the same case bank produce identical metric values on the deterministic portions.
- The exit-code behavior gates a sample CI pipeline correctly.

**Deferred from this phase.** Additional skill packs in the case bank. Vendor-provided benchmarks beyond the four core metrics. Hosted dashboards.

## Deferred (v0.5+)

Explicit scope-out for v0. These have established slots (docs or schema columns) but no implementation in v0.

- **LangChain adapter.** Slot established in [`adapters/langchain.md`](../architecture/adapters/langchain.md). Implementation v0.5.
- **OpenAI Agents SDK adapter.** Slot established in [`adapters/openai.md`](../architecture/adapters/openai.md). Implementation v0.5.
- **BoN-MAV mode.** Validator pipeline supports the configuration; the harness re-prompt and selection logic ships v0.5+.
- **Web HITL UI.** HITL routing and state machine ship in v0 (REST + database); the UI is v0.5.
- **Multi-tenant SaaS.** Schema and isolation modes ship in v0; the SaaS operational story (tenant onboarding, per-tenant dashboards, billing) is v0.5.
- **Hosted skill registry with signing.** Skill packs ship as local files in v0; the registry with signed-pack verification is v0.5+.
- **Additional skill packs.** Recon ships in v0. AP coding, expense review, sanctions disposition, SAR drafting are v0.5+.
- **Additional tool backends.** SQL ledger, Textract, keyword policy, OpenFX, OFAC (partial) ship in v0. pgvector policy retrieval, AzureDI extract, Plaid ledger backends, additional external feeds are v0.5+.
- **Additional model classes.** Claude verified in v0. OpenAI, Azure, local (vLLM/TGI/llama.cpp) ship as runtime support in v0.5+; v0 leaves the abstraction in place.
- **Second non-Go binding.** Phase 5 ships one binding to validate the embedding pattern. A second binding (TypeScript or Rust) is v0.5+.
- **Kubernetes operator.** v0 ships Helm charts; the declarative operator is v0.5+ per [`cross-cutting/deployment.md`](../architecture/cross-cutting/deployment.md).
- **Streaming model responses.** Open question in [`subsystems/harness.md`](../architecture/subsystems/harness.md); v0 batches.
- **Cross-skill token sharing within a tenant.** Open question in [`subsystems/kernel.md`](../architecture/subsystems/kernel.md); v0 rejects cross-run references entirely.

## Dependencies and parallelism

- Phase 0 must precede every other phase.
- Phase 1 and Phase 2 can overlap once Phase 0's interfaces are stable (Phase 2 uses the in-memory `Store` until Phase 1's Postgres `Store` is ready).
- Phase 3 requires Phase 1 and Phase 2.
- Phase 4 requires Phase 3.
- Phase 5 requires Phase 4 (the runtime API surface) and benefits from the open-question resolutions in [`architecture/OVERVIEW.md`](../architecture/OVERVIEW.md) §2.1.
- Phase 6 can begin in parallel with Phase 4 once Phase 3 is complete (the four metrics need the harness, not the runtime adapters).

## What this plan does not contain

- Specific calendar dates. Phase ordering is the commitment; pacing is set elsewhere.
- Hiring or staffing plans.
- A go-to-market plan. This is engineering, not commercial.
- Speculative phases beyond Phase 6. Anything past v0 lives in the Deferred section and gets a real phase when its turn comes.
