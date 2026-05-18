# Phase 2

## Goal
Expand structured-reader coverage and selector/window edge cases on the proven Phase 1 kernel: same source-bytes input, same captured-read -> kernel-minted token -> bundle verify/replay loop, more reader shapes and stricter fail-closed selector behavior.

## Packet sequence
Use `.planning/phases/phase-two/sequential-work-packets.md` for the PR-by-PR packet order, same-worktree sequencing, and commit boundaries. Parallel work is only for read-only planning/review, or isolated worktrees.

## Coverage target
- Finish and narrowly extend `extract.json` to the declared structured-files contract.
- Add `extract.jsonl`, then `extract.csv_rows`, then `extract.spreadsheet`.
- Expand the fixture, replay, and doc-alignment matrix only enough to prove the new readers and selector/window edge cases on the same kernel.

## Build
- The smallest selector/capture contract delta Phase 2 needs for structured readers and windowed selectors.
- A completed `extract.json` reader with unique selector handling, richer locator/output fields, and fail-closed mismatch behavior.
- `extract.jsonl` with deterministic line windows, pagination, sparse-index capture, and replay coverage.
- `extract.csv_rows` with pinned dialect/encoding, bounded row windows, optional header/column projection, row/column provenance, and malformed-input rejection.
- `extract.spreadsheet` with sheet/range/table selectors, cached-only formula policy, window/projection metadata, and ambiguity rejection.
- Expanded per-reader positive/negative fixtures, replay checks, and docs that match shipped behavior.

## Do not build
- CLI, HTTP, MCP, daemon/runtime/API surfaces.
- Postgres, SQLite, S3, filesystem-backed persistence, or any new runtime/backend surface.
- Proof, audit, minting, or replay semantic changes.
- OCR, PDF, unstructured document-AI, or alternate read paths outside the structured-files lane.
- Export polish, broad proof frameworks, or speculative abstractions for later phases.

## Pass criteria
- Each reader uses source bytes/fixtures only and has focused positive and negative tests.
- Selector, window, pagination, locator, and malformed-input edge cases fail closed when ambiguity or mismatch appears.
- The captured-read -> mint -> bundle verify/replay loop still passes without proof/audit semantic changes.
- Replay reproduces the same deterministic outputs and captured-read evidence across the expanded fixture matrix.
- Phase 2 docs/checklist reflect the final reader set, boundaries, and deferred work accurately.
