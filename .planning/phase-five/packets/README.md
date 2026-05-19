# Phase 5 Packets - Deterministic Reconciliation Harness

Execute these packets in order. Each packet should be independently reviewable and produce one PR. Every packet directory contains a README plus an `implementation-plan.md`; use both before coding.

## Packet 00 - Reconciliation Request Contract

Path: `.planning/phase-five/packets/00-request-contract/`

Implement the request structs, decoder, schema validation, and fail-closed config checks for multi-file reconciliation. The request must capture file purpose, format, reader, selector, column roles, required fields, uniqueness, tolerance, and reconciliation mode.

## Packet 01 - Reader Router

Path: `.planning/phase-five/packets/01-reader-router/`

Implement deterministic routing from config to existing kernel readers. The router loads bytes, computes source hashes, builds selectors, and calls `extract.json`, `extract.jsonl`, `extract.csv_rows`, or `extract.spreadsheet`.

## Packet 02 - Reconciliation Engine

Path: `.planning/phase-five/packets/02-reconciliation-engine/`

Implement deterministic reconciliation modes: exact match, amount tolerance, existence/completeness, one-to-many/many-to-one, and schema/value validation. The engine must classify matches and mismatches without minting or reporting concerns.

## Packet 03 - Provenance and Token Minting

Path: `.planning/phase-five/packets/03-provenance-token-minting/`

Connect reconciliation evidence to kernel provenance. Mint configured keys, amounts, currency values, dates, references, and result facts through `kernel.ReadAndMint`, using `memio` for in-process store/audit state.

## Packet 04 - CLI Command

Path: `.planning/phase-five/packets/04-cli-command/`

Expose the harness through `provenant reconcile --request reconcile.json --out audit-report.zip`. The CLI validates inputs, executes the deterministic harness, and fails closed on invalid config or unsupported files.

## Packet 05 - Audit Report Package

Path: `.planning/phase-five/packets/05-audit-report-package/`

Generate the report ZIP with `report.md`, `report.json`, `mismatches.csv`, `captured_reads.json`, `tokens.json`, `audit.jsonl`, `bundle.json`, and `manifest.json`. Verify manifest hashes and bundle integrity.

## Packet 06 - End-to-End Fixtures and Docs

Path: `.planning/phase-five/packets/06-e2e-fixtures/`

Add happy-path and fail-closed fixture files for bank/ledger/invoice reconciliation. Document how to run the CLI and how to inspect report output.
