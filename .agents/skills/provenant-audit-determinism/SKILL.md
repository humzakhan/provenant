---
name: provenant-audit-determinism
description: Use when changing audit entries, deterministic payloads, canonical hash input, bundle verify/replay, or enum/source-table generation.
---

# Provenant Audit Determinism

Use this skill for `audit`, hash input, bundle verify/replay, enum tables, or any replay-sensitive provenance change.

## Triggers
`audit`, `AuditEntry`, `payload_deterministic`, `payload_non_deterministic`, `hash`, `canonical JSON`, `bundle`, `Verify`, `Replay`, `error_code`, `failure_subcode`, `recovery_reason`, `enum`, `tamper`, `mutation`

## Rules
- Keep deterministic and non-deterministic payloads separated.
- Hash input must be canonical; do not rely on ordinary JSON marshalling on the hash path.
- `EntryKind`, `error_code`, `failure_subcode`, and `recovery_reason` come from the canonical source tables; generated Go is derived.
- Bundle verify/replay must stay pure over bundle bytes, with no DB or CLI dependency.
- Bundle scope is phase-bound: carry only the captured reads/artifacts plus the token graph needed by the current phase to verify and replay.
