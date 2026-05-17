# Adapter: MCP Server

Status: **v0 / primary adapter**

> The MCP adapter is Provenant's primary integration surface. Any MCP-capable host — Claude Code, Cursor, Claude.ai connectors, custom MCP clients — points at the Provenant MCP server and gains the full finance tool surface plus the validator-gated `submit_answer`. Zero code from the integrator.

## Depends on
- [`subsystems/harness.md`](../subsystems/harness.md) — partial harness role in Mode B
- [`subsystems/tools.md`](../subsystems/tools.md) — tool registry exposed as MCP tools
- [`subsystems/skills.md`](../subsystems/skills.md) — skill prompts exposed as MCP prompts
- [`subsystems/guards.md`](../subsystems/guards.md) — `submit_answer` enforcement
- The official Go MCP SDK

## Why MCP first

See [ADR-0007](../adrs/0007-mcp-as-primary-adapter.md). MCP has converged into the universal agent tool protocol; both [Anthropic](../references.md#anthropic-finance-agents) and [Moody's](../references.md#moodys-agentic) ship finance agents through it. Shipping as an MCP server places Provenant inside every MCP-capable surface for free.

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

What Provenant **does** enforce in Mode B:
- Tool input validation (schema-first).
- Token minting from tool responses.
- Pre-LLM guards on tool response payloads, before they return to the host.
- `submit_answer` validation: token grounding, validator pipeline, fail-closed on errors.
- Audit logging of all of the above.

What Provenant **cannot** enforce in Mode B:
- That the host's system prompt includes the kernel header (depends on whether the host auto-attaches MCP prompts).
- That the host actually calls `submit_answer` rather than just emitting text directly.
- The agent's loop bounds (max_steps, max_wall_clock, max_cost) — those are the host's concern.

For full enforcement, Mode A (the CLI harness) is the recommended deployment. Mode B is recommended for: interactive workflows where the user is already in Claude Code; gradual adoption; experimentation.

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

tenants:
  - id: acme
    skills:
      - recon@0.3.0
      - ap_coding@0.2.0
  - id: globex
    skills:
      - sanctions_disposition@0.1.0

postgres:
  dsn: "postgres://provenant@localhost:5432/provenant?sslmode=verify-full"

object_store:
  endpoint: "https://s3.amazonaws.com"
  bucket_prefix: "provenant-"

logging:
  level: info
  json: true
```

## Deployment

### Local development

```bash
docker compose up    # postgres + minio + provenant-server
```

### Single-tenant production

```bash
docker run -d \
  -p 8080:8080 \
  -v /etc/provenant:/etc/provenant:ro \
  -e PROVENANT_CONFIG=/etc/provenant/server.yaml \
  ghcr.io/provenant/server:0.1.0
```

State lives in Postgres and S3. The server itself is stateless; multiple replicas behind a load balancer scale horizontally for shared tenants.

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

## Streaming and async

- Tool calls: synchronous request/response over MCP.
- HITL escalation: the `submit_answer` response indicates `awaiting_review` and includes a review handle. The host can poll the `provenant://runs/<run_id>` resource for status updates, or the customer's review UI handles it out-of-band.
- Long-running tools (e.g., extract on a 100-page PDF): MCP supports progress notifications; we use them for tools with p95 latency > 5 seconds.

## Limitations and recommendations

| Limitation | Recommendation |
|---|---|
| Kernel header not in the host's system prompt unless the host attaches the MCP prompt | Document prominently; instruct users to invoke `/prompt provenant.<skill>` |
| Host can emit text outputs without `submit_answer` | Skill pack documentation must clarify that only `submit_answer` outputs are signed |
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
- See [ADR-0007](../adrs/0007-mcp-as-primary-adapter.md) for the MCP-first decision
