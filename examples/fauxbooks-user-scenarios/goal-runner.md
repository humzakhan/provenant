# Goal And Runner Spec: Fauxbooks End-User Scenario E2E

## Goal

Create a runnable e2e suite for three natural finance questions over Fauxbooks-generated files. The suite must generate data, run Claude Code with Fauxbooks and Provenant MCP enabled, verify numeric correctness, prove Provenant grounding, and write one final markdown report.

## Command Contract

Suggested command:

```sh
hack/e2e/fauxbooks-user-scenarios.sh \
  --seed 424200 \
  --out /tmp/provenant-fauxbooks-user-scenarios
```

Environment overrides:

```text
FAUXBOOKS_DIR=/Users/humza/projects/fauxbooks
FBX_BIN=<prebuilt fbx>
CLAUDE_MODEL=sonnet
CLAUDE_MAX_BUDGET_USD=10
KEEP_E2E_WORKSPACE=1
RESULTS_PATH=examples/fauxbooks-user-scenarios/report.md
```

Exit codes:

- `0`: all scenarios pass.
- `1`: one or more scenario verification or grounding checks fail.
- `2`: setup/runtime failure before scenario verification completes.

## Runner Steps

1. Build `provenant` and `fbx`.
2. Create a clean disposable workspace unless `KEEP_E2E_WORKSPACE=1`.
3. Install Provenant Claude hooks in the workspace.
4. Configure Provenant and Fauxbooks MCP servers.
5. Generate all scenario datasets with deterministic seeds.
6. Run `fbx verify` for every generated dataset and require exit `0`.
7. Build one prompt per scenario in end-user language.
8. Run Claude Code headlessly for each scenario.
9. Save final answer, stdout, stderr, debug log, MCP logs, and hook evidence.
10. Run `provenant check-answer`.
11. Run direct grounded Stop-hook and negative Stop-hook controls.
12. Parse the final answer JSON and verify against CSV recomputation.
13. Write scenario verification JSON.
14. Write final markdown report.

## Scenario Prompt Rules

Each prompt should say:

- "Use the generated files as the source of truth."
- "Use Fauxbooks MCP at least once to inspect available bundles or documents."
- "Use Provenant MCP to mint every numeric value in your final answer."
- "Do not include numeric claims outside the final JSON unless they are also minted."
- "Return exactly one machine-readable block between scenario-specific begin/end markers."

Example marker:

```text
FINAL_USER_SCENARIO_JSON_BEGIN
{ ... }
FINAL_USER_SCENARIO_JSON_END
```

## Verifier Rules

General:

- Parse only the JSON block between the final markers.
- Reject missing, extra, or wrong-type fields.
- Reject unsupported numeric-looking fields.
- Normalize decimals to 9 fractional digits before comparison.
- Compare values to both a generated `expected.json` and direct recomputation from CSVs.

Quarterly revenue:

- Expected datasets: `2026-01`, `2026-02`, `2026-03`.
- Required files: `profit-loss.csv`.
- Required checks: monthly total revenue, Q1 revenue total, top revenue account per month.

Cash vs book revenue:

- Expected dataset: `2026-03`.
- Required files: `profit-loss.csv`, `customer_payments.csv`, `customer-statements.csv`.
- Required checks: total book revenue, total customer cash receipts, book-to-cash delta, payment total cross-check.

Accounts receivable aging:

- Expected dataset: `2026-03`.
- Required files: `ar-aging.csv`, `customer-statements.csv`.
- Required checks: bucket totals, AR aging total, largest customer by AR total, customer-statement closing-balance cross-check.

## Evidence Files

Each scenario should emit:

```text
prompts/<scenario>.md
answers/<scenario>.final.md
evidence/<scenario>.claude.out
evidence/<scenario>.claude.err
evidence/<scenario>.claude-debug.log
evidence/<scenario>.check-answer.out
evidence/<scenario>.check-answer.err
evidence/<scenario>.grounded-stop.json
evidence/<scenario>.grounded-stop.out
evidence/<scenario>.negative-stop.json
evidence/<scenario>.negative-stop.out
verification/<scenario>.json
```

The suite report must link to each file by path.

## Report Contract

The final `report.md` should include:

1. Run metadata: date, branch, commit, model, suite seed, binaries.
2. Scenario matrix: scenario, seed, `fbx verify`, truth check, grounding check, negative control, status.
3. Scenario findings: user prompt, key computed metrics, source files used.
4. Failure detail: exact mismatch, expected, actual, and artifact paths.
5. Final verdict and exit code.

Use `examples/fauxbooks-user-scenarios/golden-report.md` as the target shape.
