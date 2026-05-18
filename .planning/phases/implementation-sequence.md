# Implementation sequence

Keep the loop tight: each phase must be independently buildable, testable, and reviewable. No phase should require building the whole ecosystem for days before it can produce runnable, verifiable evidence.

## Phase 2 — Expand reader coverage on the same kernel

- **Goal:** cover the remaining reader shapes and edge cases without changing the proven core model.
- **Implementation boundary:** additional source-reader coverage, selector/window edge cases, replay compatibility. No proof/audit semantic changes and no new access surfaces.
- **Functionality boundary:** more source shapes are readable through the same kernel; edge cases resolve the same way under replay.
- **Pass criteria:** focused tests exist for each new reader or edge case; replay/proof still pass unchanged; negative cases remain fail-closed.
- **Suggested validation:** reader-specific regression tests and replay verification across the expanded fixture set.

## Phase 3 — Add thin access surfaces and derived export

- **Goal:** expose the proven core through thin wrappers and a derived export path.
- **Implementation boundary:** CLI/HTTP/MCP shells, export artifacts, convenience views. No alternate provenance store and no divergent read path.
- **Functionality boundary:** the same verified core behavior is reachable from each access surface; exported artifacts come only from captured provenance.
- **Pass criteria:** each surface has at least one end-to-end smoke test; export validates against the same bundle bytes/proof; no surface introduces a second source of truth.
- **Suggested validation:** one smoke test per surface plus export verification against the canonical bundle.

## Phase 4 — Harden and hand off at a clear release-ready boundary

- **Goal:** widen confidence, lock in failure behavior, and close the phase with a clear handoff point.
- **Implementation boundary:** broader negative and recovery coverage, lightweight perf checks, final test-matrix/doc alignment. No new features and no new surfaces.
- **Functionality boundary:** bad inputs fail closed; recoverable flows return to the verified state; the documented source-to-verified-artifact path is complete.
- **Pass criteria:** negative/recovery tests pass; perf smoke stays within agreed bounds; the phase checklist matches implemented behavior; no open gaps remain in the handoff.
- **Suggested validation:** full targeted regression, perf smoke, and final checklist review.
