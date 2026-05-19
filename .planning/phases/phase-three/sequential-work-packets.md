# Phase 3 work packets - sequential by default

Phase 3 is a same-worktree plan first. Execute the packets in order, one at a time, in the same worktree. Parallel work is allowed only for read-only planning/review, or when each packet is isolated in its own git worktree. Keep each packet narrow; Phase 3 exposes the same verified core through thin surfaces and derived export without widening provenance semantics.

## PR 1 / Packet 1 - access boundary contract
**What the PR should do:** define the smallest shared boundary that proves CLI, HTTP, MCP, and export all use the same verified core path.

**What it should build:** minimal surface-neutral request/response expectations, shared smoke-test fixture shape, and checks that prevent wrappers from introducing a second source of truth.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no transport-specific feature work beyond the shared boundary; no proof/audit/replay changes; no persistence or daemon work.

**Pass criteria / validation:** existing core tests remain unchanged; the boundary can express each planned surface and export smoke; bypass or divergent-read attempts fail review or tests.

**Commit/PR boundary:** first. Land one small conventional-commit PR in one PR; do not start Packet 2 until this PR is merged or otherwise accepted.

## PR 2 / Packet 2 - thin CLI surface
**What the PR should do:** add the thin CLI wrapper over the verified core path.

**What it should build:** CLI entrypoints and smoke coverage only enough to prove access to the same core behavior and bundle/proof outputs.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no daemon, persistent runtime, migrations, broad command suite, or alternate verification logic.

**Pass criteria / validation:** CLI smoke test reaches the verified core path and produces evidence comparable to the canonical bundle/proof fixture.

**Commit/PR boundary:** second, after Packet 1. Land one small conventional-commit PR limited to the CLI lane.

## PR 3 / Packet 3 - thin HTTP surface
**What the PR should do:** add the thin HTTP wrapper over the verified core path.

**What it should build:** minimal HTTP route(s), request/response handling, and smoke coverage proving the same core behavior.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no runtime daemon lifecycle, persistent store, auth expansion, observability platform, or HTTP-only provenance behavior.

**Pass criteria / validation:** HTTP smoke test reaches the verified core path, rejects malformed input fail-closed, and produces evidence comparable to the canonical bundle/proof fixture.

**Commit/PR boundary:** third. Land one small conventional-commit PR dedicated to the HTTP lane.

## PR 4 / Packet 4 - thin MCP surface
**What the PR should do:** add the thin MCP wrapper over the verified core path.

**What it should build:** minimal MCP tool/prompt/resource exposure and smoke coverage proving the same core behavior.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no MCP-primary logic, no MCP-only enforcement, no divergent canonicalization, and no persistence or daemon expansion.

**Pass criteria / validation:** MCP smoke test reaches the verified core path and produces evidence comparable to the CLI and HTTP lanes.

**Commit/PR boundary:** fourth. Land one small conventional-commit PR dedicated to the MCP lane.

## PR 5 / Packet 5 - derived export path
**What the PR should do:** add the derived export path from captured provenance.

**What it should build:** export generation and validation that read from canonical bundle bytes/proof and fail closed when required captured provenance is absent or mismatched.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no export sourced from live readers, alternate stores, or non-captured data; no proof/audit/replay semantic changes.

**Pass criteria / validation:** export verification passes against the canonical bundle/proof fixture and fails closed on missing or mismatched captured provenance.

**Commit/PR boundary:** fifth. Land one small conventional-commit PR limited to derived export behavior.

## PR 6 / Packet 6 - smoke matrix and doc alignment
**What the PR should do:** close Phase 3 by aligning smoke coverage and docs to the shipped surfaces and export path.

**What it should build:** the minimal cross-surface smoke matrix, export verification evidence, and docs that describe shipped behavior, stop conditions encountered, and Phase 4 deferred hardening.

**Skills/guidance to load:** `engineering-workflow`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new features or late scope additions beyond Packets 1 through 5.

**Pass criteria / validation:** each surface has smoke evidence, export validates against canonical bundle bytes/proof, docs match implementation, and the phase PR is up for review.

**Commit/PR boundary:** last. Land one final small conventional-commit PR for the smoke-matrix/doc-alignment pass only.
