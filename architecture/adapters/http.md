# Adapter: HTTP API

Status: **v0**

> The HTTP REST API exposes the kernel for any client that isn't MCP-native. Custom in-house agents, scripts, third-party automation platforms (Zapier-class), and internal services use this surface. The API mirrors the MCP server's capability surface; the difference is protocol, not behavior.

## Depends on
- [`subsystems/harness.md`](../subsystems/harness.md) — Mode B (adapter-side harness)
- [`subsystems/tools.md`](../subsystems/tools.md), [`subsystems/guards.md`](../subsystems/guards.md), [`subsystems/skills.md`](../subsystems/skills.md)

## Depended on by
- Custom integrations
- The HITL web UI (when shipped)
- Internal observability tooling

## Endpoint surface

All endpoints are versioned (`/v1/`). JSON in, JSON out. Bearer-token auth on every endpoint.

### Runs

```
POST   /v1/runs                                    start a run
GET    /v1/runs/<run_id>                           run metadata + status
GET    /v1/runs/<run_id>/trajectory                tool calls in order
GET    /v1/runs/<run_id>/tokens                    minted tokens
GET    /v1/runs/<run_id>/audit                     audit entries
DELETE /v1/runs/<run_id>                           soft delete (audit retained)
```

`POST /v1/runs` body:
```json
{
  "skill": "recon@0.3.0",
  "model": "claude-opus-4-7",
  "input": {...},
  "tenant_id": "acme",            // optional, derived from token if omitted
  "bounds": {                     // optional, capped at skill maximums
    "max_steps": 30,
    "max_wall_clock_seconds": 300,
    "max_cost_usd": 1.00
  }
}
```

Response:
```json
{
  "run_id": "run_01HXYZ...",
  "status": "running",
  "subscribe_url": "/v1/runs/run_01HXYZ.../events"
}
```

### Tools

```
GET    /v1/tools                                   list all registered tools
GET    /v1/tools/<name>                            tool details
POST   /v1/tools/<name>/call                       direct invocation (not via agent)
```

Direct `/call` is for utility scripts: e.g., a one-off `calc.percent_change` invocation. Direct calls still go through the kernel — tokens are minted, audit entries written.

### Skills

```
GET    /v1/skills                                  list installed skills
GET    /v1/skills/<id>                             versions
GET    /v1/skills/<id>/<version>                   manifest + tools + guards + thresholds
GET    /v1/skills/<id>/<version>/prompts/<name>    rendered prompt text
```

### Reviews (HITL)

See [`subsystems/hitl.md`](../subsystems/hitl.md). Endpoints under `/v1/reviews/...`.

### Cases

```
GET    /v1/cases?skill=<id>&status=<status>        case bank query
GET    /v1/cases/<case_id>                         single case
POST   /v1/cases/<case_id>/replay                  replay N times
```

### Audit

```
GET    /v1/audit/runs/<run_id>                     audit entries for a run
GET    /v1/audit/verify?tenant=<id>                chain verification
POST   /v1/audit/pack/<run_id>                     generate audit pack (returns download URL)
```

### Events (server-sent events)

```
GET    /v1/runs/<run_id>/events                    SSE stream of run events
```

Stream of audit-entry-shaped events for real-time UI updates. Filtered by run scope.

## Streaming a run

For long-running runs (recon at quarter-end can take minutes), clients SSE-subscribe to `/v1/runs/<run_id>/events` and receive each audit entry as it's appended. The CLI's `run` command displays via this stream when in server-attached mode.

## Authentication

Same as MCP: bearer tokens scoped to tenant, skills, and tools. Issued via `provenant token issue` or `POST /v1/admin/tokens`.

```
Authorization: Bearer prv_<tenant>_<random>
```

Admin endpoints under `/v1/admin/*` require an admin-scoped token.

## Error format

Consistent across endpoints:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "input.statement_url is required",
    "details": {
      "field": "input.statement_url",
      "schema": "https://schemas.provenant.dev/skills/recon/0.3.0/input.json"
    }
  }
}
```

HTTP status codes mirror REST conventions; the structured `error.code` is the stable contract for clients.

## Rate limiting

Per-token RPS limits, configurable per tenant. Default 10 RPS. Headers:

```
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 7
X-RateLimit-Reset: 1684332000
```

`429 Too Many Requests` on overflow.

## OpenAPI

A complete OpenAPI 3.1 spec is served at `/v1/openapi.json`. Generated from the Go types via `oapi-codegen`. Customers can generate clients in any language.

## Tests

- Contract: every endpoint covered by integration tests against a running server with seed data.
- Auth: tokens missing required scopes get `403`; missing tokens get `401`.
- Schema: malformed payloads return structured `validation_failed` errors.
- SSE: event stream survives transient network interruptions (client reconnect with `Last-Event-Id`).

## Open questions

1. **gRPC alongside REST.** Higher-throughput tenants may prefer gRPC. Proposal: defer to v0.5; OpenAPI-generated gRPC is feasible.
2. **GraphQL surface for the UI.** The HITL UI currently uses REST. GraphQL would reduce round trips. Defer; benchmark first.

## References

- See [`adapters/mcp.md`](mcp.md) for the parallel MCP surface
- [FINRA 2026](../references.md#finra-2026) and [SR 11-7](../references.md#sr-11-7) — auditability requirements that the audit endpoints satisfy
