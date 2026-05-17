# Subsystem: Storage

Status: **v0**

> Postgres holds every transactional artifact: runs, tokens, tool calls, guard outcomes, audit entries, cases, skill pack metadata. Object storage (S3-compatible) holds large payloads: source documents, OCR outputs, full prompts and model responses. The kernel is stateless; this layer is the source of truth.

## Depends on
- Postgres 15+
- S3-compatible object store (S3, MinIO, GCS, R2)

## Depended on by
- Every other subsystem

## Choice rationale

- **Postgres for transactional state.** Battle-tested, single-binary-friendly, supports JSONB for variable payloads, supports row-level security for multi-tenant isolation, scales to single-tenant deployments easily and to multi-tenant with schema-per-tenant or DB-per-tenant. See [ADR-0008](../adrs/0008-postgres-as-canonical-store.md).
- **Object store for blobs.** Source PDFs, OCR raw, full model responses — kilobytes to megabytes each, not the right shape for Postgres. S3-compatible enables any cloud or on-prem MinIO.
- **No specialized stores in v0.** pgvector is available as a Postgres extension; we use it for policy retrieval optionally (see [`tools/policy/backends/pgvector.md`](../tools/policy/backends/pgvector.md)). No separate Elasticsearch, ClickHouse, Redis, etc.

## Schema overview

All tables live in a single schema by default (`provenant`). Multi-tenant deployments use schema-per-tenant or DB-per-tenant; row-level security policies enforce the boundary in shared-schema mode.

### Tables

| Table | Purpose | Doc |
|---|---|---|
| `runs` | A single agent execution | [harness.md](harness.md) |
| `tokens` | Provenance tokens | [kernel.md](kernel.md) |
| `tool_calls` | Tool invocations within a run | [tools.md](tools.md) |
| `captured_reads` | External reads captured for replay | [tools.md](tools.md) |
| `guard_runs` | Pre-LLM and post-LLM guard executions | [guards.md](guards.md) |
| `audit_entries` | Hash-chained immutable log | [audit.md](audit.md) |
| `cases` | Case bank entries | [eval.md](eval.md) |
| `case_metrics` | Per-case metric records | [eval.md](eval.md) |
| `skills` | Loaded skill pack metadata | [skills.md](skills.md) |
| `tools_registry` | Registered tool versions | [tools.md](tools.md) |
| `hitl_reviews` | HITL approval state | [hitl.md](hitl.md) |
| `tenants` | Tenant identity and configuration | — |

All tables include `tenant_id TEXT NOT NULL` (except `tools_registry` and `tenants`). Foreign keys to `runs(id)` cascade on tenant lifecycle operations.

### Common columns

Every table includes:

```sql
id          TEXT PRIMARY KEY                  -- ULID, lexicographically sortable
tenant_id   TEXT NOT NULL                     -- where applicable
created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

## Migrations

Managed by [atlas](https://atlasgo.io). Migrations live in `/migrations` at the repo root. CI runs migrations against a clean Postgres in each PR.

Naming: `YYYYMMDDHHMMSS_<verb>_<object>.sql`. Migrations are append-only; rollbacks are forward-only (a "rollback" is a new migration that reverses the change). This keeps the audit chain over time consistent.

## Object store

### Bucket structure

```
provenant-<tenant_id>/
├── docs/
│   ├── <run_id>/<filename>          # raw uploaded documents
├── extracts/
│   ├── <tool_call_id>.json          # raw OCR / extraction outputs
├── captured-reads/
│   ├── <captured_read_id>.json      # external read payloads for replay
├── prompts/
│   ├── <run_id>/<step_index>.txt    # full prompts as sent to model
├── responses/
│   ├── <run_id>/<step_index>.json   # full model responses
└── audit-packs/
    ├── <run_id>.zip                 # generated audit pack exports
```

Postgres rows reference object-store keys. Object store is the authoritative storage for blobs; Postgres holds metadata and pointers.

### Retention

- Documents and extracts: retained per tenant policy (default 7 years for SOX-affected workflows).
- Prompts and model responses: retained per tenant policy (default 3 years; configurable).
- Captured reads: retained as long as the case bank entry that references them.
- Audit pack exports: retained 90 days after generation; regenerable from source data within retention window.

Lifecycle policies enforced at the bucket level. Manual deletion is blocked; retention extension is logged.

## Multi-tenant isolation

### Modes

**Shared schema (v0 default for SaaS).** All tenants share the `provenant` schema. Row-level security policies filter every query by `tenant_id` set in the Postgres session. Bearer-token middleware sets the session variable.

```sql
CREATE POLICY tenant_isolation ON runs
  USING (tenant_id = current_setting('provenant.tenant_id'));
```

**Schema-per-tenant (recommended for enterprise).** Each tenant gets its own schema (`tenant_acme.runs`, `tenant_globex.runs`). Stricter isolation; supports per-tenant migrations and customizations. Higher operational cost.

**DB-per-tenant (compliance-driven).** Each tenant gets a separate database, possibly on separate Postgres clusters. Maximum isolation; used for regulated tenants with data residency requirements.

Tenant config records which mode is in use. The connection pool routes accordingly.

### Cross-tenant guards

- Tokens carry `tenant_id`; verification checks tenant match.
- Tool calls are issued with the run's tenant; cross-tenant tool calls are rejected at the registry.
- Audit log queries enforce tenant scoping at the API layer; no SQL bypass exposed.

## Connection pool and concurrency

Implemented with `pgx` + connection pool (`pgxpool`). Pool size scales with replica count. Long-running transactions are forbidden in production paths; they are replaced by application-level locking where needed.

The audit log append is the only operation requiring strict ordering. It uses a `SERIALIZABLE` transaction or an `INSERT ... RETURNING seq` pattern with a per-tenant `audit_seq` table to ensure monotonic sequence numbers. See [audit.md](audit.md).

## Backup and disaster recovery

- Postgres: point-in-time recovery (PITR) with WAL archiving to object store. Tenant-specific backups for DB-per-tenant deployments.
- Object store: cross-region replication where the cloud supports it.
- Tested via quarterly DR drills (restore a sample tenant; verify replay correctness).

## Performance characteristics

- Single-tenant deployments: target <50ms p95 for kernel reads on warm cache, <200ms p95 for token-grounding queries.
- Multi-tenant SaaS (deferred): scaling targets defined per-deployment.

## Open questions

1. **Distributed transactions across DB-per-tenant deployments.** Cross-tenant operations (none in v0) would need a saga pattern. Defer.
2. **Audit log archival.** After N years, can the audit log move to cold storage with the chain still verifiable? Proposal: yes — chain hashes are computed from immutable inputs, archival doesn't break verification, but the API must transparently fetch from cold storage.
3. **Token store sharding.** For very high run volumes, the tokens table may need partitioning by `created_at`. Defer until we have data on production volume.

## References

- [DFAH](../references.md#dfah) — replayability requirements
- See [ADR-0008](../adrs/0008-postgres-as-canonical-store.md) for the Postgres-as-canonical decision
