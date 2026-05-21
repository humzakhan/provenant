# Presentation Wording Inventory

## Purpose

Inventory report and presentation labels that can be simplified without changing generated outputs, and identify labels that are embedded in byte-compared or serialized artifacts and require explicit output-contract or golden review before changing.

This is a spec-only note. It does not authorize template, fixture, serialized field, manifest, ZIP layout, audit kind, public API, test, code, or CLI changes.

## Safe Without Output Changes

These wording changes are safe only when applied to source documentation, planning notes, sales/product presentation material, or other non-generated, non-golden explanatory copy.

| Existing technical wording | Preferred presentation wording | Boundary |
|---|---|---|
| submitted answer, operating result, root token value | final number | Safe in docs and presentation guidance. Do not change report template text or JSON fields. |
| captured reads, source files, selectors | source rows or source evidence | Safe in explanatory docs. Keep `CapturedRead`, `captured_read_id`, `source_path`, `selector`, and report headings unchanged without contract review. |
| contribution, leaf contribution | row impact or source row impact | Safe as a human explanation. Keep `contribution` in trace JSON and golden assertions unchanged. |
| contribution invariant, prev-hash continuity, provenance verified | check status | Safe as a summary label in docs. Do not collapse bundle verify/replay integrity with reconciliation/business correctness. |
| mismatches, outcomes, differences | comparison differences | Safe in docs where it explains reconciliation output. Keep `mismatch`, `missing_right`, `outcome`, `differences`, and CSV headers unchanged. |
| token graph, derivation graph, audit chain, manifest internals | technical detail | Safe as guidance for what belongs outside a default user narrative. Do not rename the generated `Technical detail` tab without golden review. |
| sidecar report artifacts | companion artifacts | Safe in docs that are not defining ZIP entries, manifests, or existing test names. Do not rename ZIP entries or manifest artifact names. |
| bundle | audit bundle | Safe in prose. Keep serialized `format`, bundle JSON fields, and file names unchanged. |

Recommended default presentation stack:

1. Final number.
2. Source rows.
3. Row impact.
4. Check status.
5. Comparison differences.

Recommended technical-detail stack:

1. Token IDs and captured read IDs.
2. Graph topology, operations, and derivation nodes.
3. Audit sequence, audit kinds, and prev-hash continuity.
4. Bundle hashes, report manifest entries, and replay metadata.
5. Raw reconciliation outcomes and serialized evidence references.

## Output-Contract And Golden-Review Required

The following labels are embedded in generated, byte-compared, serialized, or exact-layout outputs. Changing any of them is not a presentation-only edit.

| Surface | Embedded wording or field names | Evidence | Required review before change |
|---|---|---|---|
| Audit HTML template | `Provenant Audit Report`, `Plain view`, `Technical detail`, `Tokens`, `Source Reads`, `Audit Entries`, `Mismatches`, `Source Files`, `Submitted Answer`, `Audit Chain`, `Run Identity`, `Contribution Invariant`, `Reconciliation Outcomes`, `Derivation Graph`, `Captured Reads`, `Manifest` | `audit/report/html.go:772-1008` | Golden-output review and regenerated `audit/report/testdata/golden.html`; scaled report golden review if examples change. |
| Audit HTML status copy | `Contribution invariant: OK`, `Contribution invariant: VIOLATED`, `grounded -- no literal value in request`, `provenance verified`, `Prev-hash continuity` | `audit/report/html.go:811-915`, `audit/report/html_test.go:426-455` | Golden-output review; preserve distinction between verify/replay integrity and reconciliation correctness. |
| Audit HTML tabs and JS-adjacent labels | `Plain view`, `Technical detail`, `Source file detail`, `Open file`, `Captured read`, `audit pack` | `audit/report/html.go:780-1080`, `audit/report/html_test.go:375-420` | Golden-output review and no-external-assets check. |
| Audit HTML golden fixture | Entire rendered HTML byte stream | `audit/report/html_test.go:294-319` | Explicit golden update using the documented fixture path; no incidental wording changes. |
| Scaled demo HTML reports | Entire rendered `golden.report.html` byte stream for happy and broken runs | `examples/derivation-graph/scaled/e2e_test.go:364-392`, `examples/derivation-graph/scaled/broken/e2e_test.go:279-311` | Scaled-demo golden review for both happy and broken outputs. |
| Reconciliation Markdown report | `# Reconcile Report`, `Run ID`, `Mode`, `Result count`, `Mismatch count`, `Outcomes`, `Mismatches`, `No non-matched results.` | `reconcile/report/report.go:380-408` | Report ZIP contract review because it is emitted as `report.md`. Add tests or golden coverage before changing. |
| Reconciliation CSV | `result_id`, `outcome`, `key`, `field`, `left`, `right`, `reason`; values such as `mismatch` and `missing_right` | `reconcile/report/report.go:419-458`, `reconcile/report/report_test.go:208-229` | CSV contract review and exact expected-output updates. |
| Reconciliation JSON | `run_id`, `mode`, `left_file_id`, `right_file_id`, `result_count`, `outcome_counts`, `mismatches`, `result_id`, `outcome`, `key`, `differences`, `reason`, `evidence` | `reconcile/report/report.go:36-52`, `reconcile/report/report_test.go:130-163` | Serialized schema review; compatibility or versioning decision if field names change. |
| Reconciliation ZIP layout | `report.md`, `report.json`, `mismatches.csv`, `captured_reads.json`, `tokens.json`, `audit.jsonl`, `bundle.json`, `manifest.json` | `reconcile/report/report_test.go:24-38` | ZIP layout and manifest review. Do not rename entries as presentation cleanup. |
| Report manifest | `schema_version`, `bundle_hash`, `artifacts`, artifact `name`, `sha256`, `size` | `reconcile/report/report.go:23-40`, `reconcile/report/report_test.go:93-128` | Manifest schema/version review and hash impact review. |
| Trace JSON | `root`, `root_value`, `root_unit`, `nodes`, `edges`, `leaves`, `token_id`, `value_canonical`, `unit`, `kind`, `operation`, `formula_id`, `audit_seq`, `from`, `to`, `position`, `captured_read_id`, `source_name`, `source_path`, `source_sha256`, `selector`, `leaf_value`, `contribution` | `kernel/public/trace/trace.go:25-67`, `.planning/simplification-audit-2026-05-21/findings.md:74-80`, `examples/derivation-graph/scaled/e2e_test.go:125-154` | Trace schema and golden review; field renames affect byte-compared canonical JSON. |
| Audit pack JSON | `schema_version`, `bundle_sha256`, `bundle`, `captured_reads`, `tokens`, `audit_jsonl`, `report`, `markdown`, `json`, `mismatches_csv`, `manifest`, manifest `schema_version`, `bundle_hash`, `artifacts`, artifact `name`, artifact `sha256` | `audit/pack/pack.go:59-91` | Audit pack schema, manifest, canonicalization, and versioning review. Do not rename fields as presentation cleanup. |
| Bundle JSON and audit JSONL | Bundle format and audit payload/kind names, including audit JSONL line fields `at`, `hash`, `kind`, `payload_deterministic`, `payload_non_deterministic`, `prev_hash`, `run_id`, `seq`, `tenant_id` | `reconcile/report/report.go:55-64`, `.planning/simplification-audit-2026-05-21/findings.md:72-85` | Determinism, replay, audit kind, and versioning review. Not part of presentation-only wording. |

## Recommended Deferred Changes

These are desirable presentation improvements, but should be deferred to a later output-contract PR if they need generated output changes:

1. Rename default HTML summary tiles from implementation counts to user concepts such as `Final number`, `Source rows`, `Row impact`, and `Check status`.
2. Rename or add display aliases for reconciliation outcomes such as `matched` to `same`, `mismatch` to `changed`, and `missing_right` to `missing from comparison file`.
3. Move `Tokens`, `Audit Entries`, token IDs, hash data, and manifest internals out of the HTML default tab.
4. Rename `Contribution invariant` to a user-facing check label while preserving precise technical-detail wording.
5. Replace report-adjacent `sidecar` wording in generated artifacts with `companion artifact` if the ZIP/report contract is explicitly reviewed.

## Implementation Gate

Before any wording implementation PR changes report or artifact text, require all of the following:

1. Identify whether the target string is docs-only, template-rendered, fixture-rendered, serialized JSON, CSV, manifest, ZIP entry, audit kind, public API, or CLI text.
2. If the string appears in `audit/report/html.go`, `reconcile/report/report.go`, generated example artifacts, or any golden file, treat it as output-contract work.
3. Preserve or explicitly review the distinction between audit bundle verify/replay integrity and reconciliation/business correctness.
4. Include targeted golden or exact-output tests for any changed generated output.
5. Keep serialized field names, audit kinds, public APIs, and CLI verbs unchanged unless the PR is explicitly scoped as a versioned migration.
