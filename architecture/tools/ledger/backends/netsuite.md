# Ledger Backend: NetSuite

Status: **v0.5 / deferred**

> Native Oracle NetSuite integration via SuiteQL. Planned for v0.5 after enterprise customer demand.

## Why deferred

NetSuite is the dominant ERP in mid-market and upper mid-market. The reason to defer follows QuickBooks: NetSuite integration has its own auth model (Token-Based Authentication or OAuth 2.0), SuiteQL has gotchas, and customers in this segment usually already have a data warehouse with NetSuite replicated — making the SQL backend a reasonable v0 path.

## Design constraints when we build it

1. **Auth**: Token-Based Authentication (TBA) primarily; OAuth 2.0 supported as a secondary.
2. **Transport**: SuiteQL via the REST API. We do not depend on SuiteScript or other in-NetSuite custom code.
3. **Schema mapping**: NetSuite's `account`, `transaction`, `transactionline`, `subsidiary` map to the canonical schema with subsidiary as a parent dimension.
4. **Multi-subsidiary**: A NetSuite OneWorld customer typically wants reconciliation scoped per subsidiary. The backend supports subsidiary as a filter on every query.
5. **Custom segments**: NetSuite users heavily customize the chart of accounts with custom segments. The canonical schema's `metadata` JSONB carries them.

## Known integration friction

- SuiteQL has surprising limits (3000 rows per page, no certain operators).
- TBA tokens have long lifetimes but no refresh; rotation is a manual operation customers must schedule.
- Multi-subsidiary balances require explicit consolidation logic; we do not assume a default.

## Schedule

v0.5 timeline depends on enterprise customer pull. Earliest realistic: 6-9 months after v0 GA.

## References

- [`tools/ledger/README.md`](../README.md)
- NetSuite SuiteQL: https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_156257770590.html
