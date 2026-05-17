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
    EntryKindTokenMint         EntryKind = "token_mint"
    EntryKindGuardRun          EntryKind = "guard_run"
    EntryKindSubmitAttempt     EntryKind = "submit_attempt"
    EntryKindSubmitRejected    EntryKind = "submit_rejected"
    EntryKindAnswerReleased    EntryKind = "answer_released"
    EntryKindHITLDispatched    EntryKind = "hitl_dispatched"
    EntryKindHITLDecision      EntryKind = "hitl_decision"
    EntryKindRunCompleted      EntryKind = "run_completed"
    EntryKindRunFailed         EntryKind = "run_failed"
)
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
hash = sha256(prev_hash || seq_bytes || run_id || tenant_id || kind || canonical(payload) || at_iso)
```

Where:

- `canonical(payload)` is JCS (RFC 8785) canonical JSON.
- `at_iso` is RFC 3339 nanosecond precision.
- The first entry per tenant has `prev_hash = sha256("provenant-genesis:" || tenant_id)`.

Verification walks the tenant's chain from genesis, recomputing each `hash` and comparing to the stored value. Any mismatch surfaces the corrupted entry.

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
