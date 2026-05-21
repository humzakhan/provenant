# Output Contract Migration Gates

## Purpose

This note defines the gates for any future output-contract migration that changes generated output, serialized schemas, audit kinds, bundle/report layouts, manifests, trace artifacts, reconciliation artifacts, or golden files.

This is a planning-only contract note. It does not authorize implementation and does not change code, tests, generated files, serialized fields, audit kinds, APIs, CLI verbs, manifests, bundle schemas, ZIP layout, report templates, or golden outputs.

## Contract Surfaces

Treat a change as output-contract work when it touches any of these surfaces:

| Surface | Examples | Required gate |
|---|---|---|
| Generated HTML wording | Report titles, tabs, status strings, contribution wording, reconciliation labels, technical-detail headings | Golden-output review and explicit regenerated-fixture approval. |
| Serialized schemas | Trace JSON, reconciliation JSON, CSV headers, pack JSON, manifest JSON, bundle JSON, audit JSONL field names | Schema-version decision and compatibility review before implementation. |
| Audit kinds and audit payloads | `EntryKind` names, deterministic payload fields, non-deterministic payload fields, error or failure-code enums | Canonical source-table review, deterministic-payload review, and versioning decision. |
| Bundle layout | Bundle `format`, top-level bundle fields, captured reads, tokens, audit JSONL embedding | Bundle format version review and verify/replay compatibility review. |
| Report ZIP and pack manifests | ZIP entry names, manifest `schema_version`, artifact names, hash fields, size fields | Manifest schema review, ZIP layout review, and downstream-consumer impact review. |
| Trace artifacts | Node, edge, leaf, source, selector, hash, contribution, and operation fields | Trace schema version review and golden canonical JSON review. |
| Reconciliation artifacts | Outcome names, `differences`, `mismatches.csv`, report JSON, report Markdown | Reconciliation schema review and distinction from verify/replay integrity. |

## Versioning Policy

Every output-contract migration must make an explicit versioning decision before code changes:

1. If deterministic audit payload fields, audit hash input, audit kinds, or bundle verify/replay inputs change, require a `kernel_version` or bundle format version decision and document why the selected version changes or remains stable.
2. If trace, reconciliation, report manifest, pack manifest, ZIP layout, or generated HTML semantics change, require a schema or artifact version decision even if the implementation keeps the same file names.
3. If a field rename is proposed, prefer additive fields plus documented deprecation only when downstream compatibility requires it; otherwise record why a breaking change is acceptable.
4. If a presentation alias is enough, keep serialized fields unchanged and document the alias as display-only.
5. If no version changes, document the invariant that makes the change non-contractual, such as docs-only wording or non-generated presentation material.

## Determinism Requirements

Output migrations must preserve canonical and deterministic handling:

1. Deterministic and non-deterministic audit payload fields remain separated.
2. Hash input remains canonical and does not rely on ordinary JSON marshalling on the hash path.
3. Generated bundle, trace, reconciliation, and manifest bytes are reviewed as intentional contract changes, not incidental fixture churn.
4. Bundle verify/replay remains pure over bundle bytes, with no DB, CLI, network, wall-clock, or filesystem dependency beyond the provided artifacts.
5. Audit kind, error-code, failure-subcode, and recovery-reason changes start from canonical source tables; generated files are derived, never hand-edited.

## Review Gates

No implementation PR may change an output contract unless its design note answers these questions:

1. Which contract surfaces change, and which remain byte-for-byte unchanged?
2. Which version identifiers change, and which compatibility promises are retained?
3. Which golden fixtures are expected to change, and what user-visible behavior explains each diff?
4. Which downstream consumers may parse, compare, hash, unzip, or display the affected artifact?
5. How does the migration preserve the separation between verify/replay integrity and reconciliation/business correctness?
6. What is the rollback path if the migration breaks a downstream parser or replay path?

Review ownership should include:

1. Determinism review for audit payloads, hash inputs, bundle format, trace canonical JSON, and replay behavior.
2. Golden-output review for generated HTML, scaled demo reports, report Markdown, CSV, JSON, ZIP entries, and manifests.
3. Compatibility review for serialized schemas, public APIs, CLI output, pack format, and bundle layout.
4. Documentation review to keep presentation wording aligned with the implemented contract and avoid hiding technical distinctions.

## Downstream Impact

Before implementation, list known and likely downstream consumers for the affected artifacts:

1. Tests that byte-compare `golden.bundle.json`, `golden.trace.json`, generated HTML, reconciliation JSON, CSV, or report ZIP contents.
2. Users or tools that parse trace JSON fields such as `source_path`, `selector`, `source_sha256`, `leaf_value`, `contribution`, `token_id`, or `captured_read_id`.
3. Users or tools that parse reconciliation outcomes such as `matched`, `mismatch`, `missing_right`, `differences`, and CSV headers.
4. Users or tools that unpack report ZIP entries by exact name.
5. Users or tools that hash bundle, manifest, report, or audit artifacts.
6. Documentation, examples, or demos that describe generated wording, report tabs, invariant status, or bundle layout.

## Rollback And Acceptance Criteria

A migration is acceptable only when all criteria are met:

1. The design note identifies every changed output-contract surface and every intentionally unchanged adjacent surface.
2. Versioning decisions are recorded before implementation.
3. Golden diffs are reviewed as product and contract changes, not blanket fixture updates.
4. Verify/replay behavior remains deterministic across repeated runs on the same input.
5. Reconciliation/business correctness remains documented and tested separately from bundle integrity.
6. Downstream compatibility impact is documented, including any breaking changes and migration guidance.
7. Rollback can restore the previous emitted contract without corrupting existing audit chains or invalidating historical bundle verification.

Rollback must not rewrite historical audit chains. If emitted artifacts have already shipped, rollback means restoring the previous emitter contract for new runs or adding a forward-compatible reader, not mutating already-produced bundles, reports, or audit entries.

## Validation Matrix

Use this matrix to select validation for an implementation PR. Planning-only PRs should run `git diff --check` and confirm only planning/design docs changed.

| Change type | Minimum validation |
|---|---|
| Planning-only note | `git diff --check`; inspect diff for planning/design docs only; confirm no output-contract files changed. |
| Generated HTML wording | HTML report tests; golden HTML diff review; scaled demo report golden review if examples change; no-external-assets check. |
| Trace JSON schema or wording | Trace tests; canonical JSON byte comparison; scaled demo golden trace review; downstream field compatibility review. |
| Reconciliation JSON or CSV | Reconciliation report tests; exact expected-output review; outcome semantics review. |
| Report ZIP layout | ZIP entry assertion tests; manifest schema/hash tests; unpacking compatibility review. |
| Pack manifest schema | Pack tests; manifest hash review; schema-version review. |
| Bundle layout or format | Bundle verify/replay tests; repeated-run determinism tests; format-version review; historical fixture compatibility review. |
| Audit kind or deterministic payload | Canonical source-table update review; generated enum review; `kernel_version` decision; audit hash/replay tests. |
| Public API or CLI output | API/CLI compatibility review; targeted tests for all equal access surfaces; release-note decision. |

## Deferred Implementation Checklist

For a future migration PR, complete this checklist before editing implementation files:

1. Open or update a design note that names the exact contract surfaces to change.
2. Record versioning and compatibility decisions.
3. Identify the golden files expected to change.
4. Identify generated files and the generator command that owns them.
5. Define targeted validation commands and expected fixture diffs.
6. Obtain review approval for deterministic, golden, and downstream compatibility impact.
7. Only then change emitters, schemas, manifests, templates, generated files, or golden outputs.
