# ADR-0005: Determinism and faithfulness as independent metrics

Status: **Accepted**
Date: 2026-05-17

## Context

The dominant way LLM-system reliability is reported in industry is as a single "accuracy" number — typically a benchmark score. This conflates several distinct properties:

- Does the system reach the same conclusion when re-run? (decision determinism)
- Does it take the same path? (trajectory determinism)
- Does its output trace to evidence? (faithfulness)
- Is the conclusion correct against ground truth? (accuracy)

Treating these as one number obscures the underlying behavior. [IBM DFAH's 2026 paper](../references.md#dfah) provides the central finding: in tool-using financial agents, decision determinism and accuracy are **not correlated** (r = -0.11). A highly deterministic system can be reliably wrong; a highly accurate system can be inconsistent. Reporting one number loses information critical for compliance and operational confidence.

## Decision

Provenant reports four metrics, separately, for every skill pack at every release:

1. **Trajectory determinism.** Across N replays, the fraction of runs producing identical tool-call sequences.
2. **Decision determinism.** Across N replays, the fraction producing identical structured outputs.
3. **Faithfulness.** The fraction of claims that trace correctly to evidence (tool calls and tokens).
4. **Accuracy.** Correctness against ground truth (where available, from the case bank).

Skill packs declare per-metric thresholds. CI blocks releases that regress below threshold on any metric. The audit pack for a run includes all four.

## Rationale

**[DFAH's r = -0.11](../references.md#dfah) finding makes the case directly.** Reporting a single metric obscures behavior; the right design surfaces all four.

**Each metric speaks to a different stakeholder.**

- Trajectory determinism speaks to the SRE: "will I see identical traces in production?"
- Decision determinism speaks to the auditor: "will the system answer consistently?"
- Faithfulness speaks to the compliance officer: "is every number traceable?"
- Accuracy speaks to the business: "is the system right?"

A single conflated metric satisfies none of them well.

**CI gating per metric is operationally sound.** A skill pack that suddenly drops in trajectory determinism while holding decision determinism is a signal to investigate (likely a new code path introducing non-determinism); the system shouldn't keep shipping while this investigates. Same with accuracy regression while determinism holds.

**Faithfulness as a release-blocker.** Of the four, faithfulness is the strictest: it must be 1.00. Any unfaithful claim is by definition a hallucination per Provenant's contract — release-blocking, always.

## Consequences

**Positive:**
- Customers and auditors can reason about each property separately.
- CI catches regressions that a single-number gate would miss.
- Skill authors know which metric a change improves or regresses.
- The framing is research-backed; defensible in customer conversations.

**Negative:**
- Four metrics is more to report and explain than one. Mitigated by clear documentation and tooling.
- Some metrics require ground truth (accuracy) which the case bank may not always have. Reported as "n/a" rather than fabricated.
- Replay infrastructure must support N=20 by default for determinism measurement. Higher per-CI-run cost. Acceptable.

**Out of scope:**
- We do not propose a "composite score." Composite scores are exactly what DFAH's finding warns against; they trade away the information that makes the metrics actionable.

## Alternatives considered

**Single accuracy metric (industry default).** Rejected as the load-bearing decision; we offer accuracy alongside three others, but not alone.

**Three metrics (collapsing trajectory and decision determinism).** Considered. Rejected because the two have different mechanisms (path variance vs final-state variance) and different remediations. Skill authors need both signals.

**Five+ metrics (e.g., adding calibration, latency stability, cost stability).** Considered. Rejected for v0 simplicity; the first four are the load-bearing four. Operational metrics live in observability dashboards, not the release gate.

## References

- [DFAH](../references.md#dfah) — the four-metric framework's empirical justification; r = -0.11 between determinism and accuracy
- [FAITH](../references.md#faith) — independent benchmark for faithfulness specifically
- See [`subsystems/eval.md`](../subsystems/eval.md) for the metric implementations
