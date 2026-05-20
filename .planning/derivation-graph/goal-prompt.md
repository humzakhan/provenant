> /goal harness prompt for Phase 09. The full technical spec — Implementation Steps, Tests Required, Definition of Done — lives at `.planning/derivation-graph/phase-09-scaled-demo-and-html-report.md`. Paste everything below the header (from the `---` frontmatter onward) into the **/goal** command to kick off the autonomous run. Keep this file under 4,000 characters; if the plan changes, re-condense and update here.

---
description: Execute Phase 09 — Scaled Demo, Broken Dataset, and HTML Audit Report
---

Autonomously execute Phase 09 at `.planning/derivation-graph/phase-09-scaled-demo-and-html-report.md`. Deliver all six Implementation Steps sequentially, one PR per step.

## Mode

Enter orchestrator mode. Load `engineering-workflow` for branch/validation/review/PR/merge discipline and `conventional-commit-guard` before every commit and PR title. Load `go-implementation-quality` for all Go work, `go-testing-fuzzing` for tests, `go-input-validation-security` only for the step-5 CLI verb. Models: `claude-sonnet-4-6` implementation; `claude-opus-4-7` (thinking on) architecture + reviewer; `claude-haiku-4-5` mechanical edits and golden generation.

## Before starting

Read the phase plan in full, plus `examples/derivation-graph/e2e_test.go` and `audit/pack/pack.go`. Do not modify the phase plan unless a discovered constraint forces an amendment — land that in a separate `docs(derivation-graph): amend phase-09 plan` PR before resuming.

## Per-step loop (NN = 1..6, in order)

1. Sync: `git fetch origin && git checkout main && git pull --ff-only origin main`.
2. Branch `feat/derivation-graph-phase-09-step-NN-<slug>` from updated main.
3. Delegate implementation to a sonnet-4-6 subagent with that step's Implementation Steps entry, its relevant Technical Design subsections, and its Tests Required rows as the sole spec.
4. On first run, implementer regenerates goldens via the `UPDATE_GOLDEN=1 go test ...` command from the plan's Determinism and goldens section and commits them with the implementation.
5. Gates (all must pass): `go test ./...`, `go vet ./...`, step-specific tests, and for steps 2/3/5 the phase-08 sentinel `go test ./examples/derivation-graph/... -run TestFauxbooksEndToEnd` byte-equal.
6. Independent opus-4-7 reviewer (thinking on) checks the step's Tests Required + Definition of Done. No approval if any golden is missing, any test is skipped, or `go test ./...` fails.
7. On CHANGES_REQUESTED, route fixes to a sonnet-4-6 subagent and re-review until APPROVE or APPROVE_WITH_FIXES.
8. PR title `feat(derivation-graph): phase 09 step NN — <short title>` via conventional-commit-guard. Body: satisfied Tests Required rows, checked Definition of Done items, golden regen command.
9. On clean CI + reviewer approval, squash-merge with `--delete-branch`, reset local main to `origin/main`.
10. Record merged SHA. Proceed to NN+1.

## Stop conditions

Stop and report if: reviewer returns CHANGES_REQUESTED twice on the same step; gates fail after two implementer attempts; a cross-step dependency contradicts the plan; the user interrupts. On stop, report last merged step, current branch, blocker, and smallest unblocking action.

## Final step (after 6 merges)

Run `go test ./examples/derivation-graph/...` on main — both phase-08 and phase-09 green. Run `provenant audit-html` on both scaled bundles (happy and broken) and confirm each HTML opens from disk with the correct invariant banner. Report: final SHA, full Definition of Done checked, total wall time.

## Constraints

- One step = one branch = one PR = one squash commit. Never bundle steps.
- Phase-08 demo (`examples/derivation-graph/`) untouched throughout; `TestFauxbooksEndToEnd` is the regression sentinel.
- Goldens are committed. `UPDATE_GOLDEN=1` is for initial generation only; never in CI.
- Determinism: fixed seeds, sorted map keys, JCS for embedded JSON, no `time.Now()` in any deterministic payload or HTML.
- HTML self-contained: no `<script>`, no external URLs, openable from disk. `TestRenderHTMLNoExternalAssets` is a hard gate.
- Never push to main directly; never skip hooks, signing, or CI.
- Broken-run semantics: `trace.Trace` does NOT raise `ErrContributionMismatch` on the broken bundle. Use `OutcomeMissingRight` (not `MissingLeft`) for EMP-0042 and assert `len(Differences) > 0` (not a literal reason string) for the tampered expense row.
