# Reconciliation Fixture

This directory is the current Phase 5 reconciliation example for the local CLI. It is deterministic and config-driven: the request names the local input files, readers, selectors, columns, key fields, and compared fields. It is not a general reconciliation product.

Run from this directory:

```sh
go run ../../cmd/provenant reconcile --request reconcile.json --out report.zip --workdir .
```

The command writes `report.zip` and prints no stdout on success. Re-run with `--overwrite` if `report.zip` already exists.

## Inputs

- `reconcile.json`: request contract for the run, files, readers, selectors, logical columns, and reconciliation rule.
- `bank.csv`: left-side `bank_statement` source read with `extract.csv_rows`.
- `ledger.xlsx`: right-side `general_ledger` source read with `extract.spreadsheet` from sheet `Ledger`.
- `invoices.csv`: adjacent fixture data for later examples; it is not consumed by `reconcile.json`.

## Request Shape

`reconcile.json` has three top-level sections:

- `run`: deterministic run identity, currently `id: reconciliation_fixture_run_1` and `tenant_id: tenant_demo`.
- `files`: explicit local source declarations. Each file includes `id`, `purpose`, `format`, `path`, `reader`, a reader-specific `selector`, and logical `columns` mapped to source columns.
- `reconcile`: the deterministic rule. This fixture uses `mode: exact_match`, compares left file `bank` to right file `ledger`, matches on `transaction_id`, and compares `amount` plus `currency`.

The request does not infer file roles or columns. Changing column names, spreadsheet ranges, or match fields requires changing `reconcile.json`.

## ZIP Artifacts

The current `report.zip` contains exactly these entries:

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

The report ZIP is deterministic for the same inputs and request. ZIP entry modified times are pinned to the ZIP epoch used by the implementation.

## Report Excerpts

`report.md` summarizes the fixture run:

```md
# Reconcile Report

- Run ID: reconciliation_fixture_run_1
- Mode: exact_match
- Result count: 4
- Mismatch count: 3

## Outcomes

- matched: 1
- mismatch: 1
- missing_left: 1
- missing_right: 1
```

`report.json` contains the same counts plus evidence for non-matched results:

```json
{
  "run_id": "reconciliation_fixture_run_1",
  "mode": "exact_match",
  "left_file_id": "bank",
  "right_file_id": "ledger",
  "result_count": 4,
  "outcome_counts": {
    "matched": 1,
    "mismatch": 1,
    "missing_left": 1,
    "missing_right": 1
  }
}
```

`mismatches.csv` starts with:

```csv
result_id,outcome,key,field,left,right,reason
res_53300e2c30ce1408f21bdc5c8f0702a5,missing_left,"[{""field"":""transaction_id"",""value"":""ONLY-LEDGER""}]",,,,
res_581ff502a9941a868182e6df9431b79f,mismatch,"[{""field"":""transaction_id"",""value"":""WIRE-003""}]",amount,70.00,75.00,
res_6f5d553fdcfbcc133748e689c40eca0a,missing_right,"[{""field"":""transaction_id"",""value"":""ONLY-BANK""}]",,,,
```

`audit.jsonl` is the tamper-evident audit chain. The first entry is a deterministic token mint for `bank.csv`:

```json
{"at":"1970-01-01T00:00:00Z","hash":"1c3a31b2c15870aa4b205af0263bf882123987affaa55e75222f89ee74203163","kind":"token_mint","payload_deterministic":{"captured_read_id":"read_c3182a42dc24532fcb411f81391f198b","reader_name":"extract.csv_rows","reader_version":"v1","source_sha256":"c430a1f9c8f25c2603459acb5f77b0531f5bddcd2429abafd6e06dc1f59911e0","source_value_raw":"100.00","token_id":"tok_b96a501573ad7fd5edb9185ed93604d5","value_canonical":"100.000000000"},"prev_hash":"4533226321c664c56ea0c277312cb192f27ae2cdad5bb632780ba79a7f9de1ba","run_id":"reconciliation_fixture_run_1","seq":1,"tenant_id":"tenant_demo"}
```

The real line includes the full locator, origin, output path, reader options, source selector, and source path. Absolute `source_path` and `origin` values depend on where you run the fixture.

## Manifest Excerpt

`manifest.json` records the verified bundle hash and each sidecar artifact hash:

```json
{
  "schema_version": "provenant.reconcile.report.manifest.v1",
  "bundle_hash": "43985b8db3be88c9c1b7dbef2fcd5028349eff173302f29f6ad815662983be25",
  "artifacts": [
    {
      "name": "audit.jsonl",
      "size": 14740,
      "sha256": "7d4af8e201608b42c09d41ead8dec89feb5875eaa099a7bf926f1f31be51f8c8"
    },
    {
      "name": "bundle.json",
      "size": 29318,
      "sha256": "43985b8db3be88c9c1b7dbef2fcd5028349eff173302f29f6ad815662983be25"
    }
  ]
}
```

The full manifest also includes `captured_reads.json`, `mismatches.csv`, `report.json`, `report.md`, and `tokens.json`.

## Verification Guidance

- Confirm ZIP membership matches the artifact list above.
- Confirm `manifest.json.bundle_hash` equals the SHA-256 of `bundle.json`.
- Confirm every `manifest.json.artifacts[].sha256` equals the SHA-256 of the named ZIP entry.
- Verify and replay `bundle.json` with the kernel bundle package; the CLI tests exercise this via `bundle.Verify(entries["bundle.json"])` and `bundle.Replay(entries["bundle.json"])`.
- Run `go test ./access/cli` from the repo root to exercise this fixture, ZIP membership, manifest hashes, bundle verify/replay, and failure fixtures.

## Failure Fixtures

Negative and edge-case fixtures live under `failures/`:

- `missing_required_source`: missing required source data fails as `selector_invalid`.
- `malformed_amount`: invalid numeric evidence fails as `evidence_failed`.
- `unsupported_format`: unsupported input format fails as `request_invalid`.
- `invalid_selector`: invalid reader selector fails as `request_invalid`.
- `duplicate_key`: produces `duplicate_key` outcomes without writing a failure.
- `invalid_record`: produces `invalid_record` and `missing_left` outcomes.
- `currency_mismatch`: produces a deterministic `mismatch` outcome.

## Explicit Non-Support

This fixture does not support:

- LLM inference or agent reasoning.
- Fuzzy matching.
- Automatic column guessing.
- PDF or OCR input.
- UI workflows.
- Remote files, URLs, object stores, or network reads.
- A complete end-user reconciliation product.
