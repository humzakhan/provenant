# Simplification Audit - 2026-05-21

## Question

Can Provenant be simplified without compromising output?

## Answer

Yes, but only by simplifying the user-facing model, docs, non-golden UI copy, and presentation guidance. The output contracts themselves are already doing important work and should not be reduced casually. Provenant's core promise is that files with financial numbers pass through a provenance layer and every output number can be traced back to origin evidence. The current artifacts already support a simpler story: final number, source rows, row impact, check status, and comparison differences. The project can hide or de-emphasize implementation vocabulary without weakening deterministic bundles, trace JSON, audit chains, reconciliation outputs, generated report output, or report manifests.

The safe simplification path is therefore presentation-first: explain the system as a provenance layer for finance files, preserve the golden artifacts, and move token graph/audit plumbing into technical detail. Changing serialized field names, bundle shapes, audit event names, report ZIP entries, or byte-compared HTML wording is risky and should be treated as an output-contract migration, not a simplification.

## Scope

This audit synthesizes two read-only evidence reports:

- Architecture and terminology review across public/product docs, architecture docs, ADRs, and public Go declarations.
- Golden/e2e contract review across deterministic bundles, trace JSON, reconciliation artifacts, HTML reports, ZIP reports, manifests, and replay/verify assertions.

The audit focuses on whether Provenant can become easier to understand while preserving its stated product goal: run alongside existing workflows so number-bearing finance files pass through Provenant and can be traced back to origin.

## Findings

### 1. Simplification Is Feasible Without Output Changes

The trace model already supports a simpler user-facing explanation: this answer equals these source-row contributions. The scaled demo asserts `sum(leaf contributions) == operating_income.ValueCanonical` in `examples/derivation-graph/scaled/e2e_test.go:156-181`, and `kernel/public/trace/doc.go:3-7` grounds trace output in source path, selector, SHA-256, leaf value, and contribution.

Recommended default user view:

- Final number.
- Source rows.
- Row impact.
- Check status.
- Comparison differences.

Technical detail should still expose token IDs, captured read IDs, graph nodes/edges, audit hashes, manifests, and replay metadata.

### 2. Core Vocabulary Should Stay, But Be Qualified

Several terms are invariant-critical or output-contract-facing and should remain:

- `provenance`, `lineage`, `origin`: core product terms. `docs/thesis.md:15` frames provenance as an enforcement boundary, and `kernel/public/types.go:149-156` exposes `TokenSource.Origin`.
- `captured read`: the replayable source-read evidence unit before token minting. See `README.md:19`, `kernel/public/types.go:107-122`, and `access/sources/sources.go:1-5`.
- `source reader`: useful when kept concrete as a reader of source bytes/files. See `kernel/public/interfaces.go:22-25` and `architecture/tools/extract/README.md:77-84`.
- `provenance token`: central per-number wrapper. `architecture/subsystems/kernel.md:35-38` says every numeric value in final output must correspond to exactly one token, and `kernel/public/types.go:124-147` defines `Token`.
- `audit chain`, `audit.jsonl`, `audit bundle`, `verify`, `replay`, `deterministic replay`: required for tamper evidence, offline verification, and reconstructability.

Simplification should qualify ambiguous terms on first use: use `provenance token`, not bare `token`; use `audit bundle`, not bare `bundle`; distinguish `skill manifest`, `report manifest`, and `source registry`.

### 3. Implementation Vocabulary Can Move Out Of Product Narrative

The project can be clearer by moving plumbing terms out of top-level docs and default UI copy:

- `AuditSink` is an implementation append target. `kernel/public/interfaces.go:40-45` defines it, but buyer-facing docs can say `append audit entry` or `audit chain`.
- `kernel` is important for Go boundaries, but product docs can prefer `Provenant library`, `provenance engine`, or `provenance layer`. `architecture/subsystems/kernel.md:5` already calls it a provenance engine.
- `TransactionalScope`, `transactional substrate`, and marker/canary rules are architecture/API details. See `architecture/OVERVIEW.md:103-105`, `architecture/subsystems/kernel.md:340-358`, and `kernel/public/interfaces.go:11-15`.
- `sidecar` should be limited to deployment docs because it collides with report sidecar artifacts. For report files, use `companion artifact`.
- `substrate` can stay as one crisp identity definition, then practical docs should say `provenance layer`, `library`, or `enforcement layer`.

This simplification does not require artifact changes.

### 4. Minting Language Is A User-Facing Simplification Candidate

The behavior behind `mint`, `read-mint`, and `mintable_paths` is essential: create a provenance token from verified captured-read evidence. But the metaphor is more crypto-like than the buyer goal requires. `README.md:21`, `architecture/subsystems/kernel.md:93-102`, `kernel/public/mint.go:42-59`, and `ADR-0004:37` establish the current API/doc language.

Safe simplification: public docs can say `issue a provenance token`, `bind a value to origin evidence`, or `record a traced value`, while preserving API names unless a separate migration is planned.

Risky simplification: renaming serialized audit kinds, CLI verbs, Go request/result types, or golden fixture fields as a casual cleanup. That would be an output/API contract change.

### 5. Golden Artifacts Are Real Output Contracts

The following are deterministic or golden-output contracts and should not be changed merely to simplify terminology:

- `golden.bundle.json`: bit-identical deterministic artifact checked with `bundle.Verify`, `bundle.Replay`, and repeated-run determinism at `examples/derivation-graph/e2e_test.go:6-10` and `examples/derivation-graph/scaled/e2e_test.go:93-154`.
- `golden.trace.json`: JCS-canonicalized and byte-compared at `examples/derivation-graph/e2e_test.go:115-142` and `examples/derivation-graph/scaled/e2e_test.go:125-154`.
- Trace fields from `kernel/public/trace/trace.go:28-67`: `root`, `root_value`, `root_unit`, `nodes`, `edges`, `leaves`, `token_id`, `value_canonical`, `operation`, `captured_read_id`, `source_path`, `source_sha256`, `selector`, `leaf_value`, `unit`, `contribution`.
- Scaled demo deterministic guarantees: root value `20357801.810000000`, `1991` nodes, `1990` edges, `1980` leaves, bundle format `phase1.bundle/v1`, `1980` captured reads, `1991` tokens, and `1994` audit lines.
- Broken reconciliation JSON fields: `run_id`, `payroll_reconciliation`, `expenses_reconciliation`, `mode`, `left_file_id`, `right_file_id`, `match_on`, `compare`, `results`, `outcome`, `key`, `left`, `right`, `position`, `differences`, byte-compared at `examples/derivation-graph/scaled/broken/e2e_test.go:234-251`.
- `golden.mismatches.csv`: two asserted anomaly rows at `examples/derivation-graph/scaled/broken/golden.mismatches.csv:1-3`.
- HTML report golden wording and tabs in `audit/report/testdata/golden.html:156-184` and submitted-answer status wording at `audit/report/testdata/golden.html:214-220`, byte-compared by `audit/report/html_test.go:294-319`.
- Scaled demo generated report output in `examples/derivation-graph/scaled/golden.report.html` and `examples/derivation-graph/scaled/broken/golden.report.html`, byte-compared by the scaled demo E2E tests.
- Audit pack manifest fields from `audit/pack/pack.go:62-91`: `schema_version`, `bundle_sha256`, `bundle`, `captured_reads`, `tokens`, `audit_jsonl`, `report`, `manifest`, `bundle_hash`, `artifacts`, `name`, `sha256`.
- Reconciliation ZIP entries asserted by `reconcile/report/report_test.go:24-38`: `report.md`, `report.json`, `mismatches.csv`, `captured_reads.json`, `tokens.json`, `audit.jsonl`, `bundle.json`, `manifest.json`.

These contracts can be documented and explained more simply, but changing byte-compared generated output requires golden/migration review rather than a presentation-only simplification.

### 6. The Minimum User-Facing Trace-Back Fields Are Clear

For the project goal, a user-facing trace-back view only needs a subset of the output fields by default:

- `source_path`.
- `selector`.
- `source_sha256`.
- `leaf_value`.
- `contribution`.
- `value_canonical`.
- `unit`.
- `outcome` for reconciliation.

This preserves traceability from final financial number to source file/row/value while hiding graph topology and IDs until needed.

### 7. Bundle Integrity And Semantic Reconciliation Must Stay Separate

The broken-run fixtures show that bundle integrity is not the same as business correctness. `examples/derivation-graph/scaled/broken/e2e_test.go:100-108` asserts `bundle.Verify` and `bundle.Replay` still pass, while `examples/derivation-graph/scaled/broken/e2e_test.go:149-159` asserts the broken operating income differs from the happy-path value.

Simplified messaging should say:

- Verify/replay prove the evidence chain and deterministic reconstruction.
- Reconciliation detects business mismatches, missing records, or changed values.

Do not collapse these into one generic `valid` or `verified` status.

### 8. Documentation Drift Creates Avoidable Complexity

Two architecture/documentation inconsistencies should be corrected because they make the system look more complex or contradictory than it is:

- Audit event names drift: architecture docs list `answer_released`, `submit_attempt`, etc. in `architecture/subsystems/audit.md:37-56`, while current public code uses `answer_submitted` and `token_derive` in `kernel/public/types.go:181-198`.
- Bundle layout drift: `architecture/subsystems/audit.md:99-105` describes a directory-style bundle with `audit.jsonl`, `audit.csv`, `audit.pdf`, and `token-graph.json`, while `kernel/public/bundle/bundle.go:37-43` defines a single JSON document with `format`, `run`, `captured_reads`, `tokens`, and `audit_jsonl`; `README.md:125` matches the single JSON document.

Two demo/report doc drifts should also be fixed:

- `examples/derivation-graph/scaled/README.md:159-160` says HTML has no JavaScript, but `audit/report/html_test.go:355-402` allows and requires inline JavaScript. The actual guarantee is no external assets or network.
- `examples/derivation-graph/scaled/broken/artifact/README.md:35-57` says the broken report has a contribution invariant violated banner, but the broken E2E notes internal self-consistency at `examples/derivation-graph/scaled/broken/e2e_test.go:10-18`, and committed broken golden HTML shows `Contribution invariant: OK` at `examples/derivation-graph/scaled/broken/golden.report.html:184`.

Fixing drift is a no-output-change simplification unless the golden text itself is updated.

## Recommended Simplification Plan

### No-Output-Change Simplifications

1. Rewrite top-level product narrative around one sentence: Provenant is a provenance layer for finance workflows that records where every output number came from and packages evidence for verify/replay.
2. Keep core terms, but qualify first use: `provenance token`, `captured read`, `audit chain`, `audit bundle`, `deterministic replay`.
3. De-emphasize implementation terms in product docs: `AuditSink`, `TransactionalScope`, `kernel`, `sidecar`, `transactional substrate`, raw `mint` metaphors.
4. Present default report guidance, docs, and non-golden UI copy as `Final number`, `Source rows`, `Row impact`, `Check status`, and `Comparison differences`; generated HTML report wording remains governed by byte-compared golden output and should change only through golden/migration review.
5. Move `token_id`, `captured_read_id`, `nodes`, `edges`, `audit_seq`, `bundle_hash`, and manifest internals under `Technical detail`.
6. Rename user-facing explanations of `contribution` to `row impact` or `source row impact`, while preserving JSON field names.
7. Map reconciliation outcomes in UI/docs without changing artifacts: `matched` as `same`, `mismatch` as `changed`, `missing_right` as `missing from comparison file`.
8. Correct documentation drift around audit event names, current bundle layout, HTML no-external-assets guarantee, and broken report contribution invariant status.
9. Standardize buyer-facing bundle wording on `audit bundle`; reserve `pack` for CLI/package implementation only if both terms must remain.
10. Use `companion artifact` for report files instead of `sidecar artifact` to avoid collision with runtime sidecar deployment.

### Risky Or Breaking Output-Contract Changes

Treat these as separate migrations with golden updates, compatibility notes, and explicit approval:

1. Renaming serialized trace fields such as `contribution`, `leaf_value`, `source_path`, `selector`, `source_sha256`, `token_id`, or `captured_read_id`.
2. Renaming bundle fields, bundle `format`, audit payload fields, audit kinds, or pack manifest fields.
3. Changing ZIP report entries, manifest schema, or artifact hash semantics.
4. Changing byte-compared HTML report wording, tabs, banners, or status strings without updating golden tests deliberately.
5. Collapsing verify/replay integrity status with reconciliation/business correctness status.
6. Replacing `token`/`CapturedRead`/`Derivation` concepts in public Go APIs without a compatibility plan.
7. Changing the single JSON bundle shape to the older architecture directory-style layout, or vice versa, without treating it as a format version change.

## Decision

Proceed with simplification as a docs and presentation pass first. Do not change golden artifacts, serialized fields, audit event names, bundle schema, ZIP entries, or API names as part of the simplification pass. Any later output-contract simplification should be planned as a versioned contract migration.
