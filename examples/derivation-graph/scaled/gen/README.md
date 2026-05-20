# Scaled fauxbooks generator

This directory contains the deterministic CSV generator for the phase-09 scaled
fauxbooks demo.

## What it produces

Running the generator writes four CSV files to
`examples/derivation-graph/scaled/data/`:

| File | Columns | Rows |
|------|---------|------|
| `payroll.csv` | `employee_id, department, salary, bonus` | 100 |
| `expenses.csv` | `expense_id, category, department, amount, date` | 500 |
| `revenue.csv` | `txn_id, product, region, amount` | 1 200 |
| `refunds.csv` | `refund_id, product, region, amount` | 80 |

All amounts are canonical decimal strings with exactly 2 fractional digits
(e.g. `"12345.67"`). Files are UTF-8 with Unix line endings.

## Seed strategy

The generator uses base seed 42 with four independent per-table RNGs:

```go
payrollRand  = rand.New(rand.NewSource(42 + 1))
expensesRand = rand.New(rand.NewSource(42 + 2))
revenueRand  = rand.New(rand.NewSource(42 + 3))
refundsRand  = rand.New(rand.NewSource(42 + 4))
```

Each sub-seed produces a fully independent sequence, so changing one table's
schema does not affect any other table's values.

## Normal usage — do not regenerate unless necessary

The four CSVs are committed to the repository under
`examples/derivation-graph/scaled/data/`. **CI does not run the generator.**
Tests consume the committed CSVs directly.

## How to regenerate

Run the generator from the repository root:

```sh
go run ./examples/derivation-graph/scaled/gen \
  --out ./examples/derivation-graph/scaled/data
```

After regenerating, verify determinism by running the generator a second time
and confirming `git diff` is empty:

```sh
go run ./examples/derivation-graph/scaled/gen \
  --out ./examples/derivation-graph/scaled/data
git diff -- examples/derivation-graph/scaled/data/
```

If the CSV output changed (i.e. generator logic was modified), you must also
regenerate the phase-09 e2e goldens:

```sh
UPDATE_GOLDEN=1 go test ./examples/derivation-graph/scaled/... \
  -run TestScaledFauxbooksEndToEnd -v
```

And the broken-dataset goldens:

```sh
UPDATE_GOLDEN=1 go test ./examples/derivation-graph/scaled/broken/... \
  -run TestBrokenFauxbooksEndToEnd -v
```

Commit all regenerated files (CSVs + goldens) together in a single conventional
commit.

## Note on UPDATE_GOLDEN

`UPDATE_GOLDEN=1` is an environment variable consumed by the phase-09 e2e test
harness (Step 2 onward) to overwrite golden bundle/trace files. It has no
effect on the generator itself. The generator always writes fresh output
regardless of `UPDATE_GOLDEN`.
