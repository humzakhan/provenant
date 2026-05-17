# ADR-0008: Postgres as the canonical transactional store

Status: **Accepted**
Date: 2026-05-17

## Context

The kernel persists a substantial amount of transactional state: runs, tokens, tool calls, captured reads, guard runs, audit entries, cases, skill metadata, HITL reviews. Choice criteria:

- ACID properties for the audit chain (monotonic sequence numbers, no lost appends).
- Multi-tenant isolation models (shared, schema-per, DB-per).
- Mature client libraries in Go.
- Mature operational tooling (backup, PITR, replication).
- Support for JSONB for variable-shape payloads.
- Optional vector retrieval (pgvector) without a separate store.
- Air-gapped deployability.

## Decision

Postgres 15+ is the canonical transactional store. Every transactional artifact lives in Postgres. Object storage (S3-compatible) holds large blobs (PDFs, OCR raw, full prompts/responses) keyed by Postgres rows.

We do not use a separate analytics store, document store, vector store, or message queue in v0. Vector retrieval via pgvector is opt-in within Postgres; we do not run Elasticsearch / Pinecone / Qdrant. Pub/sub for event streaming is in-process Go channels; we do not run Kafka.

## Rationale

**ACID for the audit chain.** The hash-chained audit log requires monotonic, gapless sequence numbers across concurrent appends. Postgres delivers via advisory locks + transactional inserts. Other stores either don't provide this primitive or require additional service complexity.

**JSONB for variable-shape payloads.** Token payloads, tool call inputs/outputs, validator results, case content — all are JSON-shaped and benefit from structured-but-flexible storage. JSONB with GIN indexes is the right tool.

**Operational maturity.** Postgres operations are well-understood; every infra engineer has the runbook for backup, restore, replication, monitoring. This matters for our customer base (financial services IT teams).

**Multi-tenancy options.** Postgres supports shared-schema (RLS-based), schema-per-tenant, and DB-per-tenant — the three multi-tenant models customers actually need. We get all three for free without changing data model.

**Air-gapped deployable.** Single-binary Postgres image runs anywhere. Air-gapped customer requirements are satisfied without architecture changes.

**Reduces operational surface.** Every additional store is another operational concern, another set of backups, another set of failure modes. Postgres-only halves what an operator has to know.

## Consequences

**Positive:**
- One backup/restore story, one monitoring story, one auth story.
- Customers' DBAs already know Postgres operations.
- Audit chain ACID guarantees come from a well-understood primitive.
- Vector retrieval (pgvector) is opt-in inside the same database.
- Air-gapped deployments are trivial.

**Negative:**
- Postgres is not a search engine. We accept slightly worse search quality vs. Elasticsearch for the simplification benefit. Skill packs that need sophisticated search can register custom backends.
- Postgres is not a vector store at the scale of dedicated vector DBs. pgvector is sufficient for tenant-scale policy retrieval (<1M chunks). Customers with billion-chunk needs go elsewhere.
- Postgres is not a queue. In-process Go channels are sufficient for kernel-internal pub/sub; for cross-process queueing we'd use Postgres `LISTEN/NOTIFY` or add a small queue later. Not a v0 concern.

**Out of scope:**
- We do not provide a "Postgres-incompatible" backend option in v0. Customers who insist on a non-Postgres store fork the kernel; we don't accept upstream patches for alternative stores until the v0 surface stabilizes.

## Alternatives considered

**Postgres + Elasticsearch.** Adds a search store for policy and trajectory queries. Rejected: tsvector + trigram + GIN indexes are sufficient for v0 corpora; doubles operational surface.

**Postgres + ClickHouse.** Adds an analytics store for case-bank metrics. Rejected: Postgres with proper indexes handles the case-bank query patterns; ClickHouse is over-investment for v0.

**SQLite for single-tenant.** Considered for very small deployments. Rejected as primary: WAL contention and lack of advisory locks make the audit-chain primitive awkward. SQLite remains useful for embedded test fixtures.

**A dedicated event store (EventStoreDB or similar).** Tempting for the audit chain. Rejected: Postgres-as-event-store via append-only inserts plus the sha256 chain is operationally simpler.

## References

- [pgx](https://github.com/jackc/pgx), [sqlc](https://github.com/sqlc-dev/sqlc), [atlas](https://atlasgo.io)
- [pgvector](https://github.com/pgvector/pgvector) — opt-in vector path
- See [`subsystems/storage.md`](../subsystems/storage.md) for the schema details
