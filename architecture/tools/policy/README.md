# Tool Category: Policy

Status: **v0**

> Retrieve cited passages from policy documents — chart of accounts rules, expense policies, tolerance documents, regulatory filings. Every retrieval includes the passage text, a stable anchor (document ID + section ID), and a confidence score. The `policy.citation_exists` validator gates `submit_answer` to ensure every policy citation in the answer resolves.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)

## Depended on by
- `recon` skill pack (tolerance lookup)
- `ap_coding` skill pack (GL code policy, approval thresholds)
- `expense_review` skill pack (T&E policy)
- `sanctions_disposition` skill pack (sanctions disposition guidelines)

## Purpose

Finance workflows are policy-driven. The recon skill needs to know "what is the tolerance threshold for accepting unmatched bank fees?" The AP coding skill needs to know "for vendor type X, what GL accounts are valid?" The expense review skill needs to know "what is the meal allowance for executive travel?"

Hard-coding policy in skill packs is brittle (policies change quarterly; packs don't). Inlining policy in prompts at runtime makes context bloated and makes citations unreliable. The policy tool category solves both: retrieve relevant passages by query, produce typed citations, validate that citations resolve.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `policy.lookup` | Retrieve cited passages relevant to a query | CapturedDeterministic |
| `policy.get_section` | Retrieve a specific section by stable anchor | CapturedDeterministic |
| `policy.list_documents` | List indexed policy documents | CapturedDeterministic |

## Schemas

### `policy.lookup`

Input:
```json
{
  "query": "What is the tolerance threshold for accepting unmatched bank fees?",
  "documents": ["recon-policy", "general-controller-manual"],
  "top_k": 5,
  "min_relevance": 0.5
}
```

Output:
```json
{
  "results": [
    {
      "document_id": "recon-policy",
      "document_title": "Bank Reconciliation Policy v2.3",
      "section_id": "recon-policy#unmatched-residuals",
      "anchor": "Section 4.2",
      "passage": "Unmatched residuals below USD 50 may be auto-released without escalation. Residuals between USD 50 and USD 500 require single-reviewer approval. Residuals above USD 500 require dual approval and a written explanation.",
      "relevance": 0.91,
      "page": 7
    },
    ...
  ]
}
```

Mintable paths: none directly numeric. Citation references are non-token typed references.

### `policy.get_section`

Input:
```json
{ "section_id": "recon-policy#unmatched-residuals" }
```

Output: the full passage at that section.

### `policy.list_documents`

Input: `{}`

Output:
```json
{
  "documents": [
    {
      "id": "recon-policy",
      "title": "Bank Reconciliation Policy v2.3",
      "version": "2.3",
      "effective_date": "2026-01-01",
      "owner": "Corporate Controller",
      "sections": 14
    },
    ...
  ]
}
```

## Citation semantics

A citation is the tuple `(document_id, section_id, version)`. The version is included so audit packs can pin to the exact policy version in force when the run executed. Policy revisions invalidate prior section IDs only via explicit migration metadata maintained by the skill author.

When the agent's `submit_answer` references a citation, the `policy.citation_exists` validator verifies the citation resolves to an actual section. Unresolvable citations fail closed — there is no policy reference without a resolved anchor.

## Determinism and replay

`CapturedDeterministic`: same query against the same policy version yields the same results (assuming a deterministic index — see backends). The kernel captures the response for replay.

Captured-read identifier:
```
policy:<backend>:<tool>:<sha256_of_input_plus_index_version>
```

The index version is part of the capture key — if the policy index is rebuilt, prior captures remain valid for replay but new runs against the same query may differ.

## Confidence

Confidence (the `relevance` field) is backend-specific:

- Keyword backend: BM25-derived score normalized to [0, 1].
- Pgvector backend: cosine similarity, normalized.

Skill packs configure `min_relevance` to filter low-quality matches. The `confidence.extraction_min` validator does **not** apply to policy retrievals (it's extraction-specific); skill packs that need strict policy-confidence gates implement a custom validator.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `policy.document_not_found` | Document ID not indexed | Surface; skill aborts or proceeds without that document |
| `policy.section_not_found` | section_id doesn't resolve in `get_section` | Hard failure on the validator if surfaced in `submit_answer` |
| `policy.index_unavailable` | Backend store unreachable | Retry per policy |
| `policy.no_results` | Below `min_relevance` threshold | Skill decides whether to broaden query or escalate |

## Performance budget

| Tool | p50 | p95 |
|---|---|---|
| `lookup` | 100ms | 500ms |
| `get_section` | 20ms | 100ms |
| `list_documents` | 50ms | 200ms |

Per-call timeout: 10 seconds.

## Backends

| Backend | Status | Doc |
|---|---|---|
| Keyword (BM25) | v0 | [backends/keyword.md](backends/keyword.md) |
| pgvector | deferred (v0.5) | [backends/pgvector.md](backends/pgvector.md) |

The keyword backend is sufficient for v0 use cases. Policy documents are typically <100 pages per tenant; BM25 with stemming and per-document boost provides workable retrieval. Semantic retrieval via pgvector is a v0.5 enhancement for tenants with large policy corpora.

## Tests

- Schema conformance.
- Citation validity: every `section_id` returned by `lookup` resolves via `get_section`.
- Replay: captured responses reproduce identically.
- Backend integration: fixture policy documents indexed and queryable.

## Open questions

1. **Versioned policies.** When a policy document updates, do prior runs reference the old version? Proposal: yes — versions are immutable; reruns against new policy are explicit "policy migration" operations.
2. **Multi-tenant policy isolation.** Each tenant has its own policy index. Shared global policies (e.g., regulatory text) live in a system-wide tenant. Codify in storage docs.

## References

- [Anthropic finance agents launch](../../references.md#anthropic-finance-agents) — multiple template agents perform policy-grounded reasoning
- [Moody's modular AI](../../references.md#moodys-modular) — "context layer" model where policy lives outside model weights
