# Implementation Plan

This plan sequences Provenant from empty repository to v0 — the scope defined in [`architecture/README.md`](architecture/README.md#v0-scope-summary) plus the dual-mode pluggability requirement stated in [`thesis.md`](thesis.md) and [`architecture.md`](architecture.md). Each phase ends with a working, testable artifact and a falsifiable "done when" criterion. Phases are sequential; later phases assume earlier-phase artifacts exist and pass.

Decisions and rationale belong in [ADRs](architecture/adrs/). This document is a plan, not a justification.

## Phase 0 — Establish the kernel module boundary

**Goal.** Carve out the kernel as a pure-Go library with no I/O at the public API surface, and **pin the load-bearing constants and headers** that downstream phases will hash into the audit chain. This is the mechanical foundation of the dual-mode pluggability promise — "ratify-and-pin" the immovable surface so Phases 1+ can build on stable bytes. Everything downstream depends on the boundary being right.

**Tasks.**

- Create the repository layout: `kernel/` (pure library), `runtime/` (transport + process lifecycle), `adapters/` (host-side shims), `tools/` (tool backends), `internal/` (shared non-public helpers).
- Define kernel-public types: `Token`, `Source`, `Derivation`, `ToolDef`, `ToolCall`, `Claim`, `Payload`, `ValidationError`, `AuditEntry`, `EntryKind`, `Run`, run status enum.
- Define kernel-public interfaces (every I/O dependency): `Store`, `BlobStore`, `ModelClient`, `Clock`, `RandSource`, `AuditSink`, `ToolBackend`, `TransactionalScope` (joint-transaction primitive across `Store` and `AuditSink` — required so the parallel-block atomicity invariant in [`subsystems/audit.md`](architecture/subsystems/audit.md#parallel-sibling-deterministic-order) survives separately-injected backends).
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
- **`EMPTY_CJSON_SHA256` pinned-constant test passes.** A unit test in the kernel package asserts byte-identity between `sha256(0x7B, 0x7D)` and the pinned hex constant `44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a` (see [`architecture/subsystems/audit.md`](architecture/subsystems/audit.md#host_conformance_attestation)). A canonicalizer regression that emitted whitespace or a BOM would fail this test at build time rather than silently changing the chain on Mode A runs.
- **Kernel-header release-gate test passes.** A release-gate test under `kernel/testdata/header-fixtures/` enumerates each `kernel_version` and asserts that `sha256(kernel/internal/promptkernel/header_v<N>.go's exported text bytes)` equals the version-pinned hex in the fixture. A drift between the header file and the fixture (e.g., a contributor edits the header text without bumping `kernel_version`) fails the release.
- A short `architecture/adrs/0009-kernel-public-surface.md` (or equivalent) ratifies the import-guard list. (If the open question about Go-as-kernel from [`architecture.md`](architecture.md) §2.1 resolves to "switch language", this phase restarts; that is acceptable cost.)

**Deferred from this phase.** Postgres-backed `Store`. S3-backed `BlobStore`. Real `ModelClient`. Tool implementations. (The kernel-internal pinned constants and `header_v1.go` ship in Phase 0 per the "ratify-and-pin" goal above; they are explicitly **not** deferred. The cycle-5 `Deferred` line "Audit chain hash format finalization beyond the SHA-256-over-canonical-JSON contract" is retracted in cycle 6 as it contradicted the Phase 0 done-when items (d) `EMPTY_CJSON_SHA256` test and (e) header release-gate hash test, which require the format to be pinned in Phase 0.)

## Phase 1 — Storage and hash-chained audit log

**Goal.** Persist runs, tokens, tool calls, and audit entries to Postgres. Build the hash chain. Make replay possible.

**Tasks.**

- Implement the Postgres-backed `Store` (`runtime/store/pg/`) using `pgx` + `sqlc`. Implements every kernel `Store` method.
- Implement the S3-backed `BlobStore` (`runtime/blobstore/s3/`) with the in-memory `BlobStore` retained for tests.
- Implement the Postgres-backed `AuditSink` with the per-tenant monotonic sequence and `prev_hash`/`hash` columns. The chain construction logic lives in the kernel as a pure function; the sink writes the rows the kernel produces.
- Run atlas migrations for the v0 schema described in [`subsystems/storage.md`](architecture/subsystems/storage.md): `runs`, `tokens`, `tool_calls`, `captured_reads`, `guard_runs`, `audit_entries`, `cases`, `case_metrics`, `skills`, `tools_registry`, `hitl_reviews`, `tenants`. The initial migration includes the cycle 3–5 schema deltas: `tokens.source_value_raw`, `tokens.normalizer_applied`, `runs.credential_identity_baseline`, `runs.kernel_header_hash`, and the `audit_entries.payload_deterministic` / `payload_non_deterministic` / `hash_input_canonical` three-column split (see [`subsystems/storage.md`](architecture/subsystems/storage.md#schema-deltas-required-by-cycles-35-load-bearing-columns)).
- Implement `provenant migrate up` (atlas wrapper) as a CLI subcommand stub.
- Implement audit chain verification: given a `run_id` or `tenant_id`, walk entries and verify each `hash == HashEntry(prev_hash, payload, ...)`.
- Implement audit replay: read all entries for a run in order, reconstruct the run state.

**Done when.**

- An integration test starts Postgres (testcontainers or docker-compose), inserts a synthetic run with 50 tokens, 20 tool calls, and 100 audit entries; the test then verifies the chain and replays the entries to reconstruct the run state byte-identically.
- Killing the test process mid-write and re-running the verifier flags the truncated chain.
- A property test mutates a random audit row and confirms the verifier detects the tamper. The same property test mutates a random field inside `payload_non_deterministic` and confirms the verifier does **not** flag it (the non-deterministic column is excluded from the hash input); a mutation of `payload_deterministic` or `hash_input_canonical` is flagged.
- **Orphan-run recovery is implemented and tested.** On `provenant-server` startup, any run in `running` or `pending` status whose last audit entry is older than a **per-skill** threshold is moved to `failed: orphaned` (preceded by an `EntryKindRunOrphanRecovered` entry recording the recovery). The threshold for each candidate run is `skill.bounds.max_wall_clock_seconds × skill.bounds.orphan_multiplier` (default `orphan_multiplier = 2`, **pinned in the skill manifest and not deployment-overridable** — see [`subsystems/harness.md`](architecture/subsystems/harness.md#orphan-run-threshold-per-skill) and [`subsystems/skills.md`](architecture/subsystems/skills.md)), looked up from the `skills` table for the run's pinned `(skill_id, skill_version)` — never a global default — so concurrent runs of skills with different `max_wall_clock` values are evaluated independently, a long-budget skill is not prematurely swept, and two deployments of the same `(skill_id, skill_version)` compute identical `threshold_seconds_used` (required for cross-deployment chain byte-identity on orphan-recovered runs). The applied threshold is recorded in the recovery entry's deterministic payload (`threshold_seconds_used`). A test forcibly kills the process mid-write across two concurrent runs (one short-budget skill, one long-budget skill) and verifies that on restart only the short-budget run past its threshold is marked `failed: orphaned`, the long-budget run remains `running` until its own threshold elapses, and both audit chains remain intact.

**Deferred from this phase.** Multi-tenant SaaS isolation modes beyond the schema (row-level security wiring is v0; SaaS-grade tenant onboarding is v0.5). Cross-region object-store replication.

## Phase 2 — Tool registry and guard pipeline (pre + post)

**Goal.** A tool can be registered, dispatched with schema validation, mint tokens, and have its output flow through pre-LLM guards. The post-LLM validator pipeline runs end-to-end against payloads without a model in the loop.

**Tasks.**

- Implement the tool registry: typed schema registration, `mintable_paths`, version pinning. Backends register via the kernel `ToolBackend` interface. See [`subsystems/tools.md`](architecture/subsystems/tools.md).
- Implement audit middleware around every tool call: `tool_call_started`, `tool_call_completed` entries; full input + output captured (large payloads to blob store with pointer in the entry).
- Implement token minting from `mintable_paths`. Unit-incompatible derivations rejected.
- Implement the pre-LLM guard pipeline ([`subsystems/guards.md`](architecture/subsystems/guards.md#pre-llm-input-guards)): `pii.redact`, `injection.scan`, `injection.heuristic`, `source.authenticate`, `length.cap`. Severity tiers L1/L2/L3 honored. Every execution writes a `guard_runs` row.
- Implement the post-LLM validator pipeline: `schema.output`, `arith.crossfoot`, `arith.tie`, `arith.sign`, `arith.balance`, `policy.coa_membership`, `policy.citation_exists`, `confidence.extraction_min`, `currency.consistent`.
- Implement `kernel.SubmitAnswer`: numeric extraction, per-claim token grounding via `kernel.VerifyToken`, validator pipeline (collect-all and fail-fast modes), persistence on pass.
- Implement at least the pure-Go tool backends end-to-end: `reconcile.match`, `calc.subtract`, `calc.sum`, `calc.percent_change`, `calc.variance`. These need no external systems and can be the first end-to-end tools.

**Done when.**

- An integration test feeds a recorded reconciliation tool-call sequence (no model) into the kernel; every tool call mints tokens correctly; every guard runs; a final synthetic `submit_answer` payload is verified end-to-end. The resulting trace is fully typed and schema-validated.
- Injecting an ungrounded number into the synthetic answer is rejected by token grounding.
- Injecting a prompt-injection pattern into a synthetic tool output triggers L2 quarantine.
- All validators have unit tests; the pipeline has integration tests.
- **A pure-kernel micro-benchmark** measures `kernel.SubmitAnswer` p95 overhead on a representative recon payload with mocked I/O. The §7.3 budget is "50 ms on top of validator costs" — a budget on kernel-orchestration overhead, not on the whole pipeline. The benchmark therefore reports two numbers and compares each to the correctly-scoped budget:
  - `t_kernel_orchestration` = total `SubmitAnswer` wall time **minus** the summed wall time of the individual validator and per-claim token-grounding calls (each validator and `VerifyToken` is instrumented to emit its own duration; orchestration is the residual: numeric extraction, dispatch glue, claim/token mapping, audit append, pipeline mode handling). This is the figure compared against the 50 ms §7.3 budget.
  - `t_total_with_validators` = total `SubmitAnswer` wall time including validator and token-grounding costs. This is recorded as the kernel's intrinsic floor for Phase 4's transport-overhead measurement to subtract from end-to-end latency.
  Both numbers are persisted to a bench-results JSON file that Phase 4 reads. If `t_kernel_orchestration` exceeds 50 ms, Phase 3 does not start until the kernel orchestration path is optimized or the budget is renegotiated in an ADR; validator cost overruns are a separate ADR concern (per-validator budgets live in [`subsystems/guards.md`](architecture/subsystems/guards.md)). This separation prevents the cycle-2 critic #8 failure mode where Phase 2 reports "under budget" by conflating orchestration with validator costs, leaving Phase 4 no slack for MCP overhead.

**Deferred from this phase.** External-system tool backends (Textract, OFAC, OpenFX) wire-up; that's Phase 3's responsibility. BoN-MAV mode (validator pipeline is wired for it but the harness re-prompt logic ships in v0.5). Custom per-skill numeric extractors.

## Phase 3 — Harness (Mode A) with one model class

**Goal.** The CLI runs a reconciliation end-to-end against a real model (Claude), with one signed skill pack and the v0 tool backend set. Full audit + tokens + guard outcomes persisted.

**Tasks.**

- Implement the kernel harness (Mode A) per [`subsystems/harness.md`](architecture/subsystems/harness.md): prompt assembly with immutable kernel header, model call loop, parallel tool dispatch, bound enforcement (`max_steps`, `max_wall_clock`, `max_cost`).
- Implement the model abstraction interface and the Claude implementation (`runtime/model/anthropic/`).
- Implement the v0 skill pack format ([`subsystems/skills.md`](architecture/subsystems/skills.md), [ADR-0006](architecture/adrs/0006-skill-pack-format.md)) and ship the reconciliation skill pack as a local directory.
- Wire up the external tool backends required by the recon skill: SQL ledger, Textract extract, keyword policy, OpenFX external. Each follows the conformance template in [`architecture/tools/README.md`](architecture/tools/README.md).
- Implement `provenant run`, `provenant replay`, `provenant cases`, `provenant audit show`, `provenant audit verify`, `provenant audit pack`, `provenant skill list/show/install`, `provenant tools list/show`.
- Implement cost and latency tracking; update `runs.cost_usd`, `runs.tokens_in`, `runs.tokens_out`.

**Done when.**

- `provenant run recon --input ./fixtures/may-2026-recon.json --model claude-opus-4-7 --tenant acme` completes end-to-end with a passing `submit_answer` for a known-good fixture.
- `provenant audit verify <run_id>` returns OK; `provenant audit pack <run_id>` produces a bundle containing the audit chain, the token graph, the guard outcomes, and every prompt+response.
- `provenant replay <run_id>` reconstructs the deterministic portion (tool calls, validator outcomes, audit chain) byte-identically across two consecutive replays.
- **ModelClient stream-refusal test passes.** A unit test injects a `ModelClient` that advertises streaming capability and attempts to deliver tool_use blocks progressively on a compliance-path run. The harness must either coerce `stream=false` at the abstraction layer (the documented mitigation in KNOWN_TRADE_OFFS.md) or reject the client at run start with a typed `harness.streaming_required` error; the test asserts that no run path produces a streamed tool_use response. A second test injects a `ModelClient` that emits parallel tool_use blocks split across two consecutive response turns and asserts that each turn forms an **independent** `sibling_group_id` (no cross-turn merging), the resulting audit chain is byte-identical across two replays of the same run, and each turn's `parent_step_id` derives from its own `step_index` per [`subsystems/audit.md`](architecture/subsystems/audit.md#parent_step_id-construction). These two tests close the cycle-4 critic #4 gap where the streaming-disabled and split-turn-independence claims existed only on paper.
- **Silent-credential-rotation replay byte-identity test.** A fixture run injects a deterministic silent credential rotation at a fixed `step_index` (a test-only secret-store stub flips the credential's stable-identifier hash on the Nth tool call without surfacing an authentication failure). The harness must emit exactly one `EntryKindCredentialIdentityChanged` entry at that step and abort the run with `failure_class = aborted`, `failure_subcode = credential_identity_changed` (per [`cross-cutting/security.md`](architecture/cross-cutting/security.md#silent-credential-rotation-detection-per-run-identity-baseline)). The test replays the same fixture (same input bytes, same deterministic secret-store stub) twice and asserts that the `EntryKindCredentialIdentityChanged` entry is **byte-identical** across the two replays — including `credential_identifier_hash_baseline`, `credential_identifier_hash_observed`, `at_step_index_be8`, and the entry's `hash` and `prev_hash`. This closes the cycle-5 outstanding gap where the entry's determinism was specified but no test in the plan asserted it.
- **Harness-overhead isolation benchmark.** The CLI harness records, per run, the time spent in: (a) model API calls, (b) tool backends, (c) kernel pipeline (validators + token grounding + audit append), (d) harness orchestration (prompt assembly + scheduling). The Phase 3 done criterion publishes (c) + (d) — the Provenant-attributable overhead — as a baseline number. Phase 4's MCP-overhead measurement subtracts this baseline so transport cost is not conflated with harness cost.

**Deferred from this phase.** Other model classes (OpenAI, Azure, local). Other skill packs. The runtime layer's MCP and HTTP surfacing (Phase 4). Streaming model responses.

## Phase 4 — Runtime layer: MCP server, HTTP API, and CLI parity

**Goal.** The same case from Phase 3 runs identically via MCP and via HTTP. The runtime layer is a thin shell around kernel calls.

**Tasks.**

- Implement the MCP server ([`adapters/mcp.md`](architecture/adapters/mcp.md)) using the official Go MCP SDK. Expose every Provenant tool as an MCP tool; every skill prompt as an MCP prompt; audit and case data as MCP resources. Bearer-token auth, tenant scoping, skill scoping.
- Implement the HTTP API ([`adapters/http.md`](architecture/adapters/http.md)) covering the same surface as the MCP server: tool invocation, prompt retrieval, run inspection, audit retrieval. RESTful; same auth shape as MCP.
- Wire up the runtime process: configuration loading, signal handling, graceful shutdown, observability (Prometheus metrics, OTel traces, JSON logs per [`cross-cutting/observability.md`](architecture/cross-cutting/observability.md)).
- Ship the `provenant-server` binary (the runtime daemon) alongside the existing `provenant` CLI.
- Add the `provenant-mcp-client` stdio-to-HTTP wrapper used by Claude Code et al.
- **Ship the v0 HITL routing surface** (the web UI is deferred to v0.5; the routing itself is v0 scope per [`subsystems/hitl.md`](architecture/subsystems/hitl.md)). This includes: the `hitl_reviews` state machine (`pending` → `approved` | `edited` | `rejected` | `escalated` | `expired`), the canonical REST surface under `/v1/hitl/...` (`POST /v1/hitl/reviews/{review_id}/decision`, `POST /v1/runs/{run_id}/hitl/decision` (convenience alias), `GET /v1/hitl/pending`, `GET /v1/hitl/reviews/{review_id}`, `POST /v1/hitl/reviews/{review_id}/claim`, `POST /v1/hitl/bulk/approve`, `GET /v1/hitl/reviews/{review_id}/audit` — see [`subsystems/hitl.md`](architecture/subsystems/hitl.md#rest-api-v0)), the corresponding MCP tools, the kernel-side state transition logic with audit-log entries (`EntryKindHITLDecision.decision` covers all five enum values, with `edits_canonical` populated when `decision = edited`), and the harness path that moves a run from `awaiting_review` back to `running` (on approval / edited / escalated as configured) or `failed` (on rejection or expiry as configured) when a decision arrives.

**Done when.**

- A Claude Code instance configured against the Provenant MCP server completes the same recon fixture from Phase 3. The audit chain's deterministic portion is byte-identical to the Phase 3 CLI run on the same input. (LLM-portion variance is allowed and measured.)
- The same fixture run via HTTP API directly produces the same byte-identical deterministic-portion audit chain.
- The same fixture run via MCP and via HTTP — neither compared against the CLI — produces byte-identical deterministic-portion chains. ADR-0010 requires all three pairs (MCP-vs-CLI, HTTP-vs-CLI, MCP-vs-HTTP) to be byte-identical; transitivity through CLI is not accepted as a substitute, because a bug that affects MCP and HTTP in the same way (e.g., a shared runtime-layer canonicalization shim that differs from the CLI's direct kernel path) would pass the two CLI-pairwise tests and silently fail this third one.
- Cross-tenant access attempts are rejected. Invalid bearer tokens are rejected. MCP latency overhead is below the 50ms p95 budget on top of tool execution.
- An integration test escalates a run to `awaiting_review`, posts a HITL decision via the REST endpoint, and verifies the run resumes (or fails) deterministically; an audit-chain entry records the decision.

**Deferred from this phase.** LangChain and OpenAI Agents adapters (v0.5). Per-tenant Grafana dashboards. Per-tenant rate limits beyond the global cap. Web HITL UI.

## Phase 5 — Code-level embedding path (non-Go binding)

**Goal.** Demonstrate the code-level pluggability promise. A host application written in a language other than Go embeds Provenant — either through FFI or through a strict-latency sidecar protocol — and produces an audit chain byte-identical to a pure-Go run of the same case.

**Tasks.**

- **Phase 5a — decision gate (must complete before 5b or 5c).** Resolve between FFI-binding and sidecar-only embedding. The decision rests on Phase 0-4 ergonomics learnings, the Go-as-kernel-language open question ([`architecture.md`](architecture.md) §2.1.1) resolution, and prototype-driven latency measurements. Document the decision in an ADR before implementation. Prerequisites for each path:
  - **Sidecar path prerequisites** (must hold at Phase 4 exit, verified explicitly): Phase 4's measured MCP/HTTP overhead on the recon fixture is at or below the 50 ms p95 budget set in [`architecture.md`](architecture.md) §7.3, *after* subtracting the Phase 3 harness-overhead baseline. If the budget is missed, the team produces an explicit overhead-reduction plan or renegotiates the budget in an ADR before entering Phase 5b.
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
- The latency overhead vs. pure-Go embedding is documented; if sidecar, the overhead is below the budget set in §7.3 of [`architecture.md`](architecture.md).
- The binding choice has an ADR.

**Deferred from this phase.** TypeScript binding. Rust binding. A second non-Go binding to validate that the chosen approach generalizes. (One binding establishes the pattern; the second proves it.)

## Phase 6 — Eval harness and case bank

**Goal.** The case bank captures runs, the eval harness reports the four DFAH-style metrics, and the metric set is the evidence finance buyers and model risk reviewers need.

**Tasks.**

- Implement the case bank capture path: every completed run optionally captured to the `cases` table with full input, full audit chain reference, and labels (correctness, HITL decision, etc.). See [`subsystems/eval.md`](architecture/subsystems/eval.md).
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

- **LangChain adapter.** Slot established in [`adapters/langchain.md`](architecture/adapters/langchain.md). Implementation v0.5.
- **OpenAI Agents SDK adapter.** Slot established in [`adapters/openai.md`](architecture/adapters/openai.md). Implementation v0.5.
- **BoN-MAV mode.** Validator pipeline supports the configuration; the harness re-prompt and selection logic ships v0.5+.
- **Web HITL UI.** HITL routing and state machine ship in v0 (REST + database); the UI is v0.5.
- **Multi-tenant SaaS.** Schema and isolation modes ship in v0; the SaaS operational story (tenant onboarding, per-tenant dashboards, billing) is v0.5.
- **Hosted skill registry with signing.** Skill packs ship as local files in v0; the registry with signed-pack verification is v0.5+.
- **Additional skill packs.** Recon ships in v0. AP coding, expense review, sanctions disposition, SAR drafting are v0.5+.
- **Additional tool backends.** SQL ledger, Textract, keyword policy, OpenFX, OFAC (partial) ship in v0. pgvector policy retrieval, AzureDI extract, Plaid ledger backends, additional external feeds are v0.5+.
- **Additional model classes.** Claude verified in v0. OpenAI, Azure, local (vLLM/TGI/llama.cpp) ship as runtime support in v0.5+; v0 leaves the abstraction in place.
- **Second non-Go binding.** Phase 5 ships one binding to validate the embedding pattern. A second binding (TypeScript or Rust) is v0.5+.
- **Kubernetes operator.** v0 ships Helm charts; the declarative operator is v0.5+ per [`cross-cutting/deployment.md`](architecture/cross-cutting/deployment.md).
- **Streaming model responses.** Open question in [`subsystems/harness.md`](architecture/subsystems/harness.md); v0 batches.
- **Cross-skill token sharing within a tenant.** Open question in [`subsystems/kernel.md`](architecture/subsystems/kernel.md); v0 rejects cross-run references entirely.

## Dependencies and parallelism

- Phase 0 must precede every other phase.
- Phase 1 and Phase 2 can overlap once Phase 0's interfaces are stable (Phase 2 uses the in-memory `Store` until Phase 1's Postgres `Store` is ready).
- Phase 3 requires Phase 1 and Phase 2.
- Phase 4 requires Phase 3.
- Phase 5 requires Phase 4 (the runtime API surface) and benefits from the open-question resolutions in [`architecture.md`](architecture.md) §2.1.
- Phase 6 can begin in parallel with Phase 4 once Phase 3 is complete (the four metrics need the harness, not the runtime adapters).

## What this plan does not contain

- Specific calendar dates. Phase ordering is the commitment; pacing is set elsewhere.
- Hiring or staffing plans.
- A go-to-market plan. This is engineering, not commercial.
- Speculative phases beyond Phase 6. Anything past v0 lives in the Deferred section and gets a real phase when its turn comes.
