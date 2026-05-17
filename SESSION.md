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

## Critique Cycle 4 — 2026-05-17

**Issues raised:** 6
**VALID:** 4 —
- #1 `host_conformance_attestation.attested_at` replaced with content-addressed `attestation_doc_sha256` (sha256 of canonical-JSON of `host_conformance.yaml`); cross-mode byte-identity restored without removing the field from the deterministic sub-object
- #2 `TransactionalScope` added to kernel public surface; boot-time substrate-marker check + tx-local canary write enforce joint-transaction semantics on `Store` + `AuditSink`; Phase 0 done-when test added
- #4 Phase 3 done-when extended with `ModelClient` stream-refusal test and split-turn parallel independence test
- #5(a) Credential rotation mid-run defined as typed `credential.rotated` error class with documented run-abort failure path in `cross-cutting/security.md` and `tools.md` failure-mode table; Prometheus counter added
- #6 ADR-0009 (kernel public Go surface) and ADR-0010 (MCP/HTTP/CLI as equal first-class adapters) written; ADR-0001 amended with supersession header for the "Out of scope" Go-API clause; ADR-0007 marked superseded by ADR-0010; `architecture.md` §2.1.3 and §9 updated to reflect resolution

**INVALID:** 0
**TRADE-OFF:** 3 —
- #3 Locale-specific numeric forms (`1,5`, `1 234 567,89`, NBSP thousands) are not pre-canonicalized by the kernel; surface as typed `numeric.parse_error`. Per-backend skill-pack-shipped normalization shim is the documented mitigation. v0 ships only US/POSIX-locale tools.
- #5(b) Cross-deployment external-source version drift on fresh runs is not validated; v0 determinism contract is captured-deterministic replay, not cross-deployment fresh-run equivalence. Optional `external_source_pins` skill-manifest field deferred to v0.5+.
- #5(c) Case-bank `human_correction.after` has no kernel-minted-token provenance check; v0 single-operator deployments treat the field as advisory ground truth, with curator-flag mitigation and eval-time decoupling of `faithfulness` from `accuracy`. Multi-reviewer / hosted SaaS revisits this.

**Documents changed:** architecture.md, architecture/subsystems/audit.md, architecture/subsystems/tools.md, architecture/cross-cutting/security.md, architecture/adrs/0001-go-as-kernel-language.md, architecture/adrs/0007-mcp-as-primary-adapter.md, architecture/adrs/0009-kernel-public-go-surface.md (new), architecture/adrs/0010-mcp-equal-not-primary.md (new), implementation-plan.md, KNOWN_TRADE_OFFS.md, SESSION.md

**Outstanding (for cycle 5):**
- The `TransactionalScope` interface introduces a new kernel-public type whose `TransactionalSubstrate()` marker contract is not yet spec'd in `subsystems/kernel.md` (only in `architecture.md` §3.1 and `subsystems/audit.md`'s implementation note). A subsequent cycle may want the marker enum and the `WithTx` signature codified in `subsystems/kernel.md` as part of the kernel public types section, with a fixture showing a Postgres-backed Store + Postgres-backed AuditSink sharing one `*pgxpool.Pool` substrate.
- The `attestation_doc_sha256` empty-object constant (sha256 of canonical-JSON `{}`) is referenced as a "fixed constant" for Mode A and mode_b_untested but the actual hex value is not yet pinned in `audit.md`. A subsequent cycle may want to pin the literal hex value so a cross-language Phase 5 binding can hard-code the constant rather than recomputing.
- The credential-rotation typed-failure path is defined for tool-call detection of a rotated credential, but the **prevention** side (a credential being rotated by the secret store mid-run without any tool call observing the failure) is undefined: a run that completes between rotation and the next tool call uses the new credential identity for its post-rotation tool calls and the old one for pre-rotation, with no audit signal that the identity changed mid-run. May need a per-run credential-identity capture at run start + re-validation at each tool call.
- The case-bank `human_correction` trade-off documents `faithfulness` as kernel-grounded by construction, but the eval harness's `accuracy` metric still consumes the unverified `after` value. A future cycle may want a third eval metric — `correction_provenance_rate` — that reports the fraction of `human_correction.after` values that match a kernel-minted token from the original run, so a deployment can gate on "high-provenance corrections only" without losing the corrections that legitimately fix model omissions.
- ADR-0010 implies a conformance-suite expansion: all three adapters (MCP, HTTP, CLI) must produce byte-identical deterministic-portion chains. Phase 4 done-when covers MCP-vs-CLI and HTTP-vs-CLI pairwise but no explicit MCP-vs-HTTP pairwise test is named; may need to be added.

## Critique Cycle 5 — 2026-05-17

**Issues raised:** 10
**VALID:** 9 —
- #1 `TransactionalScope` Go interface + `WithTx` signature + `TransactionalSubstrate()` versioned marker grammar (`provenant.substrate/<vendor>/<engine>/v<n>`) codified in `subsystems/kernel.md` as source of truth; `architecture.md` §3.1 links to it
- #2 `EMPTY_CJSON_SHA256` pinned to literal hex `44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a` (sha256 of 2-byte `{}`); Phase 0 unit test asserts byte-identity; Mode A and `mode_b_untested` reference the named constant
- #3 Silent credential rotation closed via per-run `credential_identity_baseline` on `EntryKindRunStarted.deterministic`, re-validated at every tool call; new `EntryKindCredentialIdentityChanged` entry kind enumerated; harness aborts run on identity change
- #4 Phase 4 done-when extended with explicit MCP-vs-HTTP pairwise byte-identity test (transitivity through CLI no longer accepted as substitute)
- #5 Locale-numeric mitigation made implementable: `normalizers/` slot added to skill-pack format and ADR-0006 (cycle-5 amendment); v0 restricts to `kind: builtin` (kernel-registered IDs); slot is hashed into `skill_pack_hash`; normalizer firings audit-visible on `EntryKindTokenMint.deterministic.normalizer_applied`
- #6 Fifth eval metric (`correction_provenance_rate`) retracted; v0 remains 4-metric; case-bank trade-off mitigation now points at curator soft-warning as the v0 mechanism
- #7 `EntryKindGuardRun.action` enum extended to {`pass`, `redact`, `quarantine`, `block`, `crashed`, `timeout`}; deterministic `findings_canonical = []` for `crashed` and `timeout`; panic message in non-deterministic `error_message_raw`; wall-clock duration non-deterministic for `timeout`; guards.md failure-handling subsection updated to match
- #8 Cardinality-bomb Prometheus gauge retracted; per-run IDs forbidden as Prometheus labels (observability.md rule); replaced with OTLP `parallel_block` trace span + per-sibling span events (`sibling.started`/`sibling.completed`/`sibling.failed`) plus `provenant run inspect` SQL view; KNOWN_TRADE_OFFS.md live-progress mitigation rewritten
- #9 HITL REST surface aligned onto canonical `/v1/hitl/...` prefix; decision enum unified to `approved | edited | rejected | escalated | expired` across hitl.md and `EntryKindHITLDecision`; `edits_canonical` field added to the deterministic payload; implementation-plan.md Phase 4 tasks updated

**INVALID:** 0
**TRADE-OFF:** 1 —
- #10 Skill-pack extensibility deferred — no custom validators or sandboxed code loading in v0; v0 packs reference only kernel-registered built-in components; `normalizers/` slot is forward-compatible for v0.5+ `kind: wasm` extension

**Documents changed:** architecture.md, implementation-plan.md, architecture/subsystems/kernel.md, architecture/subsystems/audit.md, architecture/subsystems/guards.md, architecture/subsystems/skills.md, architecture/subsystems/hitl.md, architecture/cross-cutting/security.md, architecture/cross-cutting/observability.md, architecture/adrs/0006-skill-pack-format.md, KNOWN_TRADE_OFFS.md, SESSION.md

**Outstanding (for cycle 6):**
- `EntryKindCredentialIdentityChanged` adds a deterministic field `at_step_index_be8` whose semantics are clear in the new section but no test in the implementation plan asserts that two replays of a deterministically-detected silent rotation produce byte-identical entries. Phase 1 or Phase 3 done-when may want a per-tool credential-baseline re-validation test similar to the streaming-refusal test.
- The `parallel_block` OTLP span surface is defined in observability.md but the live `provenant run inspect <run_id>` SQL view is named only as "an in-memory ephemeral table"; its schema and lifecycle (when is the table populated, when is it GC'd after run completion, how does it survive process restart) are not specified. Likely a cycle-6 enumeration target.
- `normalizers/` slot is `kind: builtin` only in v0 with an empty built-in set. The first v0.5 backend that ships a non-empty normalizer (e.g., `numeric.locale.eu_comma_decimal`) will need a kernel-version bump regardless; the cycle-5 spec does not pin the registration mechanism for new built-in IDs (compile-time table vs. runtime registry). May want to lock this before Phase 2.
- HITL `expired` decision is emitted by the escalation-timeout sweep rather than by a reviewer call, but `EntryKindHITLDecision.non_deterministic.reviewer_principal` is not optional in the table; the sweep has no reviewer principal. Either the field needs to be optional / null-on-expiry, or a sentinel `reviewer_principal = "system:escalation-sweep"` rule is needed. Flagged for cycle 6.
- Guard `crashed`/`timeout` actions imply pipeline-decision-layer behavior ("treated as block") that is not separately enumerated in [`subsystems/guards.md`](architecture/subsystems/guards.md#pipeline-execution). The pipeline-execution subsection still describes `Result.Passed` and `Errors` only; it does not state how a `crashed` or `timeout` outcome maps onto `Result`. May need a small explicit table.
