# Architecture brief

## Current architecture
- **Core shape:** library-first kernel with thin access surfaces. The kernel is pure Go, no I/O at the public surface, and ships a first-class in-memory binding under `kernel/public/memio/` as the default. Storage is a deployment concern, not a kernel requirement.
  Ref: `architecture/OVERVIEW.md` §1, §3.1, §5.3, §7-8; `architecture/adrs/0011-storage-is-a-deployment-concern.md`.
- **Access surfaces:** MCP, HTTP, and CLI are equal first-class adapters; none is primary. CLI is Mode A (embedded harness/full enforcement). MCP/HTTP are Mode B shells over the same kernel public surface.
  Ref: `architecture/adrs/0010-mcp-equal-not-primary.md`; `architecture/adapters/{mcp,http,cli}.md`; `architecture/OVERVIEW.md` §3.3, §7.
- **Kernel contract:** Go-only public API, import-guarded against `net/*`, `database/sql`, MCP/HTTP/CLI types, and `os/exec`. Shared transaction semantics are enforced via `TransactionalScope` plus substrate markers.
  Ref: `architecture/adrs/0009-kernel-public-go-surface.md`; `architecture/subsystems/kernel.md` (TransactionalScope); `plans/implementation-plan.md` Phase 0.
- **Audit model:** per-tenant hash chain over canonical JSON. Deterministic and non-deterministic payloads are split; `audit.jsonl` is canonical, `audit.csv`/`audit.pdf`/`token-graph.json` are derived.
  Ref: `architecture/subsystems/audit.md` (Entry kinds, per-entry schema, hash construction, bundle.Verify).
- **Storage model:** runtime-layer reference bindings only (`runtime/sqlite/`, `runtime/postgres/`, `runtime/blobstore/s3/`). Library mode can stay fully in-memory; runtime persistence exists only for optional access surfaces / HITL persistence.
  Ref: `architecture/subsystems/storage.md`; `architecture/cross-cutting/deployment.md`; `architecture/OVERVIEW.md` §3.2, §7.

## Key assumptions
- **Byte-identity matters across modes.** Library, MCP, and CLI must produce byte-identical deterministic portions for the same input.
  Ref: `architecture/adrs/0011-storage-is-a-deployment-concern.md`; `architecture/adrs/0010-mcp-equal-not-primary.md`.
- **Kernel does not persist.** It emits typed values + bundle bytes; hosts own durability and cross-process HITL state.
  Ref: `architecture/adrs/0011-storage-is-a-deployment-concern.md` §Decision/Rationale; `architecture/subsystems/kernel.md` RunState.
- **Single-source enums/headers are pinned.** `kernel_header_hash`, `skill_pack_hash`, `EntryKind`, `failure_subcode`, and canonical `error_code` tables are spec-pinned and generated/validated from markdown.
  Ref: `architecture/subsystems/audit.md`; `architecture/subsystems/skills.md`; `plans/implementation-plan.md` Phase 0.
- **Security is kernel-enforced, not storage-enforced.** Tenant isolation, token verification, credential identity checks, and system sentinels are chain-visible invariants.
  Ref: `architecture/cross-cutting/security.md`; `architecture/subsystems/kernel.md`; `architecture/subsystems/audit.md`.

## Phase boundaries
- **Phase 0:** carve out pure kernel boundary, header/hash pinning, in-memory binding, import guard, enum generation.
  Ref: `plans/implementation-plan.md` Phase 0.
- **Phase 1:** in-memory audit chain + bundle emission/verification; orphan-run detection unit tests.
  Ref: `plans/implementation-plan.md` Phase 1; `architecture/subsystems/audit.md` bundle.Verify.
- **Phase 2:** tool registry + guards + submit pipeline + built-in normalizers; token minting and validator wiring.
  Ref: `plans/implementation-plan.md` Phase 2; `architecture/subsystems/tools.md`; `architecture/subsystems/guards.md`.
- **Phase 3:** Mode A harness/CLI with real model; orphan-recovery end-to-end is first fully exercised here in the plan.
  Ref: `plans/implementation-plan.md` Phase 3; `architecture/subsystems/harness.md`.
- **Phase 4:** runtime layer (MCP/HTTP/CLI parity), persistent SQLite/Postgres bindings, HITL routing, `audit.pdf`, and end-to-end orphan recovery against persisted state.
  Ref: `plans/implementation-plan.md` Phase 4; `architecture/cross-cutting/deployment.md`; `architecture/adapters/*.md`.
- **Phase 5:** non-Go binding (FFI or sidecar) only after Phase 4 ergonomics/latency evidence.
  Ref: `plans/implementation-plan.md` Phase 5; `architecture/adrs/0009-kernel-public-go-surface.md`.

## Where sweeper/orphan-recovery assumptions matter
- **Threshold is per skill, not deployment-wide.** Effective orphan threshold = `bounds.max_wall_clock_seconds × bounds.orphan_multiplier`; `orphan_multiplier` is pinned in the skill manifest and not deployment-overridable.
  Ref: `architecture/subsystems/harness.md` “Orphan-run threshold (per skill)”; `architecture/subsystems/skills.md` bounds; `architecture/subsystems/audit.md` `run_orphan_recovered`.
- **Recovery is chain-visible and status-driven.** Startup sweep finalizes stale `pending/running` runs as `failed: orphaned`, emitting `EntryKindRunOrphanRecovered` before `EntryKindRunFailed`; `restore_pending` / `restore_pending_acknowledged` are only reachable from terminal states.
  Ref: `architecture/subsystems/harness.md` state transitions; `architecture/subsystems/audit.md` `run_failed.failure_subcode=orphaned`; `architecture/cross-cutting/deployment.md` DR section.
- **Cross-run / cross-process persistence boundary.** Library mode does not own cross-process HITL recovery; the host must serialize `RunState`. Runtime mode persists the state in SQLite/Postgres.
  Ref: `architecture/adrs/0011-storage-is-a-deployment-concern.md`; `architecture/subsystems/kernel.md` `RunState`; `architecture/cross-cutting/deployment.md` library vs runtime modes.
- **Atomicity is load-bearing.** Parallel-block token mint + audit flush must be transactional; a crash mid-flush recovers as orphaned, not half-persisted.
  Ref: `architecture/subsystems/audit.md` “Parallel-sibling deterministic order”; `architecture/subsystems/kernel.md` `TransactionalScope`; `plans/implementation-plan.md` Phase 0/1/3/4 tests.
- **Security sentinel namespace touches sweeper paths.** `system:orphan-sweep` is reserved for orphan-recovery initiated principals; related audit fields must remain in the reserved namespace.
  Ref: `architecture/cross-cutting/security.md` principal namespace; `architecture/subsystems/audit.md` canonical sentinels.

## High-signal references
- `architecture/OVERVIEW.md` — synthesis view, module boundary, deployment model.
- `architecture/README.md` — core invariants and doc map.
- `architecture/subsystems/kernel.md` — public types, `TransactionalScope`, `RunState`.
- `architecture/subsystems/audit.md` — chain format, `bundle.Verify`, orphan-recovery entry kinds.
- `architecture/subsystems/harness.md` — state transitions, orphan threshold, Mode B caveats.
- `architecture/subsystems/storage.md` — runtime bindings and DR posture.
- `architecture/cross-cutting/security.md` — tenant isolation, credential identity, `system:` sentinels.
- `architecture/cross-cutting/deployment.md` — library/MCP/CLI deployment shapes and restore-verify.
- `architecture/adrs/0009-kernel-public-go-surface.md`, `0010-mcp-equal-not-primary.md`, `0011-storage-is-a-deployment-concern.md`, `0008-postgres-as-canonical-store.md` (superseded context).
- `plans/implementation-plan.md` — phase split and done-when criteria.
