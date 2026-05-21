# Storage (runtime-layer reference bindings; not a kernel concern)

Status: **v0 (runtime-layer reference bindings)**

> **Reframe (cycle 11, [ADR-0011](../adrs/0011-storage-is-a-deployment-concern.md)).** Storage is **not part of the Provenant kernel**. The kernel has no storage requirement. It takes typed inputs, accumulates state in memory during a run, and emits a self-contained single JSON audit bundle as bytes. What the caller does with those bytes is the caller's concern. This document is therefore not a description of "what provenant requires" — it is a description of the **reference SQLite and Postgres bindings** the optional MCP server and CLI access surfaces use, and which an embedding host *may* choose to wire up if it wants persistence beyond the run.

## Depends on
- (For the SQLite reference binding) the SQLite engine, bundled as a Go module dependency of `runtime/sqlite/`
- (For the Postgres reference binding) Postgres 15+
- (For the S3 reference `BlobStore`) S3-compatible object store (S3, MinIO, GCS, R2)

## Depended on by
- The optional MCP server access surface (`runtime/mcp/`) — uses one of the reference bindings when HITL pause/resume is enabled
- The optional CLI access surface (`runtime/cli/`) — uses one of the reference bindings when HITL pause/resume is enabled; the bundle-emitting batch mode requires no binding
- Hosts that embed the library and *choose* to wire one of these bindings — not required

## What the kernel exposes instead

The kernel's public Go API (under `kernel/public/`, see [`subsystems/kernel.md`](kernel.md)) declares four I/O interfaces:

- `Store` — token + run + (optional) audit row persistence handle
- `AuditSink` — hash-chain append target
- `BlobStore` — large-payload persistence handle
- `TransactionalScope` — joint-transaction primitive across `Store` and `AuditSink` (see [`subsystems/kernel.md`](kernel.md#transactionalscope-joint-transaction-primitive))

These interfaces exist so a host *may* wire up persistence. They are **not** a request that the host operate a database. The kernel ships a **first-class in-memory binding** of all four — `kernel/public/memio/` — that backs `Store` and `AuditSink` from the same in-memory map under a `NullTransactionalScope` whose substrate marker is `"provenant.substrate/memstore/sharedmap/v1"`. This is the library-mode default.

**The kernel never opens a network socket, a file handle, or a database connection.** Every Go import under `kernel/...` is constrained by the ADR-0009 import-guard list (no `net/*`, no `database/sql`, no `pgx`, no MCP SDK types, no `os/exec`). The CI check enforces it on every build. The kernel takes typed inputs and produces typed outputs including a self-contained audit bundle. **All persistence is the caller's choice.**

### The audit bundle is the canonical output

After a run, the in-memory binding can emit a self-contained bundle as bytes:

The current bundle is one canonical JSON document with format `phase1.bundle/v1`. It contains the run, captured reads, tokens, and an `audit_jsonl` string with one JSONL row per `AuditEntry`; the deterministic-portion bytes in each audit row are exactly what was hashed.

The bundle is self-contained: `provenant audit verify <bundle-path>` reads only the bundle bytes and validates the chain. **No database access at any point.** The verifier is a pure function over the bundle JSON and its embedded `audit_jsonl`.

In library mode this is the typical artifact flow:

```go
runState, bundle, err := kernel.Run(ctx, input)   // bundle is []byte (or io.Reader of a zip)
if err != nil { return err }
host.Persist(bundle)                                // host decides where it lives
```

The kernel has no opinion on whether `host.Persist` writes to S3, to local disk, to a database `BYTEA` column, to a Kafka topic, or to `/dev/null`.

### HITL pause/resume in library mode is the host's responsibility

When a run pauses for HITL, the kernel exposes a typed `RunState` snapshot — canonical-JSON encodable, schema enumerated in [`subsystems/kernel.md`](kernel.md). The host serializes the snapshot to bytes, stores it wherever the host already keeps state, and hands it back on resume:

```go
// pause
runState, awaitingReview, err := kernel.Run(ctx, input)
if awaitingReview {
    host.Persist(runState.MarshalCanonicalJSON())  // host owns the bytes
    return
}

// resume (a future call, possibly a different process)
runState := host.LoadAndUnmarshal(...)
runState2, bundle, err := kernel.Resume(ctx, runState, reviewDecision)
```

The kernel has no opinion on where the host kept the `RunState`. **The kernel pure-API accepts a `RunState` and continues.** This is the library-mode HITL pattern.

## Reference SQLite + Postgres bindings (runtime layer)

The optional MCP server and CLI access surfaces ship two reference persistence bindings. Both live **outside the kernel module**.

- `runtime/sqlite/` — the **default** runtime-mode binding **when HITL is enabled**. Single-file, zero-ops, fine for a developer machine, a single-tenant on-prem deployment, or any access-surface deployment that doesn't need multi-process coordination. SQLite is the right choice when HITL pause/resume needs to outlive the runtime process but the deployment shape is single-machine. **A stateless runtime-mode deployment that does not enable HITL needs no persistent binding** and runs entirely against `kernel/public/memio/`.
- `runtime/postgres/` — the multi-process / multi-tenant runtime binding **when HITL is enabled**. Required when multiple runtime processes need to share HITL state, when a multi-tenant deployment needs row-level isolation, or when an organization already operates Postgres for HA / DR.
- `runtime/blobstore/s3/` — S3-compatible `BlobStore` for the runtime layer's large-payload needs (PDFs, full prompts, full model responses). The `kernel/public/memio/` in-memory binding holds blobs in memory and emits them into the audit bundle.

A host that embeds the library is **free to ignore both `runtime/sqlite/` and `runtime/postgres/` entirely**. They are reference implementations for the MCP/CLI surfaces, not a required dependency of the library.

### Reference schema (Postgres binding)

The schema below is what the `runtime/postgres/` binding ships. The same logical schema, simplified for single-file deployment, is what `runtime/sqlite/` ships. The schema is **not** kernel policy — it is a projection target for the runtime layer.

#### Tables

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
| `hitl_reviews` | HITL approval state — the **only** state that *must* outlive a single call in runtime mode | [hitl.md](hitl.md) |
| `tenants` | Tenant identity and configuration | — |

All tables include `tenant_id TEXT NOT NULL` (except `tools_registry` and `tenants`). Foreign keys to `runs(id)` cascade on tenant lifecycle operations.

#### Schema deltas required by cycles 3–5 (load-bearing columns in any persistent binding)

The audit-chain payload-schema fixes adopted in cycles 3, 4, and 5 (see [`audit.md`](audit.md#per-entrykind-payload-schema-deterministic-surface-enumeration) and [`cross-cutting/security.md`](../cross-cutting/security.md#silent-credential-rotation-detection-per-run-identity-baseline)) require the following column-level additions in any persistent binding. The in-memory binding represents the same logical fields as canonical-JSON sub-objects. Without these columns the deterministic-payload schema in [`audit.md`](audit.md) cannot be persisted and replay byte-identity cannot be achieved across access surfaces.

- **`tokens.source_value_raw TEXT NOT NULL`** — the verbatim upstream string after the `source_value_raw` canonical-form rule in [`audit.md`](audit.md#source_value_raw-canonical-form). Hashed into the chain via `EntryKindTokenMint.deterministic.source_value_raw`.
- **`tokens.normalizer_applied JSONB`** — nullable; when non-null, records `{id, builtin_id, input_raw, output_canonical}` per the skill-pack `normalizers/` slot ([`skills.md`](skills.md#pack-level-normalizers-normalizers)). `null` is the canonical "no normalizer fired" value and is part of the deterministic payload.
- **`runs.credential_identity_baseline JSONB NOT NULL`** — sorted map `credential_id → credential_identifier_hash` captured at run start (see [`cross-cutting/security.md`](../cross-cutting/security.md#silent-credential-rotation-detection-per-run-identity-baseline)). Mirrored into `EntryKindRunStarted.deterministic`.
- **`runs.kernel_header_hash TEXT NOT NULL`** — sha256 hex of the kernel-immutable header text loaded from `kernel/internal/promptkernel/header_v<N>.go` at run start. Mirrored into `EntryKindRunStarted.deterministic.kernel_header_hash`.
- **`audit_entries` payload split.** Materialized as **three columns**:
  - `payload_deterministic JSONB NOT NULL` — the deterministic sub-object the canonicalizer feeds into the chain hash.
  - `payload_non_deterministic JSONB NOT NULL DEFAULT '{}'::jsonb` — the non-deterministic sub-object (wall-clock fields, retry counts, panic messages); stored on the row but excluded from the hash input.
  - `hash_input_canonical TEXT NOT NULL` — the exact canonical-JSON bytes of `payload_deterministic` that were fed into the SHA-256 hash, retained for replay-debug. This column is **not** itself hashed; it is operational debug surface.

The three-column projection is the source-of-truth surface for any persistent binding; `audit.md` continues to describe the payload as a single object with `deterministic` / `non_deterministic` sub-objects for readability, and the storage layer projects between the two shapes. The in-memory binding's single JSON bundle embeds `audit_jsonl` using the single-object representation directly — one JSONL row per entry, sub-objects as nested JSON.

#### Common columns

Every table includes:

```sql
id          TEXT PRIMARY KEY                  -- ULID, lexicographically sortable
tenant_id   TEXT NOT NULL                     -- where applicable
created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

### Migrations (Postgres binding)

The `runtime/postgres/` binding manages migrations via [atlas](https://atlasgo.io). Migrations live under `runtime/postgres/migrations/`. CI runs migrations against a clean Postgres in each PR that touches the binding.

Naming: `YYYYMMDDHHMMSS_<verb>_<object>.sql`. Migrations are append-only; rollbacks are forward-only (a "rollback" is a new migration that reverses the change). This keeps the audit chain over time consistent for deployments that use the Postgres binding.

The `runtime/sqlite/` binding ships its schema as embedded DDL applied on first open of the SQLite file; SQLite does not get an atlas pipeline because the single-file deployment model does not need one.

### Object store (S3 reference binding)

The `runtime/blobstore/s3/` binding lays out blobs as:

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

`Store` rows reference object-store keys via the `BlobStore` indirection. The object store is the authoritative storage for blobs when the runtime binding is in use; in library mode the in-memory `BlobStore` holds them and they are included in the emitted bundle.

#### Retention (runtime mode)

- Documents and extracts: retained per tenant policy (default 7 years for SOX-affected workflows).
- Prompts and model responses: retained per tenant policy (default 3 years; configurable).
- Captured reads: retained as long as the case bank entry that references them.
- Audit pack exports: retained 90 days after generation; regenerable from source data within retention window.

Lifecycle policies enforced at the bucket level. Manual deletion is blocked; retention extension is logged. **In library mode none of these apply** — retention is the host's concern.

## Multi-tenant isolation (runtime mode)

### Modes

**Shared schema (default for the Postgres binding in multi-tenant runtime mode).** All tenants share the `provenant` schema. Row-level security policies filter every query by `tenant_id` set in the Postgres session. Bearer-token middleware sets the session variable.

```sql
CREATE POLICY tenant_isolation ON runs
  USING (tenant_id = current_setting('provenant.tenant_id'));
```

**Schema-per-tenant (recommended for enterprise).** Each tenant gets its own schema. Stricter isolation; supports per-tenant migrations and customizations. Higher operational cost.

**DB-per-tenant (compliance-driven).** Each tenant gets a separate database, possibly on separate Postgres clusters. Maximum isolation; used for regulated tenants with data residency requirements.

These modes are properties of the **Postgres binding**, not of the kernel. The kernel only knows `tenant_id` is a value carried on tokens, runs, and audit entries.

### Cross-tenant guards

These guards live in the kernel and apply regardless of binding:

- Tokens carry `tenant_id`; `VerifyToken` independently checks tenant match.
- Tool calls are issued with the run's tenant; cross-tenant tool calls are rejected at the registry.
- Audit log queries enforce tenant scoping at the API layer; no SQL bypass exposed.

## Connection pool and concurrency (runtime mode)

The `runtime/postgres/` binding uses `pgx` + `pgxpool`. Pool size scales with replica count. Long-running transactions are forbidden in production paths; they are replaced by application-level locking where needed.

The audit log append is the only operation requiring strict ordering. It uses a `SERIALIZABLE` transaction or an `INSERT ... RETURNING seq` pattern with a per-tenant `audit_seq` table to ensure monotonic sequence numbers. See [audit.md](audit.md).

The `runtime/sqlite/` binding uses the standard `database/sql` driver with WAL mode and a single-writer discipline; sequence ordering is enforced via the same `audit_seq` table pattern within a single SQLite file.

## Backup and disaster recovery (runtime mode)

- Postgres binding: point-in-time recovery (PITR) with WAL archiving to object store. Tenant-specific backups for DB-per-tenant deployments.
- SQLite binding: file-level snapshots are sufficient; the runtime daemon supports `provenant-server backup --output <path>` for atomic file snapshot.
- Object store binding: cross-region replication where the cloud supports it.
- Tested via quarterly DR drills (restore a sample tenant; verify replay correctness) for runtime-mode deployments. **Library-mode deployments inherit the host's DR posture** — the kernel produces bundle bytes; what the host backs up is the host's concern.

## Performance characteristics

- In-memory binding (library mode): kernel writes are pure in-process map operations. No I/O.
- SQLite binding: target <50ms p95 for kernel reads on warm cache, <100ms p95 for token-grounding queries on a single-machine deployment.
- Postgres binding: target <50ms p95 for kernel reads on warm cache, <200ms p95 for token-grounding queries.

## Open questions

1. **Distributed transactions across DB-per-tenant Postgres deployments.** Cross-tenant operations (none in v0) would need a saga pattern. Defer.
2. **Audit log archival (Postgres binding).** After N years, can the audit log move to cold storage with the chain still verifiable? Proposal: yes — chain hashes are computed from immutable inputs, archival doesn't break verification, but the API must transparently fetch from cold storage.
3. **Token store sharding.** For very high run volumes against the Postgres binding, the tokens table may need partitioning by `created_at`. Defer until we have data on production volume.
4. **Library-mode HITL across processes.** Documented as a trade-off in [`docs/KNOWN_TRADE_OFFS.md`](../../docs/KNOWN_TRADE_OFFS.md): library-mode hosts that need cross-process HITL serialize the kernel's `RunState` themselves. Not an open question for the kernel; an explicit non-goal.

## References

- [ADR-0011](../adrs/0011-storage-is-a-deployment-concern.md) — the framing this doc is reorganized under
- [ADR-0008](../adrs/0008-postgres-as-canonical-store.md) — superseded historical decision; retained for context on the Postgres binding's rationale
- [DFAH](../references.md#dfah) — replayability requirements (which the in-memory bundle satisfies as fully as any persistent binding)
