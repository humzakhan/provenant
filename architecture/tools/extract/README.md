# Tool Category: Extract

Status: **v0**

> Extract structured data from financial documents (bank statements, invoices, receipts, contracts, board packs) with explicit confidence scores. The kernel uses extraction confidence to route low-confidence cases to HITL and to gate `submit_answer` via the `confidence.extraction_min` validator.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md)
- An OCR / document-AI backend (Textract, Azure DI, etc.)

## Depended on by
- `recon` skill pack
- `ap_coding` skill pack
- `expense_review` skill pack
- `sanctions_disposition` skill pack (for KYC document evidence)

## Purpose

Financial workflows almost always start with a document: a bank statement, a vendor invoice, an expense receipt, a 10-K. The "raw text" of the document is not the unit of value — the *structured fields with confidence* are. An extracted bank statement is a list of transactions with amounts, dates, descriptions, and a per-line confidence score reflecting OCR uncertainty and field-classifier uncertainty.

Provenant treats extraction as a tool category — not the kernel's job — because document AI evolves rapidly and the right backend depends on the document type, language, and customer preference. The category contract is stable; backends change.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `extract.bank_statement` | Statement → list of transactions with confidence | NonDeterministic |
| `extract.invoice` | Invoice → header + line items with confidence | NonDeterministic |
| `extract.receipt` | Receipt → merchant + total + line items with confidence | NonDeterministic |
| `extract.kv_pairs` | Generic key-value extraction with a schema | NonDeterministic |

Extraction is `NonDeterministic` because the underlying models (Textract, Azure DI, custom) return slightly different confidence scores across calls. For replay determinism, the kernel **captures the extraction response** and serves it from the capture on replay. This makes the *outcome* deterministic even though the *primitive* is not. See [`subsystems/tools.md`](../../subsystems/tools.md) for the captured-read convention.

## Schemas

### `extract.bank_statement`

Input:
```json
{
  "document_url": "s3://uploads/run_01HXYZ/statement.pdf",
  "expected_account_id": "1010",
  "expected_period": {
    "start": "2026-05-01",
    "end": "2026-05-31"
  },
  "language": "en"
}
```

Output:
```json
{
  "document_url": "s3://...",
  "page_count": 4,
  "account_number_extracted": "****1234",
  "account_number_confidence": 0.98,
  "statement_period": {
    "start": "2026-05-01",
    "end": "2026-05-31",
    "confidence": 1.00
  },
  "opening_balance": {
    "value": "1199423.18",
    "currency": "USD",
    "confidence": 0.99
  },
  "closing_balance": {
    "value": "1247322.18",
    "currency": "USD",
    "confidence": 0.99
  },
  "transactions": [
    {
      "line": 1,
      "date": "2026-05-02",
      "description": "DEPOSIT - ACME CORP",
      "amount": "1240.00",
      "currency": "USD",
      "sign": "credit",
      "confidence": 0.94,
      "field_confidences": {
        "date": 0.99,
        "amount": 0.98,
        "description": 0.91
      },
      "bbox": [[123, 245], [589, 267]],
      "page": 1
    },
    ...
  ]
}
```

Mintable paths:

- `$.opening_balance.value` (currency from `$.opening_balance.currency`)
- `$.closing_balance.value` (currency from `$.closing_balance.currency`)
- `$.transactions[*].amount` (currency from sibling, as_of from `$.transactions[*].date`)

### `extract.invoice`

Input: `{document_url, language}`. Output: vendor, invoice number, date, line items, totals — each with confidence.

### `extract.receipt`

Input: `{document_url, language, expected_merchant_hint?}`. Output: merchant, date, total, line items — each with confidence.

### `extract.kv_pairs`

Input: `{document_url, schema: {field_name: type}}`. Output: extracted values keyed by `field_name`, each with confidence. Used for ad-hoc extraction (board packs, contracts, custom forms).

## Token semantics

- Every numeric extracted value mints a token.
- Token `source.origin` includes the document URL and page: `s3://...statement.pdf#page=1&line=5&bbox=...`.
- Token `source.path` is the JSONPath into the extraction response.
- The bbox metadata propagates so audit packs can render annotated PDFs.

## Confidence

Three levels of confidence are emitted:

1. **Field confidence**: per individual field (date, amount, description), reflecting OCR and field-classifier uncertainty.
2. **Row/line confidence**: minimum of field confidences within a transaction line.
3. **Document confidence**: minimum of all field confidences, reflecting overall extraction quality.

The `confidence.extraction_min` validator (see [`subsystems/guards.md`](../../subsystems/guards.md)) blocks `submit_answer` when any extracted value used in the answer has confidence below the configured threshold (default 0.85). Such cases route to HITL.

**Calibration.** Confidence scores from raw backends (Textract, Azure DI) are not natively calibrated to mean "this fraction will be correct." The category implementation includes a per-backend calibration map that translates raw scores into calibrated probabilities. Calibration is recomputed quarterly against the case bank.

## Determinism and replay

`NonDeterministic` primitive; deterministic in practice via captured-read replay.

The kernel captures the full extraction response payload to S3. On replay, the captured payload is returned instead of re-running the extraction. This makes case-bank replay reliable even as backend models drift.

Limitations:
- A new document (not in the capture) cannot be replayed; only previously-extracted documents are replayable.
- If a customer demands "rerun extraction with the new model" semantics, that is a separate skill operation, not a replay.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `extract.document_not_found` | URL unreachable | Skill aborts; surface to user |
| `extract.unsupported_format` | Not a supported file type | Skill aborts |
| `extract.empty_page` | Backend returned no content for a page | Pass-through with warning; skill decides |
| `extract.low_confidence` | All fields below threshold (document essentially unusable) | Route to HITL; skill may attempt fallback backend |
| `extract.upstream_unavailable` | Backend network error | Harness retries per policy |
| `extract.unsupported_language` | Document language not configured | Skill aborts |

## Performance budget

| Tool | p50 | p95 |
|---|---|---|
| `bank_statement` (≤4 pages) | 3s | 10s |
| `bank_statement` (>4 pages) | 6s | 20s |
| `invoice` | 2s | 6s |
| `receipt` | 1s | 4s |
| `kv_pairs` | 2s | 8s |

Per-call timeout: 90 seconds (extraction can be slow).

## Backends

| Backend | Status | Doc |
|---|---|---|
| AWS Textract | v0 | [backends/textract.md](backends/textract.md) |
| Azure Document Intelligence | deferred (v0.5) | [backends/azure-di.md](backends/azure-di.md) |

Single backend at v0; multi-backend with consensus selection is a v0.5 enhancement.

## Tests

- Schema conformance per tool.
- Confidence calibration test: backend's reported confidence correlates with case-bank ground-truth correctness above r=0.7.
- Replay: captured extraction payloads produce identical tokens.
- Backend integration: fixture documents (3 statement variants, 5 invoice variants, 5 receipt variants) extracted with known expected outputs.
- Stress: 100-page board pack extracts within budget.

## Open questions

1. **Multi-backend consensus.** Should extraction run two backends and use agreement as a confidence signal? Defer to v0.5; measure first if single-backend confidence is well-calibrated enough.
2. **Per-customer fine-tuning.** Some customers have document templates that benefit from fine-tuning. Out of v0 scope; the bbox + field-by-field response shape leaves room for it later.

## References

- [Anthropic finance agents launch](../../references.md#anthropic-finance-agents) — statement-auditor agent shape
- [FAITH](../../references.md#faith) — tabular hallucination benchmark; relevant when extraction feeds reasoning over tables
- See [`subsystems/guards.md`](../../subsystems/guards.md#post-llm-validators) for the `confidence.extraction_min` validator
