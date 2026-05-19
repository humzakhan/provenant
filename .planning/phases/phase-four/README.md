# Phase 4

## Goal
Harden the shipped Phase 3 behavior for release handoff. Phase 4 widens confidence only through negative and recovery coverage, lightweight performance smoke, and final test-matrix and documentation alignment without adding any new product surface.

## Packet sequence
Use `.planning/phases/phase-four/sequential-work-packets.md` for the PR-by-PR packet order, same-worktree sequencing, and commit boundaries. Parallel work is only for read-only planning/review, or isolated worktrees.

## Build
- Negative regression coverage for fail-closed behavior on shipped paths.
- Recovery behavior coverage only for already-shipped flows that can be cited from existing tests, docs, or code.
- Lightweight performance smoke for the shipped paths.
- Final test-matrix, docs, handoff alignment, and a final handoff checklist that shows no open handoff gaps for the Phase 4 closeout.

## Do not build
- New features or new user-facing surfaces.
- Any expansion of the Phase 3 wrapper or export scope.
- New storage, runtime, model, or adapter work.
- Future Phase 5+ roadmap work or speculative cleanup outside release hardening.

## Pass criteria
- Negative coverage proves shipped paths still fail closed on malformed, missing, or invalid inputs.
- Recovery coverage proves only repo-cited shipped recovery behavior matches the current contract; if no concrete shipped flow can be cited from existing tests, docs, or code, this lane stops instead of inventing one.
- Lightweight performance smoke runs on shipped paths and reports whether the current baseline is acceptable for handoff.
- Final test matrix and docs match implemented behavior, deferred work, and handoff expectations.
- Final handoff checklist is attached, maps to `.planning/implementation-plan.md` Phase 4 exit criteria, and shows no open handoff gaps.
- The phase PR is up for review with validation evidence attached.

## Closure
- Phase 4 closes the current release sequence at the hardening and handoff boundary described in `.planning/implementation-plan.md`.
- The final deliverable is a handoff checklist with observable evidence for regression, recovery/fail-closed, performance smoke, and doc/test-matrix alignment, with no unresolved handoff gaps left open.
- Stop instead of widening scope if hardening reveals a need for new surfaces, new storage/runtime/model/adapter work, or any Phase 3 contract expansion.
- Stop the recovery lane if the only candidate flow is a proposed restart, retry, replay-adjacent, or equivalent behavior that cannot be cited from existing shipped behavior in this repo.
- Deferred work beyond hardening stays out of this phase and must be replanned separately.
