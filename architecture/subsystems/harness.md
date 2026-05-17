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

Tool calls within a single agent step can be **parallel** if the model emits multiple in the same response (Claude's parallel tool use). The harness dispatches them concurrently and waits on all before the next model call. **v0 supports single-turn, non-streaming parallel only**: every member of a parallel sibling group must be emitted as a fully-formed `tool_use` block within the same model response turn. Streaming tool_use is disabled at the model-abstraction layer in v0; if the configured `ModelClient` advertises streaming capability, the harness forces it off for compliance-path runs. Parallel-batches split across multiple response turns are treated as **independent** sibling groups (one per turn) with their own `sibling_group_id`. See [`audit.md`](audit.md#parallel-sibling-deterministic-order) "Scope and constraints (v0)" and KNOWN_TRADE_OFFS.md.

**Parallel-failure semantics (resolved).** When one sibling fails (schema mismatch, backend error, guard rejection), the harness **does not cancel** other in-flight siblings; it waits for all to complete and reports the union of results in the next agent observation. This is the only choice that preserves replay determinism: cancellation order would depend on real-wall-clock scheduling, which is non-deterministic. Each sibling's success or failure is recorded as its own `tool_call_completed` (or `tool_call_failed`) audit entry; tokens minted by successful siblings persist regardless of sibling failures (their lineage is sibling-independent). The agent sees the structured per-call errors and decides whether to retry, abort, or proceed.

**Parallel-sibling audit ordering (load-bearing for replay byte-identity).** Real-wall-clock completion order of parallel siblings is non-deterministic and must not influence `audit_entries.seq`. The harness implements the buffer-and-flush rule defined in [`audit.md`](audit.md#parallel-sibling-deterministic-order): for any batch of N parallel tool_use blocks emitted by the model in a single response, the harness assigns each sibling a canonical `sibling_index_in_group` from the model's emission order in the canonicalized response envelope, buffers all `tool_call_started`/`tool_call_completed`/`tool_call_failed`/`token_mint` entries for the batch in kernel-internal state, and flushes the buffer to the audit sink in canonical order (ascending `sibling_index_in_group`, then intra-sibling sub-step order) only after all siblings complete. The audit sink assigns `seq` strictly in flush order. The harness MUST NOT stream parallel-sibling entries to the sink during the dispatch window; doing so reintroduces the cycle-2 critic #9 failure mode where every later entry's `prev_hash` diverges across replays. `tool_call_id` is the deterministic kernel-assigned identifier defined in [`audit.md`](audit.md#tool-call-id-and-seq-determinism), not a freshly-drawn ULID.

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
| Orphan-run sweep at startup (last entry older than per-skill threshold) | `failed: orphaned` (with `EntryKindRunOrphanRecovered` preceding the `EntryKindRunFailed`) |
| DR restore-verify finds missing blob refs for this run | `restore_pending` (transient; not a terminal state) |
| Operator acknowledges partial blob loss for this run | `restore_pending_acknowledged` (terminal for replay/audit-pack purposes; the run remains queryable but is excluded from `audit pack` unless `--allow-acknowledged-gaps` is passed) |

Status transitions are recorded in the audit log.

### State transitions (canonical)

```
pending  ──▶ running ──▶ awaiting_review ──▶ completed
                    └──▶ completed
                    └──▶ aborted
                    └──▶ failed
                    └──▶ <process death> ──▶ (orphan sweep) ──▶ failed [reason=orphaned]

(any non-running terminal state on restore-verify failure):
  completed | failed | aborted ──▶ restore_pending ──▶ restore_pending_acknowledged
                                              │
                                              └──▶ (operator restores lagging store) ──▶ prior_terminal_state
```

`restore_pending` and `restore_pending_acknowledged` are reachable only from a terminal state; a `running` or `awaiting_review` run cannot transition into them (those runs are first moved to `failed: orphaned` by the sweep, since by definition they are no longer being driven by a live harness when restore-verify runs). The kernel's run-state-machine enforces this; an attempt to transition `running` → `restore_pending` is a typed `run.invalid_transition` error and itself audited.

### Orphan-run threshold (per skill)

The orphan-recovery threshold is **per skill**, not a global default. Each loaded skill manifest's `bounds.max_wall_clock_seconds` is the input; the sweep's effective threshold is `max_wall_clock_seconds × orphan_multiplier`, where `orphan_multiplier` is pinned in the skill manifest (`bounds.orphan_multiplier`, default 2) — see [`skills.md`](skills.md) manifest schema. `orphan_multiplier` is **not** deployment-overridable: the deployment's `config.yaml` cannot supersede the skill's pinned value. This is necessary for cross-mode byte-identity: two deployments running the same `(skill_id, skill_version)` must compute the same `threshold_seconds_used`, otherwise `EntryKindRunOrphanRecovered.deterministic.threshold_seconds_used` diverges and chain byte-identity fails on any orphan-recovered run. When the sweep runs, it looks up each candidate run's `skill_id` + `skill_version`, reads the pinned manifest from the `skills` table, applies that skill's threshold, and records the actual threshold used in `EntryKindRunOrphanRecovered.deterministic.threshold_seconds_used`. Concurrent runs with different `max_wall_clock` values are evaluated independently; a long-budget skill is not prematurely killed because a short-budget skill shares the deployment.

## Mode B specifics (adapter-side harness)

In MCP / LangChain / OpenAI Agents modes, the host agent owns the loop. Provenant cannot guarantee:

- That the kernel header is in the system prompt (the host may have its own).
- That tool descriptions are presented in our preferred order.
- That the agent calls `submit_answer` rather than emitting a text answer through the host's normal channel.

We mitigate these by:

1. The MCP adapter exposes the skill pack's prompts as **MCP prompts**. Hosts that support prompt invocation (Claude.ai) attach our kernel header. For hosts that don't auto-attach (Claude Code by default), the user must explicitly invoke `/prompt provenant.recon` or similar.
2. The `submit_answer` tool is the only path that returns "official" output to the user. Anything the agent emits through the host's text channel without going through `submit_answer` is **not Provenant-signed**. The audit pack's `run_completed` entry explicitly records whether the run ended via `submit_answer` or without it; `exit_kind` is a deterministic enum field on `EntryKindRunCompleted` with exactly two legal values — `submit_answer` and `host_text_only` — enumerated in [`audit.md`](audit.md#per-entrykind-payload-schema-deterministic-surface-enumeration). The kernel-immutable `submit_answer` tool's output envelope carries a `signature_marker` field (defined below) that the audit pack and `provenant audit pack` prominently display; consumers of MCP output can use this marker to detect that host-channel text is unsigned by Provenant.

#### `signature_marker` (definition)

`signature_marker` is a top-level field on the `submit_answer` tool's **output** envelope returned to the host agent on a passing `SubmitAnswer` call. It is **not** an authentication credential — it is a visible, well-known, kernel-emitted string whose presence on user-facing output is the agreed signal that the bytes came through Provenant's enforcement boundary. Its absence on host-channel text is what makes the lack-of-attestation visible to downstream consumers.

Shape:

```json
{
  "signature_marker": {
    "version": "1",
    "literal": "[PROVENANT-ATTESTED v1 run_id=<run_id> chain_seq=<seq_of_answer_released>]",
    "run_id": "<run_id>",
    "chain_seq": "<seq>",
    "answer_payload_hash": "<sha256 of canonical(payload)>"
  }
}
```

- The `literal` field is the kernel-rendered string a host SHOULD include verbatim when surfacing the answer. The literal is the user-visible signal; the structured fields are for programmatic verification.
- The `literal` format is part of the kernel-immutable surface: skills cannot override it, hosts cannot rename it, and `provenant audit pack` rejects pack contents whose released-answer text contains a `[PROVENANT-ATTESTED v1 ...]` substring that does not match the chain. This catches Mode B hosts that forge a marker without going through `SubmitAnswer`.
- Any Mode B host that strips the marker before surfacing text produces output that is, by definition, indistinguishable from text never routed through `submit_answer`; from Provenant's perspective the run terminates with `exit_kind: host_text_only` (the kernel records no `answer_released` entry), and consumers see no marker. The thesis-level scoping in [`../../thesis.md`](../../thesis.md) covers this case: Mode B hosts that strip the marker are not Provenant-attested, period. For deployments where the marker must reach the end user, Mode A is required.

#### Kernel-immutable header text (definition)

The kernel-immutable header referenced under "Prompt assembly" above is the harness-owned, skill-unoverridable preamble injected into every assembled prompt. Its concrete text is checked into the kernel at `kernel/internal/promptkernel/header_v1.go` and locked: changes require a kernel version bump and break the `skill_pack_hash` audit assumption only if a skill's prompt depended on absent header content (it should not). The v1 header text is:

```
You are operating inside Provenant, a substrate that enforces provenance and audit on every released answer.

The ONLY way to produce output that reaches the user as a Provenant-attested answer is to call the tool named `submit_answer` with a payload conforming to its declared schema. Any text you emit through any other channel — assistant prose, "final answer" text, host-channel output — is NOT Provenant-attested, will NOT appear in the audit pack as a released answer, and SHOULD be treated by you as scratch reasoning, not as output to the user.

A successful `submit_answer` call returns a `signature_marker` object whose `literal` field begins with the exact string `[PROVENANT-ATTESTED v1 ...]`. If you surface output to the user, include this literal verbatim adjacent to the answer text so the user can see that the answer is attested.

The available tools, their schemas, and the claim/token-reference syntax are enumerated below. This header is read-only; the skill prompt that follows cannot override it.
```

The header is byte-identical across all runs (Mode A and Mode B) for a given kernel version and is hashed into `EntryKindPromptSent.deterministic.tool_def_hash`'s parent `prompt_hash`. A Mode B host that strips the header before sending to the model is detectable post-hoc: the model's response will not match the response distribution the kernel-vs-no-kernel-header A/B fixture establishes during Phase 4 conformance testing, and the `provenant mcp-conformance` check explicitly verifies that the host's pulled `provenant.<skill>` MCP prompt resource begins with this header literal. A host that pulls the prompt resource via MCP and then mutates it before model dispatch is out of Provenant's enforcement scope and is documented as such in (4) below.
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
    status          TEXT NOT NULL,                     -- pending | running | awaiting_review | completed | failed | aborted | restore_pending | restore_pending_acknowledged
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
- Conformance: `provenant mcp-conformance` verifies that the kernel-immutable header text appears verbatim as the leading content of every `provenant.<skill>` MCP prompt resource the runtime exposes. A test harness simulates a Mode B host that strips the header from the pulled prompt and dispatches anyway; the conformance check flags the resulting run as non-conformant (the released-answer text either lacks a `signature_marker` literal, in which case the audit pack records `exit_kind: host_text_only`, or contains a forged literal that fails the chain-cross-check in `provenant audit pack`).
- Property: parallel-sibling audit ordering — for any model response containing N ≥ 2 parallel tool_use blocks, two replays of the same response with deliberately-shuffled wall-clock completion order (injected via a deterministic test scheduler) produce byte-identical chains. Asserts the buffer-and-flush rule in [`audit.md`](audit.md#parallel-sibling-deterministic-order).
- Release-gate: **kernel-header content-hash test.** A release-time test recomputes the sha256 of `kernel/internal/promptkernel/header_v<N>.go`'s embedded header bytes and compares against an expected digest pinned per `kernel_version`. The test fails the build if header text and `kernel_version` drift apart. This guarantees that the `kernel_header_hash` recorded in `EntryKindRunStarted.deterministic` ([`audit.md`](audit.md#per-entrykind-payload-schema-deterministic-surface-enumeration)) corresponds to the kernel version in the same entry.
- Integration: **token-mint atomicity across parallel block.** A test injects a deterministic crash between sibling completion and buffer flush; on restart, neither `tokens` rows nor `audit_entries` rows from the crashed batch exist (the run is recovered as orphaned). Asserts the single-transaction guarantee in [`audit.md`](audit.md#parallel-sibling-deterministic-order) implementation note.

## Open questions

1. **Streaming model responses.** The current contract waits for complete responses. Streaming would improve UX but complicates guard application on tool_use blocks and breaks the intra-sibling ordering invariant in [`audit.md`](audit.md#parallel-sibling-deterministic-order). **v0 explicitly disables streaming tool_use** on compliance-path runs (see KNOWN_TRADE_OFFS.md "Streaming and split-turn parallel deferred"). v0.5+ proposal: stream text, batch tool_use, finalize at end-of-response; revisit at that point with the merging-rule design.
2. **Mode B prompt injection of the kernel header.** Should the MCP adapter attempt to forcibly inject the kernel header into the host's system prompt? Probably not (it's the host's prompt), but we should document the limitation prominently.

## References

- [Faramesh](../references.md#faramesh) — non-bypassable enforcement at the boundary; harness preserves this for Mode A and partially for Mode B
- [DCCD](../references.md#dccd) — draft-then-enforce loop shape
- [DFAH](../references.md#dfah) — temperature 0 + schema-first for determinism
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — hierarchical-with-reflexive-fallback shape
