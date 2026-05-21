# Simplification Implementation Plan

## Principles

- Prefer the smallest PR that resolves a coherent slice of simplification work.
- Start with no-output-change documentation cleanup before correcting deeper architecture drift.
- Keep generated output contracts, golden fixtures, serialized JSON fields, public APIs, bundle formats, ZIP entries, and manifest schemas unchanged unless a PR explicitly targets a versioned/golden-reviewed change.
- Isolate output-contract and golden changes later than docs-only cleanup.
- Generated golden output changes require explicit migration/versioning notes and golden review.

## PR 1: Lowest-Risk Docs Terminology Cleanup

### Goal

Simplify top-level and buyer-facing documentation terminology without changing code, generated artifacts, serialized fields, public APIs, or golden outputs.

### Scope

- Reframe user-facing prose around Provenant as a provenance layer for finance workflows that records where every output number came from and packages evidence for verify/replay.
- Qualify core terms on first use: `provenance token`, `captured read`, `audit chain`, `audit bundle`, and `deterministic replay`.
- De-emphasize implementation terms in product narrative where they are not needed: `AuditSink`, `TransactionalScope`, `kernel`, `transactional substrate`, bare `mint`, and ambiguous `sidecar`.
- Prefer `companion artifact` for report-adjacent files where runtime sidecar deployment is not meant.
- Preserve technical-detail sections where implementation vocabulary is useful.

### Files/Areas Likely Touched

- `README.md`.
- `docs/thesis.md`.
- High-level documentation under `docs/` and `architecture/` only where terminology appears in user-facing narrative.

### Non-Goals

- No Go API renames.
- No CLI verb changes.
- No serialized JSON field renames.
- No audit event, bundle, manifest, ZIP, or report output changes.
- No generated golden fixture updates.

### Validation

- Review docs diff for terminology-only changes.
- Run the repository's normal documentation or test validation if available.
- Run targeted `go test` only if documentation examples or generated docs are touched in a way that could affect tests.

### Risks

- Over-removing precise implementation language could make technical docs less useful.
- Accidental edits to byte-compared generated report text would create unintended golden changes.

### Why Ordered Here

This is the lowest-risk simplification and establishes the shared language for later PRs without touching output contracts.

## PR 2: Documentation Drift for Demo and Report Invariants

### Goal

Correct documentation drift around HTML asset guarantees and broken-demo invariant wording without changing generated report output.

### Scope

- Replace claims that scaled HTML reports have no JavaScript with the current invariant: no external assets or network dependencies.
- Correct broken-demo wording that says the contribution invariant is violated when current committed output and tests show the contribution invariant is internally self-consistent and `OK`.
- Clarify that broken-run bundle verify/replay can pass while reconciliation/business correctness detects mismatches.

### Files/Areas Likely Touched

- `examples/derivation-graph/scaled/README.md`.
- `examples/derivation-graph/scaled/broken/artifact/README.md`.
- Nearby demo documentation if it repeats the same wording.

### Non-Goals

- No changes to `golden.report.html` or `audit/report/testdata/golden.html`.
- No changes to report templates, report JavaScript, or asset handling.
- No changes to reconciliation JSON, CSV, bundle, trace, or audit outputs.

### Validation

- Review docs diff against `audit/report/html_test.go` expectations for inline JavaScript and no external assets.
- Review docs diff against broken E2E assertions and committed broken golden report wording.
- Run targeted scaled demo tests only if examples or generated fixtures are touched beyond README files.

### Risks

- The correction could be misread as weakening the offline report guarantee; wording must preserve the no-network/no-external-assets invariant.

### Why Ordered Here

It is still docs-only and low risk, but it fixes concrete contradictions after the broad terminology cleanup is in place.

## PR 3: Architecture and Current-Code Consistency

### Goal

Align architecture documentation with current code for audit event names and bundle layout while preserving current serialized contracts.

### Scope

- Update architecture docs that list stale audit event names to match current public code terminology, including `answer_submitted` and `token_derive` where applicable.
- Update architecture docs that describe an older directory-style bundle layout to reflect the current single JSON bundle with `format`, `run`, `captured_reads`, `tokens`, and `audit_jsonl`.
- Keep historical context only if clearly labeled as historical or superseded.

### Files/Areas Likely Touched

- `architecture/subsystems/audit.md`.
- Related architecture overview docs if they repeat the stale event names or bundle layout.

### Non-Goals

- No renaming of audit event kinds in code.
- No bundle format changes.
- No report pack manifest schema changes.
- No serialized field changes.
- No golden fixture updates.

### Validation

- Compare updated docs against `kernel/public/types.go` for event names.
- Compare updated docs against `kernel/public/bundle/bundle.go` and current README bundle wording.
- Run normal docs/test validation if available.

### Risks

- Architecture docs may intentionally preserve aspirational or historical design notes; stale material should be updated carefully rather than deleted wholesale.

### Why Ordered Here

It fixes deeper architecture/current-code drift after lower-risk user-facing docs are simplified, still without changing contracts.

## PR 4: User-Facing Report and Presentation Wording Design

### Goal

Decide whether report/presentation wording can be improved without golden-output changes; implement only non-golden wording updates in this PR.

### Scope

- Review non-golden UI copy, docs, screenshots, or presentation guidance for simplified labels: `Final number`, `Source rows`, `Row impact`, `Check status`, and `Comparison differences`.
- Move graph topology, IDs, hashes, manifests, and replay metadata under `Technical detail` in explanatory docs or non-golden presentation material.
- Map reconciliation outcomes in docs without changing artifacts: `matched` as `same`, `mismatch` as `changed`, and `missing_right` as `missing from comparison file`.
- If desired wording touches byte-compared HTML, generated reports, report templates, or fixtures, stop at a design/spec update that identifies required golden review instead of changing implementation.

### Files/Areas Likely Touched

- Documentation or presentation guidance under `docs/`, `architecture/`, and `examples/`.
- Non-golden report-adjacent copy only if tests confirm it is not byte-compared output.
- A design/spec note under planning or docs if implementation would require golden changes.

### Non-Goals

- No byte-compared HTML report wording changes unless this PR is explicitly converted into a design/spec-only PR.
- No report template changes that alter `audit/report/testdata/golden.html`, scaled demo golden reports, ZIP entries, manifests, trace JSON, reconciliation JSON, or CSV outputs.
- No serialized field or public API renames.

### Validation

- Identify whether each touched file is source documentation, generated output, or golden fixture before editing.
- Run relevant report tests if any report source or report-adjacent code is touched.
- Confirm no golden files changed unless the PR has explicitly become design/spec-only with no implementation change.

### Risks

- User-facing wording may be embedded in byte-compared golden output; implementation must not accidentally update golden fixtures.
- If a better presentation requires template changes, it must be deferred to a later versioned/golden-reviewed PR.

### Why Ordered Here

This follows docs drift cleanup and intentionally separates presentation design from output-contract changes. It can remain no-output-change if edits are limited to non-golden materials.

## PR 5: Output-Contract Change Specification, If Needed

### Goal

Plan any desired generated-output or serialized-contract simplification as an explicit migration/versioning effort before implementation.

### Scope

- Specify proposed changes to generated HTML wording, golden reports, trace JSON fields, reconciliation artifacts, audit kinds, bundle layout, report ZIP entries, or manifest fields.
- Define compatibility and migration expectations.
- Define golden review requirements and approval criteria.
- Decide whether version identifiers, release notes, or migration docs are required.

### Files/Areas Likely Touched

- Planning/design documentation first.
- Later implementation may touch report templates, golden fixtures, bundle/report code, trace/reconciliation schemas, or tests only after review.

### Non-Goals

- No implementation in the first design/spec pass unless explicitly approved.
- No casual renames of serialized JSON fields or public APIs.
- No collapsing of verify/replay integrity with reconciliation/business correctness.

### Validation

- Review against every golden and deterministic contract listed in `findings.md`.
- Require explicit golden review for any generated output changes.
- Require migration/versioning review for any serialized schema, public API, bundle, manifest, ZIP, or audit event changes.

### Risks

- Contract changes can break downstream consumers, reproducibility, replay, or byte-for-byte tests.
- Golden updates can hide unintended behavior changes if not reviewed separately from simplification wording.

### Why Ordered Here

Output contracts are intentionally last. The earlier PRs should deliver most simplification value without changing deterministic artifacts.
