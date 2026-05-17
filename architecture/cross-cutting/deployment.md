# Cross-cutting: Deployment

Status: **v0**

> Provenant supports four deployment shapes: local dev, single-tenant production, multi-tenant SaaS, and air-gapped. Each shape has a clear configuration story and operational runbook. The kernel binary and Postgres schema are identical across all four; what differs is topology, isolation, and operations.

## Deployment shapes

| Shape | Tenants | Postgres | Object store | Operator | Status |
|---|---|---|---|---|---|
| Local dev | 1 (self) | docker-compose | MinIO via docker-compose | developer | v0 |
| Single-tenant production | 1 | dedicated instance | S3-compatible | customer ops | v0 |
| Multi-tenant SaaS | many | shared with RLS, or schema-per | shared bucket prefix | Provenant (us) | v0.5 |
| Air-gapped on-prem | 1+ | on-prem Postgres | on-prem MinIO / Ceph | customer ops | v0 |

## Local dev

Single command:

```bash
$ docker compose up
```

What it brings up:

```yaml
# docker-compose.yml (excerpt)
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: provenant
      POSTGRES_USER: provenant
      POSTGRES_PASSWORD: dev
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]

  minio:
    image: minio/minio
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: provenant
      MINIO_ROOT_PASSWORD: devdevdev
    ports: ["9000:9000", "9001:9001"]
    volumes: ["miniodata:/data"]

  provenant:
    image: ghcr.io/provenant/server:0.1.0
    depends_on: [postgres, minio]
    environment:
      PROVENANT_CONFIG: /etc/provenant/dev.yaml
    volumes:
      - "./ops/dev:/etc/provenant:ro"
    ports: ["8080:8080"]

  prometheus:
    image: prom/prometheus
    volumes: ["./ops/prom.yml:/etc/prometheus/prometheus.yml:ro"]
    ports: ["9090:9090"]

  grafana:
    image: grafana/grafana
    ports: ["3000:3000"]
    volumes: ["./ops/grafana/provisioning:/etc/grafana/provisioning"]

  jaeger:
    image: jaegertracing/all-in-one
    ports: ["16686:16686", "4317:4317"]

volumes:
  pgdata:
  miniodata:
```

Migrations run automatically on first start. The `provenant-cli` binary is available locally and points at the running server (or runs in embedded mode against the same Postgres).

Useful endpoints in local dev:
- `http://localhost:8080` — Provenant API
- `http://localhost:8080/metrics` — Prometheus metrics
- `http://localhost:9001` — MinIO console
- `http://localhost:9090` — Prometheus
- `http://localhost:3000` — Grafana (admin/admin)
- `http://localhost:16686` — Jaeger

## Single-tenant production

Architecture:

```
                    ┌──────────────┐
                    │ Load Balancer │  (optional; for >1 replica)
                    └──────┬───────┘
                           │
                ┌──────────┼──────────┐
                ▼          ▼          ▼
         ┌──────────┐ ┌──────────┐ ┌──────────┐
         │ provenant│ │ provenant│ │ provenant│
         │  replica │ │  replica │ │  replica │
         └─────┬────┘ └─────┬────┘ └─────┬────┘
               │            │            │
               └────────────┴────────────┘
                            │
                ┌───────────┼───────────┐
                ▼                       ▼
        ┌──────────────┐         ┌──────────────┐
        │  Postgres 15  │         │ Object Store │
        │  (primary +   │         │  (S3, R2,    │
        │  replica)     │         │  GCS, etc.)  │
        └──────────────┘         └──────────────┘
```

Sizing baseline for v0 (single skill, ~100 runs/day):

| Component | Size |
|---|---|
| Provenant replica | 2 vCPU, 4 GB RAM |
| Postgres | 4 vCPU, 16 GB RAM, 100 GB SSD |
| Object store | ~50 GB / month at this volume |

Scaling guidance:

- **Throughput**: kernel is CPU-light; Postgres connections dominate. Increase replicas first; Postgres pool size second.
- **Storage growth**: prompts, model responses, and captured reads dominate object store growth. Plan ~1 MB per run, scaled by skill complexity.
- **Audit retention**: hot in Postgres for 1 year; archive to cold object storage thereafter. The kernel's archive job runs nightly.

Configuration template (`/etc/provenant/config.yaml`):

```yaml
deployment:
  mode: single_tenant
  tenant_id: acme

mcp:
  bind: "0.0.0.0:8080"
  tls:
    cert_file: /etc/provenant/tls/cert.pem
    key_file: /etc/provenant/tls/key.pem

postgres:
  dsn_secret: "postgres-dsn"      # resolved from secret store
  pool:
    max_open: 50
    max_idle: 10

object_store:
  endpoint: "https://s3.amazonaws.com"
  bucket: "provenant-acme-prod"
  region: "us-east-1"
  credentials:
    mode: "iam_role"

skills:
  - id: recon
    version: "0.3.0"
    source: "registry"           # or "local"

observability:
  metrics: { enabled: true, bind: "0.0.0.0:9090" }
  traces:  { enabled: true, otlp_endpoint: "http://otel-collector:4317" }
  logs:    { format: "json", level: "info" }

retention:
  audit_hot_days: 365
  audit_total_years: 7
  case_bank_years: 7
  prompts_responses_days: 1095   # 3 years
```

Operations:

- **Migrations**: `provenant migrate up` (idempotent; safe in CI/CD).
- **Backup**: standard Postgres PITR; S3 versioning/replication for object store.
- **Restore drill**: quarterly, against a sample tenant.
- **Skill upgrades**: `provenant skill install <new-version>` followed by `provenant eval --skill <id> --against new` before promoting; canary the first day with low traffic.

## Multi-tenant SaaS

Status: **v0.5 / deferred**.

Architecture extension over single-tenant:

- Postgres in either shared-schema with RLS, or schema-per-tenant (recommended for >10 tenants).
- Tenant config table; bearer tokens resolve to tenant; tenant set in Postgres session.
- Object store buckets per-tenant with per-tenant IAM policies.
- Per-tenant rate limits and cost caps.
- Per-tenant Grafana dashboards (templated).

The shared-schema RLS mode is simpler operationally but requires careful audit on every query path. Schema-per-tenant is the recommended default once we ship SaaS.

## Air-gapped on-prem

For customers who cannot allow Provenant to reach external services. Constraints:

- No reach to AWS Textract, OpenExchangeRates, OFAC website, model APIs.
- All dependencies must be available inside the air-gapped network.

Patterns:

1. **Model**: customer runs a local inference endpoint (vLLM, TGI, llama.cpp, or licensed Anthropic/OpenAI on-prem). Provenant's model abstraction supports OpenAI-compatible local endpoints.
2. **Extraction**: customer pre-extracts documents to canonical schema in a separate process (Tesseract + custom layout, on-prem AzureDI on appliance, etc.) and stores results in the SQL ledger schema; Provenant reads from SQL only.
3. **Sanctions**: customer mirrors OFAC SDN list to local file storage; backend points at local path.
4. **FX**: customer mirrors ECB feed daily to local path; backend reads from local snapshot.

Deployment shape:

```
┌─────────────────────────────────────────────────────────┐
│                  Air-gapped network                      │
│                                                          │
│  ┌───────────┐   ┌───────────┐   ┌───────────────────┐  │
│  │ provenant │ ◀▶│ Postgres  │ ◀▶│  MinIO / Ceph     │  │
│  │  server   │   │           │   │  (object store)   │  │
│  └─────┬─────┘   └───────────┘   └───────────────────┘  │
│        │                                                 │
│        ▼                                                 │
│  ┌────────────┐                                          │
│  │ local LLM  │ (vLLM / TGI / licensed on-prem)         │
│  └────────────┘                                          │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ Reference data mirror (OFAC, ECB) updated via      │  │
│  │ scheduled cross-domain sync (CD or one-way feed)   │  │
│  └────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

The kernel binary, Postgres, MinIO, and all dependent images are mirrored to the customer's internal registry. Provenant publishes a manifest of required image digests for each release; customers mirror via standard tools (Harbor, Artifactory).

Considerations:
- **Skill packs** ship as files; install via `provenant skill install /path/to/pack`.
- **Updates** are an operational event for the customer; we don't auto-update.
- **Telemetry** is opt-in and disabled by default for air-gapped deployments.

## Updates and migrations

Versioning:
- Kernel: semver. Major bumps may require migration; documented in release notes.
- Postgres schema: atlas migrations; forward-only. Customer runs `provenant migrate up` as a deploy step.
- Skill packs: independent semver; customer chooses when to upgrade per pack.

Upgrade procedure (production):
1. Read release notes for breaking changes.
2. Backup Postgres.
3. Run `provenant migrate up --dry-run` to preview.
4. Deploy new binary to a single canary replica.
5. Monitor metrics for 24h.
6. Roll out to remaining replicas.
7. Migrate skill packs separately, with their own eval gate.

Rollback: keep N-1 binary and N-1 schema available. Schema rollback is forward-only — a "rollback" is a new migration that reverses, not a reversion of the old migration. This keeps the audit log over time consistent.

## Disaster recovery

| Failure | RTO | RPO | Procedure |
|---|---|---|---|
| Provenant replica failure | seconds | 0 | Load balancer reroutes; replicas are stateless |
| Postgres primary failure | minutes | <1m | Failover to replica via standard Postgres HA |
| Object store region failure | minutes | <1h | Cross-region replication failover |
| Tenant data corruption | hours | <24h | PITR to last-known-good; replay from audit log |
| Full DR scenario | hours | <24h | Restore from cross-region backups; cold tenant audit packs verified |

Quarterly DR drills are required; the operator runs `provenant disaster-drill --tenant <sample>` which restores a sample tenant from backups and verifies replay correctness end-to-end.

### Postgres / object-store cross-consistency on restore

The audit chain references blobs (prompts, responses, captured reads) by key. After a DR event in which Postgres and the object store are restored from snapshots taken at different times, the two stores can be inconsistent: `audit_entries.blob_ref` may point to a blob that does not yet exist (Postgres ahead of object store) or a blob may have no referring row (object store ahead of Postgres).

The reconciliation procedure that runs **before** the restored deployment accepts traffic:

1. `provenant restore-verify --tenant <id>` walks every `blob_ref` in `audit_entries` for the tenant and checks existence in the object store. Missing blobs are reported; the run state of each affected `run_id` is marked `restore_pending` and excluded from `audit pack` until reconciled. **`audit verify` does not return OK** on a chain that references missing blobs; this addresses the failure mode in which evidence has been silently lost.
2. Conversely, blobs in the object store whose referencing audit row is not present in Postgres are listed but retained (object-store retention is the safer side; deletion is a separate operator action).
3. The operator either restores from a more recent backup of the lagging store, or accepts the partial loss by promoting affected runs to `restore_pending_acknowledged` with an audit entry recording the gap.
4. Only after `restore-verify` reports clean (or all gaps are acknowledged) does the server resume accepting writes.

This is a documented v0 DR procedure; full cross-store atomic restore (e.g., consistent snapshots across Postgres and object store) is a v0.5+ concern.

## Open questions

1. **Kubernetes operator**. Many customers run Kubernetes; should we ship a Provenant Operator for declarative tenant management? Proposal: yes, for v0.5; v0 ships Helm charts.
2. **Horizontal scaling of Postgres**. Above some volume, Postgres write throughput becomes the constraint. Options: Citus, partitioning by tenant, multiple Postgres clusters. Defer; we don't have customers at this scale yet.
3. **Air-gapped model story**. We assume customer-managed local LLM; we don't provide one. Should we partner with a vendor (vLLM-hosted, Anthropic-on-prem, etc.) for a turnkey story? Strategic decision; defer.

## References

- See [`subsystems/storage.md`](../subsystems/storage.md) for the schema and isolation modes
- See [`cross-cutting/security.md`](security.md) for the threat model that informs the deployment shape choices
- See [`cross-cutting/observability.md`](observability.md) for monitoring requirements in each shape
