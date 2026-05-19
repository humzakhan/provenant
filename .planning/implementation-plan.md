# Implementation Plan

Phases 0-3 complete. Currently at Phase 4. Phase 3 completion is based on the landed packet sequence through the smoke-matrix and doc-alignment closure pass; this plan does not claim completion of the older, broader Phase 2 tool/guard scope that existed in `plans/implementation-plan.md` before the phase sequence narrowed execution.

This is the canonical phase plan for Provenant v0 execution. It welds the completed state from the former `.planning/phases/implementation-sequence.md` into the implementation plan and uses phase-level boundaries only. Every phase definition of done is: the phase PR is up for review with validation evidence attached.

## Phase 0 - Kernel Baseline

- **Name:** Kernel module boundary and public contract baseline.
- **Goal:** Establish the pure-Go kernel boundary, public API spine, deterministic audit primitives, in-memory binding, and import guards needed by later phases.
- **Scope boundaries:** Kernel-only foundation; no runtime persistence, transport surface, model harness, or external tool/backend implementation.
- **Exit criteria:** PR up for review with kernel build/test evidence, import-guard evidence, deterministic header/enum/audit fixture evidence, and public-surface checks.
- **Dependencies:** None.
- **Status:** done.

## Phase 1 - Core Provenance Loop

- **Name:** In-memory mint, capture, bundle, verify, and replay loop.
- **Goal:** Prove the source-bytes input to captured-read to kernel-minted token to bundle verify/replay loop on `kernel/public/memio`.
- **Scope boundaries:** Library-mode core only; no CLI, HTTP, MCP, daemon/runtime surface, persistent storage binding, or export polish.
- **Exit criteria:** PR up for review with fixture-reader, mint/capture, bundle verify, replay, and core-loop acceptance evidence.
- **Dependencies:** Phase 0.
- **Status:** done.

## Phase 2 - Structured Reader Expansion

- **Name:** Expanded structured-reader coverage on the same kernel.
- **Goal:** Add the remaining structured reader shapes and selector/window edge cases without changing proof, audit, minting, or replay semantics.
- **Scope boundaries:** `extract.json`, `extract.jsonl`, `extract.csv_rows`, `extract.spreadsheet`, reader-local fixtures, replay coverage, and doc alignment only; no new access surfaces, runtime storage, proof model, audit semantics, OCR/PDF, or alternate read paths.
- **Exit criteria:** PR up for review with focused positive/negative fixtures per reader, fail-closed selector/window evidence, and replay/proof compatibility evidence.
- **Dependencies:** Phase 1.
- **Status:** done.

## Phase 3 - Thin Access Surfaces and Derived Export

- **Name:** Thin access surfaces and derived export.
- **Goal:** Expose the proven core through thin CLI, HTTP, and MCP wrappers, and add a derived export path that reads only from captured provenance.
- **Scope boundaries:** Access shells, convenience views, and derived export artifacts only; no alternate provenance store, no divergent read path, no persistence-first rewrite, and no new proof/audit semantics.
- **Exit criteria:** PR up for review with one smoke test per access surface, export verification against canonical bundle bytes/proof, and evidence that no surface introduces a second source of truth.
- **Dependencies:** Phase 2.
- **Status:** done.

## Phase 4 - Release-Ready Hardening

- **Name:** Hardening and handoff.
- **Goal:** Widen confidence, lock in fail-closed and recovery behavior, and close the current release sequence at a clear handoff boundary.
- **Scope boundaries:** Negative/recovery coverage, lightweight performance checks, and final test-matrix/doc alignment only; no new features, no new surfaces, and no scope expansion beyond behavior already exposed by Phase 3.
- **Exit criteria:** PR up for review with targeted regression evidence, recovery/fail-closed evidence, performance smoke evidence, and a final checklist showing no open handoff gaps.
- **Dependencies:** Phase 3.
- **Status:** not started.

## Deferred Future Phases

- **Name:** Post-handoff roadmap.
- **Goal:** Replan larger capabilities after the Phase 4 handoff based on the then-current kernel, access-surface, and export evidence.
- **Scope boundaries:** Larger runtime persistence, model harness, non-Go binding, eval harness, additional adapters, and SaaS/operator work are not part of Phases 1-4 unless a future plan explicitly reslices them.
- **Exit criteria:** PR up for review for each future phase plan before implementation begins.
- **Dependencies:** Phase 4.
- **Status:** not started.

## Reconciliation Flags

- The prior `plans/implementation-plan.md` used a broader Phase 2/3/4 taxonomy covering tool registry, guard pipeline, model harness, runtime persistence, MCP/HTTP, non-Go binding, and eval work. The executed `.planning/phases/implementation-sequence.md` narrowed Phase 2 to structured-reader expansion and defined Phase 3 as thin access surfaces plus derived export.
- This plan treats the executed sequence as authoritative for current state because the commit history shows all Phase 2 reader packets landed: selector contract, JSON completion, JSONL, CSV rows, spreadsheet, and structured reader matrix.
- The broader legacy work is not marked complete. It remains deferred for future replanning after Phase 4 unless a new phase plan explicitly pulls a bounded part of it forward.
- Phase 3 should stop rather than widen scope if a thin surface requires a new provenance store, a divergent read path, or proof/audit semantic changes.
