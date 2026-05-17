# Tool Category: External

Status: **v0**

> Versioned access to external data — FX rates, sanctions lists, market data, vendor data. Every external read is captured for replay and tagged with its source identifier and `as_of` timestamp so that point-in-time semantics survive into the future. The external category is where Provenant guarantees that "the FX rate we used on 2026-05-31" is reproducible forever.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)

## Depended on by
- `recon` skill pack (multi-currency)
- `ap_coding` skill pack (vendor sanctions screening)
- `sanctions_disposition` skill pack (OFAC and other sanctions lists)
- `valuation_review` skill pack (market data, future)

## Purpose

External data is the most regulatorily-sensitive surface in any finance agent. An auditor asks: "what was the EUR/USD rate you used to convert this transaction?" The answer cannot be "what Bloomberg said today." It must be "OpenFX 4pm ECB rate, 2026-05-31, value 1.0823, captured at this S3 key, hash X."

The external category enforces three properties:

1. **Versioned**: every record has a clearly identified source version and `as_of` timestamp.
2. **Captured**: every read is persisted, enabling bit-identical replay.
3. **Typed**: results are typed (FX rate, sanctions hit, market quote) — not free-form responses from third-party APIs.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `external.fx_rate` | Retrieve an FX rate at a point in time | CapturedDeterministic |
| `external.sanctions_check` | Screen a name against sanctions lists | CapturedDeterministic |
| `external.market_quote` | Retrieve a market quote (security price) at a point in time | CapturedDeterministic |
| `external.entity_lookup` | Resolve a vendor or counterparty by name to a canonical entity | CapturedDeterministic |

## Schemas

### `external.fx_rate`

Input:
```json
{
  "base": "EUR",
  "quote": "USD",
  "as_of": "2026-05-31T16:00:00Z",
  "source_preference": ["ecb_4pm", "openfx_close"]   // ordered fallback
}
```

Output:
```json
{
  "base": "EUR",
  "quote": "USD",
  "rate": "1.0823",
  "as_of": "2026-05-31T16:00:00Z",
  "source": "ecb_4pm",
  "source_version": "2026-05-31",
  "captured_at": "2026-05-31T17:02:14Z"
}
```

Mintable paths: `$.rate` with `unit=EUR/USD` (the FX rate's unit is the currency pair). This is how `calc.convert_currency` validates unit semantics — the rate token's unit must match the input/output currencies.

### `external.sanctions_check`

Input:
```json
{
  "name": "ACME Holdings International Ltd",
  "type": "entity",                   // entity | individual | vessel | aircraft
  "context": {
    "country": "GB",
    "registration_number": "12345678",
    "aliases": ["ACME Intl"]
  },
  "lists": ["ofac_sdn", "uk_ofsi", "eu_consolidated"]
}
```

Output:
```json
{
  "checked_lists": ["ofac_sdn", "uk_ofsi", "eu_consolidated"],
  "as_of": {
    "ofac_sdn":         "2026-05-30",
    "uk_ofsi":          "2026-05-29",
    "eu_consolidated":  "2026-05-30"
  },
  "hits": [
    {
      "list": "ofac_sdn",
      "score": 0.87,
      "match_type": "fuzzy",
      "matched_name": "ACME Holdings Intl Ltd",
      "list_id": "OFAC-SDN-12345",
      "list_entry_data": {
        "programs": ["UKRAINE-EO13662"],
        "addresses": [...],
        "aliases": [...]
      }
    }
  ],
  "no_hits_for": ["uk_ofsi", "eu_consolidated"]
}
```

Mintable paths: `$.hits[*].score` (unit=score).

### `external.market_quote`

Input:
```json
{
  "symbol": "AAPL",
  "exchange": "NASDAQ",
  "as_of": "2026-05-31T20:00:00Z",
  "field": "close"
}
```

Output: price with currency, exchange, source, `as_of`.

### `external.entity_lookup`

Input: name + optional disambiguators. Output: canonical entity with stable ID, tax ID, registered address, registration status. Used for AP coding and KYC.

## Token semantics

- FX rates: minted with unit = base/quote currency pair.
- Sanctions hit scores: minted with unit = score.
- Market prices: minted with the quote currency.

`as_of` is critical for every external token. The kernel's audit pack export renders `as_of` prominently for every external token.

## Determinism and replay

All four tools are `CapturedDeterministic`. The capture key includes:

- The input.
- The source (e.g., `ecb_4pm`).
- The source version (the date of the list snapshot for sanctions; the published date for FX).

Replay returns the captured value regardless of upstream availability — important when sources have rate limits or have changed pricing models.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `external.source_unavailable` | All preferred sources unreachable | Skill aborts or proceeds without (depending on policy) |
| `external.no_data_for_date` | Source has no data for requested `as_of` | Skill must decide: use nearest available, fail, or interpolate |
| `external.symbol_not_found` | Symbol/entity not recognized | Skill surfaces |
| `external.upstream_throttled` | Rate limit exhausted with no retry budget | Retry per policy |

## Performance budget

| Tool | p50 | p95 |
|---|---|---|
| `fx_rate` | 100ms | 500ms |
| `sanctions_check` | 500ms | 3000ms |
| `market_quote` | 200ms | 1000ms |
| `entity_lookup` | 300ms | 2000ms |

Per-call timeout: 30 seconds.

## Backends

| Backend | Status | Doc |
|---|---|---|
| OpenFX | v0 (FX) | [backends/openfx.md](backends/openfx.md) |
| OFAC | v0 partial (sanctions, single list) | [backends/ofac.md](backends/ofac.md) |
| PitchBook | deferred (v0.5) | [backends/pitchbook.md](backends/pitchbook.md) |

v0 covers the most commonly needed external data: FX rates for multi-currency recon and OFAC sanctions for KYC and AP screening. Market data and broader entity resolution are v0.5+ deferred.

## Tests

- Schema conformance per tool.
- Replay: captured payloads reproduce identically.
- Versioning: source-version tagging persists through audit packs.
- Failure modes covered.
- Latency budgets enforced.

## Open questions

1. **Cross-source consensus**. Should we run multiple FX sources and surface a consensus rate? Probably yes for high-stakes recon. Defer to v0.5; current behavior is preferred-source-with-fallback.
2. **List update cadence**. OFAC publishes daily; some EU lists weekly. The capture key includes source-version. Should we proactively refresh on access if a newer version is available? Proposal: yes, with a configurable max-staleness per list.

## References

- [Anthropic finance agents](../../references.md#anthropic-finance-agents) — KYC screener and FX-aware reconciliation patterns
- [FINRA 2026](../../references.md#finra-2026) — point-in-time data retention requirements
