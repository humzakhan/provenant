# Tool Category: Dedupe

Status: **v0**

> Cluster a set of token-bearing records into equivalence classes. Each cluster names a canonical representative token and the set of members judged to be the same real-world entity. Below a per-skill confidence threshold, the cluster is auto-routed through the HITL surface; the reviewer's decision is recorded via `EntryKindHITLDecision` like any other human disposition. `dedupe` is implemented on top of `match` — every cluster's member-set is justified by underlying `MatchProposal` evidence on the audit chain.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)
- [`subsystems/hitl.md`](../../subsystems/hitl.md) — clusters below the auto-confidence threshold route through the canonical `/v1/hitl/...` surface
- [`subsystems/guards.md`](../../subsystems/guards.md) — the `dedupe.*` validators gate clusters before they are released
- [`architecture/tools/match/README.md`](../match/README.md) — `dedupe` invokes `match.cross_set` internally

## Depended on by
- Vendor-master harmonization skill packs
- Customer / counterparty deduplication skill packs
- Reconciliation skill variants that need to collapse aliased payees before structural matching
- Sanctions-screening skill packs that need to collapse name variants before list comparison

## Purpose

After `match` produces pairwise proposals, the next question is "given all this pairwise evidence, what are the equivalence classes?" Naïve transitive closure (A↔B above threshold, B↔C above threshold, therefore {A,B,C} is one cluster) is the failure mode that breaks entity resolution in practice: a slightly noisy embedding model collapses unrelated entities into a single mega-cluster by chaining marginal pairs.

`dedupe` is the typed primitive that does cluster construction with explicit decision boundaries:

1. It calls `match.cross_set` over its input set to produce pairwise evidence.
2. It constructs clusters using a configurable rule (default: connected components on the pairwise graph restricted to pairs whose `confidence ≥ skill.match.confidence_min`).
3. It computes `min_pairwise_confidence` per cluster — the weakest evidence link inside the cluster. This is the cluster's own confidence band.
4. It decides per cluster: `auto` (release the cluster as the kernel-grounded answer) or `review_required` (route through HITL).
5. It enforces `dedupe.cluster_size_max` (catches embedding-model collapse — a cluster of 47 vendors when the input was 200 records is almost certainly wrong).
6. It enforces `dedupe.review_required` fail-closed: if HITL is not wired in this deployment and a cluster falls below the auto threshold, the run fails closed rather than auto-releasing an unsafe cluster.

The cluster decisions are themselves tokens (each cluster's `min_pairwise_confidence` mints a token). A skill that releases an answer claiming "we identified 17 unique vendors" can ground both the count and the per-cluster confidences to kernel-minted tokens, satisfying token-grounding at `submit_answer`.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `dedupe.cluster` | Cluster a set of token-bearing records into equivalence classes | CapturedDeterministic (via the underlying `match` captures) |

## Schemas

### `dedupe.cluster`

Input:
```json
{
  "input_set": [
    { "token_ids": ["tk_01H..."], "fields": {"name": {...}, "country": {...}} },
    ...
  ],
  "method": "cosine_embedding",
  "method_config": { "field_weights": {"name": 0.8, "country": 0.2} },
  "cluster_rule": "connected_components_above_confidence",
  "thresholds": {
    "confidence_min_for_edge": "0.850000000",
    "auto_release_min_pairwise_confidence": "0.920000000"
  }
}
```

Output:
```json
{
  "clusters": [
    {
      "cluster_id": "cl_01HCLUSTER_A",
      "representative_token_id": "tk_01HVENDOR_A_NAME",
      "member_token_ids": [
        "tk_01HVENDOR_A_NAME",
        "tk_01HVENDOR_B_NAME",
        "tk_01HVENDOR_C_NAME"
      ],
      "decision": "auto",
      "min_pairwise_confidence": "0.943000000",
      "pairwise_evidence_token_ids": [
        "tk_01HCONFIDENCE_AB",
        "tk_01HCONFIDENCE_AC",
        "tk_01HCONFIDENCE_BC"
      ]
    },
    {
      "cluster_id": "cl_01HCLUSTER_B",
      "representative_token_id": "tk_01HVENDOR_D_NAME",
      "member_token_ids": ["tk_01HVENDOR_D_NAME", "tk_01HVENDOR_E_NAME"],
      "decision": "review_required",
      "min_pairwise_confidence": "0.881000000",
      "pairwise_evidence_token_ids": ["tk_01HCONFIDENCE_DE"]
    }
  ],
  "stats": {
    "input_size": 200,
    "cluster_count": 17,
    "singleton_count": 11,
    "review_required_count": 2
  }
}
```

Mintable paths:

- `$.clusters[*].min_pairwise_confidence` (unit: confidence_unit, scale: `NUMERIC(38,9)`)
- `$.stats.cluster_count`, `$.stats.singleton_count`, `$.stats.review_required_count` (unit: count)
- `$.stats.input_size` (unit: count)

`representative_token_id` selection rule (deterministic): the lexicographically-smallest `token_id` among the cluster's members. This is stable across replays and does not depend on which `match.cross_set` order produced the evidence. Lexicographic comparison is **byte-wise over the UTF-8 encoding** of the canonical `token_id` string. The `token_id` format is pinned in [`subsystems/kernel.md`](../../subsystems/kernel.md) (`tk_` prefix + base32 characters, ASCII-only), so byte-wise and codepoint-wise comparisons coincide; the spec names byte-wise to remove cross-binding ambiguity.

## Cluster construction

The default `cluster_rule` is `connected_components_above_confidence`:

1. Build the undirected graph whose nodes are input records and whose edges are `MatchProposal` pairs with `confidence ≥ thresholds.confidence_min_for_edge`.
2. Compute connected components.
3. For each component, the cluster's `min_pairwise_confidence` is the minimum `confidence` over its constituent edges (the weakest link). Singletons (no edges) have `min_pairwise_confidence = 1.0` (a singleton has no risk of mis-clustering).
4. Decision: `auto` if `min_pairwise_confidence ≥ thresholds.auto_release_min_pairwise_confidence`; otherwise `review_required`.

Alternative cluster rules are open for v0.5 (mutual-best-pair, hierarchical agglomeration with a stop-rule); v0 ships the connected-components rule as the only supported `cluster_rule` value. A pack that names another rule is rejected at load time via the standard schema check.

## Token semantics

- Every cluster mints a token for its `min_pairwise_confidence`.
- Every count-typed `stats.*` field mints a token.
- The cluster's `representative_token_id` and `member_token_ids` are **references** to existing kernel-minted tokens (the input tokens passed in); the cluster does **not** mint new "membership" tokens. Membership is a relation expressed in the `dedupe.cluster` output payload, captured on `tool_call_completed.output_hash`, and surfaced in `pairwise_evidence_token_ids` so a reviewer can trace any cluster decision back to the underlying `match` proposals.
- A cluster's `min_pairwise_confidence` token's `derivation.parents` is the `pairwise_evidence_token_ids` list (the underlying `match.confidence` tokens); `derivation.op` is `dedupe.min_pairwise_confidence`.

The audit pack reconstructs cluster membership by reading the captured `tool_call_completed` payload — its `output_hash` resolves to canonical bytes containing the `member_token_ids` array. Membership is a relation, not a token, and the "every claim's lineage is reconstructible" invariant is satisfied via the captured-payload path, not the token-graph path. Same precedent as `extract.*`'s mintable-paths-not-included fields.

## Nested tool calls and audit trail

`dedupe.cluster` invokes `match.cross_set` once internally. The audit chain therefore contains, in order, for each `dedupe.cluster` invocation:

1. `EntryKindToolCallStarted` for `dedupe.cluster`.
2. `EntryKindToolCallStarted` for the inner `match.cross_set`.
3. The `match.cross_set` `EntryKindTokenMint` entries for every `similarity_score` / `confidence` token produced.
4. `EntryKindToolCallCompleted` for `match.cross_set`.
5. The `dedupe.cluster` `EntryKindTokenMint` entries for every `min_pairwise_confidence` and `stats.*` token.
6. `EntryKindToolCallCompleted` for `dedupe.cluster`.

The parallel-sibling deterministic-order rules from [`subsystems/audit.md`](../../subsystems/audit.md#parallel-sibling-deterministic-order) still apply: if the harness emits multiple `dedupe.cluster` calls in a single parallel batch, the buffer-and-flush rule covers their nested entries in canonical sibling order. The inner `match.cross_set` call is **not** a sibling of any outer dedupe call; it is a strict child of one outer dedupe call, and its entries are flushed in the position of their parent's `sibling_index_in_group`.

## Determinism and replay

`CapturedDeterministic` via the underlying `match.cross_set` capture. The cluster-construction step itself is pure (connected components is deterministic given the edge set), so replay reproduces clusters byte-identically from the captured pairwise evidence.

Stable ordering: clusters are sorted by `representative_token_id` (lexicographic). Within each cluster, `member_token_ids` and `pairwise_evidence_token_ids` are sorted lexicographically.

## HITL integration

A cluster whose `decision` is `review_required` is auto-routed through the HITL surface — the same cycle-5/6 canonical `/v1/hitl/...` REST endpoints documented in [`subsystems/hitl.md`](../../subsystems/hitl.md). The routing is the kernel's responsibility, not the agent's: when `dedupe.cluster` returns clusters with `decision = review_required`, the harness emits one `EntryKindHITLDispatched` per such cluster (with `routing_reason = "dedupe.review_required"` and `confidence_bucket` derived from `min_pairwise_confidence`).

The reviewer's decision is recorded via the existing `EntryKindHITLDecision` payload schema. `decision` is one of `approved | edited | rejected | escalated | expired`:

- `approved` — the cluster stands as proposed; the run resumes with the cluster's `decision` overwritten to `auto`.
- `edited` — the reviewer modified the membership; `edits_canonical` records the additions / removals (per the existing `EntryKindHITLDecision.edits_canonical` format).
- `rejected` — the cluster is dissolved into singletons; the run resumes with the cluster's members as separate clusters.
- `escalated` — routed to a second reviewer per skill config.
- `expired` — per the `/v1/hitl` expiry sweep; the cluster is treated as `rejected` by default unless the skill overrides.

If the deployment has not wired the HITL surface (`hitl_reviews` table empty, no `/v1/hitl/...` endpoint reachable), the `dedupe.review_required` validator fires fail-closed at `submit_answer` time with `error_code = dedupe.review_required_no_hitl` — the run cannot release an answer that depends on an unreviewed sub-confidence cluster.

**Phase staging.** Phase 2 ships `dedupe` with this fail-closed path only: a `review_required` cluster routed through dedupe with no HITL surface available produces typed `dedupe.review_required_no_hitl` at submit-answer time. Phase 2 does **not** ship a synthetic in-process HITL stub. Phase 4 ships the end-to-end positive path: the full `/v1/hitl/...` REST surface is wired, a `review_required` cluster is routed via the canonical endpoints, a reviewer decision is recorded as `EntryKindHITLDecision`, and the run resumes with the cluster's representative token consumed downstream by reconcile.

## Validators

The category declares two validators that skills configure in `guards.yaml`. The validators are also enumerated in [`subsystems/guards.md`](../../subsystems/guards.md):

| Validator | What it checks |
|---|---|
| `dedupe.cluster_size_max` | Rejects any cluster whose `member_token_ids` size exceeds a configured max. Default 10; configurable per skill. Catches embedding-model collapse where a noisy backend chains unrelated entities into a single mega-cluster. |
| `dedupe.review_required` | Asserts that every cluster with `decision = review_required` has been routed through HITL **and** an `EntryKindHITLDecision` recorded **before** `submit_answer`. Fails closed with `dedupe.review_required_no_hitl` if HITL is not wired in this deployment. |

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `dedupe.cluster_too_large` | A cluster exceeds `dedupe.cluster_size_max` | Hard rejection; skill should narrow the input set, lower the confidence-min-for-edge threshold, or switch backends |
| `dedupe.review_required_no_hitl` | A `review_required` cluster has no `EntryKindHITLDecision` and the deployment has no HITL wired | Hard rejection; the deployment must enable HITL or raise the auto-release threshold |
| `dedupe.empty_input` | `input_set` is empty | Skill decides; usually a configuration issue |
| `dedupe.unsupported_rule` | The pack named a `cluster_rule` other than `connected_components_above_confidence` | Schema-level rejection at pack load |

`dedupe.cluster_too_large` and `dedupe.review_required_no_hitl` are declared in the canonical `error_code` table in [`subsystems/audit.md`](../../subsystems/audit.md#canonical-error_code-table) under the `dedupe.*` namespace.

## Performance budget

| Input size | p50 | p95 |
|---|---|---|
| 100 records | 500ms | 2s |
| 1000 records | 4s | 15s |
| 10000 records | (out of v0 budget — batch at skill level) | |

Most of the cost is in the underlying `match.cross_set` call; dedupe's own cluster-construction overhead is sub-second up to 10k records.

Per-call timeout: 60 seconds.

## Backends

None. `dedupe` is a pure-Go category implementation that invokes the `match` category internally; its choice of `match` backend is governed by the skill's `tools.yaml` binding for `match.cross_set`, not a separate `dedupe` backend.

## Conformance checklist

In addition to the standard tool-conformance items in [`architecture/tools/README.md`](../README.md):

| Check | Owner | Enforced where |
|---|---|---|
| The nested `match.cross_set` call is properly audited as a child of `dedupe.cluster` | Category implementation | Integration test asserts entry ordering |
| `representative_token_id` selection is lexicographic-smallest (deterministic) | Category implementation | Property test |
| `min_pairwise_confidence` is the minimum of edge confidences, not the mean / median | Category implementation | Property test |
| `review_required` clusters block `submit_answer` until HITL decision present | Category + guard pipeline | Integration test: a `review_required` cluster + no HITL → typed `dedupe.review_required_no_hitl` |
| Membership relations are not minted as fresh tokens (only confidences and counts mint) | Category implementation | Token-graph audit test |

## Tests

- Unit: cluster construction over fixture pairwise graphs with known correct components.
- Property: idempotence — clustering the same input twice yields identical clusters.
- Property: byte-wise lexicographic representative selection — a property test asserts the byte-wise comparator over the UTF-8 encoding of `token_id` produces the same `representative_token_id` on the Go reference implementation and on any Phase 5 binding (Python, etc.).
- Property: monotonicity — raising `confidence_min_for_edge` never increases cluster sizes.
- Adversarial: a fixture pair where the embedding model deliberately produces a chain of marginal pairs A→B→C→D→E with all `confidence` just above the edge threshold; `dedupe.cluster_size_max` catches the mega-cluster.
- Integration: a fixture two-ledger input deduplicates correctly using the pgvector match backend; cluster tokens reference the source tokens; below-threshold clusters route to HITL and the HITL state machine accepts the reviewer's decision.

## Open questions

1. **Alternative cluster rules.** `connected_components_above_confidence` is the v0 rule. Mutual-best-pair and stop-rule hierarchical agglomeration are candidates for v0.5; the cluster_rule schema field is forward-compatible.
2. **Cross-tenant clustering.** v0 rejects cross-tenant references everywhere; cross-tenant dedupe (e.g., a shared sanctions watchlist clustered against tenant counterparties) is out of scope.
3. **Incremental dedupe.** When new records arrive, recomputing the entire cluster set is wasteful. An incremental cluster-update API is a v0.5 candidate; v0 is recompute-only.

## References

- [`architecture/tools/match/README.md`](../match/README.md) — the underlying fuzzy-comparison primitive
- [`architecture/subsystems/hitl.md`](../../subsystems/hitl.md) — the canonical HITL state machine and `/v1/hitl/...` endpoints
- [`architecture/subsystems/audit.md`](../../subsystems/audit.md) — nested tool-call entry ordering
