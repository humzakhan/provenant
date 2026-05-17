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

## Critique Cycle 6 — 2026-05-17

**Issues raised:** 8
**VALID:** 8 —
- #1 storage.md schema-deltas section added for `tokens.source_value_raw`, `tokens.normalizer_applied`, `runs.credential_identity_baseline`, `runs.kernel_header_hash`, and the `audit_entries.payload_deterministic` / `payload_non_deterministic` / `hash_input_canonical` three-column split; Phase 1 tasks and done-when amended
- #2 Phase 3 done-when extended with silent-credential-rotation replay byte-identity test (one `EntryKindCredentialIdentityChanged` entry, byte-identical across two replays)
- #3 `provenant run inspect` live-state table specified (columns, primary key, lifecycle, restart sentinel, concurrency model, multi-replica scope) via sidecar `/internal/inspect` endpoint
- #4 Normalizer registration mechanism pinned to compile-time `kernel/internal/normalizers/registry.go` with `init()`-registered builtins; v0 registry empty; `normalizer_applied = null` chains survive future builtin additions; ADR-0006 cycle-6 amendment added
- #5 `reviewer_principal = "system:escalation-sweep"` sentinel for `decision = expired`; `"system:"` prefix reserved and IAM-forbidden for human reviewers; hitl.md, audit.md, and security.md updated
- #6 guards.md "Pipeline execution" subsection now maps `crashed`/`timeout` onto synthesized `Result{Passed: false, Errors: [...]}` with canonical messages; collect-all continues to subsequent validators, fail-fast aborts on the crashed/timed-out entry
- #7 Phase 0 §Tasks now ships `header_v1.go`, `EMPTY_CJSON_SHA256` constant, and per-`kernel_version` fixture files (the "ratify-and-pin" framing); Phase 0 §Deferred line about audit-chain-format finalization retracted; done-when items (d) `EMPTY_CJSON_SHA256` test and (e) header release-gate test enumerated explicitly
- #8 `credential_identity_witness` deterministic field added to `EntryKindToolCallStarted` (positive evidence per-call identity check ran); parallel-batch rotation rule pinned (one entry per affected `credential_id`, canonical lexicographic order by `credential_id`, lowest-`sibling_index_in_group` observed hash, single `at_step_index_be8`, flush before `EntryKindRunFailed`)

**INVALID:** 0
**TRADE-OFF:** 0

**Documents changed:** architecture/subsystems/storage.md, architecture/subsystems/audit.md, architecture/subsystems/guards.md, architecture/subsystems/skills.md, architecture/subsystems/hitl.md, architecture/cross-cutting/observability.md, architecture/cross-cutting/security.md, architecture/adrs/0006-skill-pack-format.md, implementation-plan.md, SESSION.md

**Outstanding (for cycle 7):**
- The `credential_identity_witness` truncates `sha256(credential_id || ":" || credential_identifier_hash_observed)` to 16 base32 chars (10 bytes). The witness is positive evidence the check ran but offers no second-preimage resistance at 80 bits — an attacker who controls a backend could in principle find a `credential_id, hash` pair colliding with a known witness. Acceptable for v0 (the witness is attestation, not authentication), but flagged in case a future critic argues the truncation should be tightened to 26 chars / 130 bits to match `tool_call_id`.
- The parallel-batch rotation rule emits one entry per affected `credential_id` taking the lowest-`sibling_index_in_group` observed hash. If two siblings in the same batch using the same `credential_id` observe **different** new identities (a rotation occurring mid-batch between sibling dispatches), the rule says "the batch is aborted on the first divergence" but does not specify which sibling's observation wins or whether the divergence itself becomes a `kernel_integrity`-class error. Phase 1/Phase 3 tests cover the single-identity-change case; the dual-rotation-mid-batch case is rare but unspecified. Flagged for cycle 7 — likely fix: treat mid-batch dual-rotation as `kernel_integrity` with the batch's smallest-index observation recorded for forensic purposes.
- The `audit_entries` three-column split (`payload_deterministic`, `payload_non_deterministic`, `hash_input_canonical`) is specified in storage.md but the storage→audit projection (the in-memory `AuditEntry.Payload` Go struct that `audit.md` describes as a single object with two sub-objects) is not yet codified in `subsystems/kernel.md`'s kernel-public types section. A subsequent cycle may want to lock the Go struct shape so independent storage adapters cannot disagree on the projection.
- The `provenant run inspect` SQL view is backed by an HTTP sidecar `/internal/inspect` endpoint, but the endpoint's auth model (admin bearer token? per-tenant? unauthenticated localhost-only?) is not specified in observability.md. Likely cycle-7 spec target.
- Normalizer registry is pinned to compile-time `init()` registration in `kernel/internal/normalizers/registry.go`, but the kernel public `Lookup` accessor's exact Go signature (does it return `(Normalizer, bool)` or `(Normalizer, error)`?) is not yet codified in `subsystems/kernel.md`'s public types section. Minor; flagged.

## Critique Cycle 7 — 2026-05-17

**Issues raised:** 13
**VALID:** 13 —
- #1 Parallel-batch credential rotation rule rewritten as deterministic sequential pre-dispatch identity-check phase ordered by canonical `sibling_index_in_group`; each `credential_id` resolved exactly once per batch, eliminating mid-batch dual-rotation ambiguity by construction; whole-batch abort with `failure_subcode = credential_rotation_mid_batch`
- #2 `AuditEntry` Go struct (`Seq`, `Kind`, `At`, `PrevHash`, `Hash`, `Payload`) and `PayloadEnvelope` (`Deterministic`, `NonDeterministic`, `HashInputCanonical`) codified in kernel.md "Kernel public types"; projection rule onto three-column persistence pinned; `hash_input_canonical = JCS(Deterministic)` verbatim; round-trip property added to Phase 0 done-when
- #3 `/internal/inspect` auth model pinned: localhost-only by default, per-process admin-inspect bearer token, tenant-scoped (404 on cross-tenant), separate platform-admin token for cross-tenant operator inspection with `inspect_access.log`
- #4 architecture.md §2 "Primary wire protocol" row renamed to "Wire protocols (adapters)" listing MCP/HTTP/CLI as equal first-class with ADR-0010 as primary citation (ADR-0007 noted as superseded)
- #5 + #13 `credential_id` optional field added to `tools.yaml` entries as the static manifest input to `credential_identity_baseline`; `ToolBackend.Credentials() []CredentialRef` runtime accessor added to tools.md; manifest is the deterministic decision rule for witness vs `"none"`; runtime/manifest mismatch is `kernel_integrity` with `error_code = credential_id_manifest_mismatch`
- #6 `Normalizer.Lookup` signature pinned to `(Normalizer, error)` in kernel.md; skills.md updated; convention split resolved in favor of `error` to match Store/AuditSink/TransactionalScope
- #7 Orphan-recovery integration test moved from Phase 1 done-when to Phase 3 done-when (requires harness + skill manifest + persisted `skill_id`/`skill_version`); Phase 1 retains orphan-detection unit tests with fake skill registry
- #8 `"system:"` prefix rule extended IAM-wide to all `*_principal` fields (`reviewer_principal`, `routing_principal`, `operator_principal_*`, `initiator_principal`) and `runs.initiator`; canonical sentinel vocabulary pinned: `system:escalation-sweep`, `system:orphan-sweep`, `system:dr-restore`, `system:audit-verify`; audit.md hitl_decision row updated to reference the table
- #9 Export-format projection for the cycle-6 three-column split pinned: `audit.csv` adds `payload_deterministic`/`payload_non_deterministic` columns; `audit.jsonl` emits `deterministic`/`non_deterministic` sub-objects per record; `audit.pdf` renders deterministic in primary table, non-deterministic in "diagnostic context" footer; `hash_input_canonical` excluded from all pack files
- #10 `credential_identity_witness` widened from 16 base32 chars (80 bits) to 26 base32 chars (~130 bits) to match `tool_call_id` entropy budget; cycle-6 outstanding "should the truncation be tightened" closed
- #11 KNOWN_TRADE_OFFS.md live-progress "Would revisit if" rewritten to reference OTLP `parallel_block` span and `provenant run inspect` (the actual cycle-5 mitigation surface) rather than the retracted Prometheus gauge
- #12 Phase 0 done-when extended with snapshot test asserting kernel public surface matches the kernel.md "Kernel public types" published list, and projection round-trip test covering every `EntryKind`
- #13 Same fix as #5: static `tools.yaml` `credential_id` field is the baseline source; baseline derived from static manifest, not runtime probes

**INVALID:** 0
**TRADE-OFF:** 0

**Documents changed:** architecture.md, implementation-plan.md, architecture/subsystems/audit.md, architecture/subsystems/kernel.md, architecture/subsystems/skills.md, architecture/subsystems/tools.md, architecture/cross-cutting/observability.md, architecture/cross-cutting/security.md, KNOWN_TRADE_OFFS.md, SESSION.md

**Outstanding (for cycle 8):**
- The `Credentials() []CredentialRef` method on `ToolBackend` returns `CredentialRef` whose `Provider` field is documented as non-deterministic ("lives only on the audit entry's non-deterministic sub-object"), but `EntryKindRunStarted.deterministic.credential_identity_baseline` only carries `credential_id → credential_identifier_hash`; the audit entry where `credential_id_provider` is recorded as non-deterministic is `EntryKindCredentialIdentityChanged.non_deterministic.credential_id_provider`. Is there a more general `credential_id → provider` recording site needed on the `run_started` non-deterministic sub-object so a reviewer can answer "which secret-store backed each credential on this run?" without needing a rotation to fire? Minor; flagged.
- The static `tools.yaml.credential_id` field is optional. A skill author who forgets to declare it on a credentialed tool (e.g., a new SAP backend rolled out by a v0.5 pack author) silently drops that credential out of `credential_identity_baseline` — the run starts, no rotation entry is emitted on a real rotation, and the chain reports "witness = none" for the credentialed tool. The pack-loader should reject a tool whose backend's `Credentials()` returns non-empty `CredentialRef` slices but whose `tools.yaml` entry omits `credential_id` — a typed `skill.credential_id_missing` error at pack-load time. Not yet codified in skills.md or implementation-plan.md; flagged.
- The `system:` sentinel vocabulary table in security.md is closed (kernel rejects any non-table sentinel), but the table itself is not enumerated as a kernel-public constant. A cross-language Phase 5 binding rejecting principals client-side would need this list. Minor; consider exporting as a kernel-public slice and snapshot-testing it.
- Phase 0's new public-surface snapshot test depends on a maintained list in `kernel/public/`; a future cycle may want this list to be **generated** from kernel.md's "Kernel public types" section (e.g., a markdown-table parser) rather than hand-maintained, so the published list and the snapshot list cannot drift.
- The export-format projection pinned in audit.md "Export-format projection" does not say how the PDF cover sheet renders the `host_conformance_attestation.mode = mode_b_untested` (already specified) **plus** the cycle-7-new `credential_identity_baseline` map. A reviewer of a regulator-facing audit pack may want a one-line "credentialed tools used on this run" summary on the cover sheet derived from the baseline. Cosmetic but flagged.

## Critique Cycle 8 — 2026-05-17

**Issues raised:** 3
**VALID:** 3 —
- #1 Tool interface name drift: tools.md declared `type Tool interface` while architecture.md §3.1/§5.1, implementation-plan.md Phase 0 public-surface snapshot test, and Phase 0/2 prose all referenced `ToolBackend`. Phase 0 snapshot test would have failed day one. Renamed `Tool` → `ToolBackend` in tools.md (Go type and a clarifying note on the heading); no other site needed updating because cycle 7 had already converged on `ToolBackend` everywhere else.
- #2 `EntryKindRunFailed.failure_subcode` was a deterministic hashed field with no enumerated set. Subcodes had been introduced ad-hoc across cycles (`credential_rotated`, `credential_identity_changed`, `credential_rotation_mid_batch`) and Phase 4/5 byte-identity tests would have failed the first time two implementations disagreed on a spelling. Added a pinned `failure_subcode` enum table in audit.md (`credential_rotated`, `credential_identity_changed`, `credential_rotation_mid_batch`, `transactional_scope_split`, `pack_digest_mismatch`, `kernel_header_mismatch`, `orphan_recovered`); pinned the snake_case naming rule (kebab-case reserved for skill-pack identifiers); declared the extension rule (new subcode = kernel_version bump + single-source-of-truth table append + boot-time enum-validator update).
- #3 `failure_class` name mismatch: audit.md row listed `integrity`; every other site (tool_call_failed.error_class, TransactionalScope check, skills.md, security.md, Phase 0 boot test, kernel.md) used `kernel_integrity`. Renamed audit.md run_failed.failure_class `integrity` → `kernel_integrity`. Found and fixed one additional bare-`integrity` reference in harness.md run-status table (`failed: integrity` → `failed: kernel_integrity`).

**INVALID:** 0
**TRADE-OFF:** 0

**Documents changed:** architecture/subsystems/tools.md, architecture/subsystems/audit.md, architecture/subsystems/harness.md, SESSION.md

**Outstanding (for cycle 9):**
- Cycle 7 outstanding items remain open (provider-on-`run_started`, `tools.yaml.credential_id` pack-loader rejection, `system:` sentinel as kernel-public constant, Phase 0 snapshot-list generation from kernel.md, PDF cover-sheet rendering of `credential_identity_baseline`).
- The `failure_subcode` enum table is now the source of truth, but no Phase 0/Phase 4 done-when explicitly asserts "kernel rejects an EntryKindRunFailed with a non-table `failure_subcode` at write time." A future cycle should bind this to a Phase 0 test (similar to the EntryKind write-rejection test) so the enum is enforced at build time, not just documented.

## Critique Cycle 9 — 2026-05-17

**Issues raised:** 3
**VALID:** 3 —
- #1 `tool_call_failed.error_message_canonical` referenced a `(error_class, error_code) → canonical string` mapping that did not exist; `error_code` values were scattered across audit/security/tools/kernel/skills with inconsistent naming (snake_case vs dotted). Added the canonical `error_code` table in audit.md as single source of truth with dotted-namespace convention (`credential.*`, `numeric.*`, `skill.*`, `tool.*`, `kernel.*`, `extract.*`, `policy.*`), the identity-mapping rule (`error_message_canonical == error_code` except for explicit parameterized templates), and write-time `AuditSink.Append` rejection of any unlisted code. Renamed non-conforming sites: `credential_id_manifest_mismatch` → `credential.id_manifest_mismatch` (tools.md, skills.md), `transactional_substrate_mismatch` → `kernel.transactional_substrate_mismatch` (kernel.md), `transactional_scope_split` → `kernel.transactional_scope_split` (kernel.md). Also pinned new codes for the cycle 7 outstanding gap (`skill.credential_id_missing`, `skill.credential_id_unexpected`).
- #2 `tools.yaml.credential_id` was optional with no load-time check — a credentialed-backend pack author forgetting it silently dropped the credential from the baseline. Updated skills.md "Loading lifecycle" step 3 to bind each entry to its `ToolBackend` and reconcile `credential_id` against `Credentials()`: empty-with-declared → `skill.credential_id_unexpected`; non-empty-without-declared → `skill.credential_id_missing`; mismatched ID → `credential.id_manifest_mismatch`. Rewrote the `credential_id` field rule as "iff backend `Credentials()` is non-empty." Updated implementation-plan.md Phase 2 tasks (the load-time reconciliation step) and Phase 2 done-when (three fixture-pack rejection tests).
- #3 Phase 0 done-when did not negative-test the write-time enumeration gates (`failure_subcode`, `EntryKind`, `error_code`). Added a single Phase 0 done-when item with three negative-tests over fixture entries asserting `AuditSink.Append` rejects out-of-enum values with `kernel_integrity` and no row written; closes the cycle 8 outstanding gap and binds the new cycle-9 `error_code` enum to a build-time test too.

**INVALID:** 0
**TRADE-OFF:** 0

**Documents changed:** architecture/subsystems/audit.md, architecture/subsystems/kernel.md, architecture/subsystems/tools.md, architecture/subsystems/skills.md, implementation-plan.md, SESSION.md

**Outstanding (for cycle 10):**
- The canonical `error_code` table claims a CI-diff-check against a generated `kernel/public/errorcode/codes.go`; the exact generation / check mechanism (markdown-table parser vs hand-maintained Go const block with a snapshot test) is not pinned. A future cycle may want to specify it concretely so Phase 0 ships the gate rather than just promising it.
- The new `skill.credential_id_unexpected` rejection rule means a future pack author who adds a `credential_id` "defensively" on a pure-Go tool will see a load-time failure they did not expect. The error message text and the skills.md author guidance do not yet explain this; cosmetic but flagged.
- Cycle 7 outstanding items that remain genuinely open: provider-on-`run_started` recording, `system:` sentinel as kernel-public constant, Phase 0 snapshot-list generation from kernel.md, PDF cover-sheet rendering of `credential_identity_baseline`. None are load-bearing; all are cosmetic / Phase 0+ ergonomics.

## Critique Cycle 10 — 2026-05-17

**Issues raised:** 1
**VALID:** 1 —
- #1 Markdown↔Go enum drift-prevention mechanism was asserted (`error_code` table claims a CI diff-check against generated `kernel/public/errorcode/codes.go`; the `EntryKind` and `failure_subcode` enums similarly hand-wave at "kernel boots its enumeration validators from a single generated Go const block") but the **direction of canonicality**, the **generator tool**, and the **gate timing** were not pinned. Resolved by (a) declaring markdown table as the single source of truth, (b) introducing `tools/enumgen/` as the markdown-canonical Go generator that parses the three tables in `audit.md` by exact markdown anchors and emits `kernel/public/errorcode/codes.go`, `kernel/public/audit/entrykind.go`, `kernel/public/audit/failuresubcode.go`, (c) pinning `go generate ./kernel/public/...` as the invocation contract with hermeticity requirement (no network, no module fetch), (d) pinning `go generate + git diff --exit-code` as the CI gate, (e) requiring a `tools/enumgen/enumgen_test.go` unit test that asserts generated Go matches a golden fixture against a fixture audit.md slice. Documented at audit.md §"Canonical `error_code` table" closing paragraph and §"Per-EntryKind payload schema" footer; new Phase 0 done-when bullet in implementation-plan.md.

**INVALID:** 0
**TRADE-OFF:** 0

**Documents changed:** architecture/subsystems/audit.md, implementation-plan.md, SESSION.md

## STABLE (reached at cycle 10, hard cap)

All VALID issues across cycles 1–10 resolved (73 total) or accepted as trade-offs (8 total). Cycle 10's single VALID issue (markdown↔Go enum drift-gate mechanism unpinned) was resolved within the same cycle by introducing tools/enumgen as the markdown-canonical generator and adding a `go generate + git diff --exit-code` Phase 0 done-when criterion. The cycle-10 fix did not receive a subsequent critic-verification pass because the 10-cycle hard cap was reached at this cycle. No unresolved VALID issues remain.

Documents ready for implementation.

