# Tool Category: Match

Status: **v0**

> Typed fuzzy comparison between two token-bearing records (or two sets). Produces similarity-scored, confidence-bounded `MatchProposal` objects. The proposals themselves are provenanced — each similarity and confidence value is minted as a kernel-controlled token so a downstream `dedupe` decision (or a reviewer disposition) is reconstructable on the audit chain like any other numeric claim.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)
- [`subsystems/guards.md`](../../subsystems/guards.md) — the `match.*` validators gate proposals before they are consumed by downstream tools or the agent
- A backend that produces a similarity signal (vector, lexical, edit-distance). Operators pick the backend; provenant does not bundle an embedding model.

## Depended on by
- [`architecture/tools/dedupe/README.md`](../dedupe/README.md) — `dedupe` is implemented on top of `match`
- Entity-resolution skill packs (vendor de-duplication, customer master harmonization)
- Reconciliation skill variants that fall back from structural matching to fuzzy matching on residuals

## Purpose

Real financial inputs are messy. The same vendor appears as `ACME CORP`, `Acme Corporation`, `acme corp.`, `ACME CORP INC`. The same wire transfer arrives on the bank statement and on the GL with subtly different descriptions. The same counterparty in a sanctions feed is spelled three ways across two list versions. A reconciliation tool that only does exact set matching (see [`tools/reconcile/README.md`](../reconcile/README.md)) leaves these as unmatched residuals; an agent that resolves them by reasoning in natural language produces ungrounded claims about who-is-who.

`match` is the typed primitive the agent (or a downstream tool such as `dedupe`) calls to ask "are these two records the same thing?" with a confidence bound. The category is structured so that:

1. Inputs are **token-bearing**: every field on the left and right record either is a kernel-minted token (the normal case after an extract/ledger call) or is a raw value being introduced into the token graph as part of this `match` call (rare, but supported for skills that ingest a list directly from a fixture).
2. Outputs are **provenanced**: similarity and confidence are `NUMERIC(38,9)` values that are minted as tokens. A skill that releases an answer claiming "vendor A and vendor B are the same with confidence 0.97" can ground that 0.97 to a token like any other numeric claim, satisfying the token-grounding contract at `submit_answer`.
3. Backends are **pluggable**: the category is what `match` does (fuzzy comparison with confidence); how is the backend's concern. Provenant does not bundle an embedding model — the same rationale as [`tools/policy/`](../policy/README.md) (the operator picks the backend and the model).

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `match.pair` | Compare two records; return zero or one `MatchProposal` describing the (left, right) pair | CapturedDeterministic |
| `match.candidates` | Compare a left record against a set of right records; return `top_k` proposals above `similarity_min` | CapturedDeterministic |
| `match.cross_set` | Compare two sets; return all proposed pairs above `similarity_min` (used internally by `dedupe`) | CapturedDeterministic |

## Schemas

### `match.pair`

Input:
```json
{
  "left": {
    "token_ids": ["tk_01HVENDOR_A_NAME", "tk_01HVENDOR_A_COUNTRY"],
    "fields": {
      "name": {"token_id": "tk_01HVENDOR_A_NAME", "value": "ACME CORP"},
      "country": {"token_id": "tk_01HVENDOR_A_COUNTRY", "value": "US"}
    }
  },
  "right": {
    "token_ids": ["tk_01HVENDOR_B_NAME", "tk_01HVENDOR_B_COUNTRY"],
    "fields": {
      "name": {"token_id": "tk_01HVENDOR_B_NAME", "value": "Acme Corporation"},
      "country": {"token_id": "tk_01HVENDOR_B_COUNTRY", "value": "US"}
    }
  },
  "method": "cosine_embedding",
  "method_config": {
    "field_weights": {"name": 0.8, "country": 0.2}
  }
}
```

Output:
```json
{
  "proposal": {
    "left_token_id": "tk_01HVENDOR_A_NAME",
    "right_token_id": "tk_01HVENDOR_B_NAME",
    "similarity_score": "0.943210000",
    "confidence": "0.912000000",
    "method": "cosine_embedding",
    "backend_version": "pgvector@0.7.4+oai-text-embedding-3-small@2024-01"
  }
}
```

The `proposal` field is null when no proposal is returned (the backend produced no signal, or every candidate fell below the configured `similarity_min`).

Mintable paths:

- `$.proposal.similarity_score` (unit: similarity_unit, scale: `NUMERIC(38,9)`)
- `$.proposal.confidence` (unit: confidence_unit, scale: `NUMERIC(38,9)`)

### `match.candidates`

Input:
```json
{
  "left": { "token_ids": [...], "fields": {...} },
  "right_set": [ { "token_ids": [...], "fields": {...} }, ... ],
  "method": "cosine_embedding",
  "method_config": {...},
  "top_k": 5,
  "similarity_min": "0.700000000"
}
```

Output:
```json
{
  "proposals": [
    {
      "left_token_id": "tk_01HVENDOR_A_NAME",
      "right_token_id": "tk_01HVENDOR_B_NAME",
      "similarity_score": "0.943210000",
      "confidence": "0.912000000",
      "method": "cosine_embedding",
      "backend_version": "pgvector@0.7.4+oai-text-embedding-3-small@2024-01"
    },
    ...
  ]
}
```

Mintable paths: `$.proposals[*].similarity_score`, `$.proposals[*].confidence`.

### `match.cross_set`

Input:
```json
{
  "left_set": [ ... ],
  "right_set": [ ... ],
  "method": "cosine_embedding",
  "method_config": {...},
  "similarity_min": "0.700000000"
}
```

Output: a `proposals` list with every `(left, right)` pair above the threshold, sorted by `(left_token_id, descending similarity_score, right_token_id)` for deterministic ordering.

Mintable paths: `$.proposals[*].similarity_score`, `$.proposals[*].confidence`.

## Token semantics

- Each `MatchProposal` mints **two** tokens: one for `similarity_score`, one for `confidence`. Both are `NUMERIC(38,9)`.
- `source.path` is the JSONPath into the response (`$.proposal.similarity_score`, `$.proposals[3].confidence`).
- The token's `derivation.parents` includes `left_token_id` and `right_token_id` so the audit pack can render "this 0.943 came from comparing token A against token B." `derivation.op` is the `method` string prefixed with `match.` per the dotted `category.op` convention used elsewhere (e.g., `match.cosine_embedding`).
- `backend_version` is recorded on the `tool_call_completed` entry under `mintable_paths_hit` siblings; it is not itself minted as a token but is part of the captured output so replay can verify the same backend version produced the same proposal.
- The match decision is therefore provenanced like any other tool output: the audit pack reconstructs "agent claimed A ≡ B at confidence 0.91" back to the specific tool call and backend version that produced 0.91.

## Determinism and replay

`CapturedDeterministic`. The underlying backend may be non-deterministic on a fresh call (e.g., a hosted embedding API rev-bump), so the kernel captures the backend's raw output (per [`subsystems/tools.md`](../../subsystems/tools.md)) and serves it on replay. This matches the `extract.*` precedent — the *primitive* may drift, the *outcome* is replay-stable.

Captured-read identifier:
```
match:<backend>:<tool>:<sha256_of_input_plus_backend_version>
```

`backend_version` is part of the capture key so a backend upgrade (e.g., switching embedding models) starts a fresh capture lineage; prior captured runs remain replayable against their original `backend_version`.

Stable ordering for `match.candidates` and `match.cross_set` is enforced by the category implementation (not the backend): proposals are sorted by `(left_token_id, descending similarity_score, right_token_id)` after the backend returns. Two backends that produce the same similarity scores in different internal orders produce byte-identical kernel-visible output.

## Confidence

`confidence` is distinct from `similarity_score`:

- **`similarity_score`** is the raw signal from the method (cosine similarity, normalized edit distance, Jaccard coefficient). It is method-specific and is *not* directly comparable across methods.
- **`confidence`** is the backend's calibrated probability that this proposal corresponds to the same real-world entity. The category implementation translates raw similarity into calibrated confidence via a per-backend, per-method calibration map (the same pattern as [`extract.*`](../extract/README.md) confidence calibration). Skill packs gate on `confidence`, not on raw similarity, via the `match.confidence_min` validator.

Calibration is the backend's responsibility; the category contract requires that a confidence score have a documented calibration procedure and a recomputed-quarterly map against the case bank.

## Validators

The category declares three validators that skills configure in `guards.yaml`. The validators are also enumerated in [`subsystems/guards.md`](../../subsystems/guards.md) §"Built-in validators (v0)":

| Validator | What it checks |
|---|---|
| `match.confidence_min` | Rejects any `MatchProposal` whose `confidence` is below the per-skill threshold. Default 0.85; configurable per skill. |
| `match.similarity_min` | Rejects any `MatchProposal` whose `similarity_score` is below the per-skill threshold. Default 0.70; configurable per skill. |
| `match.method_allowed` | Restricts allowed `method` values per skill (e.g., `["cosine_embedding", "jaccard", "levenshtein_normalized"]`). A proposal whose `method` is not in the allow-list is rejected. |

Validators fail closed: a rejected proposal does not surface to downstream tools or the agent, and the rejection is audited as a `guard_run` with `action = block` and the corresponding `error_code` (see "Failure modes" below).

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `match.confidence_below_min` | A proposal's `confidence` is below the skill's `match.confidence_min` threshold | Skill drops the proposal; agent may broaden the search or route to HITL |
| `match.similarity_below_min` | A proposal's `similarity_score` is below `match.similarity_min` | Skill drops the proposal |
| `match.method_not_allowed` | The requested `method` is not in the skill's `match.method_allowed` list | Schema-level rejection; pack should declare the method or omit the call |
| `match.backend_unavailable` | Backend network / index unreachable | Retry per backend policy |
| `match.input_token_unknown` | A `token_id` on `left` or `right` does not resolve to a kernel-minted token for this run | Hard rejection; cannot construct a provenanced proposal without a real token |
| `match.empty_right_set` | `match.candidates` or `match.cross_set` was passed an empty `right_set` | Skill decides; usually a configuration issue |

The first three codes are declared in the canonical `error_code` table in [`subsystems/audit.md`](../../subsystems/audit.md#canonical-error_code-table) under the `match.*` namespace.

## Performance budget

| Tool | Input shape | p50 | p95 |
|---|---|---|---|
| `match.pair` | 2 records | 30ms | 120ms |
| `match.candidates` | left × right_set ≤ 1000 | 200ms | 800ms |
| `match.cross_set` | n × m ≤ 1000 × 1000 | 2s | 8s |

Per-call timeout: 30 seconds. `cross_set` over larger inputs should be batched at the skill level.

## Backends

| Backend | Status | Doc |
|---|---|---|
| pgvector | v0 reference | [backends/pgvector.md](backends/pgvector.md) |
| sqlite-vss | deferred (v0.5) | not yet documented |
| in-process-onnx | deferred (v0.5) | not yet documented |

The pgvector backend re-uses the same operator-configured embedding model story as [`tools/policy/backends/pgvector.md`](../policy/backends/pgvector.md): provenant does not bundle a model; the operator points the backend at one. `sqlite-vss` and `in-process-onnx` are listed as candidate v0.5 backends; their full backend docs are not written in v0.

## Conformance checklist

In addition to the standard tool-conformance items in [`architecture/tools/README.md`](../README.md):

| Check | Owner | Enforced where |
|---|---|---|
| Backend declares a `backend_version` string that is captured on every call | Backend author | CI; replay test refuses a capture missing `backend_version` |
| Backend's calibration map (similarity → confidence) is documented and tested | Backend author | Doc review + calibration test against case bank |
| `similarity_score` and `confidence` are emitted as canonical `NUMERIC(38,9)` strings, no float64 anywhere on the path | Backend author | Decimal-safe linter in CI |
| Proposals are sorted deterministically before return (kernel-visible order is stable) | Category implementation | Property test against backend output permutations |
| Every `MatchProposal` references real `token_id` values resolvable on the current run | Category implementation | `match.input_token_unknown` test |

## Tests

- Schema conformance per tool.
- Token-graph integrity: every `left_token_id` and `right_token_id` in a returned proposal resolves to a kernel-minted token on the run.
- Replay: captured backend output reproduces identical proposals and identical tokens minted from `similarity_score` / `confidence`.
- Determinism: two `match.cross_set` calls with the same input produce the same proposal list in the same order even when the backend returns its internal results in a permuted order.
- Validator integration: a configured `match.confidence_min` threshold actually rejects below-threshold proposals; the rejected proposal does not flow to downstream tool calls.

## Open questions

1. **Cross-field weights.** The pgvector backend takes `method_config.field_weights`. Should weights themselves be tokens (auditable as derivations on the proposal)? Defer to v0.5; v0 records weights in `tool_call_started.deterministic.input_hash` so they are on the chain but not as their own tokens.
2. **Mutual-best vs. one-sided.** `match.candidates` is one-sided (left vs. set). Should the category also offer a "mutual-best" variant that only returns A↔B when A is B's best candidate and B is A's best? Defer; `dedupe` consumes `cross_set` and computes mutual-best internally.

## References

- [`architecture/tools/policy/README.md`](../policy/README.md) — the precedent for backend-pluggable retrieval with confidence
- [`architecture/tools/policy/backends/pgvector.md`](../policy/backends/pgvector.md) — the precedent for operator-configured embedding model
- [`architecture/tools/extract/README.md`](../extract/README.md) — the precedent for CapturedDeterministic + confidence calibration
