# Subsystem: Audit Logger

Status: **current-code reference**

This page documents the audit subsystem as implemented today. It is not an
implementation plan and does not extend the deterministic payloads, hash input,
serialized field names, generated tables, bundle replay rules, or golden outputs.

## Depends On

- [`subsystems/kernel.md`](kernel.md) - kernel-public `AuditEntry`, `Run`,
  `Token`, and `CapturedRead` shapes
- [`subsystems/storage.md`](storage.md) - storage projections and retention
  concerns outside the pure-Go kernel

## Depended On By

- `kernel/public/mint.go` for `token_mint` entries
- `kernel/public/derive/derive.go` for `token_derive` entries
- `kernel/public/runfile/` for run lifecycle entries and bundle finalization
- `kernel/public/bundle/` for bundle emission, verification, and replay
- `access/cli/submitverb.go` for `answer_submitted` entries

## Current Scope

The current implementation records a deterministic, hash-chained audit trail for
Phase 1 run files and bundle bytes. The supported chain contents are limited to
the entry kinds, payloads, and bundle shape listed below.

The implementation does not currently emit model prompt/response, tool-call,
guard, HITL, restore, orphan-recovery, or credential-identity audit kinds. Those
kinds are not part of the current code contract.

## Entry Kinds

The current `EntryKind` set is defined in `kernel/public/types.go`:

```go
type EntryKind string

const (
    EntryKindRunStarted      EntryKind = "run_started"
    EntryKindTokenMint       EntryKind = "token_mint"
    EntryKindTokenDerive     EntryKind = "token_derive"
    EntryKindRunCompleted    EntryKind = "run_completed"
    EntryKindRunFailed       EntryKind = "run_failed"
    EntryKindAnswerSubmitted EntryKind = "answer_submitted"
)
```

`run_started` is written when a run file starts. `token_mint` records a token
minted from a captured read. `token_derive` records a deterministic derived token.
`answer_submitted` records that an existing token was designated as a labelled
answer. `run_completed` and `run_failed` are terminal lifecycle entries.

## Entry Shape

The in-memory shape is `kernel/public.AuditEntry`:

```go
type AuditEntry struct {
    Seq      uint64
    RunID    string
    TenantID string
    Kind     EntryKind
    At       time.Time
    PrevHash []byte
    Hash     []byte
    Payload  PayloadEnvelope
}
```

`PayloadEnvelope` separates deterministic bytes from optional non-deterministic
diagnostics:

```go
type PayloadEnvelope struct {
    Deterministic      json.RawMessage
    NonDeterministic   json.RawMessage
    HashInputCanonical []byte
}
```

Current emitters set `Payload.Deterministic` to JCS-canonical JSON and set
`Payload.HashInputCanonical` to the same bytes. Bundle replay re-canonicalizes
`Payload.Deterministic` and rejects entries whose provided hash input differs.

## Hash Construction

The current hash input is:

```text
sha256(prev_hash || seq_be8 || run_id || tenant_id || kind || canonical_payload)
```

Where:

- `prev_hash` is the previous entry's `Hash`, or the tenant genesis hash for the
  first entry.
- `seq_be8` is the big-endian 8-byte encoding of `Seq`.
- `run_id`, `tenant_id`, and `kind` are the exact string bytes stored on the
  entry.
- `canonical_payload` is the JCS-canonical deterministic payload.

Wall-clock `At` is recorded on each entry but is not part of the hash input.

## Current Payloads

| Entry kind | Deterministic payload | Non-deterministic payload |
|---|---|---|
| `run_started` | Empty JSON object: `{}` | Not emitted by current code |
| `token_mint` | `token_id`, `captured_read_id`, `source_path`, `source_selector`, `output_path`, optional `origin`, optional `source_name`, `reader_name`, `reader_version`, optional `reader_options`, optional `locator`, `source_sha256`, `output_sha256`, `source_value_raw`, `value_canonical`, optional `unit` | Not emitted by current code |
| `token_derive` | `token_id`, `operation`, `inputs`, optional `matched`, optional `predicate`, optional `formula_id`, optional `rounding`, `evaluated`, optional `unit` | Not emitted by current code |
| `run_completed` | Empty JSON object: `{}` | Not emitted by current code |
| `run_failed` | Empty JSON object: `{}` | Not emitted by current code |
| `answer_submitted` | `token_id`, `label`, `value_canonical`, optional `unit` | Not emitted by current code |

`token_mint` payloads are encoded in `kernel/public/mint.go` and decoded during
bundle replay in `kernel/public/bundle/bundle.go`. `token_derive` payloads are
encoded by `kernel/public/internal/derivation`. `answer_submitted` payloads are
constructed in `access/cli/submitverb.go` and validated during bundle replay.

## Bundle Layout

The current bundle is a single JCS-canonical JSON document, not a ZIP directory
and not a multi-file pack. The document shape is defined in
`kernel/public/bundle/bundle.go`:

```json
{
  "format": "phase1.bundle/v1",
  "run": { ... },
  "captured_reads": [ ... ],
  "tokens": [ ... ],
  "audit_jsonl": "{...}\n{...}\n"
}
```

The top-level fields are:

- `format` - currently `phase1.bundle/v1`.
- `run` - the kernel-public `Run` object.
- `captured_reads` - captured reads referenced by token-mint entries.
- `tokens` - minted and derived tokens for the run.
- `audit_jsonl` - a string containing the canonical JSON Lines audit chain.

`audit_jsonl` is embedded in the JSON document. It is not a separate file in the
current bundle representation.

## `audit_jsonl` Wire Shape

Each `audit_jsonl` line is a JCS-canonical JSON object with these fields:

```json
{
  "at": "2026-05-18T00:00:00Z",
  "hash": "...",
  "kind": "token_mint",
  "payload_deterministic": { ... },
  "payload_non_deterministic": { ... },
  "prev_hash": "...",
  "run_id": "...",
  "seq": 1,
  "tenant_id": "..."
}
```

`payload_non_deterministic` is omitted when empty. The `audit_jsonl` string must
end with a single LF. A missing final LF is treated as `ErrChainTruncated` by
bundle replay.

## Verification And Replay

`kernel/public/bundle.Verify` calls `Replay` and operates only on bundle bytes.
Replay currently:

1. Parses the single JSON document and rejects unknown top-level fields.
2. Requires `format == "phase1.bundle/v1"`.
3. Requires non-empty `audit_jsonl`.
4. Parses `audit_jsonl` as LF-terminated JSON Lines.
5. Verifies entry `run_id`, `tenant_id`, strict 1-based `seq`, `prev_hash`, and
   the audit hash formula above.
6. Validates supported payload shapes for `token_mint`, `token_derive`, lifecycle
   entries, and `answer_submitted`.
7. Reconstructs deterministic token state and checks referenced captured reads and
   tokens are present in the bundle document.

The current exported errors are Go errors from `kernel/public/bundle`:

- `ErrTamperDetected`
- `ErrChainTruncated`
- `ErrMissingArtifact`

## Append Guarantees

Current append behavior is implemented by run files and kernel storage bindings:

- `Seq` is strictly increasing within a run chain.
- `PrevHash` links each entry to the previous entry, with a tenant genesis hash
  for the first entry.
- `run_started` is written by run-file start logic.
- `AppendEntry` accepts only `token_mint`, `token_derive`, `answer_submitted`,
  `run_completed`, and `run_failed`.
- `run_completed` and `run_failed` finalize the run file.

## Retention

The kernel-public bundle is bytes. Retention is a host or runtime-binding concern.
Any future persistent binding must preserve the same entry bytes and verification
semantics when projecting the current chain into storage.

## Current Tests

Current audit coverage lives primarily in:

- `kernel/public/mint_test.go`
- `kernel/public/derive/*_test.go`
- `kernel/public/runfile/runfile_test.go`
- `kernel/public/bundle/bundle_test.go`
- `kernel/public/bundle/bundle_token_derive_test.go`
- `audit/report/*_test.go`

These tests exercise hash-chain validation, run-file recovery/finalization,
bundle emission/replay, token derivation, answer submission, and tamper/missing
artifact failures for the current code contract.
