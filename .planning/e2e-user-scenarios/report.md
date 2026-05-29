# Fauxbooks End-User Scenario E2E Report

## Run Metadata

- verdict: `pass`
- exit code: `0`
- date: `2026-05-29T03:53:49.414750+00:00`
- branch: `test/fauxbooks-user-scenario-e2e`
- commit: `a1ac210092a12c7da3a044213bc17e61af87e23e`
- model: `sonnet`
- suite seed: `424200`
- Provenant binary: `/tmp/provenant-fauxbooks-user-scenarios/bin/provenant`
- Fauxbooks binary: `/tmp/provenant-fauxbooks-user-scenarios/bin/fbx`
- output root: `/tmp/provenant-fauxbooks-user-scenarios/workspace`
- report path: `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/faithful-fender/.planning/e2e-user-scenarios/report.md`

## Scenario Matrix

| Scenario | Seed(s) | Dataset | fbx verify | Truth check | Grounding | Negative control | Status |
|---|---:|---|---|---|---|---|---|
| Quarterly revenue summary | `424201, 424202, 424203` | `monthly-close` | pass | pass | pass | blocked | pass |
| Cash vs book revenue reconciliation | `424301` | `monthly-close` | pass | pass | pass | blocked | pass |
| Accounts receivable aging | `424401` | `monthly-close` | pass | pass | pass | blocked | pass |

## Quarterly revenue summary

User prompt:

```text
I uploaded the January, February, and March 2026 finance exports. What was revenue for Q1 2026? Give me the monthly revenue totals, the quarter total, and the biggest revenue account driving each month.
```

Key computed metrics:

| Period | Total Revenue | Top Revenue Account | Top Account Amount |
|---|---:|---|---:|
| `2026-01` | `185951.480000000` | `4000 Sales Revenue` | `145633.670000000` |
| `2026-02` | `84784.440000000` | `4000 Sales Revenue` | `62724.560000000` |
| `2026-03` | `124325.210000000` | `4000 Sales Revenue` | `92136.280000000` |

Quarter total revenue: `395061.130000000`

Source files:

- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/quarterly_revenue_summary/2026-01/profit-loss.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/quarterly_revenue_summary/2026-02/profit-loss.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/quarterly_revenue_summary/2026-03/profit-loss.csv`

Answer JSON excerpt:

```json
{
  "months": [
    {
      "period": "2026-01",
      "top_account_amount": "145633.670000000",
      "top_account_code": "4000",
      "total_revenue": "185951.480000000"
    },
    {
      "period": "2026-02",
      "top_account_amount": "62724.560000000",
      "top_account_code": "4000",
      "total_revenue": "84784.440000000"
    },
    {
      "period": "2026-03",
      "top_account_amount": "92136.280000000",
      "top_account_code": "4000",
      "total_revenue": "124325.210000000"
    }
  ],
  "quarter": "2026-Q1",
  "quarter_total_revenue": "395061.130000000",
  "scenario_id": "quarterly_revenue_summary"
}
```

Artifact paths:

- `/tmp/provenant-fauxbooks-user-scenarios/workspace/prompts/quarterly_revenue_summary.md`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/answers/quarterly_revenue_summary.final.md`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/quarterly_revenue_summary/expected.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/.provenant/claude-code/83a909e7-d020-4f94-bfe3-d263d36ab024/current.bundle.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.claude.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.claude.err`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.claude-debug.log`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.check-answer.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.check-answer.err`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.grounded-stop.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.grounded-stop.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.negative-stop.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.negative-stop.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.provenant-mcp.requests.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.provenant-mcp.responses.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.fbx-mcp.requests.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/quarterly_revenue_summary.fbx-mcp.responses.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/verification/quarterly_revenue_summary.json`

## Cash vs book revenue reconciliation

User prompt:

```text
For March 2026, why does cash received not match booked revenue? Show booked revenue, customer cash receipts, the difference, and the main explanation categories.
```

Key computed metrics:

| Metric | Value |
|---|---:|
| Book revenue | `147136.260000000` |
| Customer cash receipts | `74002.660000000` |
| Book-to-cash delta | `73133.600000000` |
| Customer statement payment cross-check | `74002.660000000` |

Source files:

- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/cash_vs_book_revenue_reconciliation/2026-03/profit-loss.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/cash_vs_book_revenue_reconciliation/2026-03/customer_payments.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/cash_vs_book_revenue_reconciliation/2026-03/customer-statements.csv`

Answer JSON excerpt:

```json
{
  "book_revenue": "147136.260000000",
  "book_to_cash_delta": "73133.600000000",
  "customer_cash_receipts": "74002.660000000",
  "explanations": [
    "uncollected invoices",
    "timing between invoicing and payment"
  ],
  "period": "2026-03",
  "scenario_id": "cash_vs_book_revenue_reconciliation"
}
```

Artifact paths:

- `/tmp/provenant-fauxbooks-user-scenarios/workspace/prompts/cash_vs_book_revenue_reconciliation.md`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/answers/cash_vs_book_revenue_reconciliation.final.md`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/cash_vs_book_revenue_reconciliation/expected.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/.provenant/claude-code/aedc847e-f929-43bb-96bf-eebd38e49c32/current.bundle.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.claude.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.claude.err`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.claude-debug.log`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.check-answer.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.check-answer.err`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.grounded-stop.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.grounded-stop.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.negative-stop.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.negative-stop.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.provenant-mcp.requests.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.provenant-mcp.responses.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.fbx-mcp.requests.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/cash_vs_book_revenue_reconciliation.fbx-mcp.responses.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/verification/cash_vs_book_revenue_reconciliation.json`

## Accounts receivable aging

User prompt:

```text
Which customers owe us money at the end of March 2026? Break receivables into aging buckets and call out the largest customer balance.
```

Key computed metrics:

| Bucket | Value |
|---|---:|
| 0-30 | `73367.100000000` |
| 31-60 | `0.000000000` |
| 61-90 | `0.000000000` |
| 91+ | `0.000000000` |
| Total | `73367.100000000` |

Largest customer balance: `cust-005` `McKinsey` `20714.830000000`
Customer statement closing-balance cross-check: `20401.810000000`

Source files:

- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/accounts_receivable_aging/2026-03/ar-aging.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/accounts_receivable_aging/2026-03/customer-statements.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/accounts_receivable_aging/2026-03/invoices.csv`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/accounts_receivable_aging/2026-03/customer_payments.csv`

Answer JSON excerpt:

```json
{
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
  },
  "period": "2026-03",
  "scenario_id": "accounts_receivable_aging"
}
```

Artifact paths:

- `/tmp/provenant-fauxbooks-user-scenarios/workspace/prompts/accounts_receivable_aging.md`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/answers/accounts_receivable_aging.final.md`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/datasets/accounts_receivable_aging/expected.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/.provenant/claude-code/df5ae13c-5eb1-4659-8fb0-07c1ec15fba0/current.bundle.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.claude.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.claude.err`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.claude-debug.log`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.check-answer.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.check-answer.err`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.grounded-stop.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.grounded-stop.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.negative-stop.json`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.negative-stop.out`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.provenant-mcp.requests.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.provenant-mcp.responses.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.fbx-mcp.requests.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/evidence/accounts_receivable_aging.fbx-mcp.responses.jsonl`
- `/tmp/provenant-fauxbooks-user-scenarios/workspace/verification/accounts_receivable_aging.json`

## Final Verdict

`pass` with exit code `0`.
