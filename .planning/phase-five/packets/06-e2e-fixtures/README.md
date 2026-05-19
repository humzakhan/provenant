# Packet 06 - End-to-End Fixtures and Docs

## What the PR should do

Prove the full Phase 5 flow with understandable fixture files and user-facing docs. This PR is the final acceptance layer for the phase: it should show a first-time user how to run the command and should prove the happy path plus fail-closed behavior with small deterministic examples.

## What it should build

- Happy-path fixtures:
  - `bank.csv`
  - `ledger.xlsx`
  - `invoices.csv`
- Fail-closed fixtures:
  - missing required column
  - duplicate key
  - malformed amount
  - currency mismatch
  - unsupported file format
  - ambiguous selector
- README walkthrough for:
  - request config
  - CLI command
  - report ZIP inspection
  - audit/bundle verification

## Fixture scenario

Use one small business scenario throughout the docs and tests:

- `bank.csv`: bank statement rows keyed by transaction id.
- `ledger.xlsx`: internal ledger rows keyed by transaction id.
- `invoices.csv`: invoice rows used to demonstrate existence/completeness or one-to-many validation.

Keep fixture data artificial, small, and reviewable. Include enough rows to demonstrate:

- a matched transaction
- an amount mismatch within or outside tolerance
- a missing ledger row
- an extra ledger row
- duplicate transaction id handling
- currency mismatch
- malformed amount handling

The happy-path fixture should be separate from fail-closed fixtures. Do not make the happy-path example depend on intentionally invalid files.

## Expected docs

Update the repository README or the most appropriate existing user-facing doc with:

- what the project does
- what Phase 5 reconciliation supports
- what it does not support
- supported input formats
- the request JSON shape
- where the example files live
- the exact CLI command to run
- where the output ZIP is written
- a short sample `report.md` excerpt
- a short sample `manifest.json` excerpt
- how to verify the bundle/report using existing commands or tests
- common failure examples and what they mean

Also add a fixture-local README near the examples if that is more discoverable. The fixture README should include the exact relative paths used by the example request.

## Example request requirements

Add a complete example request JSON that uses the fixture files and exercises the primary path. It should define:

- run id and tenant id
- every file id, purpose, format, path, reader, selector, and column mapping
- key, amount, currency, date, reference, status, and ignored columns where relevant
- reconciliation mode
- match keys
- compare fields
- amount tolerance
- uniqueness rules
- evidence fields to mint

The example should work when copied as-is from the documented repo-relative location.

## Sample output requirements

Include committed sample output only if it is stable and reasonably small. If full ZIP output is too noisy, document selected excerpts and assert full output through tests.

Sample output should show:

- count summary
- one mismatch row
- one token or captured-read reference
- manifest hash shape
- bundle verification command or test name

Do not commit large binary outputs unless the repo already stores golden ZIPs. Prefer textual golden files, manifest checks, and generated temp ZIPs in tests.

## E2E tests to add

Add tests that execute the real command or the closest existing command harness:

- happy-path request produces `audit-report.zip`
- ZIP contains all required artifacts
- manifest hashes validate
- bundle verifies
- report references token and captured-read ids
- repeated runs produce stable report/manifest content
- missing required column fails closed
- duplicate key fails closed
- malformed amount fails closed
- currency mismatch is classified deterministically
- unsupported file format fails before reconciliation
- ambiguous selector fails before reconciliation

Keep the tests deterministic and small. Use temp directories for outputs and fixture copies where mutation might occur.

## Documentation truth rules

Docs must match the implemented command and artifact names. Do not describe future support for intelligent guessing, arbitrary file understanding, fuzzy matching, PDF/OCR, UI, or external harness APIs as if they already exist.

When documenting unsupported behavior, use clear language:

- supported files must use configured readers
- column roles must be explicit
- selectors must be deterministic
- the harness fails closed on ambiguity
- reconciliation logic is deterministic and outside the kernel

## Review checklist

Before opening the PR, verify:

- a new user can find the example from the top-level README
- the documented command runs from a clean checkout
- every sample output excerpt is generated from the current fixture behavior
- every fail-closed fixture has a deterministic expected error
- no docs imply LLM inference or automatic column guessing

## Skills/guidance to load

- `engineering-workflow`
- `go-testing-fuzzing`
- `go-input-validation-security`
- `conventional-commit-guard` before commit messages and PR creation

## Implementation boundary

No new reconciliation modes. No UI polish. No non-fixture sample data. No broad README rewrite beyond the reconciliation instructions needed for first-time use.

## Pass criteria / validation

- Happy path produces the expected report package.
- Each fail-closed fixture fails with deterministic error output.
- Docs match the actual command and output artifacts.
- `go test ./...` passes.
- Top-level docs point to the example request and fixtures.
- Sample output excerpts match generated output.

## Stop conditions

Stop if example data requires unsupported reader behavior, fuzzy matching, automatic column guessing, or any behavior not implemented by Packets 00-05.

## Commit/PR boundary

One final PR limited to E2E fixtures, user-facing docs, and acceptance tests.
