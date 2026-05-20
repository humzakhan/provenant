# Phase 08 — End-to-End Fauxbooks Demo and Golden Replay Test

## Objective

Validate the whole feature with a single end-to-end demo: generate a deterministic synthetic accounting dataset ("fauxbooks") of three files (`gl.csv`, `payroll.csv`, `revenue.csv`), mint leaf tokens from each, derive Operating Income via two `calc.subtract` calls (Revenue − COGS − OpEx), submit the answer, finalize the run, replay the bundle, and assert bit-identical replay. Walk the answer token through the Phase 07 trace tool and assert per-leaf contribution amounts sum exactly to the answer.

## Scope (in)

- New directory `examples/derivation-graph/` containing the fauxbooks dataset and the run request fixtures.
- A small deterministic generator (`examples/derivation-graph/gen/gen.go`) that produces the three CSV files from a fixed seed. The generator is run once and its outputs committed; CI does not regenerate.
- A `Makefile`-style script or `go test` target that runs the full sequence: `run start`, three `read-mint` × N rows, `filter.sum_where` and `calc.subtract` ops, `submit-answer`, `run finish`, `verify`, `replay`, `trace`.
- Golden files: `golden.bundle.json` (canonical bytes) and `golden.trace.json` (Phase 07 output).
- A Go test in `examples/derivation-graph/e2e_test.go` that re-runs the sequence and asserts byte-equality with the goldens.

## Scope (out)

- No new ops, kernel changes, or CLI verbs.
- No production-grade fauxbooks. Three small CSVs are sufficient. ~20 rows each.
- No HTTP / network surface.
- No interactive shell helpers.

## Technical Design

### Fauxbooks dataset

```
examples/derivation-graph/
├── data/
│   ├── revenue.csv      # row_id, account, amount  (account in {"Revenue","Refunds"})
│   ├── cogs.csv         # row_id, account, amount  (account in {"COGS","Inventory adj"})
│   └── opex.csv         # row_id, account, amount  (account in {"Salaries","Rent","Software"})
├── gen/
│   └── gen.go           # deterministic generator from a fixed seed
├── requests/
│   ├── run-start.json
│   ├── read-mint-revenue.json     # one per row, deterministic IDs
│   ├── read-mint-cogs.json
│   ├── read-mint-opex.json
│   ├── calc-net-revenue.json      # filter.sum_where on revenue, account == "Revenue"
│   ├── calc-refunds.json          # filter.sum_where on revenue, account == "Refunds"
│   ├── calc-gross-revenue.json    # calc.subtract net_revenue - refunds
│   ├── calc-total-cogs.json       # calc.sum cogs rows
│   ├── calc-total-opex.json       # calc.sum opex rows
│   ├── calc-gross-margin.json     # calc.subtract gross_revenue - total_cogs
│   ├── calc-operating-income.json # calc.subtract gross_margin - total_opex
│   ├── submit-answer.json         # token_id = operating_income
│   └── run-finish.json
├── golden.bundle.json
├── golden.trace.json
├── README.md
└── e2e_test.go
```

### Derivation graph

```
operating_income = calc.subtract(
    gross_margin,
    total_opex,
)
gross_margin = calc.subtract(
    gross_revenue,
    total_cogs,
)
gross_revenue = calc.subtract(net_revenue, refunds)
net_revenue   = filter.sum_where(revenue rows, account == "Revenue")
refunds       = filter.sum_where(revenue rows, account == "Refunds")
total_cogs    = calc.sum(cogs rows)
total_opex    = calc.sum(opex rows)
```

Every leaf is a `read-mint` from one row of one CSV. The trace from `operating_income` walks down through `calc.subtract`, `filter.sum_where`, and `calc.sum` nodes and bottoms out at all minted leaves.

### Determinism

- `gen.go` seeds `math/rand` with a fixed constant. Outputs are committed; CI does not regenerate.
- All CLI invocations use a fixed `Clock` injected via the test harness (matching `access/cli/cli.go`'s existing `fixedClock`).
- Run ID and tenant ID are fixed strings: `"derivation_graph_demo_run_1"`, `"tenant_demo"`.
- All `read-mint` requests use deterministic source SHA-256s computed from the committed CSV bytes.

### Golden files

`golden.bundle.json` is the JCS-canonicalized bundle produced by a full run. `golden.trace.json` is the JCS-canonicalized output of `provenant trace --bundle ... --token <operating_income_id>`.

Both goldens are committed. The e2e test:

1. Runs the full CLI sequence into a temp dir using `cli.Main` directly (no subprocess, to keep the test in-process and deterministic).
2. Reads the produced bundle bytes and asserts `bytes.Equal` with `golden.bundle.json`.
3. Runs `trace.Trace` against the bundle for the `operating_income` token, JCS-canonicalizes the result, and asserts `bytes.Equal` with `golden.trace.json`.
4. Sums the `LeafContribution.Contribution` decimals and asserts the total equals the bundle's `operating_income` token's `ValueCanonical`.
5. Calls `bundle.Verify` and `bundle.Replay`; asserts no errors and that replayed state matches the in-memory state.

### Test harness

```go
// examples/derivation-graph/e2e_test.go
func TestFauxbooksEndToEnd(t *testing.T) {
    dir := t.TempDir()
    runFile := filepath.Join(dir, "run.bundle.json")
    runCLI(t, "run", "start", "--run-file", runFile, "--request", "requests/run-start.json")
    for _, req := range mintRequests {
        runCLI(t, "read-mint", "--run-file", runFile, "--request", req)
    }
    for _, req := range calcRequests {
        runCLI(t, "calc", "--run-file", runFile, "--request", req)
    }
    runCLI(t, "submit-answer", "--run-file", runFile, "--request", "requests/submit-answer.json")
    runCLI(t, "run", "finish", "--run-file", runFile, "--request", "requests/run-finish.json")

    got, _ := os.ReadFile(runFile)
    want, _ := os.ReadFile("golden.bundle.json")
    if !bytes.Equal(got, want) {
        t.Fatalf("bundle bytes diverged from golden")
    }
    ...
}
```

`runCLI` invokes `cli.Main` with the fixed clock injected through `Runner.Now`.

## Implementation Steps

1. Write `examples/derivation-graph/gen/gen.go`. Produce CSVs with deterministic numeric values (e.g. revenue rows summing to 1,000,000.00).
2. Run the generator once; commit the resulting CSVs under `examples/derivation-graph/data/`.
3. Hand-write the request JSONs under `requests/`. Each `read-mint` request names a row by `selector.row_window`.
4. Write `e2e_test.go` that drives the sequence.
5. Run the test once with `golden.bundle.json` and `golden.trace.json` absent; capture the produced bundle and trace; write the goldens.
6. Re-run the test; confirm goldens match.
7. Add a `README.md` for the example that explains the dataset, the graph, and how to inspect outputs.
8. Update the top-level `README.md` with a short "Derivation Graph end-to-end demo" section linking to the new example.

## Tests Required

| Test | Asserts |
|---|---|
| `TestFauxbooksEndToEnd` | Full sequence runs without error; bundle bytes match golden; trace JSON matches golden; contribution sum equals operating income; `Verify` and `Replay` pass |
| `TestFauxbooksOperatingIncomeValue` | Spot-check: operating income equals revenue − refunds − cogs − opex computed independently from the CSVs |
| `TestFauxbooksTraceLeafCount` | Number of distinct leaves in trace equals the number of `read-mint` calls |
| `TestFauxbooksReplayDeterminism` | Running the demo twice produces identical bundle bytes |
| `TestFauxbooksSubmitAnswerLiteralRejected` | A modified `submit-answer` request with a `value` field returns exit 2 |
| `TestFauxbooksBackwardCompat` | Phase-5 reconciliation fixture still verifies (sanity regression) |

## Definition of Done

- [ ] Code, fixtures, and goldens merged to `main`.
- [ ] `go test ./examples/derivation-graph/...` passes on Linux and macOS.
- [ ] `go test ./...` and `go vet ./...` pass repo-wide.
- [ ] Golden bundle is bit-identical across Linux amd64 and macOS arm64. (If platform divergence appears, fix the underlying determinism bug — do not maintain per-platform goldens.)
- [ ] Contribution invariant holds: `sum(LeafContribution.Contribution) == operating_income.ValueCanonical`.
- [ ] Top-level `README.md` links to the new example and includes a one-paragraph description.
- [ ] Reconciliation fixture in `examples/reconciliation/` still passes its existing tests.

## Risks / Open Questions

- Cross-platform determinism: JSON map ordering, decimal printing, and SHA-256 are all platform-neutral; the main risk is locale-sensitive number formatting. `shopspring/decimal` is locale-insensitive, so the risk is low. Verify on both target platforms before committing the golden.
- Generator code lives under `examples/` but is not strictly an "example". We accept that placement to keep the dataset self-contained.
- If a future op changes default rounding, the golden will need to be regenerated. Document a regeneration procedure in the example's README so this is a deliberate, reviewable act, not a flaky test.
- The demo could grow over time (more derivation depth, more rows). Resist this: the demo's job is to assert the full pipeline works, not to stress-test it.

## Dependencies

- All prior phases ([01](./phase-01-token-dag-schema.md), [02](./phase-02-op-catalog-core.md), [03](./phase-03-op-catalog-rest.md), [04](./phase-04-run-lifecycle.md), [05](./phase-05-cli-surface.md), [06](./phase-06-named-sources-and-pack.md), [07](./phase-07-traceback.md)) must be merged before this phase begins. Phase 06's `source_name` is exercised by at least one `read-mint` request in the demo to keep the named-source path on the hot path.
