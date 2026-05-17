# Extract Backend: AWS Textract

Status: **v0 / reference backend**

> AWS Textract is the v0 reference backend for the extract category. It covers bank statements, invoices, receipts, and generic key-value extraction with one of the most mature confidence-scoring APIs in the document AI space.

## Why Textract first

1. **Maturity**: Textract has been GA since 2019 with strong financial-document coverage.
2. **Confidence native**: Textract returns per-field confidence scores natively, calibrated reasonably out of the box.
3. **AnalyzeExpense** and **AnalyzeID** preset modes simplify invoice and receipt extraction.
4. **AWS hosting**: customers already on AWS get a low-friction integration.

Azure DI follows in v0.5 for customers on Azure or with Microsoft-leaning IT.

## Connection

```yaml
backend: textract
config:
  region: "us-east-1"
  credentials:
    mode: "iam_role"        # iam_role | static
    role_arn: "arn:aws:iam::123456789012:role/provenant-textract"
  bucket: "provenant-acme-docs"
  features:
    - "TABLES"
    - "FORMS"
    - "QUERIES"             # for kv_pairs
    - "SIGNATURES"
```

Documents are uploaded to the configured bucket; Textract pulls from there. The backend handles upload, polling for async results, and download.

## Schema mapping

Textract returns `Block` objects (PAGE, LINE, WORD, TABLE, CELL, KEY_VALUE_SET, FORM). The backend interprets these per tool:

| Tool | Textract feature | Mapping |
|---|---|---|
| `bank_statement` | TABLES + FORMS | Table rows → transactions; FORMs → opening/closing balance, account number, period |
| `invoice` | AnalyzeExpense + QUERIES for custom fields | Expense fields → invoice header + line items |
| `receipt` | AnalyzeExpense | Expense fields → receipt total + line items |
| `kv_pairs` | QUERIES with the schema's field names as queries | Query results → kv map |

The mapping logic is in `internal/tools/extract/backends/textract/mapping.go` with extensive comments on Textract's quirks.

## Confidence calibration

Textract's raw confidence is in [0, 100]. The backend divides by 100 to normalize to [0, 1], then applies a per-tool calibration map computed from the case bank. Current calibration (recomputed quarterly):

| Tool | Raw confidence threshold for "likely correct" |
|---|---|
| `bank_statement` numeric fields | 0.92 → calibrated 0.85 |
| `invoice` totals | 0.94 → calibrated 0.85 |
| `receipt` totals | 0.88 → calibrated 0.85 |

Calibration is documented in `docs/calibration/textract-<quarter>.md` and audited as part of the case bank curator process.

## Captured reads

Convention:
```
extract:textract:<tool>:<sha256_of_input_url+config>
```

Captured payload includes the raw Textract response plus the mapped canonical response. Replay returns the canonical response without re-invoking Textract.

## Rate limits and retries

Textract has region-level limits (default 5 TPS for sync; 10 concurrent for async). The backend uses async APIs for documents >5 pages, sync for smaller. A token bucket smooths bursts; retries with exponential backoff on transient errors.

## Failure modes

| Backend error | Surfaced as |
|---|---|
| Document not in S3 | `extract.document_not_found` |
| Unsupported format (DOCX, image type Textract can't read) | `extract.unsupported_format` |
| Async job timeout (>15 min) | `extract.upstream_timeout` |
| AWS throttling exhaustion | `extract.upstream_unavailable` (with backoff) |
| Textract returned 0 blocks | `extract.empty_page` |

## Tests

- Integration: fixture statements / invoices / receipts produce expected canonical outputs.
- Mapping: Textract response shapes with known quirks (merged cells, multi-column statements) handled correctly.
- Calibration: confidence/correctness correlation r > 0.7 on the active case bank.
- Replay: captured reads enable bit-identical replay.

## Open questions

1. **Textract Queries cost.** QUERIES features are expensive per page. For `kv_pairs`, the cost can dominate the tool. Proposal: surface cost in the tool response so skill packs can budget.
2. **Custom adapters for known statement formats.** Some banks' statements (e.g., First National's PDF layout) benefit from a custom post-processor. Proposal: a `format_hints` config option that routes through a known-good adapter; defer to v0.5.

## Configuration example for recon

```yaml
backend: textract
config:
  region: "us-east-1"
  credentials:
    mode: "iam_role"
    role_arn: "arn:aws:iam::123456789012:role/provenant-textract"
  bucket: "provenant-acme-docs"
  features: ["TABLES", "FORMS"]
  retries:
    max_attempts: 3
    initial_backoff_ms: 200
  timeout_seconds: 90
```

## References

- [`tools/extract/README.md`](../README.md) — category contract
- AWS Textract docs: https://docs.aws.amazon.com/textract/
