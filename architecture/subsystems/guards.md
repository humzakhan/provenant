# Subsystem: Guard Pipeline

Status: **v0 / load-bearing**

> The guard pipeline runs deterministic checks around the model. Pre-LLM guards screen inputs (user prompts, retrieved documents, tool outputs) before they reach model context. Post-LLM validators gate the agent's `submit_answer` output. Together they implement the "no ungrounded numbers, ever" contract.

## Depends on
- [`subsystems/kernel.md`](kernel.md) — token verification
- [`subsystems/tools.md`](tools.md) — schema definitions
- [`subsystems/storage.md`](storage.md) — guard run persistence
- [`subsystems/audit.md`](audit.md) — every guard outcome is logged

## Depended on by
- [`subsystems/harness.md`](harness.md) — agent loop calls guards at fixed points
- All adapters — the `submit_answer` enforcement lives here

## Responsibilities

1. Intercept every input flowing toward the LLM (pre-LLM guards).
2. Intercept every output the agent attempts to emit (`submit_answer`).
3. Run validators in a configurable pipeline order.
4. Return structured errors that the agent can act on, or fail-close.
5. Optionally run [BoN-MAV](../references.md#bon-mav) mode for high-stakes skills.

## Architecture

```
User prompt ──┐
              │
Retrieved   ──┼──▶ ┌─────────────────┐ ──▶ Sanitized context ──▶ LLM
documents     │    │  Pre-LLM Guards │
              │    └─────────────────┘
Tool outputs ─┘            │
                           ▼
                    audit + persist
                    (guard_runs table)

LLM ──▶ tool calls (looped) ──▶ submit_answer
                                      │
                                      ▼
                          ┌─────────────────────┐
                          │  Token grounding    │  ◀── kernel.VerifyToken
                          ├─────────────────────┤
                          │  Post-LLM           │
                          │  validators         │
                          │  (pipeline)         │
                          ├─────────────────────┤
                          │  Optional BoN-MAV   │
                          └─────────────────────┘
                                      │
                                      ▼
                               released or
                               returned with errors
```

## Pre-LLM input guards

### Run timing

Pre-LLM guards run on:

1. The initial user prompt and skill-pack system prompt.
2. Every document or retrieved chunk added to context (RAG hits, uploaded files, OCR outputs).
3. Every tool response on its way back to the agent.

The harness invokes them before any input touches the model API. See [`subsystems/harness.md`](harness.md) for the call sites.

### Categories

| Guard | Purpose | Default action on hit |
|---|---|---|
| `pii.redact` | Strip or tokenize PII (SSN, account numbers, etc.) before model exposure | Redact in-place; original kept in audit |
| `injection.scan` | Detect instruction-shaped content in retrieved documents | Quarantine the passage; emit advisory |
| `injection.heuristic` | Heuristic check for "ignore previous", role-flip, role-claim patterns | Quarantine |
| `source.authenticate` | Verify document signature or trust source | Block the run if untrusted source is required-authoritative |
| `length.cap` | Cap individual context items at a configurable byte limit | Truncate at safe boundary with marker |
| Custom per-skill | Pack authors register additional guards | Pack-defined |

### Severity tiers

A guard hit fires at one of three severities, declared by the guard implementation:

- **L1 — Advisory.** Add a flag to the audit log; allow the content through. Used for "looks slightly suspicious" signals.
- **L2 — Quarantine.** Strip or replace the offending passage; allow the surrounding content. Default for prompt-injection patterns.
- **L3 — Block.** Halt the run with a structured error to the caller. Used for authentication failures on required-authoritative sources.

Severity tier is configurable per skill in `guards.yaml` (override the guard's default to be stricter or looser).

### Why pre-LLM matters

Tool outputs are model inputs. A vendor invoice PDF, an OFAC list entry, a knowledge-base snippet — all flow into the agent's context. An attacker who controls any of those sources can attempt prompt injection. Output validation alone is too late: the agent's reasoning has already been steered.

This pattern follows [Arthur AI's pre/post-LLM split](../references.md#arthur-guardrails). The split is in the architecture, not the prompt, so it cannot be bypassed by the model deciding to ignore it.

## Post-LLM validators (the `submit_answer` pipeline)

### The `submit_answer` contract

`submit_answer` is the single exit. See [ADR-0002](../adrs/0002-submit-answer-as-single-exit.md). Every adapter installs it; no other path returns output to the user.

**Input shape:**

```json
{
  "answer": "<freeform text or structured object>",
  "claims": [
    { "text": "Total reconciled: $1,247,322.18", "tokens": ["tk_01HXYZ..."] },
    { "text": "Unmatched residual: $237.42",     "tokens": ["tk_01HABC..."] }
  ],
  "skill_id": "recon",
  "skill_version": "0.3.0"
}
```

**Algorithm:**

```
function submit_answer(payload):
    # Step 1: parse numerics out of the answer
    candidates = extract_numerics(payload.answer)   # regex + optional LLM-assist fallback

    # Step 2: ground each candidate against claims
    for c in candidates:
        claim = find_claim_covering(c, payload.claims)
        if claim is None:
            return Error("ungrounded_number", value=c, path=c.location)

        verified = false
        for token_id in claim.tokens:
            err = kernel.VerifyToken(run_id, token_id, c)
            if err is None:
                verified = true
                break
        if not verified:
            return Error("token_mismatch", value=c, claim=claim.text)

    # Step 3: post-LLM validator pipeline (deterministic checks)
    errors = []
    for v in skill.validators:
        res = v.Validate(payload)
        if not res.passed:
            errors.extend(res.errors)
        if skill.fail_fast and errors:
            break
    if errors:
        return Errors(errors)

    # Step 4: optional BoN-MAV (if skill enables it)
    if skill.bon_mav.enabled:
        return run_bon_mav(payload, skill)

    # Step 5: persist + release
    persist_final_answer(payload)
    audit_log.append(EntryKindAnswerReleased, payload)
    case_bank.capture(payload)
    return OK(payload)
```

The algorithm formalizes [Draft-Conditioned Constrained Decoding (DCCD)](../references.md#dccd) for finance: the agent drafts freely, then must pass an enforcement gate. The novelty is the per-value grounding step (step 2) tied to provenance tokens, which is stricter than DCCD's structural enforcement.

### Numeric extraction (step 1)

The default extractor is a regex over the answer text recognizing:

- Currency amounts (`$1,247,322.18`, `€1.234.567,89`, `¥1,200`)
- Bare decimals (`0.78`, `47322.18`)
- Percentages (`12.5%`, `47 bps`)
- Integer counts in declared contexts

The skill pack can register additional extractors (e.g., for structured output, the extractor walks JSON fields rather than free text).

**Hybrid fallback.** When the regex misses (e.g., the answer is structured JSON), an LLM-assist fallback parses the answer with an extraction prompt and returns candidates with locations. Even with LLM-assist, the regex remains the *gate*: an LLM-assist candidate without a matching token still fails verification. Per [DFAH](../references.md#dfah), schema-first parsing is preferred for compliance-critical paths; the LLM fallback is for narrative-heavy outputs.

### Validator interface

```go
package guards

type Validator interface {
    Name() string
    Version() string
    Validate(ctx context.Context, payload Payload) Result
}

type Payload struct {
    RunID     string
    SkillID   string
    Answer    json.RawMessage
    Claims    []Claim
    Tokens    []provenance.Token  // all tokens minted in the run
    ToolCalls []ToolCall
    Context   map[string]any
}

type Result struct {
    Passed  bool
    Errors  []ValidationError
    Tracing TraceInfo
}

type ValidationError struct {
    Code    string      // machine-readable
    Message string      // human-readable
    Path    string      // location in the answer or claims
    Tokens  []string    // related token IDs
    Hint    string      // suggested remediation
}
```

### Built-in validators (v0)

| Validator | What it checks |
|---|---|
| `schema.output` | Answer matches the skill pack's declared output schema |
| `arith.crossfoot` | Subtotals sum to totals within tolerance |
| `arith.tie` | Stated totals tie to source tokens (e.g., bank ending balance = extracted statement total) |
| `arith.sign` | Sign conventions: refunds negative, charges positive, debit/credit consistency |
| `arith.balance` | Debits equal credits (where applicable) |
| `policy.coa_membership` | Every GL code referenced is in the loaded chart of accounts |
| `policy.citation_exists` | Every cited policy section ID resolves to a real anchor |
| `confidence.extraction_min` | No extraction with confidence below threshold released without HITL |
| `currency.consistent` | No silent currency mixing |

Each validator implements the interface above. Validators are pure functions of their input — easy to unit-test, easy to replay against the case bank.

### Pipeline execution

Validators run in the order declared by the skill pack's `guards.yaml`. Two modes:

- **Collect-all** (default): every validator runs; errors are aggregated and returned together so the agent sees the full list.
- **Fail-fast**: pipeline halts on first failure. Used for latency-sensitive paths.

Failure does not end the run. The agent receives the structured error list and chooses how to respond:

1. Fix the issue (recalculate, re-extract, re-classify) and resubmit.
2. Escalate to a human via the HITL layer.
3. Abort with an explicit "I cannot complete this" answer (itself a valid output that bypasses the numeric checks because it contains no numeric claims).

### Optional: BoN-MAV mode

For high-stakes skills, the validator pipeline supports an opt-in [Best-of-N Multi-Agent Verification mode](../references.md#bon-mav). Configured in the skill manifest:

```yaml
bon_mav:
  enabled: true
  n: 5                          # candidate count
  verifiers:                    # subset of validators used for selection scoring
    - schema.output
    - arith.crossfoot
    - arith.tie
    - policy.coa_membership
  tie_breaker: token_count_asc  # prefer fewer tokens cited (less surface area)
```

When enabled, the harness re-prompts the agent for N candidate `submit_answer` calls in parallel. Each candidate runs the full validator pipeline. The kernel selects the candidate with the cleanest result (zero errors preferred; ties broken by token count and answer length).

Trade-off: N× cost for measurable accuracy gain. Off by default. Recommended for SAR drafting, large-value journal entries, regulatory filings.

## Storage

```sql
CREATE TABLE guard_runs (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    stage       TEXT NOT NULL,                 -- 'pre_llm' | 'post_llm'
    guard       TEXT NOT NULL,                 -- canonical guard name
    version     TEXT NOT NULL,
    passed      BOOLEAN NOT NULL,
    actions     JSONB,                         -- redactions, quarantines, etc.
    errors      JSONB,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_guard_runs_run ON guard_runs(run_id);
```

Every guard execution writes a row. The audit log references guard run IDs.

## Failure handling and fail-closed

The pipeline is fail-closed at every join point:

- A guard implementation that panics is treated as a failure (action = block).
- A validator that takes longer than its declared timeout is treated as a failure.
- A token grounding mismatch is a failure that cannot be overridden by configuration.
- A skill pack with no declared validators is rejected at load time — there is no "validators optional" mode.

This follows [Faramesh's fail-closed guarantee](../references.md#faramesh). See [ADR-0003](../adrs/0003-fail-closed-default.md) for the rationale.

## Tests

- Unit tests per built-in validator.
- Integration tests run the full pipeline against fixtures: a recon report with known errors → exact set of validator errors returned.
- Property tests: injected ungrounded numbers in arbitrary answer strings → grounding step always rejects.
- Adversarial tests (negative testing): see [`example-projects.md`](../../example-projects.md) for the six stress tests.

## Open questions

1. **Custom claim extractor APIs.** Should skill packs be able to register their own numeric extractors? Proposal: yes, but they must implement a "regex first, LLM fallback" contract so the gate stays deterministic.
2. **Validator dependencies.** Some validators depend on others having run (e.g., `arith.tie` is only meaningful if `arith.crossfoot` passes). Proposal: explicit `depends_on` field in the validator manifest; the pipeline scheduler honors it.
3. **BoN-MAV vote weighting.** Currently we select the cleanest validator result. Should we weight verifiers (e.g., schema=10, policy=8, confidence=3)? Defer until we have production data.

## References

- [Arthur AI guardrails](../references.md#arthur-guardrails) — pre/post-LLM split
- [DCCD](../references.md#dccd) — draft-then-enforce
- [BoN-MAV](../references.md#bon-mav) — multi-verifier selection
- [DFAH](../references.md#dfah) — schema-first determinism findings
- [Faramesh](../references.md#faramesh) — fail-closed enforcement
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — reflexive verifier loop F1
