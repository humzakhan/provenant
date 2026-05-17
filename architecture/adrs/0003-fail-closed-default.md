# ADR-0003: Fail-closed by default

Status: **Accepted**
Date: 2026-05-17

## Context

Every join point in the kernel where a validation, schema check, or external call could fail must have a default behavior. Two options:

1. **Fail-open**: on uncertainty or error, allow the operation through with a warning.
2. **Fail-closed**: on uncertainty or error, block the operation and surface a structured error.

The default behavior shapes the entire safety posture of the system. In finance, the cost asymmetry is severe: a missed false-positive (allowing a bad output through) potentially produces SAR-fileable hallucinations, audit findings, and regulatory penalties. A false-negative (blocking a good output) produces a retry or an escalation to a human — both bounded costs.

## Decision

Every join point in the kernel defaults to fail-closed. The list, exhaustively:

- Tool input schema violation → reject before tool invocation.
- Tool output schema violation → reject the result; agent receives an error.
- Token verification failure (mismatch between answer numeric and minted token) → reject `submit_answer`.
- Token-not-found referenced from a claim → reject.
- Validator implementation panic or timeout → treated as failure.
- Tool panic or timeout → treated as failure; tool marked unhealthy.
- Cross-run or cross-tenant token reference → reject.
- Captured-read replay mismatch → reject; audit log entry surfaces the discrepancy.
- Audit chain hash mismatch → halt operations for the tenant until investigated.
- Pre-LLM guard hit at severity L3 (Block) → halt the run.

No configuration option silently relaxes any of these. Operators may configure *more* strict behavior (e.g., escalating L2 quarantine to L3 block in a specific deployment) but not less.

## Rationale

**[Faramesh](../references.md#faramesh) makes this explicit.** Among its six architectural guarantees, fail-closed behavior is named directly. The paper articulates that fail-open enforcement is not enforcement — it is best-effort with a structured error surface, which auditors and regulators do not accept as a control.

**Cost asymmetry favors fail-closed.** A blocked release that retries successfully is invisible to the customer. A wrong answer released is a regulatory event. The right default optimizes for the asymmetric cost.

**Operability follows from the design.** Fail-closed produces consistent error surfaces; the operator sees the same kind of error every time a class of failure occurs. Fail-open hides failures and creates inconsistent operator experiences ("sometimes the agent silently agrees, sometimes it surfaces the issue").

**Auditor framing.** In the regulatory conversation, the question "what happens when your system isn't sure?" has one good answer and many bad ones. Fail-closed is the only one regulators accept without follow-up.

## Consequences

**Positive:**
- Customer trust: customers can reason about behavior under uncertainty without reading code.
- Audit story: every error surface is structured, logged, and replayable.
- Skill authors get clear contracts; ambiguity is impossible by construction.

**Negative:**
- More retries in production than a fail-open system. Mitigated by retry-friendly error codes and the agent's ability to act on structured errors.
- Operators encounter more "blocked" states. We must invest in error surfaces that explain *why* and offer remediation paths.
- The first weeks of any new skill pack will produce more HITL escalations than a fail-open system. This is correct — it surfaces what would otherwise be silent failures.

**Out of scope:**
- "Soft" fail modes for non-critical validators are not allowed. A validator either passes, fails, or is removed from the pipeline. There is no "pass-with-warning."

## Alternatives considered

**Fail-open by default, opt-in fail-closed per validator.** Considered briefly. Rejected: customers who need fail-closed for compliance would have to configure it everywhere, and forgetting one validator is a serious bug. The default must be the safer mode.

**Per-skill default policy.** Each skill pack declares its default. Rejected: the kernel's default must be uniform and predictable across packs. Skills may add stricter validators but not relax the kernel default.

**Configurable fail-closed escalation.** Some join points (e.g., a non-critical validator timeout) could in principle be fail-open without compromising the answer's correctness. Rejected: complexity for marginal performance benefit. We choose simplicity.

## References

- [Faramesh](../references.md#faramesh) — fail-closed as architectural guarantee
- [Atlan analysis](../references.md#atlan-hallucination) — high-risk financial applications require mandatory human validation; fail-closed routes to HITL
- See [`subsystems/guards.md`](../subsystems/guards.md) for the pipeline that implements fail-closed
