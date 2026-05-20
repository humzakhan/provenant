# Packet 05 Implementation Plan - Audit Report Package

## Goal

Generate a portable audit report ZIP from deterministic reconciliation and provenance outputs. This packet produces human-readable and machine-readable artifacts whose claims are backed by captured reads, tokens, bundle bytes, and manifest hashes.

## Suggested Package Shape

Use the established reconciliation package location. A focused report package could be:

```text
reconcile/report/
├── artifacts.go
├── csv.go
├── json.go
├── manifest.go
├── markdown.go
├── zip.go
└── report_test.go
```

Suggested responsibilities:

- `artifacts.go`: artifact names, schema versions, and shared inputs
- `markdown.go`: `report.md`
- `json.go`: `report.json`, `captured_reads.json`, `tokens.json`
- `csv.go`: `mismatches.csv`
- `manifest.go`: size and SHA-256 manifest generation
- `zip.go`: deterministic ZIP writing
- `report_test.go`: artifact and manifest verification

## Implementation Sequence

1. Inspect existing export, bundle, audit, and report helper patterns.
2. Define a report input type that accepts only computed results and evidence.
3. Build each artifact in memory or through deterministic writers.
4. Hash each artifact byte-for-byte.
5. Generate `manifest.json`.
6. Write ZIP entries in a fixed order.
7. Use stable ZIP metadata where the standard library permits it.
8. Verify bundle bytes before accepting report generation.
9. Add tests for required artifacts, hashes, and missing evidence.
10. Connect the writer to CLI command plumbing if Packet 04 already has the hook.

## Artifact Contract

The ZIP must contain:

```text
report.md
report.json
mismatches.csv
captured_reads.json
tokens.json
audit.jsonl
bundle.json
manifest.json
```

`report.md`:

- concise human summary
- input files and purposes
- reconciliation mode
- count summary
- mismatch table
- evidence explanation

`report.json`:

- schema version
- run metadata
- file metadata and hashes
- normalized reconciliation settings
- result summary
- result rows with evidence references

`mismatches.csv`:

- stable header
- one row per mismatch or invalid relevant result
- deterministic ordering
- CSV writer escaping only

`captured_reads.json` and `tokens.json`:

- stable arrays
- no fabricated evidence
- include ids, source metadata, row/field locators, roles, and hashes needed for review

`audit.jsonl`:

- one JSON object per line
- canonical audit order
- no pretty-print JSONL

`bundle.json`:

- exact canonical bundle bytes

`manifest.json`:

- artifact name
- byte size
- SHA-256 hash
- bundle hash
- schema version
- deterministic metadata only

## Tests

Add tests for:

- all required artifact names are present
- ZIP entry order is stable if observable
- manifest hashes match artifact bytes
- bundle verifies before report is accepted
- missing token or captured-read evidence fails closed
- report rows reference known evidence ids
- `mismatches.csv` is stable and correctly escaped
- no mismatch case still writes header-only CSV
- repeated generation has stable manifest content
- artifact path traversal is impossible

## Validation

Run:

```bash
rtk go test ./...
rtk proxy git diff --check
```

Use small fixtures and compare manifest contents rather than large binary goldens unless the repo already uses golden ZIPs.

## Stop Conditions

Stop if report generation requires unverifiable claims, new audit semantics, source file rereads, reconciliation reruns, wall-clock metadata, external signing, or persistent storage.

## Handoff

Packet 06 uses these artifacts in user-facing docs and E2E acceptance tests. Keep artifact names and schemas stable.
