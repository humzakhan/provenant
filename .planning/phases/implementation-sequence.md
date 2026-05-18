# Implementation sequence

Keep the loop tight: each phase should be independently buildable, testable, and reviewable.

## Phase 1 — Prove the core loop in-process

### Track 1 — Kernel + one reader + provenance
- **Implementation boundaries:** kernel boundary, in-memory harness, one structured reader (`extract.json` or `extract.csv_rows`), audit-chain/proof emission, in-process verification. Do **not** add CLI/HTTP/MCP, persistence, export polish, or broad reader coverage.
- **Functionality boundaries:** a declared source path can mint a token; one reader call runs end to end; reads are captured; the emitted bundle can be replayed and verified; a targeted mutation fails closed.
- **Pass criteria:** unit/integration coverage uses source bytes/fixtures only; emitted bundle verifies; replay reproduces captured reads; mutation test fails as expected; any required fixture/doc note is updated.

### Track 2 — Fixtures + failure harness
- **Implementation boundaries:** fixture wiring, captured-read assertions, negative/mutation tests, minimal test helpers. Do **not** add new surfaces or storage.
- **Functionality boundaries:** Phase 1 remains in-memory and deterministic; no user-facing access surface beyond direct test invocation.
- **Pass criteria:** tests can be run repeatedly from fixtures; failure mode is asserted in the test; no runtime adapter code is touched.

## Phase 2 — Expand reader coverage and add thin access surfaces

### Track 1 — Remaining readers + selector/window edges
- **Implementation boundaries:** additional source-reader coverage, selector/window edge cases, replay compatibility. Do **not** change proof/audit semantics or add export/runtime layers.
- **Functionality boundaries:** more source shapes are readable through the same proven kernel; edge cases resolve the same way under replay.
- **Pass criteria:** focused tests exist for each new reader/edge case; replay/proof still passes unchanged; negative cases remain fail-closed.

### Track 2 — CLI/HTTP/MCP shells + derived export
- **Implementation boundaries:** thin wrappers over the proven core, export artifacts, convenience views. Do **not** add an alternate provenance store or divergent read path.
- **Functionality boundaries:** the same verified core behavior is reachable from each access surface; exported artifacts are derived from captured provenance.
- **Pass criteria:** each shell has at least one end-to-end smoke test; export validates against the same bundle bytes/proof; no surface introduces a second source of truth.

## Phase 3 — Harden and broaden confidence

### Track 1 — Negative, recovery, and perf coverage
- **Implementation boundaries:** broader fixtures, replay failures, mutation cases, recovery checks, lightweight perf checks. Do **not** widen the kernel contract.
- **Functionality boundaries:** known bad inputs fail closed; recoverable paths return to the verified state; core behavior stays stable under load checks.
- **Pass criteria:** negative/recovery tests pass; perf smoke stays within agreed bounds; captured-read assertions remain green.

### Track 2 — Regression docs and release-ready checks
- **Implementation boundaries:** phase-summary docs, test matrix updates, final integration validation. No new features.
- **Functionality boundaries:** the documented path from source bytes to verified artifact is complete and repeatable.
- **Pass criteria:** the doc checklist matches implemented behavior; end-to-end validation from each surface passes; no open gaps remain in the phase handoff.
