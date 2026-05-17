# Subsystem: Audit Logger

Status: **v0 / load-bearing**

> Every Provenant run produces an append-only, hash-chained audit log. The log is the artifact regulators ask for. The chain hash makes tampering detectable. The `provenant audit pack` command turns a single run into a regulator-ready bundle in one command.

## Depends on
- [`subsystems/storage.md`](storage.md) — Postgres + object store

## Depended on by
- All other subsystems write to it
- [`subsystems/eval.md`](eval.md) — replay reads from it
- [`adapters/cli.md`](../adapters/cli.md) — `audit` and `audit pack` subcommands
- Compliance buyers and auditors

## Responsibilities

1. Append every consequential event to a hash-chained log.
2. Verify chain integrity on demand.
3. Export logs in formats regulators accept.
4. Bundle related artifacts into audit packs.

## Why this design

Two regulatory requirements drive the design:

- **[FINRA 2026](../references.md#finra-2026)**: firms must demonstrate how an agent reached a decision. This requires a complete, tamper-evident record.
- **[SR 11-7](../references.md#sr-11-7)**: model risk management requires documented purpose, validation, monitoring, override. Provenant's audit log is the documented evidence trail.

The chain-hash design follows convergent industry practice (Moody's, Brex, every regulated AI vendor) and is necessary for the customer's internal audit team to certify the system. Tamper detection is not optional in finance.

## Entry kinds

```go
type EntryKind string
const (
    EntryKindRunStarted        EntryKind = "run_started"
    EntryKindPromptSent        EntryKind = "prompt_sent"
    EntryKindModelResponse     EntryKind = "model_response"
    EntryKindToolCallStarted   EntryKind = "tool_call_started"
    EntryKindToolCallCompleted EntryKind = "tool_call_completed"
    EntryKindToolCallFailed    EntryKind = "tool_call_failed"     // sibling-failure in parallel dispatch, or single-tool failure
    EntryKindTokenMint         EntryKind = "token_mint"
    EntryKindGuardRun          EntryKind = "guard_run"
    EntryKindSubmitAttempt     EntryKind = "submit_attempt"
    EntryKindSubmitRejected    EntryKind = "submit_rejected"
    EntryKindAnswerReleased    EntryKind = "answer_released"
    EntryKindHITLDispatched    EntryKind = "hitl_dispatched"
    EntryKindHITLDecision      EntryKind = "hitl_decision"
    EntryKindRunCompleted      EntryKind = "run_completed"
    EntryKindRunFailed         EntryKind = "run_failed"
    EntryKindRunOrphanRecovered EntryKind = "run_orphan_recovered" // emitted by the orphan-run sweep (Phase 1)
    EntryKindRestorePending    EntryKind = "restore_pending"      // DR restore-verify found missing blob refs
    EntryKindRestoreAck        EntryKind = "restore_pending_ack"  // operator acknowledges a partial-loss gap
)

This enumeration is the **single source of truth** for the audit-entry kind set. Exporters (audit pack, SIEM exporter, CSV walkthrough, signed-PDF generator) key on this list; an entry whose `kind` is not in the enum is rejected at write time, not silently dropped. New kinds require an enum extension plus an entry in the per-kind payload schema table (next section).
```

Every entry captures: the kind, the run ID, a structured payload (the event-specific data), a previous-hash, the computed hash of this entry, and a timestamp.

## Entry schema

```sql
CREATE TABLE audit_entries (
    seq         BIGSERIAL PRIMARY KEY,           -- monotonic per tenant; see below
    run_id      TEXT NOT NULL REFERENCES runs(id),
    tenant_id   TEXT NOT NULL,
    kind        TEXT NOT NULL,
    payload     JSONB NOT NULL,
    prev_hash   TEXT NOT NULL,
    hash        TEXT NOT NULL,
    at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_run ON audit_entries(run_id, seq);
CREATE INDEX idx_audit_tenant_seq ON audit_entries(tenant_id, seq);
```

`seq` is monotonic per tenant (managed by a per-tenant counter table or a tenant-scoped sequence). This bounds chain traversal cost per tenant.

## Hash construction

Each entry's `hash` is:

```
hash = sha256(prev_hash || seq_bytes || run_id || tenant_id || kind || canonical(payload))
```

Where:

- `canonical(payload)` is JCS (RFC 8785) canonical JSON.
- The first entry per tenant has `prev_hash = sha256("provenant-genesis:" || tenant_id)`.
- **Wall-clock timestamps are excluded from the hash.** `at` is stored on every row but is not part of the deterministic hash chain. It is recorded for human auditors and operational queries only, and is classified as a non-deterministic-portion field. This is what makes the chain byte-identical between embedded mode (host-injected `Clock`, e.g., a Python event-loop monotonic clock) and runtime mode (daemon wall clock). The deterministic portion of the chain consists of `(prev_hash, seq, run_id, tenant_id, kind, canonical(payload))`; the non-deterministic portion is `at` plus any payload field explicitly marked `non_deterministic` (model latencies, retry counters, host-clock readings).
- Payload fields that would otherwise inject clock variance (e.g., `model_response.latency_ms`, `tool_call.duration_ms`) are emitted under a reserved `non_deterministic` sub-object that the canonicalizer excludes from the hash input. The schema for each `EntryKind` declares which fields are deterministic.

Verification walks the tenant's chain from genesis, recomputing each `hash` and comparing to the stored value. Any mismatch surfaces the corrupted entry.

### Per-EntryKind payload schema (deterministic-surface enumeration)

Every `EntryKind` declares two payload sub-objects: `deterministic` (hashed) and `non_deterministic` (stored but excluded from the hash input). The kernel rejects an entry whose payload contains a field outside the declared schema for its kind. This is the source-of-truth referenced from [`harness.md`](harness.md), [`kernel.md`](kernel.md), and [`storage.md`](storage.md); cross-mode byte-identity is defined as equality on the `deterministic` sub-object set across all entries in the run, in canonical order.

| EntryKind | Deterministic fields | Non-deterministic fields |
|---|---|---|
| `run_started` | `skill_id`, `skill_version`, `skill_pack_hash` (see §6 below), `model_id`, `input_hash`, `tenant_id` (mirrored), `bounds`{`max_steps`,`max_wall_clock_seconds`,`max_cost_usd`}, `kernel_version`, `kernel_header_hash` (sha256 of the kernel-immutable header text bytes loaded from `kernel/internal/promptkernel/header_v<N>.go` for the running kernel — see [`harness.md`](harness.md#kernel-immutable-header-text-definition); guards against header-file drift vs kernel_version), `host_conformance_attestation` (see below) | `initiator_principal`, `host_clock_offset_ms` |
| `prompt_sent` | `prompt_hash` (sha256 of canonical-JSON prompt envelope), `prompt_blob_ref`, `message_count`, `tool_def_hash` (sha256 of canonicalized tool-def list emitted to model) | `prompt_bytes`, `tokenizer_estimate` |
| `model_response` | `response_hash` (sha256 of canonicalized response envelope), `response_blob_ref`, `stop_reason`, `tool_use_count`, `tokens_in`, `tokens_out` | `latency_ms`, `model_request_id`, `retry_count`, `cost_usd` |
| `tool_call_started` | `tool_call_id` (kernel-assigned ULID; deterministic per replay — see §"Tool-call ID and seq determinism"), `tool_name`, `tool_version`, `input_hash`, `input_blob_ref`, `parent_step_id` (deterministic identifier of the model-response step that emitted this tool_use — see §"parent_step_id construction"), `sibling_group_id` (set when emitted from a parallel-dispatch batch), `sibling_index_in_group` (canonical 0-based index assigned at batch start — see §"Parallel-sibling deterministic order") | `started_at_wall` |
| `tool_call_completed` | `tool_call_id`, `output_hash`, `output_blob_ref`, `mintable_paths_hit` (sorted list), `result_status` ("ok") | `duration_ms`, `backend_endpoint` |
| `tool_call_failed` | `tool_call_id`, `error_class` (kernel-defined enum: `schema_mismatch`, `guard_rejection`, `backend_error`, `timeout`, `kernel_integrity`), `error_message_canonical` (a kernel-canonicalized message — backend-error messages are mapped through a `(error_class, error_code)` → canonical string table; raw backend text is stored only in the non-deterministic sub-object) | `error_message_raw`, `duration_ms`, `retry_count` |
| `token_mint` | `token_id` (kernel-assigned ULID — deterministic per replay), `source_tool_call_id`, `source_path`, `value_canonical` (the `NUMERIC(38,9)` decimal serialized per the canonicalization rule in [`architecture.md`](../../architecture.md) §4), `source_value_raw` (the verbatim string the upstream backend returned, after the canonical `source_value_raw` normalization rule in §"`source_value_raw` canonical form" below — required so two backends that return semantically-equal but textually-different forms (`1.00` vs `1.0` vs `1`) produce byte-identical chains), `unit`, `derivation`{`parents`,`op`} | — |
| `guard_run` | `guard_name`, `guard_version`, `phase` (`pre_llm` \| `post_llm`), `severity_tier`, `action` (`pass` \| `redact` \| `quarantine` \| `block`), `target_hash` (hash of the input being guarded), `findings_canonical` | `duration_ms` |
| `submit_attempt` | `payload_hash`, `payload_blob_ref`, `numeric_claim_count` | — |
| `submit_rejected` | `payload_hash`, `validator_errors_canonical` (sorted by `(validator_name, claim_index)`) | `duration_ms` |
| `answer_released` | `payload_hash`, `signature_marker` (see [`harness.md`](harness.md#mode-b-specifics-adapter-side-harness)), `claim_to_token_map_hash` | — |
| `hitl_dispatched` | `review_id`, `routing_reason`, `confidence_bucket` | `dispatched_at_wall`, `routing_principal` |
| `hitl_decision` | `review_id`, `decision` (`approved` \| `rejected` \| `escalated` \| `expired`), `decision_rationale_hash` | `decided_at_wall`, `reviewer_principal` |
| `run_completed` | `final_status` (`completed` \| `awaiting_review`), `exit_kind` (`submit_answer` \| `host_text_only`), `final_payload_hash` (set only when `exit_kind == submit_answer`; null otherwise) | `ended_at_wall`, `total_duration_ms`, `total_cost_usd` |
| `run_failed` | `failure_class` (`exhausted` \| `timeout` \| `cost_cap` \| `integrity` \| `model_error` \| `aborted` \| `orphaned`), `failure_subcode` | `ended_at_wall`, `error_message_raw` |
| `run_orphan_recovered` | `prior_status` (`pending` \| `running`), `last_entry_seq_before_recovery`, `threshold_seconds_used` (the per-skill `max_wall_clock_seconds × orphan_multiplier` actually applied; `orphan_multiplier` is pinned in the **skill manifest** (`bounds.orphan_multiplier`, default 2) and is **not** deployment-overridable — see [`harness.md`](harness.md#orphan-run-threshold-per-skill) and [`skills.md`](skills.md); two deployments running the same `(skill_id, skill_version)` therefore compute the same `threshold_seconds_used` and the field remains in the deterministic sub-object), `recovery_reason` | `swept_at_wall` |
| `restore_pending` | `missing_blob_refs_canonical` (sorted list) | `verified_at_wall` |
| `restore_pending_ack` | `acknowledged_blob_refs_canonical`, `operator_principal_hash` | `acknowledged_at_wall`, `operator_principal_raw` |

`exit_kind == host_text_only` is the only legal value for runs whose final answer left the substrate without going through `submit_answer` (Mode B specifics in [`harness.md`](harness.md)). When `exit_kind == host_text_only`, `final_payload_hash` is the JSON null value (not absent, not the empty string) and `signature_marker` is **not** emitted on the chain — its absence is the audit-pack-visible signal that the run produced no Provenant-attested output.

### Tool-call ID and seq determinism

`tool_call_id` is a deterministic kernel-assigned identifier, **not** a fresh ULID drawn from wall clock. It is computed at dispatch time as:

```
tool_call_id = "tc_" || base32( sha256( run_id || parent_step_id || sibling_group_id || sibling_index_in_group ) )[0:26]
```

This makes the ID a pure function of the run's deterministic trajectory; replay produces the same `tool_call_id` for the same call in the same position.

`seq` is assigned by the audit sink at append time. To keep `seq` deterministic across replays in the face of parallel sibling tool dispatch (the original failure mode raised by cycle-2 critic #9), the harness does **not** stream parallel-sibling entries to the audit sink in real-wall-clock completion order. Instead:

### Parallel-sibling deterministic order

When the model emits a batch of N parallel tool_use blocks in a single response, the harness:

1. Assigns a `sibling_group_id` to the batch: `"sg_" || base32( sha256( run_id || parent_step_id || canonical(tool_use_blocks_in_emission_order) ) )[0:26]`.
2. Assigns each sibling a canonical `sibling_index_in_group` (0-based) by sorting the batch **by the model's emission order** as it appears in the canonicalized model response (`model_response.response_hash`'s input). Emission order is a deterministic property of the canonicalized response envelope; real-wall-clock dispatch order does not affect the index.
3. Dispatches the siblings concurrently (real-wall-clock parallelism preserved for latency).
4. **Buffers** every `tool_call_started`, `tool_call_completed`, `tool_call_failed`, and `token_mint` entry produced by the batch in a per-group buffer. No entries are appended to the audit sink during the parallel block.
5. Once all siblings have completed (the wait-all semantics from [`harness.md`](harness.md#tool-dispatch)), the harness flushes the buffer to the audit sink in canonical order: ascending `sibling_index_in_group`, and within a sibling, ascending intra-sibling sub-step order (`tool_call_started` → 0..N `token_mint` entries in `mintable_paths`-sorted order → `tool_call_completed` or `tool_call_failed`).
6. The audit sink assigns `seq` strictly in flush order; this is the only point where `seq` is materialized.

This rule is load-bearing for the Phase 4 byte-identity done-when: without it, two replays of the same run whose siblings happen to finish in different wall-clock orders would diverge from the first sibling's `seq` onward, breaking every subsequent `prev_hash`. The buffering window is bounded by `max_wall_clock` (the same bound that already caps the parallel dispatch), so no new failure mode is introduced.

Implementation note (atomicity of token mint with audit): `kernel.Mint` does **not** persist `tokens` rows during a parallel block. Token records produced by sibling tool calls are held in the same per-group in-memory buffer that holds the `tool_call_*` and `token_mint` audit entries; at flush time, the `tokens` rows and the `audit_entries` rows are written in a **single transaction** (per-tenant advisory-lock-scoped) so the invariant "every persisted token has a corresponding `EntryKindTokenMint` audit entry" holds even on crash. If the process crashes during a parallel block, neither the tokens nor the audit entries from that block are persisted (atomicity); the run is recovered as orphaned per the Phase-1 sweep — there is no "half-flushed batch" state, and no orphan `tokens.source_call_id` pointing at a tool_call that was never audit-logged.

The single-transaction guarantee is honored via the kernel-public `TransactionalScope` interface (see [`architecture.md`](../../architecture.md) §3.1). The kernel does not trust `Store` and `AuditSink` to share a transaction implicitly: hosts that inject separate substrates for the two interfaces are rejected at kernel boot. Phase 0 ships a test that injects an in-memory `Store` and an in-memory `AuditSink` whose `TransactionalSubstrate()` markers deliberately disagree and asserts the kernel refuses to start. Phase 1 ships a test that uses a `TransactionalScope` whose `WithTx` commits the two halves separately and asserts the kernel detects the violation via the tx-local canary and fails closed with `kernel_integrity`.

Scope and constraints (v0):

- **Single-turn parallel only.** The buffer-and-flush rule applies to N parallel `tool_use` blocks emitted in **a single model response turn**. If a model splits parallel tool-use emissions across multiple response turns (a behavior some Claude variants exhibit under load), the kernel treats each turn's tool_use blocks as an independent `sibling_group_id` derived from that turn's canonicalized response envelope. The kernel does **not** attempt to retroactively merge tool-use blocks across turns into one sibling group; doing so would make `sibling_group_id` depend on a non-local future-turn observation and break byte-identity on replay.
- **Streaming tool responses are not permitted in v0.** The intra-sibling ordering invariant (a sibling's `tool_call_started` precedes its `token_mint` entries which precede its `tool_call_completed` / `tool_call_failed`) presupposes a fully-formed `tool_use` block at dispatch time. Streaming tool_use is listed in the Open Questions of [`harness.md`](harness.md) and is **out of scope for v0**; see KNOWN_TRADE_OFFS.md.

### `parent_step_id` construction

`parent_step_id` is the deterministic identifier of the model-response step from which a tool_use block was emitted. It is computed at the time the kernel ingests the model response and is the same for every sibling in a parallel batch:

```
parent_step_id = "ps_" || base32( sha256( run_id || step_index_be8 || response_hash ) )[0:26]
```

Where:

- `step_index_be8` is the 0-based ordinal of the model-response step within the run, big-endian 8 bytes. The first model response in a run has `step_index = 0`.
- `response_hash` is the deterministic `model_response.response_hash` (sha256 of the canonicalized model response envelope) recorded on the `EntryKindModelResponse` entry produced for this step.

`step_index` is part of the kernel's per-run state and is incremented exactly when the harness records an `EntryKindModelResponse` entry. The first tool_use batch's `parent_step_id` therefore derives from `step_index=0` plus the first response envelope's hash; persistence of `step_index` across crashes is implicit because the audit chain (which is the source of truth) is replayable to recover it.

This makes `parent_step_id` (a) deterministic per replay, (b) sibling-shared within a turn, (c) independent across turns even when the model emits two parallel batches in two response turns, and (d) recoverable from the audit chain without a separate counter table. Independent implementations (Phase 5 bindings) deriving `tool_call_id` will produce identical IDs given identical canonicalized response bytes.

### `source_value_raw` canonical form

`source_value_raw` exists to preserve upstream precision (e.g., 10–12-decimal FX rates, raw extraction confidences) for provenance answers. To make byte-identity hold across backends and language bindings that return semantically-equal but textually-different numeric forms (`1.00` vs `1.0` vs `1`, `1e3` vs `1000`), the kernel normalizes `source_value_raw` at mint time using the following rule before it is hashed into the chain:

1. **Parse.** Reject inputs that do not match the canonicalization grammar in §"Numeric canonicalization" above; raise a typed `numeric.parse_error`.
2. **Re-serialize verbatim-significant.** The normalized form is the unique shortest decimal string that preserves the input's significant digits and scale, computed as:
   - Strip a leading `+`; preserve a leading `-` unless the value is exactly zero (in which case the sign is normalized to absent).
   - Eliminate exponent form: convert `[mantissa]e[exp]` to plain decimal by shifting the decimal point. Reject inputs whose absolute exponent after parsing would require more than **38 total significant digits** to represent (typed `numeric.overflow`) or whose magnitude is non-zero but rounds to zero at the kernel's representable scale of 1e-38 (typed `numeric.underflow`). These error classes are first-class members of the kernel error enum and are returned to the agent as `tool_call_failed.error_class = backend_error` with `error_code` set to `numeric.overflow` / `numeric.underflow` / `numeric.parse_error`.
   - No leading zeros on the integer part except the single `0` for values in `(-1, 1)`.
   - Preserve trailing zeros that were **present in the input** as significant trailing zeros (`1.00` → `1.00`); do **not** strip them. This is the rule that disambiguates "the upstream said 1.00 (cents precision)" from "the upstream said 1.0 (tenths precision)". Inputs without a fractional separator (`1`) normalize to no fractional separator (`1`); inputs with a separator and zero fractional digits (`1.`) are rejected as malformed.
   - `-0`, `-0.0`, etc. normalize to `0`, `0.0`, etc. (sign dropped, scale preserved).

The result is recorded on the token row's `source_value_raw TEXT` column (already declared in [`architecture.md`](../../architecture.md) §4) and on `EntryKindTokenMint.deterministic.source_value_raw`. Both Go (`shopspring/decimal` with a thin wrapper that retains scale) and Python (`decimal.Decimal` with explicit scale handling) bindings produce identical normalized strings given identical inputs; the cross-language fixture vector under `kernel/testdata/numeric-canonical/` is extended to cover the trailing-zero, overflow, underflow, and sign-of-zero cases.

`value_canonical` (the `NUMERIC(38,9)` hashed value used for arithmetic and verification) is unchanged by this rule; `source_value_raw` is the verbatim-precision sibling field, and they may differ when the upstream value carries more than 9 fractional digits.

### `host_conformance_attestation`

In Mode B (host-driven loop), runs may be initiated by a third-party host (Claude.ai, Cursor, a custom MCP host). The kernel cannot independently verify that the host runs the published `provenant mcp-conformance` suite; absence of attestation is not itself enforceable. The `host_conformance_attestation` field on `EntryKindRunStarted.deterministic` records what the host operator has declared at runtime-initialization time:

```
host_conformance_attestation = {
  "mode": "mode_a" | "mode_b_attested" | "mode_b_untested",
  "host_id": "<opaque string the host operator selects, e.g., 'claude-code-0.50.0'>",
  "attestation_version": "<semver of the conformance suite the host claims to have passed, or empty string for mode_a / mode_b_untested>",
  "attestation_doc_sha256": "<hex sha256 of the canonical-JSON-encoded `host_conformance.yaml` the operator most recently passed the suite against; empty for mode_b_untested>"
}
```

Note on cross-mode byte-identity (cycle 4 fix): `attested_at` was previously an RFC3339 wall-clock timestamp loaded at deploy time, which made the field deterministic but caused Mode A and Mode B same-input runs to produce divergent first-entry hashes whenever the operator re-passed the conformance suite at different wall-clock times. `attestation_doc_sha256` is content-addressed instead: two deployments that ran the same conformance suite against the same `host_conformance.yaml` (same suite version, same declared host facts) produce byte-identical chains regardless of when the suite was last run. The operator's "when did I last pass" wall-clock timestamp remains observable, but it lives in the non-deterministic sub-object (`host_clock_offset_ms` adjacent), not in the hashed chain.

- For Mode A (`provenant run`, embedded Go harness): the kernel fills this in itself with `mode = mode_a`, the kernel's own version as `attestation_version`, and `attestation_doc_sha256` set to the sha256 of an empty canonical-JSON object `{}` (Mode A has no operator-supplied conformance document; the empty-object hash is a fixed constant). The operator cannot override.
- For Mode B with attestation: the host operator configures `mode`, `host_id`, `attestation_version` and provides a `host_conformance.yaml` referenced by the Provenant config; the runtime layer computes `attestation_doc_sha256 = sha256(canonical-JSON(host_conformance.yaml))` once at startup and copies the four fields verbatim into every `run_started` entry. The kernel does not validate the claim — the field is a self-declared, audit-pack-visible attestation, signed only by virtue of being on the per-tenant hash chain. Cross-mode byte-identity holds: re-passing the suite at a different wall-clock without changing the document content does not change the chain hash.
- For Mode B without attestation: the kernel writes `mode = mode_b_untested` automatically when no `host_conformance.yaml` is loaded, with `attestation_doc_sha256` set to the empty-object hash constant.

`provenant audit pack` displays the attestation status prominently on the pack README and on `audit.pdf` cover sheet, so a downstream reviewer can distinguish a Mode-A run, a Mode-B run with a self-declared conformance pass, and a Mode-B run with no attestation. Stronger attestation (operator-signed JWT, hosted Provenant attestation service) is deferred to v0.5+ and tracked in KNOWN_TRADE_OFFS.md.

### Numeric canonicalization (cross-language replay)

The `value_canonical` field on `token_mint` and every other `NUMERIC(38,9)` field in the chain is serialized through a single canonical rule, enumerated here so cross-language replay (Phase 5 Python or other binding) produces byte-identical chains:

1. **Parsing.** Decimal source values are parsed with the locale-independent grammar `^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`. No locale-specific separators, no thousand separators, no surrounding whitespace. Inputs that do not match are rejected at mint time with a typed `numeric.parse_error`.
2. **Scale.** All values are scaled to exactly 9 fractional digits.
3. **Rounding mode.** Banker's rounding (IEEE 754 `roundTiesToEven` applied to the decimal at the 9-decimal boundary). Tie-break is always to even at the 1e-9 position, regardless of sign.
4. **Serialization.** The serialized form is a plain decimal string: optional leading minus, integer part (no leading zeros except the single `0` for values in `(-1, 1)`), the literal `.`, exactly 9 fractional digits (zero-padded). No exponent form; no trailing zeros stripped; no `+` sign. `0` is serialized as `0.000000000`. `-0` is normalized to `0.000000000`.

The Go reference implementation is `shopspring/decimal` with `RoundBank(9)` and `String()` followed by zero-padding to 9 fractional digits. The Phase 5 binding test compares byte-for-byte against this reference on a cross-language fixture vector of half-way values (e.g., `1.0000000005` → `1.000000000`; `1.0000000015` → `1.000000002`; `-0.5e-9` → `0.000000000`) shipped under `kernel/testdata/numeric-canonical/`.

### Skill-pack content digest (procedure)

The `skill_pack_hash` recorded in the `run_started` entry (see [`architecture.md`](../../architecture.md) §6) is computed as:

1. **Collection.** Enumerate every regular file under the pack root. Symlinks are not followed and are not included; the loader rejects packs containing symlinks. Files matching the pack's `.provenant-ignore` (if present) are excluded; the default exclusion list, applied even when `.provenant-ignore` is absent, is: `.DS_Store`, `Thumbs.db`, `.git/`, `.gitignore`, `.gitattributes`, files matching `*.swp`, `*.swo`, `*~`, `.#*`, `#*#`. The exclusion list is part of this spec; loader changes that alter it require a kernel-version bump.
2. **Path normalization.** Each path is recorded as its POSIX-style relative path from the pack root (forward slashes, no leading `./`). Path-component case is preserved exactly as on disk.
3. **Sort.** Paths are sorted by byte-wise lexicographic comparison of the UTF-8-encoded normalized path.
4. **Per-file digest input.** For each file in sorted order, the digest input contributes:
   ```
   path_bytes_length (8 bytes, big-endian u64) || path_bytes ||
   file_bytes_length (8 bytes, big-endian u64) || file_bytes
   ```
   File bytes are read verbatim — no trailing-newline normalization, no line-ending conversion, no whitespace stripping. Pack authors who want CRLF-insensitive packs must commit LF-normalized files; this is documented in [`skills.md`](skills.md) conformance.
5. **Final hash.** `skill_pack_hash = sha256( concatenation of all per-file digest inputs in sorted order )`, hex-encoded.

The procedure is locked. Two filesystems that disagree only on `atime`/`mtime` or that contain only differences in excluded files produce the same digest. A digest mismatch on replay is a typed `skill.pack_digest_mismatch` error; `provenant replay` refuses the replay rather than continuing with a different pack.

## Append guarantees

Appends must be:

- **Monotonic**: per-tenant `seq` strictly increasing without gaps.
- **Atomic**: an append succeeds completely or not at all; partial appends are impossible.
- **Durable**: synchronous fsync via Postgres `synchronous_commit=on` for the audit log path.

Implementation: a `BEGIN; SELECT pg_advisory_xact_lock(hashtext(tenant_id)); INSERT INTO audit_entries ... RETURNING hash; COMMIT;` transaction with the advisory lock keyed by tenant. This serializes appends within a tenant without serializing across tenants.

## Tamper detection

A scheduled verifier task walks every tenant's chain weekly, recomputes hashes, and emits alerts on any mismatch. The verifier writes a `chain_verified` record (separate table, not in the chain itself) on success.

Manual verification: `provenant audit verify --tenant=<id>` walks the chain on demand. Used during DR drills and on customer request.

## Export formats

Audit entries are exportable to formats auditors accept:

| Format | When to use |
|---|---|
| **CSV walkthrough** | Auditors who walk through line-by-line; Excel-friendly |
| **Signed PDF** | Regulators expecting a static, signed artifact |
| **JSON Lines** | SIEM / log aggregator ingestion |
| **Provenant audit pack** | The full bundle (see below) |

Each format is implemented as an exporter in `internal/audit/export/`. New formats are easy to add; the existing ones are the v0 set.

## The `provenant audit pack` command

A single command produces a regulator-ready bundle for a run:

```bash
$ provenant audit pack <run_id> --output ./pack-<run_id>.zip

Building audit pack for run run_01HXYZ...:
  ✓ Run metadata
  ✓ 247 audit entries (chain verified)
  ✓ 12 tool calls with captured reads
  ✓ 38 provenance tokens
  ✓ 14 guard runs
  ✓ 1 HITL review
  ✓ Skill manifest (recon@0.3.0)
  ✓ Eval metrics for this run
  ✓ Replay artifact
Pack written: ./pack-run_01HXYZ.zip (4.2 MB)
```

The pack contents:

```
pack-run_01HXYZ.zip
├── README.md                       # human-readable summary, what's where
├── run.json                        # run metadata
├── audit.csv                       # audit log as CSV walkthrough
├── audit.jsonl                     # audit log as JSON Lines
├── audit.pdf                       # signed PDF version
├── tokens.json                     # all provenance tokens
├── tool-calls.json                 # all tool calls
├── captured-reads/                 # external reads (S3 fetched)
├── prompts/                        # full prompts sent to the model
├── responses/                      # full model responses
├── guards.json                     # guard runs
├── hitl.json                       # HITL approvals
├── skill/                          # skill pack at the pinned version
│   ├── manifest.yaml
│   ├── guards.yaml
│   ├── prompts/
│   └── (...)
├── metrics.json                    # this run's eval metrics (4 metrics)
├── replay-artifact.json            # everything needed to replay
└── verify.txt                      # chain verification result
```

The pack maps directly onto the [FINRA 2026](../references.md#finra-2026) / [SR 11-7](../references.md#sr-11-7) documentation surface. The customer's compliance team can attach the pack to the regulatory file.

## Replay artifact

A subset of the pack, used by `provenant replay`:

```json
{
  "run_id": "run_01HXYZ...",
  "skill_pack_hash": "...",
  "model": "claude-opus-4-7",
  "inputs": {...},
  "captured_reads": [...],
  "expected_tool_call_sequence": [...],
  "expected_tokens": [...],
  "expected_validators": [...]
}
```

Replay produces:

- **Trajectory determinism check**: did the same tool call sequence reproduce?
- **Decision determinism check**: did the same final decision reproduce?
- **Faithfulness check**: did claims still ground to the same tokens?

See [eval.md](eval.md) for the metric definitions.

## Retention

Audit entries are retained per tenant policy:

| Default retention | Trigger for extension |
|---|---|
| 7 years | SOX-relevant workflows |
| 5 years | FINRA-supervised activity |
| 3 years | General |
| Indefinite | Litigation hold or regulator subpoena |

Hot in Postgres for the first year; archived to cold object storage thereafter with full hash-chain integrity preserved. Cold storage retrieval is transparent in the API.

## Tests

- Property: chain verification rejects any single-entry mutation.
- Property: concurrent appends within a tenant produce a consistent monotonic chain.
- Integration: `audit pack` reconstructs a complete pack from a fixture run; the pack passes a separate verifier.
- DR drill: backup-restore cycle produces a chain that still verifies.

## Open questions

1. **Signed PDF generation library.** PDF signing requires either an in-process library or a separate service. Proposal: in-process with `unidoc` or similar for v0; reconsider for high-volume tenants.
2. **Tamper response policy.** When the verifier detects a mismatch, do we halt all operations for that tenant, or alert and continue? Proposal: halt-and-alert by default; configurable per tenant.
3. **Chain segmentation.** Should very long chains be segmented (e.g., per quarter) for query performance? Proposal: keep chain monolithic; use the index on `(tenant_id, seq)` for performance; reconsider at scale.

## References

- [FINRA 2026](../references.md#finra-2026) — auditability and traceability requirements
- [SR 11-7](../references.md#sr-11-7) — model risk management documented evidence
- [Faramesh](../references.md#faramesh) — replayable provenance-complete decision records
