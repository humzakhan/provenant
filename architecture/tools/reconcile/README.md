# Tool Category: Reconcile

Status: **v0 / pure Go**

> Set matching with tolerances. The reconciliation primitives that turn two lists of transactions (bank vs. GL, AP register vs. open invoices, broker statement vs. internal positions) into matched pairs and unmatched residuals. Pure-Go, deterministic, no backends.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)

## Depended on by
- `recon` skill pack
- `ap_three_way_match` skill pack (future)
- Any skill that needs structural matching

## Purpose

Reconciliation is the most common finance workflow ([Anthropic's first finance agent template](../../references.md#anthropic-finance-agents) is a GL reconciler) and the most tedious. The agent does *not* need to invent a matching algorithm — it needs to call a primitive that takes two lists and returns matched pairs, with deterministic tolerance behavior.

By keeping the matching logic in a tool (not in the agent's reasoning), we get:

1. **Determinism**: the same two inputs produce the same matches, always. The kernel's [trajectory determinism metric](../../subsystems/eval.md) measures this.
2. **Speed**: matching 1000×1000 is a milliseconds operation in Go; doing it through LLM reasoning is unaffordably slow.
3. **Auditability**: every matched pair includes its algorithm and tolerance, exportable in audit packs.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `reconcile.match` | Match two lists of items with configurable tolerance and algorithm | PureDeterministic |

One tool, three algorithm modes. Skill packs choose the algorithm and tolerances via the input.

## Schema

### `reconcile.match`

Input:
```json
{
  "side_a": {
    "label": "bank_statement",
    "items": [
      {"id": "bnk_1", "amount": "1240.00", "currency": "USD", "date": "2026-05-02", "description": "DEPOSIT - ACME"},
      {"id": "bnk_2", "amount": "-89.00",  "currency": "USD", "date": "2026-05-03", "description": "WIRE FEE"},
      ...
    ]
  },
  "side_b": {
    "label": "ledger",
    "items": [
      {"id": "led_1", "amount": "1240.00", "currency": "USD", "date": "2026-05-02", "description": "ACME Corp invoice 1023"},
      {"id": "led_2", "amount": "-89.00",  "currency": "USD", "date": "2026-05-04", "description": "Bank fee"},
      ...
    ]
  },
  "algorithm": "greedy_with_fuzzy",   // "greedy_exact" | "greedy_with_fuzzy" | "hungarian"
  "tolerance": {
    "amount_absolute": "0.01",         // dollars
    "amount_relative": "0.001",        // 0.1%; whichever is larger applies
    "date_window_days": 3,
    "description_fuzzy_min": 0.6       // optional, only for fuzzy modes
  }
}
```

Output:
```json
{
  "matched": [
    {
      "side_a_id": "bnk_1",
      "side_b_id": "led_1",
      "amount_diff": "0.00",
      "currency": "USD",
      "date_diff_days": 0,
      "description_similarity": 0.78,
      "confidence": 1.00,
      "match_reason": ["amount_exact", "date_exact", "description_fuzzy"]
    },
    {
      "side_a_id": "bnk_2",
      "side_b_id": "led_2",
      "amount_diff": "0.00",
      "currency": "USD",
      "date_diff_days": 1,
      "description_similarity": 0.82,
      "confidence": 0.95,
      "match_reason": ["amount_exact", "date_within_window", "description_fuzzy"]
    }
  ],
  "unmatched_a": [
    {"id": "bnk_7", "amount": "237.42", ...}
  ],
  "unmatched_b": [
    {"id": "led_12", "amount": "237.41", ...}
  ],
  "stats": {
    "total_a": 248,
    "total_b": 252,
    "matched_count": 233,
    "unmatched_a_count": 15,
    "unmatched_b_count": 19,
    "algorithm": "greedy_with_fuzzy"
  }
}
```

Mintable paths:

- `$.matched[*].amount_diff` (currency from sibling)
- `$.unmatched_a[*].amount`
- `$.unmatched_b[*].amount`
- `$.stats.matched_count`, `$.stats.unmatched_a_count`, `$.stats.unmatched_b_count` (unit: count)

## Algorithms

### `greedy_exact`

For each item in `side_a`, find the first unmatched item in `side_b` with exact amount equality (within tolerance) and date within window. O(n × m) but with early termination; sufficient for n, m < 10,000.

Use when: amounts are reliably exact (no FX rounding, no partial matches). Fastest mode.

### `greedy_with_fuzzy`

Like `greedy_exact` but with description-fuzzy as a tiebreaker when multiple amount-and-date candidates exist. Default for `recon` skill.

Use when: descriptions on both sides are informative. Slightly slower than exact but produces better unmatched residuals.

### `hungarian`

Optimal bipartite matching via the Hungarian algorithm with a cost function over (amount difference, date difference, description similarity). O((n+m)^3); use only for small inputs (n+m < 500) or when greedy produces unacceptable residuals.

Use when: input is small and you need the global optimum (high-stakes recon, partial matches at risk of mis-pairing).

## Match confidence

The `confidence` field per match reflects:

- 1.00: exact amount, exact date, no fuzzy needed.
- 0.95-0.99: exact amount, date within window, possibly fuzzy description.
- 0.85-0.94: amount within tolerance, date within window, description fuzzy with high similarity.
- 0.70-0.84: amount within tolerance, date within window, weak description match.
- <0.70: not reported (would not pass tolerance gate).

Confidence informs HITL routing alongside extraction confidence.

## Token semantics

- `amount_diff` per matched pair mints a token. Used by skill packs to compute variance summaries.
- Counts (`matched_count`, `unmatched_*_count`) mint tokens with `unit=count`.

## Determinism and replay

`PureDeterministic`: same inputs always produce the same matched/unmatched output. No randomness in any algorithm. Stable ordering: `matched` is sorted by `side_a_id`; `unmatched_a` and `unmatched_b` preserve input order.

Tie-breaking in `greedy_with_fuzzy`: lexicographic on description after fuzzy score, then on `side_b_id` for absolute determinism.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `reconcile.currency_mismatch` | An item on either side has a different currency than expected | Skill aborts or routes to multi-currency mode |
| `reconcile.empty_input` | One side is empty | Skill decides; technically valid but usually a configuration issue |
| `reconcile.tolerance_invalid` | Negative or contradictory tolerance config | Schema-level error; rejected before execution |
| `reconcile.timeout` | Hungarian on too-large input exceeds budget | Skill should switch to greedy fallback |

## Performance budget

| Input size | Algorithm | p50 | p95 |
|---|---|---|---|
| 100 × 100 | any | 5ms | 20ms |
| 1000 × 1000 | greedy_exact | 50ms | 200ms |
| 1000 × 1000 | greedy_with_fuzzy | 200ms | 800ms |
| 1000 × 1000 | hungarian | (not recommended; >1s) | |
| 10000 × 10000 | greedy_with_fuzzy | 2s | 8s |

Per-call timeout: 30 seconds.

## Backends

None. This is a pure-Go implementation under `internal/tools/reconcile/`.

## Tests

- Unit: each algorithm on canonical fixtures with known correct outputs.
- Property: idempotence (running the same match twice yields identical results).
- Property: tolerance edge cases (matches exactly at the tolerance boundary handled consistently).
- Stress: 10000 × 10000 input completes within budget.
- Determinism: identical results across 1000 runs of the same input.

## Open questions

1. **Partial matching (one-to-many).** When a single bank deposit covers multiple invoices, current tools find an unmatched residual. Proposal: a `partial_matching` config flag enabling N:M matching with a sum-equality constraint. Defer to v0.5; opens algorithmic complexity worth a dedicated ADR.
2. **Description normalization.** Should descriptions be normalized (uppercase, strip punctuation, expand abbreviations) before fuzzy match? Proposal: yes, with a normalizer pipeline configurable per skill. Default normalizer ships with v0.

## References

- [Anthropic finance agents](../../references.md#anthropic-finance-agents) — GL reconciler is the canonical example
- Standard Hungarian algorithm: Kuhn 1955; we use the O(n^3) variant
