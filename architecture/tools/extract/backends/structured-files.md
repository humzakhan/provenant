# Extract Backend: Structured Files

Status: **v0**

> Pure-Go backend for selector-first CSV, XLSX, JSON, and JSONL source readers.

## Connection

- Input is a file/blob reference resolved through the runtime `BlobStore` or a bundle capture reference.
- No S3, Postgres, or external document service is required for v0.
- Supported inputs: `text/csv`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, `application/json`, `application/x-ndjson`, `application/jsonl`.

## Schema mapping

| Tool | Upstream shape | Canonical output |
|---|---|---|
| `extract.spreadsheet` | workbook → sheet/range/cells | rows/cells with absolute sheet/cell locators |
| `extract.csv_rows` | CSV rows | row windows with absolute row/column locators |
| `extract.json` | JSON DOM | selector-resolved nodes with pointer/path/span locators |
| `extract.jsonl` | line-delimited / streamed JSON | line windows with pointer/path/span locators |

Rules:

- selection is explicit and deterministic
- line/row windows are deterministic and cursor-driven where supported
- ambiguous selectors, duplicate keys, byte-span mismatches, malformed workbook relationships, or unsupported encodings fail closed
- spreadsheet formula handling is cached-value only; live formula evaluation is not supported
- spreadsheet column projection uses absolute A1 column letters and range syntax uses A1 refs
- CSV dialect and encoding are declared, not inferred

## Captured reads

Captured payloads are bundle/BlobStore references, not mandatory S3 objects.

Captured metadata includes:
- source bytes hash
- parser/version string
- selector and pagination cursor
- row/line window metadata where applicable
- normalized output payload
- source locator echo (`sheet`, `range`, `row`, `column`, `json_pointer`, `line`, `byte_span`)

Replay returns the captured normalized output and does not depend on live source drift.

## Rate limits and retries

No upstream throttling contract exists; parse failures are local and fail closed. Retry is only for transient BlobStore/bundle resolution failures.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `extract.unsupported_format` | Not one of the supported structured file formats | Abort |
| `extract.malformed_csv` | Invalid CSV quoting, ragged rows, duplicate projected columns, or invalid bytes under the declared encoding | Abort |
| `extract.invalid_json` | Invalid JSON, duplicate-key violation, or unsupported selector syntax | Abort |
| `extract.invalid_jsonl` | Invalid JSONL line, blank line, or invalid line/byte window | Abort |
| `extract.ambiguous_selector` | Selector matches multiple plausible targets or mixes selector families | Abort |
| `extract.byte_range_mismatch` | Byte span does not match the selected slice | Abort |
| `extract.formula_unsupported` | Spreadsheet formula policy cannot be satisfied deterministically or a formula lacks a cached value | Abort |
| `extract.unsupported_encoding` | Encoding is not pinned or not supported | Abort |

## Tests

- Positive and negative schema fixtures for each tool.
- Spreadsheet: sheet/range/table selection, cached-formula handling, relationship validation, unsupported cell types, and fail-closed ambiguity.
- CSV: declared dialect/encoding edge cases, row windows, pagination cursors, and projection failures.
- JSON: pointer/path resolution, byte-span checks, duplicate-key rejection, and deterministic replay.
- JSONL: line windows, cursors, projection mismatch handling, and deterministic pagination.
- Replay: captured reads reproduce identical slices and token minting.

## Configuration

```yaml
backend: structured_files
config:
  csv:
    header_row: 1
    encoding: "utf-8"
    dialect: "excel"
  spreadsheet:
    formula_policy: "cached_only"
  json:
    selector_mode: "json_pointer"
```

## References

- [`tools/extract/README.md`](../README.md)
- [`subsystems/tools.md`](../../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../../subsystems/kernel.md)
