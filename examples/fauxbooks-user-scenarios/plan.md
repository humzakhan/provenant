# Fauxbooks End-User Scenario E2E Plan

## Objective

Add a second Fauxbooks e2e suite that tests user-facing finance questions, not only the reconciliation stress benchmark. The suite should prove that a user can hand the agent generated accounting files, ask practical finance questions, and receive answers whose numeric claims are both correct and Provenant-grounded.

The three first scenarios are:

1. Quarterly revenue summary.
2. Cash vs book revenue reconciliation.
3. Accounts receivable aging.

## Working Assumptions

- Fauxbooks source lives at `/Users/humza/projects/fauxbooks`.
- The runner builds a local `fbx` binary from that repo unless `FBX_BIN` is set.
- Provenant source is the current repo under test.
- Claude Code runs headlessly the same way as `hack/e2e/fauxbooks-claude-provenant.sh`.
- Raw generated datasets stay outside git; committed artifacts are the plan, runner spec, and golden report template.

## Branch And PR Scope

Implementation should happen on a dedicated branch, for example:

```sh
git fetch origin main --prune
git switch -c test/fauxbooks-user-scenario-e2e origin/main
```

The implementation PR should add one runner and the generated report location, likely:

```text
hack/e2e/fauxbooks-user-scenarios.sh
examples/fauxbooks-user-scenarios/report.md
```

The first implementation can be bash plus embedded Python verifiers. Keep it narrow: no new product API is required.

## Workspace Layout

Use a disposable workspace per run:

```text
/tmp/provenant-fauxbooks-user-scenarios/
  bin/
    fbx
    provenant
  workspace/
    .claude/settings.json
    .mcp.json
    hack/hooks/
    datasets/
      quarterly_revenue_summary/
      cash_vs_book_revenue_reconciliation/
      accounts_receivable_aging/
    prompts/
    answers/
    evidence/
      <scenario>.claude.out
      <scenario>.claude.err
      <scenario>.debug.log
      <scenario>.check-answer.out
      <scenario>.grounded-stop.out
      <scenario>.negative-stop.out
    verification/
      <scenario>.json
    report.md
```

The runner should copy `workspace/report.md` into `examples/fauxbooks-user-scenarios/report.md` after a successful or failed run so the PR has a stable human-readable artifact.

## Tool Setup

Build binaries:

```sh
go build -o "$BASE/bin/provenant" ./cmd/provenant
go -C /Users/humza/projects/fauxbooks build -o "$BASE/bin/fbx" ./cmd/fbx
```

Install Provenant into the disposable workspace and add Fauxbooks MCP:

```sh
cd "$WORKSPACE"
PROVENANT_BIN="$BASE/bin/provenant" "$BASE/bin/provenant" install --claude
```

Then ensure `.mcp.json` contains:

```json
{
  "mcpServers": {
    "provenant": {
      "type": "stdio",
      "command": "/tmp/provenant-fauxbooks-user-scenarios/bin/provenant",
      "args": ["mcp"],
      "env": {"PROVENANT_BUNDLE_PATH": "<scenario bundle path>"}
    },
    "fauxbooks": {
      "type": "stdio",
      "command": "/tmp/provenant-fauxbooks-user-scenarios/bin/fbx",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

For each scenario, use a distinct Claude `session_id` and bundle path so grounding evidence cannot leak between scenarios.

## Dataset Generation

Use `monthly-close`, not `recon-stress-test`, because these scenarios model ordinary user questions over clean finance exports.

Recommended deterministic seed set for the first checked-in results:

```text
suite_seed: 424200
quarterly_revenue_summary:
  2026-01 seed: 424201
  2026-02 seed: 424202
  2026-03 seed: 424203
cash_vs_book_revenue_reconciliation:
  2026-03 seed: 424301
accounts_receivable_aging:
  2026-03 seed: 424401
```

Generation commands:

```sh
fbx gen monthly-close --seed 424201 --period 2026-01 --volume small --format csv --out "$DATASETS/quarterly_revenue_summary/2026-01"
fbx gen monthly-close --seed 424202 --period 2026-02 --volume small --format csv --out "$DATASETS/quarterly_revenue_summary/2026-02"
fbx gen monthly-close --seed 424203 --period 2026-03 --volume small --format csv --out "$DATASETS/quarterly_revenue_summary/2026-03"
fbx gen monthly-close --seed 424301 --period 2026-03 --volume small --format csv --out "$DATASETS/cash_vs_book_revenue_reconciliation/2026-03"
fbx gen monthly-close --seed 424401 --period 2026-03 --volume small --format csv --out "$DATASETS/accounts_receivable_aging/2026-03"
```

Run `fbx verify <dataset-dir>` for every generated directory and require exit `0`.

## Scenario 1: Quarterly Revenue Summary

User prompt:

```text
I uploaded the January, February, and March 2026 finance exports. What was revenue for Q1 2026? Give me the monthly revenue totals, the quarter total, and the biggest revenue account driving each month.
```

Source files:

- `profit-loss.csv` for each month.
- Optional supporting files: `invoices.csv`, `gl.csv`, `manifest.json`.

Verifier:

- Read `profit-loss.csv`.
- Find `section=total_revenue` for each month.
- Sum the three values for Q1 total.
- Find the largest `section=revenue` account by amount for each month.
- Compare exact decimal strings after canonical normalization.

Required answer JSON:

```json
{
  "scenario_id": "quarterly_revenue_summary",
  "quarter": "2026-Q1",
  "quarter_total_revenue": "395061.130000000",
  "months": [
    {"period": "2026-01", "total_revenue": "185951.480000000", "top_account_code": "4000", "top_account_amount": "145633.670000000"}
  ]
}
```

## Scenario 2: Cash vs Book Revenue Reconciliation

User prompt:

```text
For March 2026, why does cash received not match booked revenue? Show booked revenue, customer cash receipts, the difference, and the main explanation categories.
```

Source files:

- `profit-loss.csv`
- `customer_payments.csv`
- `bank_statement.csv`
- `invoices.csv`
- `customer-statements.csv`

Verifier:

- Book revenue: `profit-loss.csv` row where `section=total_revenue`.
- Customer cash receipts: sum `customer_payments.csv.amount`.
- Difference: `book_revenue - customer_cash_receipts`.
- Cross-check that `customer-statements.csv` payment total equals the customer payment sum.
- Optional context: report net bank movement from `bank_statement.csv`, but do not require it to equal revenue receipts because bank rows include AP and other cash activity.

Required answer JSON:

```json
{
  "scenario_id": "cash_vs_book_revenue_reconciliation",
  "period": "2026-03",
  "book_revenue": "147136.260000000",
  "customer_cash_receipts": "74002.660000000",
  "book_to_cash_delta": "73133.600000000",
  "explanations": ["uncollected invoices", "timing between invoicing and payment"]
}
```

## Scenario 3: Accounts Receivable Aging

User prompt:

```text
Which customers owe us money at the end of March 2026? Break receivables into aging buckets and call out the largest customer balance.
```

Source files:

- `ar-aging.csv`
- `customer-statements.csv`
- `invoices.csv`
- `customer_payments.csv`

Verifier:

- Sum every bucket in `ar-aging.csv`.
- Sum `total` in `ar-aging.csv`.
- Find the row with the largest `total`.
- Cross-check `customer-statements.csv.closing_balance` separately and report it as a consistency check. It is allowed to differ from AR aging total if the report definitions differ; the answer must label both values accurately.

Required answer JSON:

```json
{
  "scenario_id": "accounts_receivable_aging",
  "period": "2026-03",
  "bucket_totals": {
    "bucket_0_30": "73367.100000000",
    "bucket_31_60": "0.000000000",
    "bucket_61_90": "0.000000000",
    "bucket_91_plus": "0.000000000",
    "total": "73367.100000000"
  },
  "largest_customer": {
    "party_id": "cust-005",
    "party_name": "McKinsey",
    "total": "20714.830000000"
  }
}
```

## Provenant Grounding Requirements

For every scenario:

- Claude must use Fauxbooks MCP at least once.
- Claude must mint every numeric value included in the final answer through `provenant.read_mint_bundle`.
- The runner must run `provenant check-answer --request <json>` against the final answer.
- The runner must directly invoke `hack/hooks/answer-grounding-stop-hook.sh` with the grounded final answer and assert it does not block.
- The runner must invoke the same Stop hook with an ungrounded negative control number and assert it blocks.

Negative control examples:

- Revenue scenario: `The Q1 revenue was 999999999.000000000.`
- Cash scenario: `Customer cash receipts were 999999999.000000000.`
- AR scenario: `Total receivables were 999999999.000000000.`

## Final Report

The runner writes `examples/fauxbooks-user-scenarios/report.md`. It should follow `golden-report.md` and include:

- command line and git SHA
- suite seed and scenario seeds
- per-scenario prompt
- generated dataset paths
- answer JSON excerpt
- verifier status
- grounding status
- negative control status
- final verdict

## Definition Of Done

The implementation is done when one command can run all three scenarios and produce a report showing:

- all datasets generated by Fauxbooks and verified with `fbx verify`
- all Claude answers parsed
- all numeric answers match deterministic recomputation from generated CSVs
- all answers pass Provenant grounding checks
- every negative control blocks
- the suite exits non-zero on any scenario failure
