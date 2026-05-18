# Extract Backend: Structured Files

Status: **v0**

> Pure-Go backend for selector-first CSV, XLS/XLSX, JSON, and JSONL source readers.

## Connection

- Input is a file/blob reference resolved through the runtime `BlobStore` or a bundle capture reference.
- No S3, Postgres, or external document service is required for v0.
- Supported inputs: `text/csv`, `application/vnd.ms-excel`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, `application/json`, `application/x-ndjson`, `application/jsonl`.

## Schema mapping

| Tool | Upstream shape | Canonical output |
|---|---|---|
| `extract.spreadsheet` | workbook → sheet/range/cells | rows/cells with absolute sheet/cell locators |
| `extract.csv_rows` | CSV rows | row windows with absolute row/column locators |
| `extract.json` | JSON DOM | selector-resolved nodes with pointer/path/span locators |
| `extract.jsonl` | line-delimited / streamed JSON | line windows with pointer/path/span locators |

Rules:

- selection is explicit and deterministic
- sparse indexes are used only to narrow large reads; they never change ordering
- ambiguous selectors, duplicate keys, byte-span mismatches, or unsupported encodings fail closed
- spreadsheet formula handling is cached-value only by default; any other policy must be pinned and deterministic

## Captured reads

Captured payloads are bundle/BlobStore references, not mandatory S3 objects.

Captured metadata includes:
- source bytes hash
- parser/version string
- selector and pagination cursor
- sparse index metadata where applicable
- normalized output payload
- source locator echo (`sheet`, `range`, `row`, `column`, `json_pointer`, `line`, `byte_span`)

Replay returns the captured normalized output and does not depend on live source drift.

## Rate limits and retries

No upstream throttling contract exists; parse failures are local and fail closed. Retry is only for transient BlobStore/bundle resolution failures.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `extract.unsupported_format` | Not one of the supported structured file formats | Abort |
| `extract.malformed_csv` | Invalid CSV quoting / delimiter / encoding | Abort |
| `extract.invalid_json` | Invalid JSON or duplicate-key violation | Abort |
| `extract.invalid_jsonl` | Invalid JSONL line or byte span | Abort |
| `extract.ambiguous_selector` | Selector matches multiple plausible targets | Abort |
| `extract.byte_range_mismatch` | Byte span does not match the selected slice | Abort |
| `extract.formula_unsupported` | Spreadsheet formula policy cannot be satisfied deterministically | Abort or fall back to cached-only mode |
| `extract.unsupported_encoding` | Encoding is not pinned or not supported | Abort |

## Tests

- Positive and negative schema fixtures for each tool.
- Spreadsheet: merged cells, hidden rows, table-name selection, range windows, formula cache policy, fail-closed ambiguity.
- CSV: delimiter/quote/encoding edge cases, large row windows, deterministic chunking.
- JSON: pointer/path resolution, byte-span checks, duplicate-key rejection, sparse-index replay.
- JSONL: line windows, large-stream ordering, deterministic pagination.
- Replay: captured reads reproduce identical slices and token minting.

## Configuration

```yaml
backend: structured_files
config:
  csv:
    delimiter: ","
    header_row: 1
    encoding: "utf-8"
  spreadsheet:
    formula_policy: "cached_only"
    hidden_rows: "exclude"
  json:
    selector_mode: "json_pointer"
  jsonl:
    chunk_size: 500
```

## References

- [`tools/extract/README.md`](../README.md)
- [`subsystems/tools.md`](../../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../../subsystems/kernel.md)
