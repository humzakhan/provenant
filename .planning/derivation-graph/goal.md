---
description: Execute the full Derivation Graph plan end-to-end, one phase per PR
---

You are autonomously executing the entire Derivation Graph feature at .planning/derivation-graph/. You will deliver phases 01 through 08 sequentially, one PR per phase, end-to-end in a single run.

# Operating mode

1. Enter orchestrator mode.
2. Load the `engineering-workflow` skill and follow it for branching, validation, review cycles, PR lifecycle, and merge discipline.
3. Load `conventional-commit-guard` before every commit and PR title.
4. Models: Opus for architecture and thinking, Sonnet 4.6 for implementation, Haiku 4.5 for deterministic ops.

# Per-phase loop

For each phase NN in 01..08 (in order):

1. Read `.planning/derivation-graph/README.md` and `.planning/derivation-graph/phase-NN-*.md`.
2. Sync local main: `git fetch origin && git checkout main && git pull --ff-only origin main`.
3. Branch: `feat/derivation-graph-phase-NN-<slug>` from updated main.
4. Delegate implementation to a Sonnet 4.6 subagent with the phase file as its sole spec. Use an Opus thinking subagent to resolve any architectural ambiguity that surfaces — never resolve ambiguity in the orchestrator session.
5. Validation gates (all must pass):
   - `go test ./...`
   - `go vet ./...`
   - Any phase-specific commands listed in the phase file's Tests or Definition of Done section.
6. Run an independent reviewer subagent (Opus, thinking on) against the phase's Definition of Done.
7. If reviewer requests changes, route fixes to a Sonnet subagent and re-review. Loop until APPROVE or APPROVE_WITH_FIXES.
8. Open a PR titled `feat(derivation-graph): phase NN — <short title>` (conventional-commit-guard).
9. Wait for CI / checks (if any). On clean checks and reviewer approval, squash-merge with `--delete-branch` and reset local main to `origin/main`.
10. Record the merged squash commit SHA and proceed to phase NN+1.

# Stop conditions

- Stop and report if any of: a reviewer returns CHANGES_REQUESTED twice in a row on the same phase; validation gates fail after two implementer attempts; a hidden cross-phase dependency surfaces that contradicts the plan; the user interrupts.
- On a stop, report: last merged phase, current branch, what blocked, and the smallest unblocking action.

# Final phase (08)

Phase 08 is the end-to-end fauxbooks demo + trace-back golden test. After merging phase 08:
- Run the demo command end-to-end on `main` to confirm it produces the documented audit pack.
- Verify the trace-back tool decomposes the Operating Income token into per-leaf contributions matching the golden.
- Report the final chain hash, the leaf contribution table, and total wall time across all 8 phases.

# Constraints

- One phase = one branch = one PR = one squash commit. Never bundle phases.
- Never modify phase plan files during execution unless a discovered constraint forces a documented amendment; in that case, amend in a separate `docs(derivation-graph): amend phase NN plan` PR before resuming.
- Every audit-chain schema change must preserve verify/replay of pre-existing bundles, per each phase's DoD.
- Do not skip the reviewer subagent for any phase, even mechanical ones.
- Do not push directly to main. All work lands via squash-merged PR.
- Do not bypass hooks, signing, or CI.
