# Packet 01 - Reader Router

## Goal

Implement the deterministic bridge from the validated Packet 00 request into existing kernel structured readers. This packet proves that supported files can be loaded, hashed, routed, read, and normalized into a stable intermediate form for the reconciliation engine.

## Suggested package shape

Use a small harness-owned package, for example:

```text
reconcile/
├── router.go
├── load.go
├── normalize.go
├── types.go
└── router_test.go
```

If the repo already has a better harness package location when implementation starts, use the existing local pattern.

## Technical tasks

- Accept the validated Packet 00 `Request` and iterate files in request order.
- Open each configured path with bounded reads and compute a stable source hash before reader dispatch.
- Resolve the reader call from the validated `format` and `reader` pair:
  - `json` -> `extract.json`
  - `jsonl` -> `extract.jsonl`
  - `csv` -> `extract.csv_rows`
  - `xlsx` -> `extract.spreadsheet`
- Translate request selector config into the exact existing reader options required by each kernel reader.
- Execute the reader once per file with no retry or fallback behavior.
- Normalize reader output into a deterministic row/value structure that Packet 02 can consume without reader-specific branching.
- Preserve source provenance needed later for minting:
  - file ID
  - source path
  - source hash
  - selector
  - reader name
  - reader version if exposed
  - normalized output hash
- Fail closed on unsupported selectors, missing files, empty required reads, oversized inputs, and source-hash mismatches.

## Interfaces and contracts

Expose a narrow interface such as:

```go
type Router interface {
    Route(ctx context.Context, req Request) ([]NormalizedFile, error)
}
```

Suggested output types:

```go
type NormalizedFile struct {
    FileID         string
    Purpose        string
    Format         string
    Reader         string
    SourcePath     string
    SourceHash     string
    Selector       NormalizedSelector
    OutputHash     string
    Rows           []NormalizedRow
}

type NormalizedRow struct {
    RowKey   string
    Position RowPosition
    Values   map[string]NormalizedValue
}

type NormalizedValue struct {
    LogicalField string
    Role         string
    Raw          string
    Canonical    string
    Empty        bool
}
```

Contract rules:

- `Rows` ordering must be stable and match reader output order after selector application.
- `Values` keys must be logical field names from Packet 00 config, not raw source headers.
- Missing optional fields may be empty; missing required fields must surface as deterministic errors.
- The router may canonicalize obvious transport details needed for comparison, but it must not perform reconciliation decisions or token minting.

## Input and output boundaries

Input:

- the strict Packet 00 request
- supported local files only
- existing kernel readers only

Output:

- one `NormalizedFile` per configured file
- deterministic metadata for source hashing and downstream provenance
- stable normalized rows and values for Packet 02

Handoff to Packet 02:

- Packet 02 consumes only the normalized file set plus the Packet 00 reconcile config.
- Packet 02 must not reopen files or call readers again.
- Any reader-specific branching must end in this packet.

## Validation and tests

Add focused tests for:

- valid CSV routing with header and row-window selectors
- valid XLSX routing with workbook, sheet, and range selectors
- valid JSON routing with object/array selectors supported by the existing reader
- valid JSONL routing with stable line ordering
- stable source hash for the same fixture bytes across repeated runs
- stable output hash for the same normalized output across repeated runs
- unsupported format/reader pair
- selector invalid for the chosen reader
- missing file path
- oversized file
- empty required read result
- source bytes changed between hash check and read execution if the implementation verifies both points
- normalized logical fields mapped correctly from source columns

Expected packet validation:

```bash
rtk go test ./reconcile/...
rtk go test ./...
rtk proxy git diff --check
```

Adjust the focused package path if implementation lands elsewhere.

## Skills/guidance to load

- `engineering-workflow`
- `provenant-source-reader-provenance`
- `go-input-validation-security`
- `go-io-performance`
- `go-testing-fuzzing`
- `conventional-commit-guard` before commit messages and PR creation

## Non-goals

- No reconciliation matching or result classification.
- No token minting, bundle snapshot, or report generation.
- No new parser semantics, reader heuristics, or file-type expansion.
- No automatic column guessing or business-role inference.
- No caching, persistence, background jobs, or service runtime.

## Stop conditions

Stop and replan if:

- a requested file type is unsupported by current readers
- a selector shape cannot be expressed through existing reader options
- normalization would require changing kernel reader semantics
- a required downstream field cannot be represented deterministically from reader output alone

## Done when

- fixture CSV, XLSX, JSON, and JSONL files route to the expected readers
- normalized output is deterministic across repeated runs
- routed files carry stable source metadata needed for later provenance
- bad format/reader combinations and selector errors fail closed
- Packet 02 can consume the normalized output without any reader-specific code

## Commit/PR boundary

One PR limited to reader routing and fixture coverage.
