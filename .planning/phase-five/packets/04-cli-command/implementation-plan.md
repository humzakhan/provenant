# Packet 04 Implementation Plan - CLI Command

## Goal

Expose the deterministic reconciliation harness as a CLI command. This packet composes Packets 00-03 and the report package API from Packet 05 when available, while keeping command behavior deterministic and fail-closed.

## Suggested Package Shape

Follow the repo's existing CLI layout. If a new command package is needed, use a narrow shape such as:

```text
cmd/provenant/
├── reconcile.go
└── reconcile_test.go
```

If command implementations live elsewhere, place the new command beside existing command wiring.

## Implementation Sequence

1. Inspect existing CLI command parsing, error formatting, exit code, and test patterns.
2. Add a `reconcile` subcommand without changing existing command behavior.
3. Add flags:
   - `--request`
   - `--out`
   - `--workdir`
   - `--overwrite`
4. Validate flags before reading files.
5. Read request JSON with bounded input handling.
6. Decode and validate using Packet 00.
7. Resolve relative input paths from `--workdir` or the request file directory.
8. Route files using Packet 01.
9. Run reconciliation using Packet 02.
10. Bind provenance using Packet 03.
11. Generate the report package using Packet 05.
12. Write output atomically where practical.
13. Add command-level tests for success and deterministic failures.

## Command Contract

Primary command:

```bash
provenant reconcile --request reconcile.json --out audit-report.zip
```

Optional command:

```bash
provenant reconcile --request reconcile.json --out audit-report.zip --workdir examples/reconcile --overwrite
```

Rules:

- `--request` is required
- `--out` is required
- no stdin request payloads in this packet
- no URL inputs
- no glob expansion
- no implicit file discovery
- no interactive prompts
- no partial ZIP left on failure

## Error Contract

Use stable command-level categories:

- `request_invalid`
- `input_not_found`
- `input_unsupported`
- `selector_invalid`
- `reconciliation_failed`
- `evidence_failed`
- `report_failed`
- `output_exists`
- `output_write_failed`

Messages should include field path, file id, or artifact name where available. Avoid absolute temp paths, timestamps, and unordered map dumps.

## Tests

Add command tests for:

- missing `--request`
- missing `--out`
- invalid JSON
- request validation failure
- unsupported input format
- missing input file
- output exists without `--overwrite`
- output exists with `--overwrite`
- request-relative file path resolution
- happy path writes ZIP
- stderr is deterministic for representative failures
- existing commands still behave as before

Prefer checking report manifest and artifact names over raw ZIP byte equality unless ZIP metadata is canonicalized.

## Validation

Run:

```bash
rtk go test ./...
rtk proxy git diff --check
```

If the CLI has a focused package test command, run that too.

## Stop Conditions

Stop if CLI wiring requires new reconciliation semantics, kernel audit changes, new file formats, persistence, remote file loading, or automatic config inference.

## Handoff

Packet 06 uses this command in end-to-end fixtures and docs. Keep flag names, output behavior, and error categories stable so docs and golden tests can rely on them.
