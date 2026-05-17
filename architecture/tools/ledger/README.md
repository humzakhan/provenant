# Tool Category: Ledger

Status: **v0**

> Read-only access to the general ledger, sub-ledgers, and trial balance. The ledger category is how an agent learns "what does the books say" — balances, transactions, account hierarchy, trial balance positions at a point in time.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md) — registry contract
- [`subsystems/kernel.md`](../../subsystems/kernel.md) — token minting

## Depended on by
- `recon` skill pack
- `ap_coding` skill pack
- `expense_review` skill pack
- `month_end_close` skill pack (future)

## Purpose

Every finance workflow eventually needs to ask the books a question. Recon needs the GL bank account balance and its transactions for the period. AP coding needs to know which GL codes exist. Expense review needs to know whether an employee's department has the budget GL set up. The ledger category is the single, canonical interface for these reads.

By keeping `ledger.*` read-only and backend-pluggable, skill packs become portable across ERP systems. A recon skill written against `ledger.get_balance` runs against SQL today and QuickBooks tomorrow with only a configuration change.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `ledger.get_balance` | Read the balance of an account at a point in time | CapturedDeterministic |
| `ledger.get_transactions` | List transactions for an account over a period | CapturedDeterministic |
| `ledger.search` | Free-form search across accounts and transactions | CapturedDeterministic |
| `ledger.list_accounts` | Chart of accounts | CapturedDeterministic |

All four are read-only. Write paths (journal entries) are a separate category (planned: `journal`) with stricter HITL and audit requirements.

## Schemas

### `ledger.get_balance`

Input:
```json
{
  "account_id": "1010",
  "as_of": "2026-05-31T23:59:59Z",
  "currency": "USD"
}
```

Output:
```json
{
  "account_id": "1010",
  "account_name": "Operating Cash - First National",
  "balance": "1247322.18",
  "currency": "USD",
  "as_of": "2026-05-31T23:59:59Z",
  "metadata": {
    "ledger_version": "...",
    "source_system": "sql_backend_default"
  }
}
```

Mintable paths: `$.balance` (with `unit_from=$.currency`, `as_of_from=$.as_of`).

### `ledger.get_transactions`

Input:
```json
{
  "account_id": "1010",
  "period_start": "2026-05-01",
  "period_end": "2026-05-31",
  "currency": "USD"
}
```

Output:
```json
{
  "account_id": "1010",
  "transactions": [
    {
      "id": "txn_1",
      "date": "2026-05-02",
      "amount": "1240.00",
      "currency": "USD",
      "description": "Deposit from customer X",
      "counter_account": "4010"
    },
    ...
  ],
  "period_start": "2026-05-01",
  "period_end": "2026-05-31",
  "total_count": 248
}
```

Mintable paths: `$.transactions[*].amount` (with `unit_from=$.transactions[*].currency`, `as_of_from=$.transactions[*].date`).

### `ledger.search`

Input:
```json
{
  "query": "ACME Corp",
  "filters": {
    "date_from": "2026-05-01",
    "date_to": "2026-05-31",
    "amount_gte": "100.00",
    "amount_lte": "10000.00"
  },
  "limit": 50
}
```

Output: same transaction shape as `get_transactions`, ranked by relevance.

### `ledger.list_accounts`

Input:
```json
{
  "active_only": true,
  "category": "asset"
}
```

Output:
```json
{
  "accounts": [
    {
      "id": "1010",
      "name": "Operating Cash - First National",
      "category": "asset",
      "subcategory": "current_asset",
      "currency": "USD",
      "active": true,
      "parent_id": "1000"
    },
    ...
  ]
}
```

No mintable paths (non-numeric reference data).

## Token semantics

- **Balances and transaction amounts**: minted with `currency` from the sibling field, `as_of` from the date/timestamp, and `source.path` pointing into the response payload.
- **Account IDs**: not minted (they are identifiers, not numeric quantities).
- **Account counts** in summary responses: minted as `unit=count`.

## Determinism and replay

All four tools are `CapturedDeterministic`. Reads of upstream state vary over time (an account's balance changes as transactions post), so replay requires a captured snapshot.

The kernel persists each tool call's response to S3 keyed by the `tool_call_id`. Replay reads from the snapshot, not the live source.

Captured-read identifier convention:
```
ledger:<backend>:<tool>:<canonical-input-hash>
```

## Confidence

Ledger reads do not emit confidence scores — data is canonical. No HITL routing based on extraction confidence.

## Failure modes

| Code | When | Recommended skill handling |
|---|---|---|
| `ledger.account_not_found` | `account_id` doesn't exist | Surface to user; recon cannot proceed without a valid account |
| `ledger.permission_denied` | Backend permission rejection | Escalate to admin; not retriable by agent |
| `ledger.period_locked` | The requested period is closed | Tool returns a flag; skill decides whether to proceed |
| `ledger.upstream_unavailable` | Backend network/auth failure | Tool returns `tool_unavailable`; harness retries per policy |
| `ledger.cursor_expired` | Pagination cursor stale | Auto-restart pagination from beginning |

## Performance budget

| Tool | p50 | p95 |
|---|---|---|
| `get_balance` | 100ms | 500ms |
| `get_transactions` (≤1000 rows) | 200ms | 1000ms |
| `get_transactions` (>1000 rows) | (paginate; not measured as single call) | |
| `search` | 300ms | 2000ms |
| `list_accounts` | 200ms | 1000ms |

Per-call timeout: 30 seconds (configurable per backend).

## Backends

| Backend | Status | Doc |
|---|---|---|
| SQL | v0 | [backends/sql.md](backends/sql.md) |
| QuickBooks | deferred (v0.5) | [backends/quickbooks.md](backends/quickbooks.md) |
| NetSuite | deferred (v0.5) | [backends/netsuite.md](backends/netsuite.md) |

The SQL backend is the v0 reference. It targets a normalized schema (`accounts`, `transactions`, `account_balances`) that customers can populate from any ERP via ETL. This decouples Provenant from individual ERP integration timelines.

## Tests

- Schema conformance: 5 positive + 5 negative tests per tool input/output.
- Replay: each tool replayed from captured reads produces identical token IDs.
- Backend integration: SQL backend tested against a fixture database with known balances and transactions.
- Failure modes: each error code surfaces correctly under simulated upstream conditions.

## References

- [`subsystems/tools.md`](../../subsystems/tools.md) — registry contract
- [`subsystems/kernel.md`](../../subsystems/kernel.md) — token minting from `MintablePaths`
- [Anthropic finance agents launch](../../references.md#anthropic-finance-agents) — GL agent shape that informed this category
