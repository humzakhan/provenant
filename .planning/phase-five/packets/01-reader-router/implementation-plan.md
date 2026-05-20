# Packet 01 Implementation Plan - Reader Router

## Goal

Implement the deterministic bridge from the validated reconciliation request to existing structured readers. This packet turns configured file declarations into normalized row/value data and source metadata for the reconciliation engine.

## Suggested Package Shape

Use the existing harness location if one exists after Packet 00. Otherwise use a small package shape such as:

```text
reconcile/router/
├── load.go
├── normalize.go
├── router.go
├── selectors.go
├── types.go
└── router_test.go
```

Suggested responsibilities:

- `types.go`: normalized file, row, value, selector, hash, and routing result types
- `load.go`: local file open, size limit, read, and source hash calculation
- `selectors.go`: conversion from Packet 00 selector config to reader options
- `router.go`: format/reader dispatch and reader invocation
- `normalize.go`: conversion from reader output into logical fields from config
- `router_test.go`: fixture-backed routing and fail-closed coverage

## Implementation Sequence

1. Inspect existing source reader packages and their option/result types.
2. Define a router API that accepts the validated Packet 00 request and returns normalized files.
3. Implement deterministic path resolution contract, preferably receiving a base directory from the caller rather than using process working directory implicitly.
4. Add bounded local file loading and source hash calculation before reader dispatch.
5. Add an allowlisted dispatch table for:
   - `csv` with `extract.csv_rows`
   - `xlsx` with `extract.spreadsheet`
   - `json` with `extract.json`
   - `jsonl` with `extract.jsonl`
6. Translate configured selectors into existing reader options.
7. Invoke each reader exactly once per configured file.
8. Normalize reader output into logical fields keyed by Packet 00 column names.
9. Preserve source metadata needed by Packet 03 provenance binding.
10. Add deterministic error types and table-driven tests.

## Router Contract

The router should expose a narrow entrypoint close to:

```go
func Route(ctx context.Context, req request.Request, opts Options) ([]NormalizedFile, error)
```

Suggested `Options`:

- base directory for request-relative paths
- max file bytes
- optional reader registry for tests

Suggested output:

```text
NormalizedFile
  FileID
  Purpose
  Format
  Reader
  SourcePath
  SourceHash
  Selector
  OutputHash
  Rows

NormalizedRow
  RowKey
  Position
  Values

NormalizedValue
  LogicalField
  Role
  Source
  Raw
  Canonical
  Empty
```

Rules:

- row ordering follows selected reader output order
- logical field names come from config, not source headers
- source hashes are over exact input bytes
- output hashes are over canonical normalized output
- missing required values fail here after reader output is known
- optional missing values are represented explicitly, not omitted by accident

## Selector Handling

Implement selector conversion per format:

- CSV: encoding, dialect, header row, and row window
- XLSX: workbook sheet/table/range/header row
- JSON: configured object/array path supported by the existing reader
- JSONL: line window and field mapping supported by the existing reader

Do not add fallback discovery. If selector metadata is incomplete or cannot map to existing reader options, fail with a stable selector error.

## Error Cases

Use stable error categories such as:

- `input_not_found`
- `input_too_large`
- `reader_unsupported`
- `selector_invalid`
- `reader_failed`
- `required_value_missing`
- `normalization_failed`

Include file id, logical field, and selector path where available. Avoid absolute temp paths in golden-testable messages.

## Tests

Add fixture tests for:

- valid CSV routing
- valid XLSX routing
- valid JSON routing
- valid JSONL routing
- stable source hash
- stable output hash
- missing file
- oversized file
- invalid selector
- unsupported reader registry entry
- missing required source column
- missing required value in selected row
- optional field omitted or empty
- normalized logical fields match configured mappings

## Validation

Run:

```bash
rtk go test ./...
rtk proxy git diff --check
```

If the router lands in a focused package, also run the package test path directly.

## Stop Conditions

Stop if reader routing requires new parser semantics, automatic column discovery, fuzzy selector matching, persistent storage, or kernel reader API expansion.

## Handoff

Packet 02 consumes the normalized files and must not reopen files, re-run readers, or perform reader-specific branching.
