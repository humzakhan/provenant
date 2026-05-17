# Ledger Backend: SQL

Status: **v0 / reference backend**

> The SQL backend is the v0 reference implementation for the ledger category. It targets a normalized schema that customers populate from their ERP via ETL or CDC. This decouples Provenant from individual ERP integration timelines and gives customers full control over their financial data layout.

## Why SQL first

Three reasons:

1. **Decoupling.** Every ERP has a different API, auth model, rate limit policy, and schema. Building dedicated backends for each delays our ability to ship recon for *anyone*. A SQL backend lets every customer integrate by writing one ETL job — a well-understood operation.
2. **Air-gapped deployments.** Some customers cannot allow Provenant to reach external SaaS APIs. SQL works in air-gapped environments without modification.
3. **Performance.** Direct SQL queries are faster than any ERP API for analytical reads, and Provenant's recon workflows are read-heavy.

QuickBooks and NetSuite backends will follow in v0.5 for customers who want to skip the ETL step.

## Connection

```yaml
backend: sql
config:
  driver: postgres        # postgres | mysql | sqlserver | sqlite
  dsn: "postgres://provenant_ledger@db.acme.internal:5432/erp_replica?sslmode=verify-full"
  schema: "ledger"        # optional; defaults to the connection's default schema
  pool:
    max_open: 10
    max_idle: 5
    conn_lifetime_seconds: 300
```

Auth via DSN. The Provenant operator is expected to provision a read-only role on a read replica of the ERP database (or an ETL'd warehouse). Direct access to a production ERP database is discouraged for performance and audit reasons.

## Schema mapping

The SQL backend expects a canonical normalized schema:

```sql
CREATE TABLE accounts (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    category            TEXT NOT NULL,        -- asset | liability | equity | revenue | expense
    subcategory         TEXT,                  -- current_asset | non_current_asset | ...
    currency            TEXT NOT NULL,         -- ISO 4217
    active              BOOLEAN NOT NULL DEFAULT TRUE,
    parent_id           TEXT REFERENCES accounts(id),
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

CREATE TABLE transactions (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL REFERENCES accounts(id),
    date                DATE NOT NULL,
    posted_at           TIMESTAMPTZ NOT NULL,
    amount              NUMERIC(38, 9) NOT NULL,
    currency            TEXT NOT NULL,
    description         TEXT,
    counter_account_id  TEXT REFERENCES accounts(id),
    reference           TEXT,
    metadata            JSONB
);

CREATE INDEX idx_transactions_account_date ON transactions(account_id, date);
CREATE INDEX idx_transactions_posted_at ON transactions(posted_at);

CREATE TABLE account_balances (
    account_id          TEXT NOT NULL REFERENCES accounts(id),
    as_of               DATE NOT NULL,
    balance             NUMERIC(38, 9) NOT NULL,
    currency            TEXT NOT NULL,
    PRIMARY KEY (account_id, as_of)
);
```

The customer's ETL populates these tables. The backend uses precomputed `account_balances` when available; otherwise it computes balances from `transactions` on the fly with proper sign conventions per `accounts.category`.

### Customizing the schema

Customers with existing replication or warehouse layouts can override the canonical SQL with view-based mapping. The backend resolves table names through a config block:

```yaml
backend: sql
config:
  table_map:
    accounts: "warehouse.dim_account"
    transactions: "warehouse.fact_journal_entry"
    account_balances: "warehouse.fact_account_balance_daily"
  column_map:
    accounts.id: "account_key"
    accounts.name: "account_name"
    accounts.category: "account_type"
    # ...
```

This adds complexity; the recommended path is to populate the canonical schema directly.

## Captured reads

Every `ledger.*` call's response is persisted to S3:

```
s3://provenant-<tenant>/captured-reads/<tool_call_id>.json
```

The captured read includes:

```json
{
  "tool_call_id": "tc_01HXYZ...",
  "source": "ledger:sql:get_balance:sha256_of_input",
  "as_of": "2026-05-31T23:59:59Z",
  "input": {...},
  "output": {...},
  "captured_at": "2026-05-31T23:59:50Z",
  "schema_version": "1.0.0"
}
```

Replay reads this snapshot instead of hitting the live database, enabling bit-identical reproduction.

## Rate limits and retries

| Limit | Default |
|---|---|
| Concurrent connections | 10 (from pool) |
| Per-tool RPS | 25 |
| Query timeout | 30 seconds |
| Retries on transient error | 3 with exponential backoff (100ms, 500ms, 2s) |

Transient errors: connection lost, deadlock detected, statement timeout. Non-retriable: permission denied, syntax error, table not found.

## Failure modes

| Backend error | Surfaced as |
|---|---|
| Connection refused | `ledger.upstream_unavailable` |
| Authentication failure | `ledger.upstream_unavailable` (with admin alert) |
| Account not found in `accounts` table | `ledger.account_not_found` |
| `account_balances` row missing for the date and fallback computation hits no transactions | `ledger.balance_unknown` (skill decides) |
| Currency mismatch between requested and stored | `ledger.currency_mismatch` |
| Query timeout | `ledger.upstream_timeout` |

## Tests

- Schema conformance: positive and negative tests against a fixture database.
- Multi-driver: tests run against Postgres, MySQL, SQLite (SQL Server in CI on Linux).
- Replay: captured reads enable bit-identical replay; verified across 1000 fixture runs.
- Performance: p95 latency below budget on a 10M-row transactions table with proper indexes.

## Open questions

1. **Sign conventions for derived balances.** Some customers store debits and credits as separate columns; others use signed amounts with category-dependent signs. Proposal: support both via a `sign_convention` config option, with explicit declaration required.
2. **Multi-currency at the account level.** Some accounts hold balances in multiple currencies (foreign currency cash accounts). Proposal: support natively in the canonical schema; the v0 implementation requires per-currency rows.

## References

- [`tools/ledger/README.md`](../README.md) — category contract this backend implements
- [`subsystems/tools.md`](../../../subsystems/tools.md) — registry contract
