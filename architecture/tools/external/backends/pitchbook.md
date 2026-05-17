# External Backend: PitchBook

Status: **v0.5 / deferred**

> Vendor and counterparty enrichment via PitchBook. Planned for v0.5+ for AP coding and KYC workflows requiring deep entity intelligence.

## Why deferred

PitchBook is a premium commercial data source; the licensing model makes it inappropriate as a v0 default. Customers with PitchBook subscriptions will plug it in via this backend when v0.5 ships.

The v0 path for entity enrichment is the broader `external.entity_lookup` interface, against any backend the customer prefers. PitchBook is one specific implementation, not the only one.

## Design constraints when we build it

1. **Auth**: PitchBook OAuth 2.0 with API key.
2. **Rate limits**: Strict per-account RPS; backend implements a queue.
3. **Output mapping**: PitchBook's entity record maps to the canonical `entity_lookup` output (canonical ID, registered name, jurisdiction, registration number, status).
4. **Captured reads**: Same convention. Entity records change over time; the capture pins to the queried `as_of`.
5. **Licensing**: backend respects PitchBook's licensing constraints on cross-tenant data reuse.

## Alternatives we may ship before PitchBook

- **OpenCorporates** (free tier for basic data) — likely a v0.5 priority over PitchBook for cost reasons.
- **Customer-provided entity tables** — already supported via the SQL backend pattern; no new backend needed.

## Schedule

Defer until commercial customer demand justifies the licensing complexity.

## References

- [`tools/external/README.md`](../README.md)
- PitchBook: https://pitchbook.com
