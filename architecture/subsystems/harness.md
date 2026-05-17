# Subsystem: Harness (Agent Runtime Loop)

Status: **v0**

> The harness is the agent runtime: the loop that takes a skill pack, an input, and a model, and runs them to completion (or rejection, or HITL). The kernel enforces the rules; the harness drives execution.

## Depends on
- [`subsystems/kernel.md`](kernel.md) — token operations
- [`subsystems/guards.md`](guards.md) — pre-LLM and post-LLM guards
- [`subsystems/tools.md`](tools.md) — tool dispatch
- [`subsystems/skills.md`](skills.md) — skill pack loading
- [`subsystems/audit.md`](audit.md) — every step logged
- [`subsystems/hitl.md`](hitl.md) — escalation routing

## Depended on by
- [`adapters/cli.md`](../adapters/cli.md) — embeds the harness directly
- [`adapters/mcp.md`](../adapters/mcp.md) — the MCP server delegates the loop to the host agent; the harness role there is partial (tool dispatch + submit_answer enforcement)
- All other adapters

## Responsibilities

1. Load the skill pack and assemble the system prompt + few-shot examples + tool descriptions.
2. Connect to the configured model.
3. Drive the tool-call / observation loop until the agent calls `submit_answer` or the run halts.
4. Apply pre-LLM input guards on every context addition.
5. Dispatch tool calls through the tool registry.
6. Enforce `submit_answer` through the guard pipeline.
7. Route to HITL when configured.
8. Persist a complete audit trail.

## The two harness modes

Provenant's harness has two shapes depending on the deployment:

### Mode A: Embedded harness (CLI, standalone)

The Provenant harness *is* the agent loop. It owns prompt assembly, tool dispatch, model calls, the works. Used by:

- `provenant run` CLI
- Custom Go agents using `pkg/kernel`

### Mode B: Adapter-side harness (MCP, LangChain, OpenAI Agents)

The host agent (Claude Code, LangChain, OpenAI Agents SDK) drives its own loop. The Provenant harness's role is reduced to: tool dispatch, guard enforcement, and `submit_answer` validation. We do not control prompt assembly or the loop structure in this mode — only the I/O contract.

This is by design. See [`../README.md`](../README.md#core-invariants): the kernel enforces I/O behavior, not step-level behavior. Mode B trades step-level visibility for zero-friction integration.

## The embedded harness loop (Mode A)

```
┌──────────────────────────────────────────────────────────────┐
│  1. Load skill pack: prompts, tools, guards, eval thresholds │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  2. Build initial context:                                   │
│     - system prompt + skill prompt                           │
│     - tool descriptions (typed schemas from registry)        │
│     - retrieved policy snippets via policy.lookup            │
│     - user input                                             │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼ pre-LLM guards on each context piece
┌──────────────────────────────────────────────────────────────┐
│  3. Model call ─────────────────────────────────┐            │
│                                                  │            │
│     Returns one of:                              │            │
│       a) tool_use  ──▶  dispatch via registry    │            │
│          (mints tokens, audit-logs the call,     │            │
│           runs pre-LLM guards on the response,   │            │
│           feeds result back into context) ───────┘            │
│                                                               │
│       b) submit_answer  ──▶  guard pipeline ──▶               │
│            ├─ pass: persist, audit, optionally HITL, end      │
│            └─ fail: structured errors back to model           │
│                                                               │
│       c) explicit abort ("I cannot complete this")            │
│          ──▶  audit, end                                      │
└──────────────────────────────────────────────────────────────┘
```

Looping is bounded by:

- **`max_steps`**: hard cap on tool calls per run, declared in the skill manifest (default 20, configurable).
- **`max_wall_clock`**: per-run wall-clock timeout (default 5 minutes, configurable).
- **`max_cost`**: token cost budget (default $0.50 / run, configurable).

Exceeding any bound terminates the run with a structured `harness.exhausted` failure and an audit entry.

## Prompt assembly

The skill pack ships:

- `prompts/system.md` — base system prompt
- `prompts/skill.md` — skill-specific instructions
- `prompts/examples/*` — few-shot examples
- `prompts/templates/*` — Jinja-style templates for parameterized prompts

The harness assembles in this order:

```
1. Kernel header  (read-only, harness-owned, ~200 tokens):
   - Enumerates submit_answer's exact contract.
   - Defines the claim format and token reference syntax.
   - Lists the available tools by name with one-line descriptions.

2. Skill system prompt (from prompts/system.md)

3. Skill instructions (from prompts/skill.md)

4. Few-shot examples (sampled or all, configurable per skill)

5. Tool definitions (typed JSON schemas; emitted in the format the model expects: 
   Claude tool_use format, OpenAI function format, etc.)

6. Retrieved context (policy snippets, prior cases if enabled)

7. User input
```

The kernel header is **immutable** — skills cannot override it. This is what guarantees the agent knows about `submit_answer` and the claim format regardless of pack quality.

## Tool dispatch

When the model emits a `tool_use` (or equivalent), the harness:

1. Looks up the tool by canonical name in the registry.
2. Validates the input payload against the tool's input schema. **Fail-closed** on schema mismatch — the agent receives an error response without the tool being called.
3. Invokes the tool. Audit middleware wraps every call.
4. The tool returns a schema-validated `Result` payload (no tokens). The kernel — not the tool — then mints tokens from the values at the tool's declared `mintable_paths`, per [`architecture.md`](../../architecture.md) §5.2. Tools cannot construct `Token` values. Any field a downstream validator needs (e.g., `confidence.extraction_min`) must be on a declared `mintable_path` in the tool's schema; if it is not, the tool registration is rejected at load time with a clear error rather than silently dropping the field at runtime. The conformance test in [`tools/README.md`](../tools/README.md) verifies that every validator referenced by a skill has a `mintable_path` source in at least one tool the skill uses.
5. Pre-LLM guards run on the tool response payload (a tool can be a source of prompt injection — see [`guards.md`](guards.md#pre-llm-input-guards)).
6. The sanitized response feeds back into the model's context as a `tool_result`.

Tool calls within a single agent step can be **parallel** if the model emits multiple in the same response (Claude's parallel tool use). The harness dispatches them concurrently and waits on all before the next model call.

**Parallel-failure semantics (resolved).** When one sibling fails (schema mismatch, backend error, guard rejection), the harness **does not cancel** other in-flight siblings; it waits for all to complete and reports the union of results in the next agent observation. This is the only choice that preserves replay determinism: cancellation order would depend on real-wall-clock scheduling, which is non-deterministic. Each sibling's success or failure is recorded as its own `tool_call_completed` (or `tool_call_failed`) audit entry; tokens minted by successful siblings persist regardless of sibling failures (their lineage is sibling-independent). The agent sees the structured per-call errors and decides whether to retry, abort, or proceed.

## Model abstraction

The harness talks to models through a single interface:

```go
package model

type Model interface {
    Name() string
    Capabilities() Capabilities

    Call(ctx context.Context, req Request) (Response, error)
}

type Request struct {
    System       string
    Messages     []Message
    Tools        []ToolDef
    MaxTokens    int
    Temperature  float64        // default 0.0 for compliance paths
    StopSequences []string
}

type Response struct {
    Content   []ContentBlock    // text | tool_use blocks
    StopReason string
    Usage     Usage             // input/output tokens, cost
}
```

Implementations: `anthropic.Claude`, `openai.GPT`, `local.Ollama`, `azure.OpenAI`. Selection per-skill or per-run via configuration.

**Temperature default is 0.0** on compliance-critical paths. [DFAH](../references.md#dfah) shows this is necessary but not sufficient for determinism — see [`subsystems/eval.md`](eval.md).

## Cost and latency tracking

Every model call writes `usage` data to the run record. The harness exposes:

- Real-time cost meter (updates on each tool/model call).
- Per-skill cost report aggregated across runs.
- Cost budget enforcement (`max_cost` per run; alert thresholds).

This is plumbing, but it's the difference between a finance buyer signing off and not. CFOs don't trust unbounded LLM spend.

## Termination conditions

A run ends when:

| Condition | Status |
|---|---|
| Agent calls `submit_answer` and passes guards | `completed` |
| Agent calls `submit_answer`, passes, HITL required | `awaiting_review` |
| Agent emits explicit abort ("cannot complete") | `aborted` |
| `max_steps` exceeded | `failed: exhausted` |
| `max_wall_clock` exceeded | `failed: timeout` |
| `max_cost` exceeded | `failed: cost_cap` |
| Kernel integrity violation | `failed: integrity` |
| Model API error after retries | `failed: model_error` |

Status transitions are recorded in the audit log.

## Mode B specifics (adapter-side harness)

In MCP / LangChain / OpenAI Agents modes, the host agent owns the loop. Provenant cannot guarantee:

- That the kernel header is in the system prompt (the host may have its own).
- That tool descriptions are presented in our preferred order.
- That the agent calls `submit_answer` rather than emitting a text answer through the host's normal channel.

We mitigate these by:

1. The MCP adapter exposes the skill pack's prompts as **MCP prompts**. Hosts that support prompt invocation (Claude.ai) attach our kernel header. For hosts that don't auto-attach (Claude Code by default), the user must explicitly invoke `/prompt provenant.recon` or similar.
2. The `submit_answer` tool is the only path that returns "official" output to the user. Anything the agent emits through the host's text channel without going through `submit_answer` is **not Provenant-signed**. The audit pack's `run_completed` entry explicitly records whether the run ended via `submit_answer` (signed) or without it (`exit_kind: host_text_only`). The kernel-immutable `submit_answer` schema includes a `signature_marker` field that the audit pack and `provenant audit pack` output prominently display; consumers of MCP output can use this marker to detect that host-channel text is unsigned by Provenant.
3. Skill pack documentation (kernel-immutable header text, included in every Mode B prompt the host pulls via MCP) instructs the model that only `submit_answer` output is released to the user; non-`submit_answer` text is treated as scratch.
4. For deployments that need architectural non-bypassability of the host text channel itself, Mode A (CLI harness) is the required path. This is documented in [`thesis.md`](../../thesis.md) and called out in [ADR-0007](../adrs/0007-mcp-as-primary-adapter.md) as the consequence of choosing MCP-first.

## Storage

The harness writes:

```sql
CREATE TABLE runs (
    id              TEXT PRIMARY KEY,                  -- ULID
    tenant_id       TEXT NOT NULL,
    skill_id        TEXT NOT NULL,
    skill_version   TEXT NOT NULL,
    model           TEXT NOT NULL,                     -- e.g., "claude-opus-4-7"
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    status          TEXT NOT NULL,                     -- pending | running | awaiting_review | completed | failed | aborted
    status_reason   TEXT,                              -- structured reason on failed/aborted
    input_hash      TEXT NOT NULL,                     -- sha256 of canonicalized input
    initiator       TEXT NOT NULL,                     -- user or system identifier
    cost_usd        NUMERIC(12, 6),
    tokens_in       INTEGER,
    tokens_out      INTEGER
);
```

## Tests

- Unit: tool dispatch with schema mismatch fails-closed.
- Unit: bound enforcement (max_steps, max_wall_clock, max_cost).
- Integration: end-to-end recon run completes through Mode A; produces all expected audit entries.
- Integration: identical run produces identical token set across N replays (decision determinism).
- Property: malformed model responses do not crash the harness; they fail-close to `failed: model_error`.

## Open questions

1. **Streaming model responses.** The current contract waits for complete responses. Streaming would improve UX but complicates guard application on tool_use blocks. Proposal: stream text, batch tool_use, finalize at end-of-response.
2. **Mode B prompt injection of the kernel header.** Should the MCP adapter attempt to forcibly inject the kernel header into the host's system prompt? Probably not (it's the host's prompt), but we should document the limitation prominently.

## References

- [Faramesh](../references.md#faramesh) — non-bypassable enforcement at the boundary; harness preserves this for Mode A and partially for Mode B
- [DCCD](../references.md#dccd) — draft-then-enforce loop shape
- [DFAH](../references.md#dfah) — temperature 0 + schema-first for determinism
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — hierarchical-with-reflexive-fallback shape
