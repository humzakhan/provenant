# Tool Category: Extract

Status: **v0**

> Extract structured data from financial documents and structured source files with explicit confidence, provenance, and replay capture.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)
- OCR / document-AI backends for unstructured docs
- A pure-Go structured-files backend for CSV/XLS/XLSX/JSON/JSONL readers

## Depended on by
- `recon` skill pack
- `ap_coding` skill pack
- `expense_review` skill pack
- `sanctions_disposition` skill pack (for KYC document evidence)

## Purpose

Financial workflows often start with a document: a bank statement, a vendor invoice, an expense receipt, a 10-K. The raw text is not the unit of value — the structured fields with confidence are. Provenant keeps that as a category contract so document AI can evolve without changing the kernel surface.

The same category also covers structured source readers for spreadsheets, CSV, JSON, and JSONL. Those readers are selector-first, streaming/windowed when needed, and deterministic on replay when the captured bytes and parser/options are pinned. PDFs stay out of this slice.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `extract.bank_statement` | Statement → transactions with confidence | NonDeterministic |
| `extract.invoice` | Invoice → header + line items with confidence | NonDeterministic |
| `extract.receipt` | Receipt → merchant + total + line items with confidence | NonDeterministic |
| `extract.kv_pairs` | Generic key-value extraction with a schema | NonDeterministic |
| `extract.spreadsheet` | XLS/XLSX sheet/range/cell reader | CapturedDeterministic |
| `extract.csv_rows` | CSV / large CSV windowed row reader | CapturedDeterministic |
| `extract.json` | JSON pointer/path reader for exact object/array slices | CapturedDeterministic |
| `extract.jsonl` | JSONL / large JSON streaming reader | CapturedDeterministic |

## Schemas

Each tool publishes JSON Schema for its inputs and outputs. The examples below show the canonical field shapes; selector fields, provenance locators, and raw/normalized values are part of the published contract.

## Document-AI readers

These tools stay confidence-first and non-deterministic because they rely on OCR / document-AI backends. Replay captures the full response payload and serves it back on replay.

### `extract.bank_statement`

Input:
```json
{
  "document_url": "s3://uploads/run_01HXYZ/statement.pdf",
  "expected_account_id": "1010",
  "expected_period": {"start": "2026-05-01", "end": "2026-05-31"},
  "language": "en"
}
```

Output: account number, statement period, opening/closing balance, transactions, per-field confidence, and page/bbox locators.

Mintable paths:
- `$.opening_balance.value`
- `$.closing_balance.value`
- `$.transactions[*].amount`

### `extract.invoice`

Input: `{document_url, language}`. Output: vendor, invoice number, date, line items, totals — each with confidence.

### `extract.receipt`

Input: `{document_url, language, expected_merchant_hint?}`. Output: merchant, date, total, line items — each with confidence.

### `extract.kv_pairs`

Input: `{document_url, schema: {field_name: type}}`. Output: extracted values keyed by `field_name`, each with confidence.

## Structured source readers

Common rules:
- selector-first reads; no scan-everything default
- windowed / chunked pagination for large inputs
- deterministic ordering by source order, then byte/cell order
- fail closed on ambiguity (delimiter, sheet, selector, key, byte span)
- capture source bytes + parser/version/options + normalized output in the bundle/BlobStore reference, not as an S3 requirement
- `source.origin` carries the physical locator; `source.path` stays JSONPath into the tool output
- every selected scalar carries `raw_value`, `normalized_value`, and `source_hash`

### `extract.spreadsheet`

Reads XLS/XLSX sheets, ranges, and windows.

Input selectors: `sheet`, `range`, optional `table_name`, optional `row_window`, optional `column_projection`, optional `formula_policy` (`cached_only` or `fail_closed`).

Origin examples:
- `file:///book.xlsx#sheet=TrialBalance!R12C7`
- `s3://tenant/run/book.xlsx#sheet=TrialBalance&range=A1:D500`

Output rows/cells include absolute coordinates (`sheet`, `row`, `column`, `a1`), `raw_value`, `normalized_value`, `source_hash`, and formula/cache metadata when present.

### `extract.csv_rows`

Reads CSV in bounded row windows.

Input selectors: `row_window`, optional `column_projection`, optional `header_row`, declared `dialect` / `encoding`.

Origin examples:
- `file:///gl.csv#row=1042&col=7&header=amount`
- `blob://run-01HXYZ/gl.csv#row=1-500`

Output rows include absolute row/column coordinates, header echo, `raw_value`, `normalized_value`, `source_hash`, and `next_cursor` when chunked.

### `extract.json`

Reads exact JSON objects/arrays by selector.

Input selectors: `json_pointer` (canonical), optional `jsonpath` when it resolves uniquely, optional `byte_span`.

Origin examples:
- `file:///invoice.json#ptr=/invoice/total&byte=123-136`
- `blob://run-01HXYZ/invoice.json#ptr=/items/41/amount`

Output nodes preserve document order and include `json_pointer`, `jsonpath`, `byte_start`, `byte_end`, `raw_value`, `normalized_value`, and `source_hash`.

### `extract.jsonl`

Reads JSONL and large JSON streams with sparse indexing and line windows.

Input selectors: `line_window`, optional `json_pointer` / `jsonpath`, optional `byte_span`.

Origin examples:
- `file:///batch.jsonl#line=482&byte=88421-88502`
- `blob://run-01HXYZ/batch.jsonl#line=1-1000`

Output records include line number, pointer/path, byte span, `raw_value`, `normalized_value`, `source_hash`, and deterministic record order.

## Token semantics

- Numeric selected values mint tokens from the normalized value.
- `source.path` is the JSONPath into the tool output slice.
- `source.origin` is the physical locator (`sheet/range`, `row/col`, `json_pointer/byte_span`, `line/byte_span`).
- `source_value_raw` preserves the exact lexeme after canonical normalization rules.

## Confidence

- Document-AI tools emit field/row/document confidence.
- Structured readers default to `confidence = 1.0` for strict parses.
- Any inferred selector, boundary, or fallback path must either fail closed or emit an explicit lower confidence.

## Determinism and replay

- Document-AI tools are `NonDeterministic`; replay captures the full response payload.
- Structured readers are `CapturedDeterministic`; replay captures source bytes, parser version, options, selector, sparse index metadata, and normalized output.
- Replay must preserve byte-identical deterministic portions and deterministic ordering.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `extract.document_not_found` | URL unreachable | Abort |
| `extract.unsupported_format` | Not a supported file type | Abort |
| `extract.malformed_csv` | Bad dialect, quoting, or encoding | Abort |
| `extract.invalid_json` | Malformed JSON / duplicate keys / invalid UTF-8 | Abort |
| `extract.invalid_jsonl` | Malformed JSONL line or span | Abort |
| `extract.ambiguous_selector` | Multiple plausible matches or inferred selector is not unique | Abort |
| `extract.byte_range_mismatch` | Byte span disagrees with the parsed locator | Abort |
| `extract.formula_unsupported` | Spreadsheet formula policy is not pinned | Abort or use cached-only mode |
| `extract.low_confidence` | All fields below threshold | Route to HITL |
| `extract.upstream_unavailable` | Backend/network error | Retry per policy |
| `extract.unsupported_language` | Language not configured | Abort |

## Performance budget

| Tool | p50 | p95 |
|---|---|---|
| `bank_statement` (≤4 pages) | 3s | 10s |
| `bank_statement` (>4 pages) | 6s | 20s |
| `invoice` | 2s | 6s |
| `receipt` | 1s | 4s |
| `kv_pairs` | 2s | 8s |
| `spreadsheet` | 200ms | 1s |
| `csv_rows` | 100ms | 800ms |
| `json` | 50ms | 400ms |
| `jsonl` | 75ms | 600ms |

Per-call timeout: 90 seconds.

## Backends

| Backend | Status | Doc |
|---|---|---|
| AWS Textract | v0 | [backends/textract.md](backends/textract.md) |
| Structured files (CSV/XLS/XLSX/JSON/JSONL) | v0 | [backends/structured-files.md](backends/structured-files.md) |
| Azure Document Intelligence | deferred (v0.5) | [backends/azure-di.md](backends/azure-di.md) |

Unstructured docs use Textract; structured source readers use the pure-Go structured-files backend. Azure DI remains the deferred parity backend.

## Tests

- Schema conformance per tool (≥5 positive + ≥5 negative fixtures each).
- Confidence calibration for document-AI tools.
- Replay: captured payloads / source bytes produce identical tokens and output slices.
- Spreadsheet fixtures: sheet/range selection, merged headers, hidden rows, formula cache policy, ambiguity fail-closed.
- CSV fixtures: delimiter / encoding ambiguity, row-window pagination, row/column provenance.
- JSON fixtures: duplicate keys, pointer/path/byte-span mismatch, selector uniqueness, sparse-index replay.
- JSONL fixtures: line-window pagination, large-stream order, deterministic chunking.
- Backend integration: fixture documents and structured files extracted with known expected outputs.

## Open questions

1. **Multi-backend consensus.** Should extraction run two backends and use agreement as a confidence signal? Defer to v0.5; measure first if single-backend confidence is well-calibrated enough.
2. **Per-customer fine-tuning.** Some customers have document templates that benefit from fine-tuning. Out of v0 scope; the bbox + field-by-field response shape leaves room for it later.

## References

- [Anthropic finance agents launch](../../references.md#anthropic-finance-agents) — statement-auditor agent shape
- [FAITH](../../references.md#faith) — tabular hallucination benchmark; relevant when extraction feeds reasoning over tables
- See [`subsystems/guards.md`](../../subsystems/guards.md#post-llm-validators) for the `confidence.extraction_min` validator
