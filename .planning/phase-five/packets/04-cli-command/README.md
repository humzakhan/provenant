# Packet 04 - CLI Command

## What the PR should do

Expose the deterministic reconciliation harness through the CLI. This PR turns the request contract, reader router, reconciliation engine, provenance minting, and report-package generator into one user-facing command.

## What it should build

```bash
provenant reconcile --request reconcile.json --out audit-report.zip
```

The command should:

- decode the request
- validate config
- run reader routing
- run reconciliation
- mint configured evidence
- generate report artifacts
- write the ZIP package
- return deterministic error messages on failure

## Expected command behavior

Add a `reconcile` subcommand to the existing CLI surface without changing existing commands. The command should accept explicit paths only:

- `--request`: required path to the reconciliation request JSON.
- `--out`: required output ZIP path.
- `--workdir`: optional base directory used to resolve relative file paths in the request. If omitted, resolve relative paths from the request file directory.
- `--overwrite`: optional boolean. If false or absent, fail when `--out` already exists.

The command should not accept inline JSON, remote URLs, glob expansion, stdin payloads, or implicit discovery in this packet.

The command flow should be deterministic:

1. Parse CLI flags.
2. Reject missing, duplicate, or unknown flags using the repo's existing CLI style.
3. Read the request file with bounded input handling.
4. Decode and validate the request through the Packet 00 API.
5. Resolve configured file paths against the deterministic base directory.
6. Call the Packet 01 router to read every configured file.
7. Call Packet 02 reconciliation with the validated records.
8. Call Packet 03 provenance minting for configured evidence.
9. Call Packet 05 report package generation.
10. Write the ZIP atomically where practical, using a temporary file in the destination directory and rename on success.

## Error handling requirements

Errors should be stable enough for golden tests. Avoid embedding map iteration order, absolute temporary paths, timestamps, random IDs, or Go `%v` dumps that may change.

Classify failures in a small set of command-level categories:

- `request_invalid`
- `input_not_found`
- `input_unsupported`
- `selector_invalid`
- `reconciliation_failed`
- `evidence_failed`
- `report_failed`
- `output_exists`
- `output_write_failed`

Stderr should include the category, a concise message, and the config path when available, for example:

```text
request_invalid: files[ledger].columns[amount]: required column source is empty
```

Exit codes should follow the existing CLI conventions if present. If there is no convention, use `0` for success, `2` for user/config/input errors, and `1` for internal execution or output-write errors.

## Output requirements

On success:

- write exactly one ZIP at `--out`
- do not print report contents to stdout
- print a concise success line if the existing CLI normally does so
- include a deterministic artifact summary if already supported by the report generator

On failure:

- do not leave a partial ZIP at `--out`
- do not write report artifacts beside the ZIP
- do not mint additional evidence after a fail-closed validation error
- include enough stderr context to identify the failing request field or file id

## Tests to add

Add command-level tests with temporary directories and fixture requests:

- valid CSV/XLSX/JSON fixture request writes `audit-report.zip`
- missing `--request` fails
- missing `--out` fails
- invalid JSON fails before file reads
- request validation error fails before routing
- unsupported file format fails with `input_unsupported`
- output path exists and `--overwrite` is absent fails
- output path exists and `--overwrite` is present succeeds
- relative input paths resolve from the request file directory
- command output is deterministic across repeated runs

Where possible, assert the ZIP manifest rather than comparing raw ZIP bytes, because ZIP metadata can vary unless the package writer already canonicalizes it.

## Skills/guidance to load

- `engineering-workflow`
- `go-implementation-quality`
- `go-input-validation-security`
- `go-io-performance`
- `go-testing-fuzzing`
- `conventional-commit-guard` before commit messages and PR creation

## Implementation boundary

No HTTP/MCP surface in this packet unless explicitly replanned. No daemon/runtime persistence. No new file formats. Do not add request inference, column guessing, auto-discovery, remote file fetching, or an interactive prompt.

This PR may add glue code that composes earlier packet APIs. It should not redefine the request schema, reconciliation semantics, provenance semantics, or report artifact contract except to expose their errors clearly at the CLI boundary.

## Pass criteria / validation

- CLI accepts a valid fixture request and writes `audit-report.zip`.
- CLI rejects invalid request, unsupported file, missing output path, and ambiguous config.
- Exit codes and stderr are deterministic.
- Existing `read-mint-bundle` behavior is unchanged.
- CLI tests cover success, input validation, output safety, and deterministic failures.
- `go test ./...` passes.

## Stop conditions

Stop if CLI wiring requires changing kernel reader or audit semantics. Stop if the command cannot compose prior packet APIs without inventing inference, persistence, or new reconciliation behavior.

## Commit/PR boundary

One PR limited to CLI command wiring, command-level tests, and minimal docs needed to invoke the command.
