# Oracle solutions: architecture documentation patch plan

## Inherited decisions

- Provenant is library-first: the kernel is a pure Go enforcement library; MCP/HTTP/CLI are optional thin access surfaces, not product primacy (`architecture/OVERVIEW.md:9-45`, ADR-0010, ADR-0011).
- Storage is external to the kernel. The kernel ships `kernel/public/memio/` as the library-mode default; SQLite/Postgres/S3 live in runtime/host bindings only (`architecture/subsystems/storage.md:5-81`).
- CLI sequencing: Phase 3 is the Mode A embedded CLI/harness milestone; Phase 4 is runtime/server/persistent-binding parity (`orchestrator/plans-context.md:4-8`).
- Normalizers are v0 built-ins now, not an empty v0 registry; `tz_defaulted` is deterministic because it is derived from input bytes (`architecture/subsystems/skills.md:193-209`, ADR-0006 cycle-9 amendment).
- Cross-process orphan recovery requires host/runtime-persisted state; default in-memory library mode cannot recover after process death unless the host serialized `RunState` (`architecture/subsystems/storage.md:54-81`, `docs/KNOWN_TRADE_OFFS.md:76-82`).
- Byte-identity and tamper-evidence are scoped claims: deterministic portions only, for identical captured inputs/pinned facts; tamper-evidence is internal hash-chain detection, not external notarization (`docs/KNOWN_TRADE_OFFS.md:8-13`, `:50-53`).

## Diagnosis

The architecture direction is coherent, but older service/runtime assumptions still appear in adapter, subsystem, and plan text. Most fixes should be narrow wording changes that align stale sections with ADR-0010/0011 and preserve the existing phase split. The only sequencing pivots needed are doc-level: move pgvector wiring to the runtime/persistent phase, split CLI bundle mode from runtime-attached mode, and split Phase 6 work into harness-only vs runtime-persistence-dependent tracks.

## Drift / contradiction check

- MCP-primary and Postgres/S3-required text conflicts with ADR-0010/0011.
- Kernel/harness docs still use “persist”, “Postgres source of truth”, “startup sweeper”, and “skills table” as if universal.
- Phase 3 still contains a `provenant-server` kill/restart orphan test even though `provenant-server` and persistent bindings arrive in Phase 4.
- Phase 2 tries to wire a pgvector backend before runtime Postgres exists.
- Phase 5 wording quietly reopens the Go kernel decision after ADR-0001/0009 already accepted Go for v0.
- Phase 6 parallelism ignores that case-table capture and production runtime alarms need Phase 4 persistence/metrics.

## Recommendation

Patch the docs in small independent commits. Do not rewrite the architecture. Keep ADR-0010/0011 as the contract and treat stale text as cleanup, except where the plan sequence must be made explicit.

## Patch plan

### 1. Stale MCP-primary / Postgres/S3-required language

- **Source evidence/path(s):** `architecture/adapters/mcp.md:3-16` says “primary” and “MCP first”; `architecture/adapters/mcp.md:127-132` config only shows Postgres/S3; `architecture/adapters/mcp.md:143-157` says local dev is Postgres+MinIO and “State lives in Postgres and S3”; `architecture/README.md:127-130` lists ADR-0007 without supersession wording.
- **Exact target docs to patch:** `architecture/adapters/mcp.md`; `architecture/README.md` ADR table.
- **Concise replacement direction:** Change status/intro to “v0 / equal first-class adapter”; rename “Why MCP first” to “Why MCP matters”; cite ADR-0010 as active and ADR-0007 as historical/superseded. Replace the config example with `storage.mode: memory|sqlite|postgres` and `blob_store: memory|s3`, with SQLite as default persistent runtime binding and Postgres/S3 as optional multi-process/large-payload bindings. Change local dev examples to stateless/memio and SQLite-first; keep Postgres+MinIO as an optional multi-process profile.
- **Separate commit?** Yes.
- **PR comment text:** “Align MCP adapter docs with ADR-0010/0011: MCP is an equal access surface, and Postgres/S3 are optional runtime bindings rather than required state.”

### 2. Normalizer determinism contradictions and empty/non-empty registry conflict

- **Source evidence/path(s):** `architecture/subsystems/skills.md:193-196` puts `tz_defaulted` in deterministic payload; `plans/implementation-plan.md:97` says non-deterministic sub-object; `architecture/subsystems/skills.md:207-209` says v0 registry is non-empty and then says Phase 2 registry is empty; ADR-0006 cycle-9/cycle-6 text can still be read as conflicting (`architecture/adrs/0006-skill-pack-format.md:55-74`).
- **Exact target docs to patch:** `plans/implementation-plan.md`; `architecture/subsystems/skills.md`; `architecture/adrs/0006-skill-pack-format.md`.
- **Concise replacement direction:** Make `plans/implementation-plan.md:97` say `tz_defaulted = true` is asserted in `EntryKindTokenMint.deterministic.normalizer_applied`. Delete/replace `skills.md:209` with “Phase 2 registers exactly the five v0 built-ins and rejects unregistered IDs.” In ADR-0006, label the cycle-6 “v0 registry is empty” sentence as historical and superseded for content by cycle 9; leave the compile-time registration mechanism intact.
- **Separate commit?** Yes.
- **PR comment text:** “Resolve normalizer replay contract: v0 has five built-ins, and `tz_defaulted` is deterministic because it is derived from captured input bytes.”

### 3. Kernel doc storage/persistence leakage

- **Source evidence/path(s):** `architecture/subsystems/kernel.md:7-9` depends on “Postgres + object store”; `:19-20` says “Persist tokens”; `:89-99` and `:130-157` make Postgres the token lifecycle/source of truth. Related kernel-facing dependency leaks exist in `architecture/subsystems/audit.md:8` and `architecture/subsystems/skills.md:7-10`, `:250-287`.
- **Exact target docs to patch:** `architecture/subsystems/kernel.md`; `architecture/subsystems/audit.md`; `architecture/subsystems/skills.md`.
- **Concise replacement direction:** In `kernel.md`, replace storage dependency with injected `Store`/`AuditSink`/`BlobStore`/`TransactionalScope` interfaces plus `audit.md`. Change responsibility to “mint and verify token lineage; write through injected interfaces” rather than “persist”. Replace the lifecycle diagram and mint steps with “Store/AuditSink (memio or host/runtime binding)” and move Postgres schema prose to a “runtime projection lives in storage.md” pointer. In `audit.md` and `skills.md`, qualify storage tables as runtime-reference projections; library mode uses in-memory values/host-owned bytes.
- **Separate commit?** Yes.
- **PR comment text:** “Scrub kernel-facing docs so persistence is an injected/host-owned projection, not a kernel dependency or Postgres source-of-truth assumption.”

### 4. Orphan sweeper scoping and library-vs-runtime assumptions

- **Source evidence/path(s):** `architecture/subsystems/harness.md:199-224` presents startup sweep and `skills` table lookup as universal; `architecture/subsystems/harness.md:279-300` immediately shows a `runs` table; `architecture/subsystems/storage.md:54-81` says library-mode HITL persistence is host-owned.
- **Exact target docs to patch:** `architecture/subsystems/harness.md`; optionally cross-link `architecture/subsystems/storage.md`.
- **Concise replacement direction:** Add a “Mode scope” paragraph before the orphan threshold: pure kernel/library logic is a deterministic function over host-provided run snapshots + skill registry metadata; runtime startup sweep is an implementation in SQLite/Postgres bindings; default in-memory library mode has no cross-process orphan recovery after process death. Replace “skills table” with “skill registry / persisted skill metadata supplied by the host or runtime binding (a `skills` table is one runtime projection).” Qualify the `runs` SQL block as runtime-reference persistence only.
- **Separate commit?** Yes.
- **PR comment text:** “Clarify orphan recovery by mode: library mode supplies snapshots, runtime bindings implement startup sweeps over persisted state.”

### 5. Phase 3/4 orphan recovery mismatch

- **Source evidence/path(s):** Phase 1 defers end-to-end persisted orphan recovery to Phase 4 (`plans/implementation-plan.md:66-68`); Phase 3 still has `provenant-server` kill/restart and persisted run-row language (`plans/implementation-plan.md:126`); Phase 4 already contains the moved test (`plans/implementation-plan.md:150`).
- **Exact target docs to patch:** `plans/implementation-plan.md`; optionally `orchestrator/plans-context.md` if it remains as an artifact for future readers.
- **Concise replacement direction:** Remove the Phase 3 orphan-recovery done-when item or replace it with in-process/library snapshot detection/finalization tests only. Leave the authoritative forced-kill/restart integration test in Phase 4 and make it explicitly run against SQLite and Postgres runtime bindings.
- **Separate commit?** Yes.
- **PR comment text:** “Keep cross-process orphan recovery in Phase 4 where `provenant-server` and persistent bindings exist; Phase 3 remains Mode A harness behavior.”

### 6. Phase 1 Postgres Store reference

- **Source evidence/path(s):** `plans/implementation-plan.md:237` says Phase 2 uses in-memory Store until “Phase 1’s Postgres Store” is ready, contradicting Phase 1’s explicit no-DB scope (`plans/implementation-plan.md:48-68`).
- **Exact target docs to patch:** `plans/implementation-plan.md` Dependencies and parallelism section; optionally Phase 0 deferred line (`plans/implementation-plan.md:44`) for wording consistency.
- **Concise replacement direction:** Replace line 237 with “Phase 1 and Phase 2 can overlap once Phase 0 interfaces are stable; both use `kernel/public/memio/` and mocked/injected backends. Persistent SQLite/Postgres Store bindings arrive in Phase 4.” If touching line 44, say “runtime persistent Store bindings” rather than “Postgres-backed Store.”
- **Separate commit?** Could be bundled with issue 5 as plan storage-sequencing cleanup; separate if keeping commits one-issue-one-fix.
- **PR comment text:** “Remove the stale Phase 1 Postgres Store dependency; early phases use memio, while persistent bindings start in Phase 4.”

### 7. pgvector backend timing

- **Source evidence/path(s):** Phase 2 wires pgvector (`plans/implementation-plan.md:83`) before runtime Postgres exists; match docs mark pgvector as v0 reference (`architecture/tools/match/README.md:204-208`); pgvector backend says it uses the same canonical Postgres store (`architecture/tools/match/backends/pgvector.md:5-10`). Policy pgvector is deferred (`architecture/tools/policy/backends/pgvector.md:1-21`), showing the timing distinction matters.
- **Exact target docs to patch:** `plans/implementation-plan.md`; `architecture/tools/match/README.md`; `architecture/tools/match/backends/pgvector.md`; `architecture/README.md` v0 scope bullets (`:161-162`).
- **Concise replacement direction:** Keep Phase 2 responsible for the `match` category, schemas, validators, deterministic sorting, and tests against a fake/in-memory backend. Move pgvector backend wiring to Phase 4 after `runtime/postgres/` exists. In match docs, label pgvector “v0 runtime reference backend (Phase 4), not a kernel or library-mode prerequisite”; replace “same canonical store” with “runtime Postgres/pgvector binding or operator-provided pgvector database.” In the README v0 summary, say pgvector match ships as an optional runtime reference backend, while the match category itself is v0.
- **Separate commit?** Yes.
- **PR comment text:** “Move pgvector wiring out of the pure kernel/tool-registry phase and scope it as an optional Phase 4 runtime backend.”

### 8. CLI sequencing inconsistency

- **Source evidence/path(s):** Phase 1 says no CLI binary until Phase 4 (`plans/implementation-plan.md:48`, `:58`); Phase 3 implements many `provenant` CLI commands (`plans/implementation-plan.md:116-123`); Phase 4 then ships CLI wrapper/parity (`plans/implementation-plan.md:149-160`). CLI docs say embedded mode opens Postgres by default (`architecture/adapters/cli.md:203-210`).
- **Exact target docs to patch:** `plans/implementation-plan.md`; `architecture/adapters/cli.md`; optionally `architecture/OVERVIEW.md:218-227` if adding a cross-reference.
- **Concise replacement direction:** Define two CLI milestones. Phase 3 ships embedded Mode A batch CLI over memio/bundle artifacts (`run --bundle-out`, replay/verify against bundle or local run artifact, no runtime server, no migrations, no required DB). Phase 4 adds runtime-attached mode, run-id operations over SQLite/Postgres, `migrate`, persistent HITL, and parity tests. Update `cli.md` so embedded default uses memio/no DB; persistent embedded/server-attached configurations may use SQLite default or Postgres optional.
- **Separate commit?** Yes.
- **PR comment text:** “Split CLI docs into Phase 3 embedded bundle mode and Phase 4 runtime-attached/persistent parity, removing the Postgres-default CLI assumption.”

### 9. Go/non-Go binding decision gate timing

- **Source evidence/path(s):** `architecture/OVERVIEW.md:72-80` frames Go as an open question; Phase 0 says a language switch can restart the phase (`plans/implementation-plan.md:42`); Phase 5a says the FFI-vs-sidecar decision depends on resolving the Go-as-kernel open question (`plans/implementation-plan.md:171-173`) even though ADR-0001/0009 accept Go for v0 and non-Go binding is Phase 5.
- **Exact target docs to patch:** `architecture/OVERVIEW.md`; `plans/implementation-plan.md`; optionally ADR-0009 reference text if adding a clarifying sentence.
- **Concise replacement direction:** Clarify that Go is accepted for v0; the remaining Phase 5 decision is binding shape (FFI vs strict-latency sidecar), not whether to rewrite the kernel language. Move C-ABI feasibility to an early prerequisite: Phase 0/2 should preserve/document an FFI projection if the FFI path is to stay viable; if not done, Phase 5a may choose sidecar or explicitly ADR a return-trip. Remove “switch language restarts Phase 0” from the active plan or reframe it as a post-v0/open ADR, not a Phase 0 gate.
- **Separate commit?** Yes.
- **PR comment text:** “Stop reopening the v0 Go-kernel decision at Phase 5; Phase 5 chooses binding shape based on early ABI readiness and Phase 4 latency data.”

### 10. Live-progress Phase 4 done criteria

- **Source evidence/path(s):** `architecture/cross-cutting/observability.md:127-169` specifies OTLP `parallel_block` span events and `provenant run inspect`; it says Phase 4 done-when must require this (`:135`), but `plans/implementation-plan.md:152-161` has no corresponding done-when item.
- **Exact target docs to patch:** `plans/implementation-plan.md` Phase 4 Done when; optionally `architecture/cross-cutting/observability.md` only for wording if needed.
- **Concise replacement direction:** Add a Phase 4 done-when item: during an in-flight parallel block, OTLP emits `parallel_block` with sibling count and started/completed/failed events; `provenant run inspect <run_id>` shows the live-state table; rows are GC’d at flush; restart returns the documented `live_state: none` sentinel; no per-run Prometheus labels are emitted.
- **Separate commit?** Yes.
- **PR comment text:** “Make the documented live-progress observability contract an explicit Phase 4 acceptance criterion.”

### 11. Phase 6 parallelism dependency split

- **Source evidence/path(s):** Phase 6 tasks include `cases` table capture and runtime metrics/alarms (`plans/implementation-plan.md:198-206`); dependencies say all Phase 6 can run in parallel with Phase 4 after Phase 3 (`plans/implementation-plan.md:241`); eval docs scope runtime case-bank persistence to Phase 4 (`architecture/subsystems/eval.md:7-10`).
- **Exact target docs to patch:** `plans/implementation-plan.md`; optionally `architecture/subsystems/eval.md` if adding matching labels.
- **Concise replacement direction:** Split Phase 6 into 6a/6b or annotate tasks: 6a (metric computation over bundle/case-bank JSON, replay harness, CI report) can start after Phase 3; 6b (persistent `cases`/`case_metrics` capture in SQLite/Postgres, production runtime `submit_attempts_total`/`submit_releases_total` alarming, CLI `cases` over runtime store) depends on Phase 4. Update dependency line 241 accordingly.
- **Separate commit?** Yes.
- **PR comment text:** “Split Phase 6 parallelism so harness-only eval can start after Phase 3, while persistent case-bank/runtime alarm work waits for Phase 4.”

### 12. Caveats around byte-identity / tamper-evidence claims

- **Source evidence/path(s):** Trade-offs scope internal tamper-evidence/no notary (`docs/KNOWN_TRADE_OFFS.md:8-13`) and fresh-run external-source drift (`docs/KNOWN_TRADE_OFFS.md:50-53`); stronger wording appears in `docs/thesis.md:3-10`, `:31-45`, `:106-112`; `architecture/OVERVIEW.md:44`, `:190-192`, `:208`; `README.md:3-5`.
- **Exact target docs to patch:** `docs/thesis.md`; `architecture/OVERVIEW.md`; `architecture/README.md` core invariant text; root `README.md`.
- **Concise replacement direction:** Replace broad “tamper-evident” with “internally tamper-evident / tamper-detectable hash chain; no external notary in v0.” Replace “same input + same skill pack” byte-identity claims with “same captured inputs, captured reads, kernel version/header, skill pack hash, normalizer/backend versions, and host conformance facts; deterministic portions only.” Keep concise links to KNOWN_TRADE_OFFS rather than expanding every caveat inline.
- **Separate commit?** Yes.
- **PR comment text:** “Tighten evidence claims: byte-identity is conditional on captured/pinned facts, and v0 tamper-evidence is internal hash-chain detection, not external notarization.”

## Additional prior-review cleanup worth including

### 13. Architecture README stale navigation and tool table omissions

- **Source evidence/path(s):** `architecture/README.md:106-113` omits `match` and `dedupe` from the tools table while `:161-162` includes them; `architecture/README.md:127` lists ADR-0007 as active “MCP as primary” while `:130` lists ADR-0010.
- **Exact target docs to patch:** `architecture/README.md`.
- **Concise replacement direction:** Add `match` and `dedupe` rows to the tools table. Mark ADR-0007 as superseded by ADR-0010 in the ADR table, mirroring the ADR file’s supersession header.
- **Separate commit?** Can be bundled with issue 1 if touching README already.
- **PR comment text:** “Fix architecture README navigation so active readers see match/dedupe and ADR-0007’s superseded status.”

### 14. `run_orphan_recovered.recovery_reason` enum not pinned

- **Source evidence/path(s):** `architecture/subsystems/audit.md:159` includes deterministic `recovery_reason` without an enum; reviewer flagged possible drift between `startup_sweep`, `startup`, and `orphan_sweep`.
- **Exact target docs to patch:** `architecture/subsystems/audit.md`; `plans/implementation-plan.md` enumgen/drift-gate list if adding generated validation; optionally `architecture/subsystems/harness.md` orphan threshold section.
- **Concise replacement direction:** Add a canonical enum for `run_orphan_recovered.recovery_reason`, e.g. `startup_sweep`, `operator_sweep`, `restore_verify_prepass`; require write-time validation and include it in enumgen/drift gates like `failure_subcode`.
- **Separate commit?** Yes, or bundle with issue 4 if keeping orphan fixes together.
- **PR comment text:** “Pin `run_orphan_recovered.recovery_reason` as a deterministic enum so independent bindings cannot choose divergent strings.”

## Risks

- Overcorrecting could accidentally demote v0 scope (especially match/dedupe) rather than only moving backend wiring to the right phase.
- CLI wording needs care: Phase 3 should still be a real Mode A artifact, just not a persistent runtime/server artifact.
- The byte-identity caveat should be visible but not so verbose that it weakens the thesis unnecessarily.

## Need from main agent

No decision is required if the baseline contract above is accepted. The only judgment call is whether to include the two additional prior-review cleanups (#13 and #14) in the same documentation PR; I recommend yes because they are small and directly sourced.

## Suggested execution prompt

No implementation handoff is warranted for this oracle task. If the main agent wants a worker patch afterward, use a prompt scoped to documentation-only edits implementing the numbered patch plan above, with one commit per issue unless the plan explicitly says bundling is acceptable.
