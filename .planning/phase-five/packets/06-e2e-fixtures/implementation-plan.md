# Packet 06 Implementation Plan - End-to-End Fixtures and Docs

## Goal

Prove the complete Phase 5 flow with small fixtures, an executable example, deterministic fail-closed cases, and first-time-user documentation.

## Suggested File Shape

Use the repo's existing fixture and docs conventions. If no better location exists, use:

```text
examples/reconciliation/
├── README.md
├── reconcile.json
├── bank.csv
├── ledger.xlsx
├── invoices.csv
├── expected/
│   ├── report.md
│   ├── report.json
│   └── manifest.json
└── failures/
    ├── missing-column/
    ├── duplicate-key/
    ├── malformed-amount/
    ├── currency-mismatch/
    ├── unsupported-format/
    └── ambiguous-selector/
```

Only commit expected outputs that are stable and useful in review. Prefer generated temp ZIPs plus textual golden excerpts over large binary fixtures.

## Implementation Sequence

1. Inspect current README and fixture conventions.
2. Create one coherent artificial reconciliation scenario.
3. Add happy-path CSV/XLSX/CSV fixture files.
4. Add a complete request JSON that works from a clean checkout.
5. Add fail-closed fixture variants.
6. Add E2E tests that execute the real CLI or closest command harness.
7. Verify ZIP artifacts and manifest hashes in tests.
8. Add README instructions for what exists, what does not exist, how to run it, and where examples live.
9. Add sample output excerpts generated from the fixtures.
10. Run full validation and update docs if command behavior differs.

## Fixture Scenario

Use a simple business flow:

- bank statement records in `bank.csv`
- internal ledger records in `ledger.xlsx`
- invoice records in `invoices.csv`

The happy path should include:

- at least one matched transaction
- at least one deterministic mismatch
- at least one extra or missing row if the selected mode supports reporting it
- clear amount and currency fields
- stable transaction ids

Failure fixtures should cover:

- missing required column
- duplicate key
- malformed amount
- currency mismatch classification
- unsupported file format
- ambiguous selector

Keep all data fictional and small.

## Documentation Requirements

Update the top-level README or the closest user-facing doc with:

- project description
- current reconciliation support
- explicit non-support for LLM inference, fuzzy matching, automatic column guessing, PDF/OCR, UI, and arbitrary files outside supported readers
- supported formats
- request config overview
- exact example path
- exact command
- output ZIP artifact list
- sample `report.md` excerpt
- sample `manifest.json` excerpt
- how to inspect or verify bundle/report evidence
- common errors and what they mean

Add an example-local README with:

- fixture file descriptions
- request file walkthrough
- command to run from repo root
- expected high-level output
- fail-closed fixture descriptions

## E2E Tests

Add tests for:

- happy path writes `audit-report.zip`
- ZIP contains all required artifacts
- manifest hashes match artifact bytes
- bundle verifies
- report references token ids and captured-read ids
- repeated runs have stable report and manifest content
- missing required column fails closed
- duplicate key fails closed
- malformed amount fails closed
- unsupported format fails before reconciliation
- ambiguous selector fails before reconciliation

For currency mismatch, assert the implemented behavior: either a deterministic mismatch classification or fail-closed validation, depending on Packet 02's final contract.

## Sample Output

Document short excerpts, not full generated packages:

```text
Summary
matched: 1
mismatch: 1
missing_right: 1
```

```json
{
  "artifacts": [
    {
      "path": "report.json",
      "sha256": "<sha256>"
    }
  ]
}
```

Replace placeholders with real generated values if committed as golden docs. Do not invent sample values that tests do not verify.

## Validation

Run:

```bash
rtk go test ./...
rtk proxy git diff --check
```

Also run the documented CLI command manually or through the E2E test harness and confirm README instructions match reality.

## Stop Conditions

Stop if examples require unsupported readers, fuzzy matching, automatic column guessing, non-deterministic output, large binary goldens, or behavior not implemented by Packets 00-05.

## Final Phase Handoff

At completion, the phase should have:

- a working request format
- deterministic file routing
- deterministic reconciliation
- provenance-backed evidence
- CLI execution
- audited ZIP output
- first-time-user docs and fixtures
