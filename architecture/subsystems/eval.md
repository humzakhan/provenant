# Subsystem: Eval Harness

Status: **v0 / load-bearing**

> The case bank is the learning loop. The eval harness measures four metrics independently. Together they replace fine-tuning as Provenant's "self-improvement" mechanism — auditable, regression-tested, regulator-friendly.

## Depends on
- [`subsystems/audit.md`](audit.md) — replay reads from the audit chain
- [`subsystems/kernel.md`](kernel.md) — token verification on replay
- [`subsystems/storage.md`](storage.md) — case persistence
- [`subsystems/skills.md`](skills.md) — per-skill thresholds

## Depended on by
- [`adapters/cli.md`](../adapters/cli.md) — `cases`, `replay`, `eval` subcommands
- CI pipeline — regression gating on PRs

## Responsibilities

1. Capture every run with a human in the loop as a case.
2. Replay captured cases against new prompts, models, tools, or validators.
3. Report four metrics independently: trajectory determinism, decision determinism, faithfulness, accuracy.
4. Gate skill pack releases on regression.
5. Curate the case bank: detect conflicts, mark stale cases, archive resolved ones.
6. Integrate external benchmarks (FAITH, DFAH, multi-agent finance).

## The four metrics

Provenant reports four metrics, separately. The motivation is [IBM DFAH's central finding](../references.md#dfah) that decision determinism and task accuracy are not correlated in tool-using agents (r = -0.11). Treating them as a single metric loses information critical for compliance.

```go
package eval

type Metrics struct {
    // TrajectoryDeterminism: across N replays, fraction of runs producing
    // identical tool-call sequences (tool names + inputs in order).
    TrajectoryDeterminism float64

    // DecisionDeterminism: across N replays, fraction producing identical
    // final decisions (ignoring narrative variance — the structured output
    // fields must match).
    DecisionDeterminism float64

    // Faithfulness: fraction of claims in the output that trace correctly
    // to evidence in the agent's trajectory (tool calls and tokens).
    Faithfulness float64

    // Accuracy: standard correctness against ground truth (when available).
    Accuracy float64
}
```

### Trajectory determinism

> "If I rerun this case 20 times, do I get the same sequence of tool calls?"

Replay the case N times. For each replay, record the tool-call sequence. Hash each sequence canonically. Count distinct hashes:

```
TrajectoryDeterminism = (count of replays matching the most common trajectory) / N
```

Frontier models with `temperature=0` typically achieve 0.7–0.95 on this metric for narrow tasks. Smaller schema-first models reach near 1.0. [DFAH](../references.md#dfah) is the reference.

### Decision determinism

> "If I rerun this case 20 times, do I get the same structured answer?"

Same replay; compare the structured output fields, ignoring free-text narrative variance.

Decision determinism is typically *higher* than trajectory determinism: the model takes different paths to the same answer. But it can also be *lower* in adversarial cases. Both must be measured. *(DFAH found r = -0.11 between this and accuracy — the headline finding.)*

### Faithfulness

> "Does every claim in the answer trace to evidence?"

After a single run completes, walk every claim in the `submit_answer` payload. For each:

- Verify the cited token IDs exist.
- Verify the token values match the claim's numeric content.
- Verify the supporting tool calls exist in the trajectory.

```
Faithfulness = (claims that pass all three checks) / (total claims)
```

A faithfulness of 1.0 is required for any release. A claim that fails faithfulness is by definition a hallucination per Provenant's contract; release-blocking.

### Accuracy

> "Is the answer correct against ground truth?"

Standard task correctness. Requires ground truth in the case (the human-corrected final answer).

Accuracy is the metric most likely to vary across model versions; it is measured at the field level (per output field) and aggregated.

## Case bank

### Capture

Every run captures a case if any of the following:

- HITL review occurred (regardless of outcome).
- A skill pack's `eval.always_capture` flag is set.
- The run completed without HITL but was tagged for inclusion (e.g., a regression test seed).

Case content:

```json
{
  "id": "case_01HXYZ...",
  "run_id": "run_01HXYZ...",
  "skill_id": "recon",
  "skill_version": "0.3.0",
  "model": "claude-opus-4-7",
  "input": {...},
  "captured_reads_ref": ["..."],
  "trajectory": [
    {"tool": "extract.bank_statement", "input_hash": "...", "tokens_minted": [...]},
    {"tool": "ledger.get_transactions", ...},
    ...
  ],
  "draft_answer": {...},
  "final_answer": {...},
  "human_correction": {
    "reviewer": "humza@example.com",
    "changes": [
      {"field": "unmatched_residual", "before": "237.42", "after": "237.42", "kind": "approved"}
    ],
    "notes": "Confirmed against statement page 3."
  },
  "captured_at": "2026-05-16T14:32:11Z",
  "status": "active",
  "labels": ["v0-bootstrap", "bank-fees"]
}
```

### Storage

```sql
CREATE TABLE cases (
    id                TEXT PRIMARY KEY,
    run_id            TEXT NOT NULL REFERENCES runs(id),
    tenant_id         TEXT NOT NULL,
    skill_id          TEXT NOT NULL,
    skill_version     TEXT NOT NULL,
    input             JSONB NOT NULL,
    draft_answer      JSONB NOT NULL,
    final_answer      JSONB NOT NULL,
    human_correction  JSONB,
    captured_at       TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL,           -- active | stale | conflicted | archived
    labels            TEXT[] NOT NULL DEFAULT '{}'
);

CREATE TABLE case_metrics (
    case_id           TEXT NOT NULL REFERENCES cases(id),
    skill_version     TEXT NOT NULL,
    model             TEXT NOT NULL,
    measured_at       TIMESTAMPTZ NOT NULL,
    trajectory_det    NUMERIC(5,4),
    decision_det      NUMERIC(5,4),
    faithfulness      NUMERIC(5,4),
    accuracy          NUMERIC(5,4),
    PRIMARY KEY (case_id, skill_version, model, measured_at)
);
```

### Replay

```bash
$ provenant replay case_01HXYZ --n 20 --skill recon@0.4.0
```

The replay machinery:

1. Loads the case.
2. Spins up N parallel runs against the new skill version (or new model, or new validator config).
3. Each run uses the captured reads from the original case for `CapturedDeterministic` tools — bit-identical replay of external state.
4. Records all four metrics.
5. Writes a `case_metrics` row per run.

### Curation

The case bank is a living dataset, not append-only. The curator (a background worker) flags:

- **Conflicting**: two cases with the same input but inconsistent corrections (human reviewers disagreed, or policy changed). The labels surface for review.
- **Stale**: a case's referenced policy version, chart of accounts, or skill version has changed enough that the original correction may no longer apply. Stale cases are excluded from CI gates until reviewed.
- **Archived**: resolved cases that should not influence future evaluation.

## CI gating

Every PR that touches a skill pack or a tool runs the case bank for affected skills:

```bash
$ provenant eval --skill recon --against HEAD
Running 247 active cases for recon@0.4.0-rc.1 with claude-opus-4-7...
  Trajectory determinism: 0.972 (threshold 0.85) ✓
  Decision determinism:   0.998 (threshold 0.98) ✓
  Faithfulness:           1.000 (threshold 1.00) ✓
  Accuracy:               0.961 (threshold 0.95) ✓
PASS
```

Threshold YAML (in the skill pack at `eval/thresholds.yaml`):

```yaml
trajectory_determinism: 0.85
decision_determinism: 0.98
faithfulness: 1.00
accuracy: 0.95

replay_n: 20
release_blocks_on_regression: true
```

A regression below threshold on *any* metric blocks the release.

## External benchmark integration

Three external benchmarks are integrated as reference:

### FAITH

[FAITH](../references.md#faith) provides datasets and methodology for evaluating tabular hallucinations in finance. Provenant runs against FAITH in CI and publishes the result. Used as a third-party credibility signal.

### DFAH

[DFAH's open-source harness](../references.md#dfah) includes three benchmarks: compliance triage (50 cases), portfolio constraints (50 cases), DataOps exceptions (50 cases). Provenant adopts these as part of the regression set for skill packs touching those domains.

### Multi-agent finance document benchmark

[Kulkarni & Kulkarni 2026](../references.md#multi-agent-finance) evaluated four orchestration architectures on 10,000 SEC filings. Provenant reproduces a subset to compare hierarchical-with-reflexive-fallback (our default) against pure sequential and pure reflexive baselines on real data.

## Workflow: how a skill pack gets better

Without retraining, here is how a skill pack improves over time:

```
1. Production runs flow through HITL.
2. Human corrections are captured into cases.
3. Cases enter the active bank (after a quarantine period for review).
4. The curator detects patterns:
   - Recurring failure mode → new validator proposal
   - Recurring correction → updated few-shot example proposal
5. Pack maintainer evaluates proposals; merges promising ones.
6. New skill version runs against the full case bank in CI.
7. If all four metrics pass thresholds, release ships.
8. Behavior change is fully auditable: every contributing case is listed.
```

This is the "case-based improvement" referenced in [`thesis.md`](../../thesis.md). It is the answer to the regulatory question "how does your AI learn?" — *by adding regression tests, not by retraining*.

## Tests

- Property: identical inputs to a replay produce identical trajectory determinism scores.
- Integration: case bank curator correctly flags conflicting and stale cases.
- Integration: CI gate blocks a deliberately regressed skill version.
- Performance: replay throughput sufficient for 1000 cases × 20 replays in <10 minutes on a single machine for v0 scope.

## Open questions

1. **Case bank privacy.** Customer-specific cases may include PII even after redaction. Proposal: store the case bank inside the tenant's own database; never aggregate across tenants without explicit opt-in.
2. **Federated case sharing.** Could anonymized cases improve everyone's skill packs? Strategic decision — defer to commercial offering.
3. **Synthetic case generation.** Should we generate adversarial synthetic cases (e.g., LLM-generated mutations) to expand coverage? Defer to v0.5.

## References

- [DFAH](../references.md#dfah) — four-metric framework; determinism and accuracy uncorrelated
- [FAITH](../references.md#faith) — tabular hallucination benchmark
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — 10k SEC filings reference
- [BoN-MAV](../references.md#bon-mav) — basis for the optional verifier mode tracked in eval
- See [ADR-0005](../adrs/0005-determinism-faithfulness-independent.md) for the metric independence decision
