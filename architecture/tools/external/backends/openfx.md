# External Backend: OpenFX

Status: **v0 / reference backend**

> OpenFX-style FX rate retrieval. Targets free/low-cost FX rate providers (open.er-api.com, exchangerate.host, central bank reference rates) with consistent point-in-time semantics.

## Why OpenFX first

1. **Free tier sufficient**: open APIs have generous free tiers; sufficient for low-volume v0 customers.
2. **ECB reference rates**: high-quality, regulator-accepted source published daily at 4pm CET.
3. **Determinism**: published rates are immutable once posted; perfect fit for captured-read replay.
4. **No vendor lock-in**: the backend is API-shape compatible across multiple providers.

Commercial-grade providers (Bloomberg BFIX, Refinitiv) are v0.5+ for customers needing intraday or tick-level rates.

## Connection

```yaml
backend: openfx
config:
  sources:
    - id: "ecb_4pm"
      url: "https://www.ecb.europa.eu/stats/eurofxref/"
      type: "ecb_xml"
      currencies_published: ["USD", "GBP", "JPY", "CHF", ...]
      schedule: "daily_1600_cet"
    - id: "openfx_close"
      url: "https://openexchangerates.org/api/historical/{date}.json"
      type: "openexchangerates"
      api_key_secret: "openfx-api-key"
      schedule: "daily"
  fallback_order: ["ecb_4pm", "openfx_close"]
  cache_ttl_hours: 24    # in-memory cache for repeated lookups of the same date
```

## Data ingestion

The backend pulls daily snapshots on schedule and stores them:

```sql
CREATE TABLE fx_rate_snapshots (
    source        TEXT NOT NULL,
    source_version TEXT NOT NULL,        -- typically the publication date
    base          TEXT NOT NULL,
    quote         TEXT NOT NULL,
    rate          NUMERIC(20, 10) NOT NULL,
    as_of         TIMESTAMPTZ NOT NULL,
    pulled_at     TIMESTAMPTZ NOT NULL,
    raw_payload_key TEXT NOT NULL,       -- S3 key for the full upstream payload
    PRIMARY KEY (source, source_version, base, quote)
);
```

Lookups query this table first; missing dates trigger an on-demand pull within configured rate limits. Snapshot raw payloads persist to S3 for full audit reproducibility.

## Schema mapping

ECB XML format → canonical `external.fx_rate` output via a parser in `internal/tools/external/backends/openfx/ecb.go`. OpenExchangeRates JSON has a more direct mapping.

Cross-rate computation: ECB publishes EUR-based rates; non-EUR pairs (e.g., GBP/JPY) compute as `(EUR/JPY) / (EUR/GBP)`. The kernel records the cross-rate derivation in the token metadata for audit.

## Captured reads

```
external:openfx:fx_rate:<sha256_of_input>
```

The capture stores the canonical response plus a pointer to the underlying snapshot. Replay returns the canonical response.

## Rate limits

ECB feed: pulled once daily, no rate-limit concern.
OpenExchangeRates free tier: 1000 requests/month. Backend uses the snapshot table to minimize live API hits.

## Failure modes

| Backend error | Surfaced as |
|---|---|
| All sources unreachable and snapshot table empty | `external.source_unavailable` |
| No rate published for requested date | `external.no_data_for_date` |
| Unsupported currency | `external.symbol_not_found` |
| API key rejected | `external.source_unavailable` (admin alert) |

## Tests

- Fixture: ECB XML samples parse to canonical responses with known rates.
- Cross-rate: derived non-EUR pairs match published cross-rates within precision tolerance.
- Replay: captured rates reproduce identically.
- Snapshot ingestion: daily pull job is idempotent.

## Open questions

1. **Weekend/holiday handling.** No published rate on weekends and holidays. Proposal: use prior published date; surface the difference in `as_of` so audit reflects reality.
2. **Bid/ask spreads.** Most free sources publish midrates only. Customers needing bid/ask need a commercial backend. Defer.

## References

- [`tools/external/README.md`](../README.md)
- ECB FX reference rates: https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/
