# External Backend: OFAC

Status: **v0 partial (OFAC SDN only; broader sanctions backends in v0.5)**

> OFAC SDN list-based sanctions screening. Loads the daily-published OFAC SDN list and screens names with fuzzy matching. Partial v0 coverage — single list, single source.

## Why partial v0

Full sanctions coverage (OFAC sectoral lists, UK OFSI, EU consolidated, UN consolidated, country-specific lists) is large enough to be a dedicated effort. v0 ships OFAC SDN (the most commonly required US list) as a complete reference implementation; broader coverage is the v0.5 backlog.

This is sufficient for many v0 customers who only need OFAC-compliance screening; multi-jurisdiction screening is an upgrade path.

## Connection

```yaml
backend: ofac
config:
  sources:
    - id: "ofac_sdn"
      url: "https://www.treasury.gov/ofac/downloads/sdn.xml"
      type: "ofac_sdn_xml"
      schedule: "daily_morning"
  matching:
    algorithm: "jaro_winkler"     # jaro_winkler | metaphone | hybrid
    threshold: 0.85
    name_normalization:
      strip_legal_suffixes: true   # "Ltd", "Inc", "GmbH", etc.
      transliterate: true          # Cyrillic, Arabic, CJK → Latin
```

## Data ingestion

Daily snapshot of the OFAC SDN XML:

```sql
CREATE TABLE sanctions_list_snapshots (
    list_id          TEXT NOT NULL,         -- "ofac_sdn"
    snapshot_version TEXT NOT NULL,         -- publication date as YYYY-MM-DD
    raw_payload_key  TEXT NOT NULL,         -- S3 key for full XML
    pulled_at        TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (list_id, snapshot_version)
);

CREATE TABLE sanctions_list_entries (
    list_id          TEXT NOT NULL,
    snapshot_version TEXT NOT NULL,
    list_entry_id    TEXT NOT NULL,         -- OFAC entity ID
    type             TEXT NOT NULL,         -- entity | individual | vessel | aircraft
    primary_name     TEXT NOT NULL,
    aliases          TEXT[] NOT NULL,
    addresses        JSONB NOT NULL,
    programs         TEXT[] NOT NULL,
    raw_metadata     JSONB NOT NULL,
    PRIMARY KEY (list_id, snapshot_version, list_entry_id)
);

CREATE INDEX idx_sanctions_primary_name_trgm ON sanctions_list_entries
  USING GIN (primary_name gin_trgm_ops);
```

`pg_trgm` enables candidate filtering by trigram similarity; the matching algorithm refines candidates with Jaro-Winkler.

## Matching algorithm

1. **Candidate generation**: trigram similarity over `primary_name` and unnested aliases, returning top 50.
2. **Scoring**: Jaro-Winkler distance with name-normalization (legal suffixes stripped, transliterated to Latin).
3. **Threshold**: configurable, default 0.85.
4. **Address/country disambiguation**: when `context.country` is provided in input, candidates with matching address country score higher.

Match score in the output reflects the post-disambiguation score. Audit metadata captures the raw score and disambiguation deltas.

## Captured reads

```
external:ofac:sanctions_check:<sha256_of_input_plus_snapshot_version>
```

Replay returns the captured hits without re-running matching.

## Failure modes

| Backend error | Surfaced as |
|---|---|
| Snapshot table empty (cold start) | `external.source_unavailable` |
| OFAC source unreachable | Logged; snapshot remains stale but queries continue |
| Snapshot >7 days stale | Hits include a `staleness_warning` flag; skills decide whether to proceed |
| Name input empty | Schema validation error |

## Tests

- Fixture: known SDN entities (declassified historical entries) screened with expected hits.
- Negative: non-SDN names (with similar-sounding aliases) below threshold.
- Replay: captured screening results reproduce identically.
- Snapshot age: queries against stale snapshots surface the warning.

## Open questions

1. **Phonetic matching for CJK and Arabic.** Jaro-Winkler is weak on non-Latin scripts even with transliteration. Proposal: per-script matchers in v0.5.
2. **Sectoral lists.** OFAC publishes sectoral SDN-equivalents. They share schema but have different programs. Plan to add in v0.5 with one config block per list.
3. **Hit dispositioning workflow.** Hits typically go to a compliance officer. The `sanctions_disposition` skill pack (v0.5) operationalizes this; the tool's job is detection.

## References

- [`tools/external/README.md`](../README.md)
- OFAC SDN list: https://www.treasury.gov/ofac/downloads/
- Jaro-Winkler distance: Winkler 1990
