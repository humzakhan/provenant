This fixture is a minimal happy-path reconciliation example for the local CLI.

Run from this directory:

```sh
go run ../../cmd/provenant reconcile --request reconcile.json --out report.zip --workdir .
```

The fixture compares `bank.csv` to `ledger.xlsx` using exact transaction IDs and
amount/currency comparisons. The expected report summary includes one `matched`,
one `mismatch`, one `missing_left`, and one `missing_right` outcome.

`invoices.csv` is included as a small adjacent source for later examples; it is
not consumed by `reconcile.json` in this first slice.
