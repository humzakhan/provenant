# ADR-0002: `submit_answer` as the single exit

Status: **Accepted**
Date: 2026-05-17

## Context

The kernel must guarantee that no answer returns to the user without passing the validator pipeline. If an agent can emit free-form text outputs through the host framework's normal channel (Claude.ai response, LangChain `agent.run()` return value, CLI stdout), the validator gate is bypassable and the entire enforcement story collapses.

We have two paths to enforce this:

1. **Prompt-level**: instruct the model not to emit final answers anywhere but a designated tool. Easy to implement; trivially bypassable by the model or by a malicious operator.
2. **Architectural**: make the host framework's only exit a single Provenant-controlled tool.

## Decision

The agent's only path to a user-visible answer is a single tool named `submit_answer`. Every adapter installs `submit_answer` and treats it as the official exit. Any output emitted through the host's normal text channel is logged but explicitly marked as unsigned and unverified in the audit trail.

`submit_answer`'s contract:

```json
{
  "answer": "<freeform text or structured object>",
  "claims": [
    { "text": "...", "tokens": ["tk_..."] }
  ],
  "skill_id": "...",
  "skill_version": "..."
}
```

The kernel runs token grounding plus the validator pipeline on this payload. Pass → answer released. Fail → structured errors back to the agent.

## Rationale

**Architectural enforcement beats prompt enforcement.** [Faramesh](../references.md#faramesh) is explicit: enforcement guarantees that depend on the model's cooperation are not guarantees. The agent cannot accidentally or intentionally bypass a tool it has no alternative to calling.

**[DCCD](../references.md#dccd) is the conceptual model.** DCCD decouples semantic planning (the model drafts freely) from structural enforcement (the constraint is applied at decode time). `submit_answer` is DCCD applied to the agent loop level: the agent drafts via reasoning, but the structure of the final output is constrained by the tool's schema and the validator pipeline. DCCD's experimental result — consistent accuracy improvements across model scales — supports the design.

**Token grounding is stricter than document-citation grounding.** Other implementations (Moody's, document-RAG patterns) ground at the document level: "this claim is supported by passages X and Y." That permits hallucinated numbers within otherwise-cited claims. Token-level grounding requires per-value lineage: every number in the answer maps to a real token with a verifiable source. This is what enables the "ungrounded number rate = 0" guarantee.

## Consequences

**Positive:**
- The validator gate is non-bypassable in Mode A (CLI harness).
- Audit log has a clean "released" event with a guaranteed shape.
- Skill packs have a clear authoring target: "your skill's job is to call `submit_answer` with valid claims."
- Adapters share a uniform exit contract; nothing platform-specific.

**Negative:**
- Adds an extra tool call to every run. Latency overhead is small (single Go function plus validator pipeline; typically <50ms).
- In Mode B (MCP, LangChain, OpenAI Agents), the host can emit text without calling `submit_answer`. We document this limitation and recommend Mode A for full enforcement.
- Skill packs that want narrative outputs must structure them through the `answer` field; the agent can't just "speak freely" at the end.

**Out of scope:**
- We do not attempt to enforce in Mode B that the host *only* releases `submit_answer` outputs. The host owns its UX. Customers who want full enforcement use Mode A.

## Alternatives considered

**Multiple exit tools (e.g., `submit_answer`, `submit_partial`, `submit_decline`).** Adds combinatorial validator paths and complicates the audit story. Rejected; the single exit takes any payload shape via the structured `answer` field.

**Prompt-only enforcement.** "You must end every response by calling submit_answer." Considered for low-stakes skills. Rejected as a default: prompt-only is not a guarantee, only a hope. Skills that need lighter-touch enforcement opt into a relaxed validator pipeline rather than skipping `submit_answer`.

**Architectural shim that intercepts text outputs and post-hoc grounds them.** Tempting in Mode B. Rejected because it requires the host to surface intermediate outputs back to the kernel in a structured way, which not every host does.

## References

- [Faramesh](../references.md#faramesh) — non-bypassable enforcement guarantee
- [DCCD](../references.md#dccd) — draft-then-enforce model
- See [`subsystems/guards.md`](../subsystems/guards.md) for the full `submit_answer` algorithm
