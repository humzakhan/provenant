# Session Log — provenant

> Append-only audit trail of every critique/defender cycle.
> Never edit entries retroactively. Each cycle is logged as it completes.

## Bootstrap

- Repository initialized with existing `architecture/` tree (subsystems, adapters, ADRs, cross-cutting, tool backends) on `main`.
- Hone branch `hone/cycle-loop` created.
- Pluggability goal carried into the spec: provenant must be embeddable at the **code level** (library/SDK) and runnable at the **runtime level** (sidecar/daemon/MCP).
- ADR-0001 (Go-as-kernel-language) and the current top-level structure are explicitly **on the table** for the critique loop.

## Critique Cycle 1 — 2026-05-17

**Issues raised:** 16
**VALID:** 15 —
- #1 Non-Go embedded-mode promise unspecified
- #2 Clock canonicalization for byte-identical audit chain across modes
- #3 Mode B non-`submit_answer` text — invariant scoping
- #4 50 ms `submit_answer` budget unmeasured before Phase 4
- #6 Phase 5 conflates FFI and sidecar — split with prerequisite gates
- #7 v0 HITL routing endpoints + state machine never shipped
- #8 Harness overhead not isolated from MCP transport overhead
- #9 NUMERIC(38,9) loses 10–12-decimal upstream precision; preserve raw
- #10 `VerifyToken` did not independently assert tenant match
- #11 Tool returning tokens contradicts §5.2; registration-time enforcement
- #12 Orphan-run recovery procedure undefined
- #13 Postgres / object-store cross-consistency on DR restore undefined
- #14 Parallel-tool failure semantics resolved (wait-all, no cancel)
- #15 No release_rate alarm — 0% release rate would look "fine"
- #16 Skill prompt not pinned by content digest in audit chain

**INVALID:** 0
**TRADE-OFF:** 1 —
- #5 Audit-pack tamper-evidence relies on internal hash chain only (no external notary in v0)
**Documents changed:** thesis.md, architecture.md, implementation-plan.md, architecture/subsystems/audit.md, architecture/subsystems/harness.md, architecture/cross-cutting/deployment.md, KNOWN_TRADE_OFFS.md
**Outstanding:** New tension may need a fresh architecture open question: "Which audit-entry fields are deterministic vs non-deterministic, and is the per-EntryKind schema declaring this surface enumerated anywhere?" Flagged for orchestrator / next critic pass. Also: the Mode B "exit_kind: host_text_only" path in `run_completed` is named in harness.md but not yet enumerated in `audit.md`'s payload schemas.
