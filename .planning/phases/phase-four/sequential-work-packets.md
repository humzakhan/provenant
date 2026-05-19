# Phase 4 work packets - sequential by default

Phase 4 is a same-worktree plan first. Execute the packets in order, one at a time, in the same worktree. Parallel work is allowed only for read-only planning/review, or when each packet is isolated in its own git worktree. Keep each packet narrow; Phase 4 hardens the shipped Phase 3 behavior for release handoff without widening the product surface.

## PR 1 / Packet 1 - fail-closed negative regression coverage
**What the PR should do:** add the smallest hardening coverage that proves shipped paths still fail closed on malformed, missing, or invalid inputs.

**What it should build:** targeted negative regression tests for the already-shipped surfaces and export path, plus any fixture updates strictly required to express those failures.

**Skills/guidance to load:** `engineering-workflow`; `go-testing-fuzzing`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new features, no new user-facing surface area, no proof/audit/replay semantic changes, and no storage/runtime/model/adapter expansion.

**Pass criteria / validation:** negative-path coverage demonstrates fail-closed behavior on the targeted shipped paths and leaves existing behavior unchanged outside the added regression evidence.

**Commit/PR boundary:** first. Land one small conventional-commit PR limited to fail-closed negative regression coverage.

## PR 2 / Packet 2 - recovery behavior coverage
**What the PR should do:** add focused coverage for already-supported recovery behavior that matters to release handoff confidence.

**What it should build:** recovery-path tests for existing restart, retry, replay-adjacent, or equivalent already-supported flows, plus minimal fixtures needed to prove that behavior.

**Skills/guidance to load:** `engineering-workflow`; `go-testing-fuzzing`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new recovery mechanisms, no new runtime/storage behavior, no new access surface behavior, and no semantic widening to make an unsupported scenario pass.

**Pass criteria / validation:** recovery coverage proves the targeted already-supported flows behave as documented and fail closed when the existing contract is exceeded.

**Commit/PR boundary:** second, after Packet 1. Land one small conventional-commit PR dedicated to recovery behavior coverage.

## PR 3 / Packet 3 - lightweight performance smoke
**What the PR should do:** add lightweight performance smoke over the shipped paths to catch obvious release-blocking regressions without turning Phase 4 into a benchmarking phase.

**What it should build:** small smoke checks, harness wiring, or reporting needed to record a current performance baseline for shipped behavior.

**Skills/guidance to load:** `engineering-workflow`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no benchmarking framework expansion, no optimization campaign, no new feature work, and no extra infrastructure beyond what the smoke evidence needs.

**Pass criteria / validation:** the performance smoke runs against shipped paths, reports the current baseline plainly, and surfaces any obvious handoff caveat without widening scope.

**Commit/PR boundary:** third. Land one small conventional-commit PR dedicated to lightweight performance smoke only.

## PR 4 / Packet 4 - final test matrix, docs, and handoff alignment
**What the PR should do:** close Phase 4 by aligning the final test matrix, docs, and handoff notes to the hardened shipped behavior.

**What it should build:** final matrix evidence, doc updates, and handoff notes that describe shipped behavior, hardening evidence, deferred work, and any explicit release caveats.

**Skills/guidance to load:** `engineering-workflow`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new features, no late surface additions, and no cleanup outside what is required to align the final hardening evidence and handoff docs.

**Pass criteria / validation:** docs and matrix evidence match the shipped behavior and Phase 4 hardening results, deferred work remains clearly out of scope, and the phase PR is up for review.

**Commit/PR boundary:** last. Land one final small conventional-commit PR for test-matrix, docs, and handoff alignment only.
