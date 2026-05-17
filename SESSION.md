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

## Critique Cycle 2 — 2026-05-17

**Issues raised:** 10
**VALID:** 10 —
- #1 Per-EntryKind deterministic-vs-non-deterministic payload schema enumerated as source of truth
- #2 `exit_kind` (submit_answer | host_text_only) enumerated on `EntryKindRunCompleted`
- #3 `signature_marker` shape and kernel-immutable header text defined + conformance test
- #4 Orphan-run threshold made per-skill (per-manifest `max_wall_clock × orphan_multiplier`)
- #5 NUMERIC(38,9) canonicalization rule (parse grammar, banker's rounding, serialization, fixture vector) enumerated
- #6 Skill-pack digest procedure (file enumeration, exclusion list, symlink rejection, path normalization, length-prefixed concatenation, no newline normalization) enumerated
- #7 `restore_pending` / `restore_pending_acknowledged` added to `runs.status` enum and to `EntryKind` enum with state-transition map
- #8 Phase 2 done-when now reports `t_kernel_orchestration` (compared to 50 ms §7.3 budget) separately from `t_total_with_validators`
- #9 Parallel-sibling deterministic order rule (canonical-emission-order index + buffer-and-flush in kernel before audit-sink append) defined, with explicit replay-shuffle property test
- #10 `EntryKindToolCallFailed`, `EntryKindRunOrphanRecovered`, `EntryKindRestorePending`, `EntryKindRestorePendingAck` added to the EntryKind enumeration as source of truth

**INVALID:** 0
**TRADE-OFF:** 0
**Documents changed:** architecture.md, implementation-plan.md, architecture/subsystems/audit.md, architecture/subsystems/harness.md, architecture/cross-cutting/deployment.md, SESSION.md
**Outstanding:** The buffer-and-flush rule for parallel siblings introduces a new failure mode worth a Phase-1/Phase-2 test: process death during a parallel block leaves no entries from that block in the audit sink, and the orphan sweep is the only recovery path. The implementation-plan tests cover this transitively (orphan sweep + parallel-sibling shuffle property test) but no single integration test combines them. Flagged for the next critic pass as: "Is there a test that asserts a crash mid-parallel-batch is recovered as exactly one orphan-recovery entry, with no half-flushed sibling entries, on restart?" Also: the kernel-immutable header text is locked at `kernel/internal/promptkernel/header_v1.go`; any subsequent header revision is a kernel-version bump but its interaction with `skill_pack_hash` (which does not cover the kernel header) is one indirection the audit pack does not yet make verifiable — consider hashing kernel header text into `EntryKindRunStarted.deterministic.kernel_version` derivation or adding a `kernel_header_hash` deterministic field.

## Critique Cycle 3 — 2026-05-17

**Issues raised:** 10
**VALID:** 7 —
- #3 `kernel_header_hash` added to `EntryKindRunStarted.deterministic` + release-time content-hash test
- #5 Token mint atomicity with audit entry under parallel-batch (single-transaction flush; tokens buffered alongside audit entries)
- #6 Typed `numeric.overflow` / `numeric.underflow` / `numeric.parse_error` error classes; >38-significant-digit overflow and underflow-to-zero handling defined; fixture vector extended
- #8 `orphan_multiplier` pinned in skill manifest (`bounds.orphan_multiplier`); not deployment-overridable; cross-deployment byte-identity preserved on orphan-recovered runs
- #9 `source_value_raw` canonical normalization rule defined (preserves trailing-zero significance, sign-of-zero normalization, exponent elimination, overflow/underflow typed errors)
- #10 `parent_step_id` construction defined (`ps_` + base32 of `sha256(run_id || step_index_be8 || response_hash)`)
- #1+#2+#5 implementation-note tightening: parallel-block atomicity (tokens + audit entries in a single transaction at flush time)

**INVALID:** 0
**TRADE-OFF:** 3 —
- #1+#2 Streaming and split-turn parallel tool_use deferred (v0 is single-turn, non-streaming parallel only)
- #4 Live progress observability for buffered parallel blocks (Prometheus gauge surfaces sibling state live; audit chain remains buffered)
- #7 Host conformance attestation is self-declared in v0 (`host_conformance_attestation` field on `run_started.deterministic`; cryptographic / hosted-service attestation deferred)

**Documents changed:** architecture/subsystems/audit.md, architecture/subsystems/harness.md, architecture/subsystems/skills.md, implementation-plan.md, KNOWN_TRADE_OFFS.md, SESSION.md
**Outstanding:**
- The `host_conformance_attestation` field is deterministic but its content is operator-supplied at startup; two deployments of the same Mode-B host claiming different attestation versions will produce different chains. This is by design (the field captures a deployment-time fact), but it means "cross-mode byte-identity" is only guaranteed when `host_conformance_attestation` is held constant — flagged for cycle 4: is the cross-mode byte-identity claim in [`thesis.md`](thesis.md) and [`architecture.md`](architecture.md) §3 narrow enough to admit this, or do we need a "non-deterministic" classification for this field after all?
- The token-mint-atomicity-across-parallel-block guarantee assumes the `Store` and `AuditSink` share a transaction scope at flush time. Today's interface contracts ([`architecture.md`](architecture.md) §3.1) declare them as separate injected interfaces; a host that injects two stores backed by different databases would silently lose atomicity. Flagged for cycle 4: does the kernel public surface need a `TransactionalScope` interface that `Store` and `AuditSink` jointly implement, or is the contract that "embedding hosts MUST use a shared transactional substrate" sufficient documentation?
- Numeric `source_value_raw` rule rejects `1.` as malformed but accepts `1` and `1.0`. A backend that returns trailing-separator-only values (uncommon but legal in some JSON dialects) will surface as `numeric.parse_error` rather than normalizing. Acceptable for v0; flagged in case real backends are observed emitting this form.
