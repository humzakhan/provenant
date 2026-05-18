# Architecture Review Report

## Thesis and mission
Provenant’s thesis is strong and differentiated: it is a library-first enforcement substrate for regulated-finance LLM agents, not a logging layer, agent framework, or hosted control plane. The architecture generally preserves the core contract: `submit_answer` remains the intended release boundary, deterministic portions of the audit chain are meant to be replayable across surfaces, and storage is supposed to remain a deployment concern rather than a kernel dependency.

## Alignment assessment
At a high level, the architecture docs align with the thesis. The overview, ADR set, and storage model consistently describe a pure Go kernel with thin MCP/HTTP/CLI adapters and optional runtime persistence. The architecture also gives concrete support to the thesis claim that preprocessing and evidence handling are part of enforcement rather than off-chain hygiene.

The main issues are not with the core direction, but with stale or conflicting document text that weakens the stated boundary. Several subsystem and adapter docs still read as if MCP is primary, Postgres/S3 are required, or runtime persistence semantics leak into the kernel contract. There are also determinism contradictions around normalizers and some sequencing inconsistencies in the implementation plan.

## Prioritized findings

### 1. High: stale adapter language still conflicts with the library-first thesis
Some MCP adapter content still describes MCP as primary and frames Postgres/S3-backed runtime storage as the default or required shape. That contradicts the current architecture position that MCP, HTTP, and CLI are equal adapters over the same kernel and that storage is optional and deployment-specific.

**Recommendation:** update adapter docs to reflect ADR-0010/0011 consistently: library-first product, equal adapters, SQLite/in-memory examples where appropriate, and Postgres/S3 described only as optional runtime bindings.

### 2. High: normalizer determinism is internally inconsistent
The docs disagree on whether `tz_defaulted` belongs in deterministic or non-deterministic output, and one subsystem section still implies an empty normalizer registry while the plan assumes built-in normalizers in scope. This undercuts the replay-deterministic claim exactly where messy real-world finance data is supposed to be normalized on-chain.

**Recommendation:** make the determinism contract explicit and consistent across architecture and plan docs, treat `tz_defaulted` the same everywhere, and remove stale empty-registry language.

### 3. High: kernel docs still leak storage/persistence assumptions
Some kernel text still speaks as if the kernel depends on Postgres/object storage or “persists tokens” directly. That conflicts with the stronger, current boundary that the kernel mints/verifies against injected interfaces and that durability belongs to the host or runtime binding.

**Recommendation:** scrub kernel-facing docs so persistence is always described as an external projection, not an intrinsic kernel responsibility.

### 4. High: orphan sweeper scope still mixes runtime behavior with library semantics
The orphan-recovery model is conceptually sound, but some harness text reads as though every mode has a startup daemon, database tables, and persistent state. That is only true for runtime bindings or hosts that serialize run state. Default in-memory library mode cannot promise cross-process orphan recovery.

**Recommendation:** split the contract clearly between pure recovery logic over host-provided snapshots and runtime startup sweep implementations over persisted state.

### 5. High: Phase 3/4 orphan recovery sequencing is inconsistent
The plan mostly defers persistent orphan recovery to Phase 4, but parts of Phase 3 still imply kill/restart or persisted orphan-recovery testing before the runtime layer is fully introduced. That sequencing blurs the architecture boundary and risks reintroducing storage assumptions too early.

**Recommendation:** keep Phase 3 focused on embedded/CLI harness behavior and replay; reserve end-to-end persisted orphan recovery for Phase 4.

### 6. Medium: Phase 1 still carries stale Postgres implications
Despite the current storage stance, some plan/kernel references still echo older database-centric assumptions during early phases. That makes the implementation sequence read less pure than the architecture intends.

**Recommendation:** remove early-phase wording that implies Postgres is architecturally canonical for the kernel path.

### 7. Medium: pgvector timing needs tighter framing
The broader architecture now emphasizes kernel purity and optional runtime bindings, so any vector-store or pgvector-oriented language should be treated carefully as runtime-specific and later-phase, not as a hidden prerequisite for v0 enforcement.

**Recommendation:** keep pgvector-related language explicitly out of the kernel baseline and scoped to later runtime or retrieval-specific needs only.

### 8. Medium: CLI sequencing needs clearer positioning
The CLI is described as an equal adapter or embedded harness in some places, but its sequencing relative to runtime/server behavior is not always crisp. That can make the reader infer that CLI persistence, sweeper behavior, and runtime parity all arrive together.

**Recommendation:** distinguish more sharply between Mode A embedded CLI/harness milestones and later runtime-surface parity work.

### 9. Medium: byte-identity and tamper-evidence claims need disciplined caveats
The thesis is directionally supported, but the strongest wording needs qualification. The docs already acknowledge no external notary, self-declared host conformance, and dependence on captured inputs and pinned deployment facts. Without those caveats, the report would overstate what “byte-identical” and “tamper-evident” mean in v0.

**Recommendation:** consistently phrase the claim as byte-identical deterministic portions for the same captured inputs and pinned deployment facts, with internally tamper-evident audit chains rather than universal external notarization.

## Overall judgment
The architecture remains substantially aligned with the thesis. The core shape is coherent: enforcement stays in the kernel, adapters are supposed to be thin, and persistence is meant to stay outside the kernel boundary. The main risk is documentary drift from older MCP-first and database-centric assumptions. Those inconsistencies do not overturn the thesis, but they do create avoidable ambiguity about the real enforcement boundary, determinism guarantees, and the scope of runtime persistence.

## Recommended next steps
1. Normalize the architecture docs around ADR-0010 and ADR-0011 language.
2. Resolve the normalizer determinism contradictions and stale registry wording.
3. Remove kernel-level persistence leakage and relocate concrete storage assumptions to runtime/storage docs.
4. Clarify orphan-recovery scoping by mode and fix the Phase 3/4 sequencing mismatch.
5. Tighten wording around byte-identity and tamper-evidence so claims match the documented caveats.
