# Goal: Fauxbooks Claude/Provenant E2E Harness

## Objective
Build and validate an end-to-end harness that proves a headless Claude Code session can solve a randomly generated Fauxbooks reconciliation task while every numeric claim in the final answer is grounded through Provenant MCP pass-through verification.

## Scope and Constraints
- Work only against this repo's e2e harness surface and the existing plan at `.planning/e2e-harness/plan.md`.
- Tie the harness to `/Users/humza/projects/fauxbooks` as the dataset/tool source and to Provenant as the MCP grounding layer.
- Do not modify `plan.md` or unrelated code in this goal.
- The harness must be bash-verifiable outside the product.
- Treat `MCV` as `MCP`.

## Concrete Deliverables
- A runnable harness flow that:
  - builds local `provenant` and `fbx` binaries,
  - creates a disposable workspace,
  - installs Provenant Claude hooks,
  - configures both Provenant and Fauxbooks MCP servers,
  - generates a random but recorded Fauxbooks reconciliation dataset,
  - launches Claude Code non-interactively,
  - captures transcript and hook evidence,
  - verifies the final answer against Fauxbooks truth data,
  - verifies grounding through `provenant check-answer`,
  - runs a negative control that proves ungrounded numeric claims are blocked.
- A final results artifact with seed, dataset path, bundle path, exit codes, and evidence locations.

## Bash-Verifiable Acceptance Criteria
- `command -v claude` succeeds.
- `fbx gen recon-stress-test ...` runs and records a seed.
- `fbx verify "$OUT"` exits `2` for the stress dataset and that exit is expected.
- Claude is launched against the temp workspace with both MCP servers enabled.
- The Claude final answer includes a machine-readable anomaly block that can be parsed and compared to `truth.json`.
- Every numeric value in the final answer can be traced to a Fauxbooks source row or truth entry and passes `provenant check-answer`.
- The Provenant Stop hook returns `0` for grounded answers and blocks an ungrounded negative control containing a fake numeric claim.
- The harness emits a JSON summary proving `truth_matched: true`, `check_answer_exit: 0`, and `blocked_negative_control: true`.

## Stop Conditions and Failure Criteria
- Stop immediately if the local `claude` CLI cannot run non-interactively.
- Stop if the workspace cannot be configured with both MCP servers or Provenant hooks.
- Fail if the final answer contains numeric claims that cannot be grounded through Provenant.
- Fail if the parsed answer disagrees with `truth.json` or if any referenced amount does not match the source row exactly after canonical normalization.
- Fail if the negative control does not block.
- Fail if `provenant check-answer` does not exit `0`.

## Evidence Artifacts
- `evidence/claude-transcript.jsonl` or equivalent transcript surrogate.
- `evidence/claude.out` and `evidence/claude.err`.
- `evidence/provenant-hook.log`.
- `evidence/fbx-mcp.log` and `evidence/provenant-mcp.log`.
- `evidence/verification.json`.
- `data/generated/recon-$SEED/truth.json`.
- `data/answers/final-answer.md`.
- `evidence/negative-stop.json` and `evidence/negative-stop.out`.
- Exact `.mcp.json` and `.claude/settings.json` used for the run.

## Definition of Done
The goal is complete when the harness can be run end to end with bash-verifiable output showing:
1. Fauxbooks generated the dataset.
2. Claude solved the reconciliation task.
3. Provenant passed through and validated every numeric claim.
4. The parsed answer matched Fauxbooks truth.
5. The negative control was blocked.
