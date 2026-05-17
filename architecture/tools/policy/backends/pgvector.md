# Policy Backend: pgvector

Status: **v0.5 / deferred**

> Semantic policy retrieval via pgvector embeddings. Planned for tenants with large or unstructured policy corpora where keyword retrieval misses relevant passages.

## Why deferred

Keyword retrieval is sufficient for most v0 customer policy corpora (small, well-structured, internal terminology). Semantic retrieval becomes worth its complexity when:

- The corpus exceeds ~500 pages.
- The corpus mixes formal language and informal handbook prose.
- Cross-document concepts (e.g., "what's our policy on entertainment expenses" across travel policy, gift policy, and AP manual) need to be found together.

These conditions are less common in v0 customers; we will ship pgvector when demand signals.

## Design constraints when we build it

1. **Embedding model**: configurable; defaults to OpenAI `text-embedding-3-small` for cost. Customers in air-gapped deployments use a local embedding model (BGE, GTE).
2. **Hybrid retrieval**: combine BM25 (keyword backend) and vector similarity. Final ranking uses reciprocal rank fusion.
3. **pgvector extension**: required on the Postgres instance. Customers without it use the keyword backend.
4. **Captured reads**: same convention; the embedding vector itself is part of the index version.
5. **Embedding regeneration**: when the embedding model changes, the index rebuilds. Prior captured reads remain valid for replay.

## Schedule

v0.5 timeline depends on demand from customers with large corpora. Earliest realistic: 6 months after v0 GA.

## References

- [`tools/policy/README.md`](../README.md)
- pgvector: https://github.com/pgvector/pgvector
