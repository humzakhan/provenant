/goal
Kickstart Phase 2 only. Treat `.planning/phases/implementation-sequence.md` and `.planning/phases/phase-two/README.md` as the Phase 2 contract. Expand structured-reader coverage and selector/window edge cases on the proven Phase 1 kernel without changing proof/audit semantics or adding any new access surface.

Follow `.planning/phases/phase-two/sequential-work-packets.md` in order. Execute in the same worktree, one packet at a time; parallelism is only for read-only planning/review or isolated worktrees.

Objective:
- finish and narrowly extend `extract.json` to the declared structured-files contract
- add `extract.jsonl`
- add `extract.csv_rows`
- add `extract.spreadsheet`
- finish with the expanded fixture/replay matrix and doc alignment pass

Non-goals:
- no CLI, HTTP, MCP, daemon, model, or API integration
- no Postgres, SQLite, S3, network, or filesystem-backed persistence
- no proof/audit semantic changes, no new minting semantics, and no alternate read path
- no OCR, PDF, unstructured document-AI, export polish, or broad framework work

Allowed scope:
- the smallest additive selector/capture contract delta needed to support structured readers and windowed selectors on the same kernel
- the matching additive bundle/verifier/replay validation needed to carry and validate those declared Phase 2 selector and captured-read fields without changing proof/audit semantics
- reader-local locator/output metadata, pinned parser/options metadata, and replay evidence that preserve the Phase 1 mint/audit/replay model
- focused fixture additions, replay coverage, and doc updates for the implemented readers
- fail-closed handling for selector, path, span, window, encoding, dialect, sheet, and formula-policy ambiguity

Disallowed actions:
- widening into new public transport or runtime surfaces
- changing audit entry meaning, token semantics, or verification semantics; making non-additive bundle changes; or introducing alternate proof/replay models
- depending on live filesystem reads, live network reads, non-deterministic formula evaluation, or external document services
- implementing readers outside `extract.json`, `extract.jsonl`, `extract.csv_rows`, and `extract.spreadsheet`

Escalation / stop conditions:
- stop if a reader requires a new access surface or storage/runtime dependency to work
- stop if selector/window support requires changing proof/audit semantics, token mint semantics, or replay verification behavior beyond the additive declared-selector and captured-read validation Phase 2 allows
- stop if JSON path/span, CSV dialect/encoding, or spreadsheet sheet/range/table resolution cannot be made deterministic and fail closed
- stop if the code or architecture reveals that a proposed reader order or contract addition is impossible without crossing the Phase 2 boundary

Observable evidence to report:
- exact packet completed and reader(s) touched
- files changed
- tests run
- per-reader positive coverage and negative fail-closed cases
- replay verification result and proof/audit compatibility result
- fixture/doc updates made
- anything deferred explicitly to Phase 3+
