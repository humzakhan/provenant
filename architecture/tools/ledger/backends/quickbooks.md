# Ledger Backend: QuickBooks

Status: **v0.5 / deferred**

> Native QuickBooks Online integration. Planned for v0.5 after the SQL backend stabilizes and we have customer demand signal.

## Why deferred

QuickBooks has a sizable installed base in SMB and lower mid-market — exactly the segment we expect to adopt Provenant earliest. The reason to defer is operational: QuickBooks Online's API has rate limits, OAuth refresh complexity, and a schema that doesn't map cleanly to the canonical `accounts`/`transactions`/`account_balances` shape. Building it before v0 stability would consume bandwidth needed elsewhere.

The bridge for v0 customers on QuickBooks: write a periodic ETL job (Fivetran, Airbyte, or custom) that lands QuickBooks data into the canonical SQL schema. The SQL backend then serves Provenant. Latency: hours, not seconds — acceptable for batch workflows like nightly recon.

## Design constraints when we build it

1. **OAuth 2.0**: Standard QuickBooks OAuth with refresh token storage in tenant secrets.
2. **Rate limits**: 500 requests/minute per company. Backend implements a token bucket and queues bursts.
3. **Schema mapping**: A mapping layer translates QBO's `Account`, `Item`, `JournalEntry`, `Bill`, `Invoice`, etc., into the canonical schema.
4. **Captured reads**: Same convention as SQL backend.
5. **Currency**: QBO's multi-currency support maps to per-currency entries.
6. **Realms**: Each QBO company is a separate realm; the backend supports multi-realm config under one tenant.

## Known integration friction

- QBO returns balances only at "now"; for `as_of` queries we must reconstruct via transactions. This is computationally expensive on large companies.
- QBO's `Account` IDs are realm-scoped and not stable across realms; the canonical schema needs realm-qualified IDs.
- QBO closes books periodically; closed-period reads are reliable, open-period reads can drift mid-day.

## Schedule

v0.5 timeline depends on:
- v0 GA and stability
- Customer pull from QuickBooks-shop pilots
- Resolution of the open question on realm-qualified IDs in the canonical schema

Earliest realistic: 6 months after v0 GA.

## References

- [`tools/ledger/README.md`](../README.md) — category contract this backend will implement
- QuickBooks Online API: https://developer.intuit.com/app/developer/qbo/docs/get-started
