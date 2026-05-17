# Policy Backend: Keyword (BM25)

Status: **v0 / reference backend**

> BM25 keyword search over policy documents. Sufficient for v0 use cases — small policy corpora, structured documents with clear section anchors, customers who prefer no ML retrieval dependency.

## Why keyword first

1. **Zero infra**: implements purely in Postgres using `tsvector` / `tsquery`, or in-process via a Go BM25 library. No vector store, no embedding model.
2. **Predictability**: keyword matches are inspectable. A controller asking "why did the agent pull this passage?" gets a concrete answer (these terms matched).
3. **Sufficiency**: most policy corpora are under 100 pages per tenant. Semantic retrieval is overkill at this scale.

Semantic retrieval (pgvector backend) is available for tenants with large or unstructured corpora.

## Connection

```yaml
backend: keyword
config:
  store: "postgres"          # postgres | inmem
  index_refresh:
    on_publish: true          # rebuild index when a policy doc is republished
    on_schedule: "0 3 * * *"  # nightly safety rebuild
  bm25:
    k1: 1.5
    b: 0.75
  language: "english"
```

The Postgres store uses `tsvector` indexed columns; the in-memory store is for single-tenant dev deployments and small policy corpora.

## Schema mapping

```sql
CREATE TABLE policy_documents (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    title              TEXT NOT NULL,
    version            TEXT NOT NULL,
    effective_date     DATE NOT NULL,
    owner              TEXT,
    sections           JSONB NOT NULL,     -- [{id, anchor, page, text}]
    created_at         TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, id, version)
);

CREATE TABLE policy_sections_fts (
    document_id        TEXT NOT NULL,
    document_version   TEXT NOT NULL,
    section_id         TEXT NOT NULL,
    tenant_id          TEXT NOT NULL,
    text               TEXT NOT NULL,
    tsv                TSVECTOR,
    PRIMARY KEY (tenant_id, document_id, document_version, section_id)
);

CREATE INDEX idx_policy_fts_tsv ON policy_sections_fts USING GIN (tsv);
```

`policy.lookup` issues a Postgres `tsquery` derived from the input query, ranks by `ts_rank_cd`, applies per-document filters, and returns top-k passages.

## Indexing

Policy documents are indexed on publish:

1. Markdown or PDF source ingested.
2. Sectioned by header structure (`#`, `##`, ...) or by PDF outline.
3. Each section produces a row in `policy_sections_fts` with `tsv = to_tsvector('english', text)`.
4. Anchor IDs are stable across versions when section headers don't change; an explicit migration is required when they do.

CLI:
```bash
$ provenant policy index ./docs/recon-policy.md --document-id recon-policy --version 2.3
Indexed 14 sections from recon-policy v2.3.
```

## Captured reads

```
policy:keyword:lookup:<sha256_of_input_plus_index_version>
```

The capture stores the ranked results so replay returns identical passages.

## Rate limits

Postgres-backed; rate limits per the broader Postgres connection pool. No external rate limits.

## Failure modes

| Backend error | Surfaced as |
|---|---|
| Document not in index | `policy.document_not_found` |
| section_id not resolvable | `policy.section_not_found` |
| Postgres unavailable | `policy.index_unavailable` |
| Query yields no results above threshold | `policy.no_results` |

## Tests

- Fixture: a sample recon policy document indexed; queries return expected passages with reasonable ranking.
- Versioning: republishing a document keeps the prior version queryable.
- Determinism: identical queries yield identical results across replays.

## Open questions

1. **Stemming language selection.** Tenants with non-English policies need different `tsvector` configurations. Proposal: per-document language override.
2. **Synonyms and abbreviations.** Customer-specific glossaries (e.g., "AR" → "accounts receivable") would improve recall. Proposal: a per-tenant synonyms table; defer to v0.5.

## Configuration example for recon

```yaml
backend: keyword
config:
  store: "postgres"
  bm25: { k1: 1.5, b: 0.75 }
  language: "english"
```

## References

- [`tools/policy/README.md`](../README.md)
- BM25: Robertson & Zaragoza 2009
