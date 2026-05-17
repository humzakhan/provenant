# Subsystem: HITL Router

Status: **v0** (REST endpoints; web UI deferred)

> Human-in-the-loop is a first-class subsystem, not a bolt-on. Confidence-based routing, structured approvals, diff capture, and feedback loops into the case bank are all defined here. The pattern comes from [Brex's incremental automation](../references.md#brex-first-round): auto-handle high-confidence cases, route the rest to humans with AI assistance.

## Depends on
- [`subsystems/storage.md`](storage.md) — review state persists
- [`subsystems/audit.md`](audit.md) — every human action logged
- [`subsystems/eval.md`](eval.md) — corrections flow into the case bank

## Depended on by
- [`subsystems/harness.md`](harness.md) — runs may transition to `awaiting_review`
- [`subsystems/guards.md`](guards.md) — confidence validators trigger HITL routing
- All adapters

## Responsibilities

1. Route runs to the appropriate review tier based on confidence and skill policy.
2. Manage approval state machines (single review, dual review, maker-checker).
3. Capture structured corrections from reviewers.
4. Feed corrections back into the case bank.
5. Expose REST endpoints for review UIs (Provenant-provided or customer-built).

## Routing model

Skill packs declare HITL policy in their manifest:

```yaml
hitl:
  routing:
    auto_below: 0.85           # confidence < 0.85: always human review
    single_review_below: 0.95  # 0.85 ≤ confidence < 0.95: one reviewer
    dual_review_above: 0.95    # confidence ≥ 0.95: auto-release if also dual-review-enabled, else single review
  dual_review:
    enabled: false             # turn on for high-stakes runs
    independent: true          # reviewers cannot see each other's decisions before committing
  reviewer_pools:
    standard: ["controller-team"]
    senior: ["controller-team", "senior-acc-team"]
  escalation:
    after_minutes: 60          # if no review action after 60 min, page on-call
```

The "confidence" input is whatever the skill's confidence validators produce — for recon, it's the minimum extraction confidence; for AP coding, it's a weighted combination of extraction and policy-match confidence. The HITL router does not interpret confidence; the skill defines it.

## Approval state machine

```
            ┌──────────────────────────────────────┐
            │           AwaitingReview             │
            └──────────────────────────────────────┘
                       │
       ┌───────────────┼────────────────┐
       │               │                │
       ▼               ▼                ▼
   Single         Dual review        Maker-Checker
   review         (parallel)         (sequential)
       │               │                │
       │   ┌───────────┴──────┐         │
       ▼   ▼                  ▼         ▼
   approved  partial         conflict   maker_approved
   rejected  approval        (escalate) → checker_approved
   edited    pending                    → released
                                        OR rejected
```

State transitions are recorded in the audit log. Every transition includes reviewer identity, timestamp, structured action, and free-text notes.

## Review payload

When a run reaches `awaiting_review`, the HITL router produces a structured review record:

```json
{
  "run_id": "run_01HXYZ...",
  "review_id": "rev_01HXYZ...",
  "tier": "single_review",
  "reviewer_pool": "standard",
  "draft_answer": {...},
  "claims_and_tokens": [...],
  "trajectory_summary": [...],
  "confidence_breakdown": {
    "extract.bank_statement[line_7]": 0.78,
    "extract.bank_statement[line_12]": 0.94,
    ...
  },
  "highlighted_issues": [
    {"kind": "low_confidence", "location": "line_7", "value": "237.42", "confidence": 0.78}
  ],
  "context_link": "/audit/run/run_01HXYZ"
}
```

Reviewers see the draft, the confidence breakdown, the highlighted issues, and a link to the full audit trail.

## Diff capture

When a reviewer edits the draft answer, the system captures structured corrections:

```json
{
  "field": "unmatched_residuals[0].amount",
  "before": "237.42",
  "after": "237.41",
  "kind": "edited",
  "reason": "Statement page 3 shows 237.41, not 237.42. Extraction error."
}
```

Edits are categorized:

- **Approved**: reviewer confirms the agent's draft.
- **Edited**: reviewer changes a value or text.
- **Rejected**: reviewer rejects the whole output; the run fails.
- **Escalated**: reviewer routes to a higher tier.

Each edit becomes a case bank entry on run completion. The pattern recurring across cases (e.g., "extract.bank_statement consistently off by $0.01 on this customer's statements") becomes a candidate for a new validator or a few-shot example. See [eval.md](eval.md).

## Bulk review

When a skill produces many similar outputs (invoice coding, expense reviews), reviewers are presented with a grouped UI:

```
Recon batch for Q2 2026:
  ┌─ 87 high-confidence runs (auto-approved) ────── (toggle to show)
  ├─ 12 medium-confidence runs (your queue) ──────────────────────
  │    [✓] run_01... · vendor "ACME Corp"   · diff: 1 field
  │    [✓] run_02... · vendor "Beta LLC"    · diff: 0 fields
  │    [✗] run_03... · vendor "Gamma Inc"   · diff: 3 fields (review carefully)
  │    ...
  └─ 3 low-confidence runs (escalated to senior) ── (read-only)
```

Bulk approve/reject with spot-check sampling. Audit captures bulk operations as well as per-run actions.

## REST API (v0)

**Canonical surface: `/v1/hitl/...`.** Cycle 5 aligned three previously inconsistent REST surfaces (`/v1/reviews/...`, `/v1/hitl/...`, `/v1/runs/{run_id}/hitl/...`) onto a single `/v1/hitl/...` prefix referenced by implementation-plan.md Phase 4. The `/v1/reviews/...` and `/v1/runs/{run_id}/hitl/...` forms are no longer part of the v0 surface.

```
GET    /v1/hitl/pending?tier=<tier>&status=<status>  list pending reviews (across runs)
GET    /v1/hitl/reviews/{review_id}                   fetch a single review
POST   /v1/hitl/reviews/{review_id}/decision          submit a decision (review_id-scoped)
       { "kind": "approved" | "edited" | "rejected" | "escalated" | "expired",
         "edits": [...],
         "notes": "..." }
POST   /v1/runs/{run_id}/hitl/decision                submit a decision (run_id-scoped convenience)
       — equivalent to POST /v1/hitl/reviews/{review_id}/decision where review_id is
         resolved from run_id; provided as a convenience for adapters that hold run_id
         but not review_id. The kernel-side state-transition logic is identical.
POST   /v1/hitl/reviews/{review_id}/claim             claim for current reviewer (locking)
POST   /v1/hitl/bulk/approve                          bulk approval with sampling
GET    /v1/hitl/reviews/{review_id}/audit             audit trail for this review
```

**Decision enum (cycle-5 alignment).** The decision-`kind` enum is the **union** of the values previously declared in hitl.md and in the `EntryKindHITLDecision` payload schema in [`audit.md`](audit.md):

```
approved | edited | rejected | escalated | expired
```

- `approved`: reviewer confirms the agent's draft.
- `edited`: reviewer changes a value or text; `edits` array required.
- `rejected`: reviewer rejects the whole output; the run fails.
- `escalated`: reviewer routes to a higher tier.
- `expired`: no human action within the configured deadline; emitted by the escalation-timeout sweep, not by a reviewer call. Treated as `failed` at the run level (see harness state machine) unless the skill manifest configures `on_expiry: escalate`. For `expired` decisions, the `EntryKindHITLDecision` entry's non-deterministic `reviewer_principal` field carries the reserved sentinel string **`"system:escalation-sweep"`** (a literal; not a hash; not interpolated). The `"system:"` prefix is reserved for kernel-emitted decisions; **reviewer principals from the human-reviewer IAM directory are forbidden from using the `"system:"` prefix** — the IAM provisioning step (see [`cross-cutting/security.md`](../cross-cutting/security.md#api-security)) rejects any reviewer principal whose identifier starts with `system:`. This rule preserves "every decision has a non-null `reviewer_principal`" without polluting the human-reviewer namespace, and makes "was this decision human or system-emitted?" a string-prefix test on the chain.

[`audit.md`](audit.md)'s `EntryKindHITLDecision` enum is amended below to match.

Auth: bearer tokens scoped to reviewer pool membership. Multi-tenant deployments add tenant scoping.

The Provenant-hosted web UI consumes this API. Customers can build their own UI against it — the API is the contract.

## Storage

```sql
CREATE TABLE hitl_reviews (
    id              TEXT PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs(id),
    tenant_id       TEXT NOT NULL,
    tier            TEXT NOT NULL,                    -- single | dual | maker_checker
    pool            TEXT NOT NULL,
    status          TEXT NOT NULL,                    -- awaiting | claimed | decided | escalated
    claimed_by      TEXT,
    claimed_at      TIMESTAMPTZ,
    decision_kind   TEXT,                             -- approved | edited | rejected | escalated
    decided_by      TEXT,
    decided_at      TIMESTAMPTZ,
    edits           JSONB,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

For dual review, two `hitl_reviews` rows per run, joined by a `review_group_id`.

## Audit integration

Every review action emits audit entries:

- `hitl_dispatched` when a run transitions to `awaiting_review`.
- `hitl_claimed` when a reviewer claims a review.
- `hitl_decision` when a decision is submitted.
- `hitl_escalated` when the review escalates.

These flow into the same chain as everything else, included in `audit pack` exports.

## Tests

- Unit: routing logic for every threshold permutation.
- Integration: full state machine transitions (single review, dual review, maker-checker, escalation).
- Property: no review can be decided twice; no run can release without an approved decision.
- Load: bulk approval of 1000 expense reviews completes in <5 seconds.

## Open questions

1. **Reviewer rate limiting.** Should we cap how many low-confidence reviews a single reviewer can approve per hour to prevent rubber-stamping? Proposal: yes, configurable, default 50/hour. With audit logging of the cap hits.
2. **Sampling for bulk approve.** What fraction of a bulk-approved batch should be sampled for spot-check? Proposal: configurable per skill, default 5% with floor of 3 items.
3. **Mobile review UX.** Many controllers want to review on phones. Defer the mobile UI to v0.5; v0 is desktop web only.

## References

- [Brex incremental automation](../references.md#brex-first-round) — confidence-routed HITL as industry standard
- [Arthur AI guardrails](../references.md#arthur-guardrails) — post-LLM correction loops, conceptually adjacent
- [FINRA 2026](../references.md#finra-2026) — supervision requirements that HITL satisfies
