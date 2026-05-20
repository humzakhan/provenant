# Packet 05 - Audit Report Package

## What the PR should do

Generate the understandable audited report package. This PR makes the reconciliation result portable by writing a ZIP whose human-readable and machine-readable artifacts can be traced back to captured reads, tokens, audit entries, bundle bytes, and manifest hashes.

## What it should build

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

Artifact requirements:

- `report.md`: human-readable summary, counts, and mismatch table.
- `report.json`: machine-readable reconciliation result.
- `mismatches.csv`: flat mismatch export.
- `captured_reads.json`: captured read evidence.
- `tokens.json`: minted token evidence.
- `audit.jsonl`: audit chain extracted from bundle state.
- `bundle.json`: canonical bundle.
- `manifest.json`: artifact names, sizes, sha256 hashes, and bundle hash.

## Package writer API

Add a small package-level API that accepts only already-computed deterministic inputs:

- run metadata from the validated request
- reconciliation summary and row-level results
- captured-read evidence from the reader/provenance flow
- token evidence from Packet 03
- audit events or bundle-backed audit export
- canonical bundle bytes

The package writer should not read source files, rerun reconciliation, mint tokens, or infer missing report values. Its responsibility is formatting, manifesting, hashing, and ZIP writing.

The API should support writing to an `io.Writer` or a file path depending on existing repo style. Prefer `io.Writer` for testability and keep file creation in the CLI layer if that matches existing package boundaries.

## Artifact details

`report.md` should be concise and deterministic:

- run id and tenant id if present
- reconciliation mode
- input file ids and purposes
- counts for matched, mismatched, missing-left, missing-right, duplicate, invalid, and skipped rows
- mismatch table with stable ordering
- evidence section explaining which artifacts prove the result

`report.json` should be the canonical machine-readable summary:

- request/run metadata
- input file metadata and source hashes
- reconciliation mode and normalized settings
- count summary
- row-level result ids
- references to token ids and captured-read ids
- report artifact schema version

`mismatches.csv` should be flat and stable:

- one header row
- stable column order
- deterministic row order by result id, then configured key fields
- escaped values through a CSV writer, not string concatenation
- empty file with only the header when there are no mismatches

`captured_reads.json` should include only the evidence needed for report verification. If the existing bundle already contains the canonical form, export references and hashes rather than inventing a second source of truth.

`tokens.json` should list token ids, source file ids, selector paths, column roles, source hashes, and any result fact tokens. Do not include secrets or raw file bytes unless the existing token contract already does so.

`audit.jsonl` should preserve deterministic audit order. Every line must be valid JSON. Do not pretty-print JSONL.

`bundle.json` should be exactly the canonical bundle bytes produced by the bundle snapshot/export path, not a reserialized approximation.

`manifest.json` should include:

- manifest schema version
- generated artifact list
- artifact path
- byte size
- sha256 hash
- bundle hash
- request/run id when available
- deterministic creation metadata only. Do not include wall-clock time unless the repo has an injected deterministic clock for this path.

## Determinism requirements

- Sort maps before encoding where the encoder does not guarantee order.
- Use stable row/result ids from the reconciliation engine.
- Use fixed JSON indentation rules per artifact.
- Avoid wall-clock timestamps, random ids, filesystem-dependent path order, or OS-specific separators in report content.
- ZIP entries should have stable names and, where supported, stable metadata.
- Never claim a row is audited unless the row has token or captured-read references.

## Tests to add

Add focused tests for the package writer:

- generated ZIP contains every required artifact
- `manifest.json` hashes match exact artifact bytes
- `bundle.json` verifies with `bundle.Verify`
- optional replay validation passes if Packet 03 exposes bundle replay evidence
- `report.json` references existing token and captured-read ids
- `mismatches.csv` header and ordering are stable
- repeated package generation from identical inputs produces identical manifest hashes
- missing evidence for a claimed row fails closed
- invalid bundle bytes fail closed
- artifact path traversal is impossible

Prefer fixture-sized inputs. The report package tests should not need large files.

## Skills/guidance to load

- `engineering-workflow`
- `provenant-audit-determinism`
- `go-io-performance`
- `go-testing-fuzzing`
- `conventional-commit-guard` before commit messages and PR creation

## Implementation boundary

No polished UI, PDF export, external storage, signing infrastructure, non-deterministic report ordering, or report-only reconciliation logic.

Do not add new audit semantics. If an artifact requires information not available from existing captured reads, tokens, reconciliation results, or bundle state, stop and replan the producing packet instead of fabricating report fields.

## Pass criteria / validation

- ZIP package contains every required artifact.
- Manifest hashes match artifact bytes.
- Bundle verifies.
- Report rows reference token and captured-read IDs.
- Output order is deterministic.
- Missing evidence fails closed.
- `go test ./...` passes.

## Stop conditions

Stop if any report claim cannot be proven from bundle, token, captured-read, reconciliation, or manifest evidence. Stop if ZIP determinism requires broad infrastructure changes outside this packet.

## Commit/PR boundary

One PR limited to report package generation, artifact validation, deterministic package tests, and minimal CLI integration if Packet 04 already introduced the command hook.
