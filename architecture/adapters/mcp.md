# Adapter: MCP Server

Status: **v0 / equal first-class adapter**

> The MCP adapter is one of Provenant's equal first-class integration surfaces. Any MCP-capable host — Claude Code, Cursor, Claude.ai connectors, custom MCP clients — points at the Provenant MCP server and gains the full finance tool surface plus the validator-gated `submit_answer`. Zero code from the integrator, and no adapter has architectural primacy.

## Depends on
- [`subsystems/harness.md`](../subsystems/harness.md) — partial harness role in Mode B
- [`subsystems/tools.md`](../subsystems/tools.md) — tool registry exposed as MCP tools
- [`subsystems/skills.md`](../subsystems/skills.md) — skill prompts exposed as MCP prompts
- [`subsystems/guards.md`](../subsystems/guards.md) — `submit_answer` enforcement
- The official Go MCP SDK

## Why MCP matters

See [ADR-0010](../adrs/0010-mcp-equal-not-primary.md). MCP has converged into the universal agent tool protocol; both [Anthropic](../references.md#anthropic-finance-agents) and [Moody's](../references.md#moodys-agentic) ship finance agents through it. Shipping as an MCP server places Provenant inside every MCP-capable surface, alongside the CLI and HTTP access surfaces.

## Server surface

A Provenant MCP server exposes three MCP primitives:

### Tools

Every registered Provenant tool is exposed as an MCP tool:

```
ledger.get_balance
ledger.get_transactions
ledger.search
extract.bank_statement
extract.invoice
extract.receipt
reconcile.match
calc.subtract
calc.sum
calc.percent_change
calc.variance
policy.lookup
external.fx_rate
external.sanctions_check
submit_answer                  <- the single exit
```

Tool descriptions, input schemas, and output schemas are emitted in MCP format. The MCP host displays them to the model exactly as it does for native MCP tools. The agent does not know — and does not need to know — that these come from Provenant.

### Prompts

Each loaded skill pack's prompts surface as MCP prompts:

```
provenant.recon                     <- the recon workflow prompt
provenant.ap_coding                 <- AP coding prompt
provenant.expense_review            <- expense compliance prompt
provenant.sanctions_disposition     <- sanctions hit prompt
```

When the user (or the host) invokes a prompt, the MCP server returns the assembled prompt text including the **kernel header** (an immutable preamble that tells the model about `submit_answer`, the claim format, and token references). See [`subsystems/harness.md`](../subsystems/harness.md#prompt-assembly).

### Resources

Read-only inspection surfaces:

```
provenant://skills/recon/manifest                       skill metadata
provenant://skills/recon/policy/tolerance.md            policy document
provenant://runs/<run_id>/trajectory                    audit trail summary
provenant://runs/<run_id>/tokens                        token list
provenant://cases/<case_id>                             case bank entry
```

Resources are useful for human inspection inside the MCP host. They are not part of the enforcement contract.

## Mode B harness behavior

In MCP mode, the host (Claude Code, Cursor, etc.) drives the agent loop. Provenant's role is reduced; see [`subsystems/harness.md`](../subsystems/harness.md#the-two-harness-modes).

The official local-agent pattern is enforcement by attestation, not by control of the host's arbitrary text channel. MCP can expose `submit_answer`, prompts, tools, and resources, but it cannot force an MCP host or model to route every assistant message through Provenant. Final regulated conclusions should go through `submit_answer`; the visible marker `[PROVENANT-ATTESTED v1 ...]` is official only when produced by a passing `submit_answer` and matched against the audit chain/audit pack. Host-channel answers that lack verified attestation are non-attested scratch output, even if they were produced in a session that had Provenant tools available.

What Provenant **does** enforce in Mode B:
- Tool input validation (schema-first).
- Token minting from tool responses.
- Pre-LLM guards on tool response payloads, before they return to the host.
- `submit_answer` validation: token grounding, validator pipeline, fail-closed on errors.
- Audit logging of all of the above.

What Provenant **cannot** enforce in Mode B:
- That the host's system prompt includes the kernel header (depends on whether the host auto-attaches MCP prompts).
- That the host actually calls `submit_answer` rather than just emitting text directly.
- That arbitrary host-channel model output includes, preserves, or respects Provenant's attestation marker.
- The agent's loop bounds (max_steps, max_wall_clock, max_cost) — those are the host's concern.

Restricting regulated files and values to Provenant-controlled tools/readers mitigates direct raw-file bypass, but it does not make Mode B host-channel output non-bypassable; regulated conclusions still require `submit_answer` and audit verification. For hard non-bypassable enforcement, use Mode A (the CLI or embedded harness) or another Provenant-owned harness. Mode B is recommended for: interactive workflows where the user is already in Claude Code; gradual adoption; experimentation; and environments where downstream consumers treat unverified host text as non-attested.

## Authentication

Bearer-token auth on the MCP transport:

```
Authorization: Bearer prv_<tenant_id>_<random>
```

Tokens are scoped to:
- A tenant.
- A subset of skills (configurable per token).
- A subset of tools (default: all skills' required tools; can be restricted).

Token issuance via the CLI (`provenant token issue --tenant acme --skills recon,ap_coding`) or admin REST API.

## Server configuration

```yaml
# provenant-server.yaml
mcp:
  bind: "0.0.0.0:8080"
  transport: "stdio"        # or "sse" or "http"
  tls:
    enabled: true
    cert_file: /etc/provenant/tls/cert.pem
    key_file: /etc/provenant/tls/key.pem

storage:
  mode: memory               # memory | sqlite | postgres
  sqlite:
    path: /var/lib/provenant/provenant.db
  postgres:
    dsn: "postgres://provenant@localhost:5432/provenant?sslmode=verify-full"

blob_store:
  mode: memory               # memory | s3
  s3:
    endpoint: "https://s3.amazonaws.com"
    bucket_prefix: "provenant-"

tenants:
  - id: acme
    skills:
      - recon@0.3.0
      - ap_coding@0.2.0
  - id: globex
    skills:
      - sanctions_disposition@0.1.0

logging:
  level: info
  json: true
```

## Deployment

### Local development

```bash
docker compose up    # memio/stateless by default; use sqlite or postgres profiles when HITL persistence is needed
```

### Single-tenant production

```bash
docker run -d \
  -p 8080:8080 \
  -v /etc/provenant:/etc/provenant:ro \
  -e PROVENANT_CONFIG=/etc/provenant/server.yaml \
  ghcr.io/provenant/server:0.1.0
```

SQLite is the default persistent binding when HITL pause/resume is enabled. Postgres and S3 are optional runtime bindings for multi-process or large-payload deployments; the MCP server can also run stateless against `kernel/public/memio/` when persistence is not needed.

State lives in the selected runtime binding: memory for stateless runs, SQLite for single-machine persistence, or Postgres + S3 for multi-process deployments. The server itself can remain stateless; multiple replicas behind a load balancer scale horizontally for shared tenants.

### Connecting from Claude Code

```bash
claude mcp add provenant \
  --command "provenant-mcp-client" \
  --args "--server=http://localhost:8080 --token=prv_acme_xxxx --skill=recon"
```

The `provenant-mcp-client` binary is a thin local wrapper that handles stdio-to-HTTP translation (since Claude Code uses stdio MCP by default).

### Connecting from Claude.ai

Via the connector marketplace: customer installs the Provenant connector, configures their server URL and token, picks a skill. The connector handles MCP over HTTP with SSE for streaming.

## Per-call flow

```
1. Host agent decides to call a tool (e.g., ledger.get_balance).
2. MCP request arrives at the Provenant server.
3. Server validates token + tenant + skill + tool scope.
4. Server routes to the tool registry.
5. Tool registry validates input schema, dispatches, mints tokens, audit-logs.
6. Server runs pre-LLM guards on the tool's output payload.
7. Server returns the (possibly redacted) payload to the host as an MCP tool_result.
8. Host agent continues its loop.
9. When agent calls submit_answer (also an MCP tool):
   a. Server runs the grounding + validator pipeline.
   b. On pass: marks the run completed (or awaiting_review), returns success.
   c. On fail: returns structured errors as the tool_result; agent retries.
```

The agent visibly retries `submit_answer` on validator failure — this is the demonstration of enforcement working. See [`example-projects.md`](../../example-projects.md) Demo 2.

In local-agent MCP deployments, the user-facing acceptance rule is simple: a regulated final answer is official only if it came from a passing `submit_answer` and its `[PROVENANT-ATTESTED v1 ...]` marker matches the audit chain/audit pack. Marker text by itself is not proof of attestation; any answer without verified attestation is outside the Provenant enforcement boundary.

## Streaming and async

- Tool calls: synchronous request/response over MCP.
- HITL escalation: the `submit_answer` response indicates `awaiting_review` and includes a review handle. The host can poll the `provenant://runs/<run_id>` resource for status updates, or the customer's review UI handles it out-of-band.
- Long-running tools (e.g., extract on a 100-page PDF): MCP supports progress notifications; we use them for tools with p95 latency > 5 seconds.

## Limitations and recommendations

| Limitation | Recommendation |
|---|---|
| Kernel header not in the host's system prompt unless the host attaches the MCP prompt | Document prominently; instruct users to invoke `/prompt provenant.<skill>` |
| Host can emit text outputs without `submit_answer` | Skill pack documentation must clarify that only `submit_answer` outputs are signed |
| MCP cannot force arbitrary host-channel model output | Treat unmarked text as scratch; require `submit_answer` for regulated conclusions |
| Regulated files are available outside Provenant | Restrict file/data access to mitigate direct raw-file bypass; use Mode A or a Provenant-owned harness for hard non-bypassable control |
| No control over the host's loop bounds | Provide our own per-tenant rate limits and cost caps server-side |
| Some MCP clients don't surface MCP prompts well | Mode A (CLI) is the fallback for production-grade enforcement |

## Tests

- End-to-end: Claude Code → Provenant MCP → reconciliation skill → audit pack produced.
- Conformance: every Provenant tool is callable via MCP and produces identical token IDs to direct kernel calls.
- Security: invalid bearer tokens rejected; cross-tenant access attempts blocked.
- Latency: p95 overhead from MCP layer < 50ms on top of tool execution.

## Open questions

1. **MCP versioning.** As the MCP spec evolves, we will need to support multiple protocol versions concurrently. Proposal: maintain N-1 and N for at least 12 months.
2. **Prompt attachment UX.** Different MCP clients handle prompt attachment differently. Should we ship a per-client integration guide? Yes.
3. **Resource URIs.** Should we use `provenant://` or stick to HTTPS URLs? MCP supports both; `provenant://` is cleaner for the consumer.

## References

- [Anthropic finance agents launch](../references.md#anthropic-finance-agents) — MCP as the finance distribution channel
- [Moody's MCP launch](../references.md#moodys-agentic) — co-validates the strategy
- See [ADR-0010](../adrs/0010-mcp-equal-not-primary.md) for the equal-first-class adapter decision
