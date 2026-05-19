# Phase 3

## Goal
Expose the proven core through thin CLI, HTTP, and MCP wrappers, and add a derived export path that reads only from captured provenance. Phase 3 proves reachability and export from the same verified core without introducing a second source of truth.

## Packet sequence
Use `.planning/phases/phase-three/sequential-work-packets.md` for the PR-by-PR packet order, same-worktree sequencing, and commit boundaries. Parallel work is only for read-only planning/review, or isolated worktrees.

## Build
- Thin CLI access to the existing verified core behavior.
- Thin HTTP access to the existing verified core behavior.
- Thin MCP access to the existing verified core behavior.
- A derived export path that validates against canonical bundle bytes/proof.
- Smoke tests and docs showing each surface uses the same core path.

## Do not build
- Alternate provenance stores or divergent read paths.
- New proof, audit, minting, replay, or reader semantics.
- Runtime persistence-first rewrites, daemon lifecycle work, or storage backends unless a thin wrapper cannot be tested without an existing in-memory path.
- Feature expansion beyond access shells, convenience views, and derived export.
- Export artifacts sourced from anything other than captured provenance.

## Pass criteria
- Each access surface has at least one end-to-end smoke test.
- Export validation uses the same bundle bytes/proof as the core path.
- No surface introduces a second source of truth or bypasses verified core behavior.
- Phase 3 docs and handoff notes match implemented behavior and deferred work.
- The phase PR is up for review with validation evidence attached.

## Closure
- Shipped surfaces: `access/corepath`, `access/cli`, `access/httpapi`, `access/mcpapi`, and the derived export path in `access/corepath`.
- Smoke evidence: `access/smoke/smoke_test.go` exercises the core path, CLI, HTTP, and MCP lanes against one deterministic request and proves byte-identical bundle and export outputs across all shipped surfaces.
- Export evidence: the smoke matrix replays canonical bundle bytes through `VerifyExportSource`, `GenerateDerivedExport`, and `ValidateDerivedExport` for every shipped surface.
- Stop conditions encountered: none. Packet 6 did not require a new provenance store, divergent read path, or proof/audit semantic change.
- Deferred to Phase 4: broaden negative and recovery coverage, add lightweight performance smoke, and do the final hardening sweep without widening the Phase 3 surface set.
