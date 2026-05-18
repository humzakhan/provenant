# Phase 1

## Goal
Prove the in-process core loop with less code, not more: fixture/source bytes -> one structured reader -> captured read -> kernel-minted token -> minimal bundle -> verify/replay -> mutation fails closed.

## Recommended single reader
Use the smallest JSON fixture reader: `extract.json`.
If the implementation base already exposes the same narrow backend as `extract.json_value`, keep that name, but do not widen the reader surface.

## Build
- In-memory source bytes under a logical source path.
- One structured reader call.
- Captured read metadata/hash.
- Kernel-owned token minting.
- Minimal bundle emit, verify, and replay in memory.
- One deterministic mutation test that fails closed.

## Do not build
- CLI, HTTP, MCP, daemon/runtime surfaces.
- Postgres, SQLite, S3, filesystem-backed persistence.
- Broad reader coverage.
- PDF/XLSX/export polish.
- Extra provenance features beyond the phase loop.

## Pass criteria
- Tests use fixture/source bytes only.
- One declared source path mints at least one token.
- The reader completes end to end and records a captured read.
- The emitted bundle verifies and replays in memory.
- A targeted deterministic mutation fails closed.
- Phase 1 docs/checklist reflect the final boundary.
