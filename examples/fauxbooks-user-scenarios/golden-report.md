# Fauxbooks End-User Scenario E2E Report

This is the golden markdown shape for the future runner output. The example values below were derived from local Fauxbooks `monthly-close` datasets generated with the listed seeds.

## Run Metadata

- verdict: `pass`
- suite seed: `424200`
- fbx version: `v0.1.0`
- Provenant command: `go build -o <workspace>/bin/provenant ./cmd/provenant`
- Fauxbooks command: `go -C /Users/humza/projects/fauxbooks build -o <workspace>/bin/fbx ./cmd/fbx`
- output root: `/tmp/provenant-fauxbooks-user-scenarios/workspace`
- report path: `examples/fauxbooks-user-scenarios/report.md`

## Scenario Matrix

| Scenario | Seed(s) | Dataset | fbx verify | Truth check | Grounding | Negative control | Status |
|---|---:|---|---|---|---|---|---|
| Quarterly revenue summary | `424201`, `424202`, `424203` | `monthly-close`, Jan-Mar 2026 | pass | pass | pass | blocked | pass |
| Cash vs book revenue reconciliation | `424301` | `monthly-close`, Mar 2026 | pass | pass | pass | blocked | pass |
| Accounts receivable aging | `424401` | `monthly-close`, Mar 2026 | pass | pass | pass | blocked | pass |

## Quarterly Revenue Summary

User prompt:

```text
I uploaded the January, February, and March 2026 finance exports. What was revenue for Q1 2026? Give me the monthly revenue totals, the quarter total, and the biggest revenue account driving each month.
```

Expected verified findings:

| Period | Total Revenue | Top Revenue Account | Top Account Amount |
|---|---:|---|---:|
| `2026-01` | `185951.480000000` | `4000 Sales Revenue` | `145633.670000000` |
| `2026-02` | `84784.440000000` | `4000 Sales Revenue` | `62724.560000000` |
| `2026-03` | `124325.210000000` | `4000 Sales Revenue` | `92136.280000000` |

Quarter total revenue: `395061.130000000`

Source files:

- `datasets/quarterly_revenue_summary/2026-01/profit-loss.csv`
- `datasets/quarterly_revenue_summary/2026-02/profit-loss.csv`
- `datasets/quarterly_revenue_summary/2026-03/profit-loss.csv`

Expected JSON excerpt:

```json
{
  "scenario_id": "quarterly_revenue_summary",
  "quarter": "2026-Q1",
  "quarter_total_revenue": "395061.130000000"
}
```

## Cash vs Book Revenue Reconciliation

User prompt:

```text
For March 2026, why does cash received not match booked revenue? Show booked revenue, customer cash receipts, the difference, and the main explanation categories.
```

Expected verified findings:

| Metric | Value |
|---|---:|
| Book revenue | `147136.260000000` |
| Customer cash receipts | `74002.660000000` |
| Book-to-cash delta | `73133.600000000` |

Expected explanation categories:

- uncollected invoices
- timing between invoice recognition and customer payment

Source files:

- `datasets/cash_vs_book_revenue_reconciliation/2026-03/profit-loss.csv`
- `datasets/cash_vs_book_revenue_reconciliation/2026-03/customer_payments.csv`
- `datasets/cash_vs_book_revenue_reconciliation/2026-03/customer-statements.csv`

Expected JSON excerpt:

```json
{
  "scenario_id": "cash_vs_book_revenue_reconciliation",
  "period": "2026-03",
  "book_revenue": "147136.260000000",
  "customer_cash_receipts": "74002.660000000",
  "book_to_cash_delta": "73133.600000000"
}
```

## Accounts Receivable Aging

User prompt:

```text
Which customers owe us money at the end of March 2026? Break receivables into aging buckets and call out the largest customer balance.
```

Expected verified findings:

| Bucket | Value |
|---|---:|
| 0-30 | `73367.100000000` |
| 31-60 | `0.000000000` |
| 61-90 | `0.000000000` |
| 91+ | `0.000000000` |
| Total | `73367.100000000` |

Largest customer balance:

| Party ID | Party Name | Total |
|---|---|---:|
| `cust-005` | `McKinsey` | `20714.830000000` |

Customer statement closing-balance cross-check: `20401.810000000`

Source files:

- `datasets/accounts_receivable_aging/2026-03/ar-aging.csv`
- `datasets/accounts_receivable_aging/2026-03/customer-statements.csv`
- `datasets/accounts_receivable_aging/2026-03/invoices.csv`
- `datasets/accounts_receivable_aging/2026-03/customer_payments.csv`

Expected JSON excerpt:

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

## Grounding And Controls

For every scenario:

- `provenant check-answer`: expected exit `0`
- grounded Stop hook: expected exit `0`, no block decision
- negative Stop hook: expected `{"decision":"block"}` for an ungrounded `999999999.000000000` claim

## Artifact Paths

The runner should fill these with concrete paths:

- prompts: `<workspace>/prompts/*.md`
- answers: `<workspace>/answers/*.final.md`
- verification JSON: `<workspace>/verification/*.json`
- check-answer evidence: `<workspace>/evidence/*.check-answer.out`
- grounded Stop-hook evidence: `<workspace>/evidence/*.grounded-stop.out`
- negative Stop-hook evidence: `<workspace>/evidence/*.negative-stop.out`

## Final Verdict

`pass`: all generated datasets verified, all scenario answers matched recomputed truth, all numeric claims were Provenant-grounded, and all negative controls blocked.
