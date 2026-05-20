# Phase 5 Extension Plan - Deterministic Reconciliation Harness

## Goal

Build the deterministic harness contract and implementation path needed for a caller to pass supported structured files, describe their business roles, and receive an understandable reconciliation output with a complete audited evidence chain.

By the end of this extension, Provenant should support a harness-facing reconciliation flow that:

- accepts multiple supported files through explicit configuration
- records each file purpose, format, reader, selector, and column role
- routes each file to the correct kernel reader
- reconciles records through deterministic modes
- mints provenance tokens for configured evidence fields
- emits a report package with bundle, audit chain, captured reads, tokens, mismatches, and manifest
- fails closed when config, file shape, selectors, column mapping, or reconciliation assumptions are ambiguous

## Product Boundary

The harness may be intelligent in how it selects already-declared kernel tools from explicit config, but it must not infer business meaning from arbitrary files. The first implementation is deterministic and config-driven.

The user supplies a request that says:

- what each file is for
- what format each file uses
- which reader should parse it, or enough format metadata for deterministic routing
- which rows/ranges/tables are in scope
- which columns are keys, amounts, currency, dates, references, entities, status fields, or ignored fields
- which reconciliation mode to run
- what tolerance and uniqueness rules apply

The harness validates the request, calls kernel readers, mints evidence through the kernel, runs reconciliation, snapshots the bundle, verifies it, and packages report artifacts.

## Supported Reconciliation Families

Phase 5 should support these deterministic families first:

1. Exact match reconciliation: join records by configured key fields and report matched, missing, duplicate, and extra records.
2. Amount tolerance reconciliation: compare configured numeric fields with explicit tolerance and currency rules.
3. Existence and completeness reconciliation: prove every required record in one file exists in another file or report absences.
4. One-to-many and many-to-one reconciliation: compare one configured parent record to a configured group of child records, with summed amount checks.
5. Schema and value validation reconciliation: validate required columns, duplicate keys, empty keys, amount/date/currency parsing, and allowed-value constraints before matching.

## Required Config Shape

The request format should be explicit. A representative shape:

```json
{
  "run": {
    "id": "run_recon_001",
    "tenant_id": "tenant_1"
  },
  "files": [
    {
      "id": "bank",
      "purpose": "bank_statement",
      "format": "csv",
      "path": "bank.csv",
      "reader": "extract.csv_rows",
      "selector": {
        "row_window": "2:",
        "encoding": "utf-8",
        "dialect": "excel"
      },
      "columns": {
        "transaction_id": {
          "source": "Transaction ID",
          "role": "key",
          "required": true,
          "unique": true
        },
        "amount": {
          "source": "Amount",
          "role": "amount",
          "required": true,
          "numeric": {
            "scale": 2,
            "currency_field": "currency"
          }
        },
        "currency": {
          "source": "Currency",
          "role": "currency",
          "required": true
        },
        "posted_date": {
          "source": "Posted Date",
          "role": "date",
          "required": true,
          "format": "YYYY-MM-DD"
        }
      }
    }
  ],
  "reconcile": {
    "mode": "amount_tolerance",
    "left": "bank",
    "right": "ledger",
    "match_on": ["transaction_id"],
    "compare": ["amount", "currency"],
    "tolerance": {
      "amount": "0.01"
    }
  }
}
```

## Harness Flow

1. Decode and validate the reconciliation request.
2. Load each file as bytes and calculate source hashes.
3. Route each file to the configured kernel reader.
4. Read configured rows, ranges, or tables.
5. Validate configured columns and roles.
6. Call kernel minting for configured evidence fields.
7. Run deterministic reconciliation.
8. Classify matches, mismatches, missing records, duplicate keys, and invalid records.
9. Snapshot the canonical bundle.
10. Verify the bundle and optionally replay it during tests.
11. Generate report artifacts.
12. Write a package manifest with hashes for every artifact.

## Kernel Calls Used

The harness should use existing kernel primitives wherever possible:

- structured source readers: `extract.json`, `extract.jsonl`, `extract.csv_rows`, `extract.spreadsheet`
- `kernel.ReadAndMint` for configured evidence fields
- `kernel/public/memio` store and audit sink for in-process runs
- `corepath.SnapshotBundle` for canonical bundle bytes
- `bundle.Verify` for bundle integrity
- `bundle.Replay` for replay tests and report validation
- derived export helpers only when report artifacts need bundle-backed export validation

The reconciliation engine itself lives outside the kernel. The kernel remains responsible for evidence capture, token minting, audit determinism, and bundle verification.

## Output Package

The CLI should produce:

```text
audit-report.zip
├── report.md
├── report.json
├── mismatches.csv
├── captured_reads.json
├── tokens.json
├── audit.jsonl
├── bundle.json
└── manifest.json
```

## Non-Goals

- No LLM inference.
- No fuzzy matching.
- No automatic column guessing.
- No runtime database, object store, daemon, or persistence binding.
- No PDF/OCR input.
- No polished UI.
- No kernel expansion beyond necessary reader/mint/bundle calls.

## Stop Conditions

Stop and replan instead of widening scope if:

- the harness needs to infer column meaning without config
- a reconciliation mode needs probabilistic or fuzzy matching
- a report requires unverifiable values not backed by captured reads or tokens
- a file format is unsupported by current readers
- the implementation requires persistent storage or a runtime service
- matching logic would require new audit semantics

## Packet Sequence

Use `.planning/phase-five/goal-harness-prompt.md` for the autonomous `/goal` execution prompt. Use `.planning/phase-five/packets/README.md` for the packet sequence. Start implementation with Packet 00, the reconciliation request contract.
