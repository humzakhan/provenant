# Phase 2 work packets - sequential by default

Phase 2 is a same-worktree plan first. Execute the packets in order, one at a time, in the same worktree. Parallel work is allowed only for read-only planning/review, or when each packet is isolated in its own git worktree. Keep each packet narrow; Phase 2 expands reader coverage on the same kernel and does not widen the proof, audit, or access-surface boundary.

## PR 1 / Packet 1 - selector/capture contract delta
**What the PR should do:** define the smallest shared selector/capture contract Phase 2 readers need before any new reader lands.

**What it should build:** minimal additive selector fields, locator echoes, window/cursor metadata, and shared fixture helpers needed by `extract.json`, `extract.jsonl`, `extract.csv_rows`, and `extract.spreadsheet` without changing mint/audit/replay semantics.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new reader surfaces beyond the structured-files lane; no CLI/runtime/API work; no proof/audit semantic changes; no persistence.

**Pass criteria / validation:** shared types compile; existing Phase 1 end-to-end behavior remains unchanged; the contract can express planned selectors, locator echoes, and pagination/window metadata; ambiguous or unsupported selector forms are rejected deterministically.

**Commit/PR boundary:** first. Land one small conventional commit in one PR; do not start Packet 2 until this PR is merged or otherwise accepted.

## PR 2 / Packet 2 - `extract.json` contract completion
**What the PR should do:** finish `extract.json` to the declared structured-files contract on the existing kernel.

**What it should build:** canonical JSON Pointer selection, unique `jsonpath` resolution when supplied, byte-span matching when declared, richer locator/output fields (`json_pointer`, `jsonpath`, byte bounds, raw/normalized value, source hash), and fail-closed selector/span mismatch behavior.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no `extract.jsonl`, `extract.csv_rows`, or `extract.spreadsheet`; no broad query-language surface beyond unique selector resolution; no mint/audit/replay changes.

**Pass criteria / validation:** positive fixtures cover scalar, object, and array slices with stable ordering; negative fixtures cover duplicate keys, missing selectors, non-unique path resolution, and pointer/path/span mismatch; replay and captured-read assertions remain deterministic.

**Commit/PR boundary:** second, after Packet 1. Land one small conventional-commit PR that changes only the JSON reader lane.

## PR 3 / Packet 3 - `extract.jsonl`
**What the PR should do:** add the bounded JSONL reader on the same selector/capture contract.

**What it should build:** `extract.jsonl` line-window selection, optional per-record pointer/path projection, deterministic pagination cursoring, sparse-index capture metadata, and replay coverage for large-stream ordering.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-io-performance`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** source bytes only; no filesystem/network streaming dependency; no CSV/spreadsheet work in this packet; no proof/audit semantic changes.

**Pass criteria / validation:** positive fixtures cover line windows, pagination, and deterministic record order; negative fixtures cover malformed lines, invalid spans, and ambiguous selector/path requests; replay reproduces the same records, locators, and captured metadata.

**Commit/PR boundary:** third. Land one small conventional-commit PR dedicated to JSONL read/capture behavior.

## PR 4 / Packet 4 - `extract.csv_rows`
**What the PR should do:** add the CSV row-window reader with pinned parse options and row/column provenance.

**What it should build:** `extract.csv_rows` row windows, optional `header_row`, optional `column_projection`, declared dialect/encoding handling, deterministic `next_cursor`, and output rows/cells with absolute row/column locators, `raw_value`, `normalized_value`, and `source_hash`.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-io-performance`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no spreadsheet implementation in this packet; no delimiter or encoding inference that can silently widen behavior; no proof/audit/replay changes.

**Pass criteria / validation:** positive fixtures cover bounded row windows, header echo, column projection, and deterministic pagination; negative fixtures cover malformed CSV, ambiguous dialect/encoding, and selector/window mismatches; replay preserves row order and locator fidelity.

**Commit/PR boundary:** fourth. Land one small conventional-commit PR that changes only the CSV reader lane plus its focused fixtures/tests.

## PR 5 / Packet 5 - `extract.spreadsheet`
**What the PR should do:** add the spreadsheet reader with deterministic selector policy and cached-only formula handling.

**What it should build:** `extract.spreadsheet` sheet/range selectors, optional `table_name`, optional `row_window`, optional `column_projection`, cached-only formula policy, and row/cell outputs with absolute coordinates and provenance metadata.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no OCR/PDF/unstructured document work; no live workbook service dependency; no non-deterministic formula evaluation; no proof/audit/replay changes.

**Pass criteria / validation:** positive fixtures cover sheet/range/table selection, window/projection behavior, and cached-formula handling; negative fixtures cover ambiguous sheet/table/range resolution and unsupported formula-policy cases; replay preserves absolute coordinates and deterministic output ordering.

**Commit/PR boundary:** fifth. Land one small conventional-commit PR limited to spreadsheet read/capture behavior and its focused fixtures/tests.

## PR 6 / Packet 6 - expanded fixture/replay matrix and doc alignment
**What the PR should do:** close Phase 2 by expanding the conformance matrix and aligning docs to the shipped reader behavior.

**What it should build:** the shared per-reader positive/negative fixture matrix, replay verification across all Phase 2 readers, and the minimal doc updates needed so Phase 2 guidance and architecture notes match the implemented selectors, locators, and fail-closed behavior.

**Skills/guidance to load:** `engineering-workflow`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new reader capabilities beyond what Packets 1 through 5 already landed; no late feature additions; no proof/audit semantic changes.

**Pass criteria / validation:** each reader has focused positive and negative conformance coverage; replay reproduces deterministic captured reads across the full Phase 2 fixture set; docs reflect the implemented reader set, boundaries, and any explicitly deferred work.

**Commit/PR boundary:** last. Land one final small conventional-commit PR for the matrix/doc-alignment pass only.
