# Phase 09 — Scaled Demo, Broken Dataset, and HTML Audit Report

> This document is a detailed technical spec for Phase 09, broken into six sequential pull requests (one per step in **Implementation Steps**), followed by a single **/goal** prompt at the end that an autonomous Claude session uses to execute all six PRs in order.

## Objective

Scale the fauxbooks demo to enterprise size (~100 employees, ~20 000–30 000 total rows across the audit bundle), introduce a deliberately broken sibling dataset that surfaces two distinct reconciliation/grounding failures, and deliver a self-contained HTML audit report renderer (`audit/report`) with a CLI verb `audit-html` that turns any existing audit pack into a human-readable, golden-testable HTML file. Phase 08's small demo stays untouched; all new work lives under new subfolders.

## Scope (in)

- New directory `examples/derivation-graph/scaled/` with its own generator, CSVs, request fixtures, goldens, and README. No changes to `examples/derivation-graph/` (phase-08 demo).
- Deterministic generator `examples/derivation-graph/scaled/gen/gen.go` producing four CSVs: `payroll.csv`, `expenses.csv`, `revenue.csv`, `refunds.csv` from seed 42 with per-table sub-seeds.
- Scaled happy-path e2e test `examples/derivation-graph/scaled/e2e_test.go` that asserts byte-equal golden bundle, byte-equal golden trace, contribution invariant, and exact Operating Income canonical decimal.
- Broken sibling dataset `examples/derivation-graph/scaled/broken/` with two deliberate breakages; sibling e2e test `examples/derivation-graph/scaled/broken/e2e_test.go` asserting bundle still verifies, reconciliation report surfaces exactly the expected outcomes, and a cross-run canonical-value comparison surfaces the value difference introduced by the missing-row breakage.
- New package `audit/report/` exporting `RenderHTML(*pack.Document) ([]byte, error)` — a single self-contained HTML renderer, deterministic and golden-testable.
- New CLI verb `provenant audit-html --pack <pack.json> --out <report.html>` implemented in `access/cli/audithtmlverb.go`. Decoupled from pack generation; consumes an already-produced pack.
- Integration test generating HTML for both the scaled happy-path and broken audit packs; both HTML files committed as goldens.
- `examples/derivation-graph/scaled/README.md` with full walkthrough; link added to the existing `examples/derivation-graph/README.md`.

## Scope (out)

- No changes to the phase-08 `examples/derivation-graph/` demo, its goldens, or its test.
- No new kernel ops, token-DAG schema changes, or CLI verbs beyond `audit-html`.
- No JavaScript in the HTML report (v1 is style-only).
- No external CSS/font/image assets; HTML must be openable from disk with no network access.
- No per-platform golden divergence; if determinism breaks on one platform, fix the root cause.
- No interactive shell helpers or Makefile targets beyond `go generate` and `go test` invocations documented in the README.
- No new third-party dependencies — stdlib `html/template` and existing modules only.
- No production-grade data quality or realistic payroll numbers. The dataset is a demo fixture.

## Technical Design

### Scaled fauxbooks dataset (happy path)

```
examples/derivation-graph/scaled/
├── gen/
│   └── gen.go                  # deterministic generator, seed 42 + per-table offsets
├── data/
│   ├── payroll.csv             # employee_id, department, salary, bonus
│   ├── expenses.csv            # expense_id, category, department, amount, date
│   ├── revenue.csv             # txn_id, product, region, amount
│   └── refunds.csv             # refund_id, product, region, amount
├── requests/
│   ├── run-start.json
│   ├── read-mint-payroll.json      # one entry per row via selector.row_window
│   ├── read-mint-expenses.json
│   ├── read-mint-revenue.json
│   ├── read-mint-refunds.json
│   ├── calc-total-salaries.json    # filter.sum_where payroll, bonus == false (salary column)
│   ├── calc-total-bonuses.json     # filter.sum_where payroll, bonus column
│   ├── calc-total-payroll.json     # calc.add salaries + bonuses
│   ├── calc-total-expenses.json    # calc.sum expenses rows
│   ├── calc-total-opex.json        # calc.add total_payroll + total_expenses
│   ├── calc-gross-revenue.json     # calc.sum revenue rows
│   ├── calc-total-refunds.json     # calc.sum refunds rows
│   ├── calc-net-revenue.json       # calc.subtract gross_revenue - total_refunds
│   ├── calc-operating-income.json  # calc.subtract net_revenue - total_opex
│   ├── submit-answer.json          # token_id = operating_income
│   └── run-finish.json
├── golden.bundle.json
├── golden.trace.json
├── e2e_test.go
└── README.md
```

**Row counts (targets — generator produces exactly these):**

| File | Rows | Notes |
|---|---|---|
| `payroll.csv` | 100 | One row per employee; 5 departments |
| `expenses.csv` | 500 | ~5 expense records per employee |
| `revenue.csv` | 1 200 | ~12 revenue transactions across 4 product lines × 3 regions |
| `refunds.csv` | 80 | ~0.8 refunds per revenue transaction |

Total rows minted: 1 880. Each row becomes one `read-mint` call. The bundle accumulates ~1 880 leaf tokens plus derived tokens across the calc chain. Bundle JSON will be larger than phase-08 but still deterministic and suitable for byte-equal golden comparison (see [Determinism and goldens](#determinism-and-goldens)).

**Derivation graph (happy path):**

```
operating_income = calc.subtract(net_revenue, total_opex)
net_revenue      = calc.subtract(gross_revenue, total_refunds)
gross_revenue    = calc.sum(revenue rows)
total_refunds    = calc.sum(refunds rows)
total_opex       = calc.add(total_payroll, total_expenses)
total_payroll    = calc.add(total_salaries, total_bonuses)
total_salaries   = filter.sum_where(payroll rows, column="salary")
total_bonuses    = filter.sum_where(payroll rows, column="bonus")
total_expenses   = calc.sum(expenses rows)
```

All 1 880 leaf tokens bottom out as `read-mint` rows from the committed CSVs. The contribution invariant (`sum(LeafContribution.Contribution) == operating_income.ValueCanonical`) must hold.

**Generator seed strategy:**

```go
const baseSeed = 42
var (
    payrollRand  = rand.New(rand.NewSource(baseSeed + 1))
    expensesRand = rand.New(rand.NewSource(baseSeed + 2))
    revenueRand  = rand.New(rand.NewSource(baseSeed + 3))
    refundsRand  = rand.New(rand.NewSource(baseSeed + 4))
)
```

Each sub-seed produces a fully independent sequence. Amounts are generated as integer cents and formatted as `shopspring/decimal` canonical strings (`"12345.67"` form) to guarantee locale-neutral output. The generator writes UTF-8 CSVs with Unix line endings. It is run once; outputs are committed. CI does not regenerate.

**Fixed identifiers for the scaled run:**

- `run_id`: `"derivation_graph_scaled_run_1"`
- `tenant_id`: `"tenant_demo"`
- Fixed injected clock matching the existing `access/cli` `fixedClock` convention.

### Broken sibling dataset

```
examples/derivation-graph/scaled/broken/
├── data/
│   ├── payroll.csv       # payroll.csv with one row dropped (employee_id "EMP-0042")
│   ├── expenses.csv      # expenses.csv with one amount changed by $0.01 (tolerance violation)
│   ├── revenue.csv       # identical to happy-path revenue.csv (symlink or copy)
│   └── refunds.csv       # identical to happy-path refunds.csv
├── requests/
│   └── (mirrored from scaled/requests/, same structure)
├── golden.bundle.json
├── golden.reconciliation-report.json
├── golden.mismatches.csv
└── e2e_test.go
```

**Breakage (a) — missing payroll row:**

`EMP-0042` is removed from `broken/data/payroll.csv`. The engine compares the happy-path `payroll.csv` (left/"expected") against the broken `payroll.csv` (right/"actual") row by row via `reconcile/engine.Reconcile`. Because the row is present on the left but absent on the right, the engine surfaces `OutcomeMissingRight` for the key `employee_id=EMP-0042`.

Because the broken run is internally self-consistent (its `operating_income` was derived from 99 employees and its leaf contributions sum to its own stated value), `trace.Trace` on the broken bundle does NOT return `ErrContributionMismatch`. The value difference is instead surfaced by a separate cross-run assertion: the test independently recomputes `operating_income` from the broken CSVs and asserts that the result differs from the happy-path `operating_income.ValueCanonical` by exactly EMP-0042's total compensation.

**Breakage (b) — off-by-one-cent expense:**

One row in `broken/data/expenses.csv` has its `amount` field incremented by `0.01` (one cent). The broken-run `total_expenses` token therefore differs from the happy-path value by `0.01`. When the happy-path `expenses.csv` (left) is reconciled against the broken `expenses.csv` (right) with `ModeAmountTolerance` and `ToleranceConfig{Amount: "0.00"}` (zero tolerance), the engine surfaces `OutcomeMismatch` for the tampered row with a non-empty `Differences` array. The exact `Reason` string is produced by the engine at runtime (format: `"<field> delta <delta> exceeds tolerance <tol>"`); the test asserts `Outcome == OutcomeMismatch` and `len(Differences) > 0` only — do not pre-assert the literal reason string. Capture the actual reason in the committed golden and assert byte-equality against it.

**Broken run assertions (`broken/e2e_test.go`):**

1. The broken bundle still passes `bundle.Verify` and `bundle.Replay` — cryptographic integrity is unaffected by the semantic breakages.
2. After row-level CSV reconciliation (happy-path CSVs as left, broken CSVs as right) via `reconcile/engine.Reconcile(ctx, req, []router.NormalizedFile{...})`:
   - Exactly one `OutcomeMissingRight` record with key field `employee_id` = `"EMP-0042"`.
   - Exactly one `OutcomeMismatch` record for the tampered expense row with `len(result.Differences) > 0`.
3. The broken run's independently-recomputed `operating_income` canonical value differs from the happy-path `operating_income.ValueCanonical`.
4. The reconciliation report JSON, mismatches CSV, and HTML audit report are committed as goldens and asserted byte-equal.

**Reconciliation contract:** `reconcile/engine.Reconcile` signature is `Reconcile(ctx context.Context, req request.Request, files []router.NormalizedFile) (*ResultSet, error)`. It consumes a reconcile-request fixture (specifying `left` and `right` file IDs, `match_on`, `compare`, `mode`, and `tolerance`) and parsed `router.NormalizedFile` slices. It does NOT accept audit packs or cross-pack token graphs. The reconciliation is performed directly between the two `payroll.csv` files (and separately, the two `expenses.csv` files) represented as `NormalizedFile` values.

**Direction convention:** Happy-path CSV = left ("expected"); broken CSV = right ("actual"). A row present in happy but absent in broken → `OutcomeMissingRight`. A row present in broken but absent in happy → `OutcomeMissingLeft`.

### HTML audit report

**Package placement:** `audit/report/html.go` (sibling to `audit/pack/`).

Rationale: `audit/report/` is the natural peer of `audit/pack/` — pack produces the document, report renders it. A sibling `audit/html/` would require a deeper import path for no added clarity. The `audit/report/` name also leaves room for future text or PDF renderers alongside `html.go`.

**Exported API:**

```go
// audit/report/html.go
package report

import "github.com/provenant/provenant/audit/pack"

// RenderHTML renders doc as a self-contained HTML file.
// The output is deterministic: identical input always produces identical bytes.
// No external network requests, no JavaScript, no wall-clock timestamps in the body.
func RenderHTML(doc *pack.Document) ([]byte, error)
```

**Rendered sections (in order):**

1. **Header** — `schema_version`, `bundle_sha256` (full hex), `run_id`, `tenant_id`. Title: "Provenant Audit Report".
2. **Summary tiles** — token count, captured-reads count, audit-entry count, mismatch count (0 if no reconciliation data present), submitted-answer value + unit.
3. **Submitted answer** — `token_id`, `value_canonical`, `unit`; provenance note ("grounded — no literal value in request").
4. **Reconciliation outcomes** (section omitted entirely if pack has no reconciliation data) — outcome counts table (one row per `reconcile/engine` outcome kind), followed by mismatches table with columns: `result_id`, `outcome`, `key`, `field`, `left`, `right`, `reason`.
5. **Derivation graph** — tree of derived → leaf tokens rooted at the submitted-answer token, rendered as nested `<ul><li>` elements with contribution amounts. Banner at top: `Contribution invariant: OK` (green) or `VIOLATED` (red).
6. **Captured reads** — table: source name, selector, sha256 (first 16 hex chars + "…"), bytes read.
7. **Audit chain** — kind histogram table; prev-hash continuity check (OK / BROKEN with first broken link).
8. **Manifest** — artifacts table: name, sha256 (full).

**Determinism guarantees:**

- All map iteration uses `sort.Strings` / `sort.Slice` directly (the module targets Go 1.22; `slices.Sorted(maps.Keys(...))` is Go 1.23 and must not be used). State the sort idiom in a code comment at each map-iteration site.
- Embedded JSON snippets (e.g. individual audit entries in a tooltip) are JCS-canonicalized via `encoding/json` with sorted keys.
- No `time.Now()` calls anywhere in the renderer. The report header shows the pack's own `bundle_sha256` as its identity; there is no "generated at" timestamp.
- HTML is produced via `html/template` with a single embedded template string constant — no external template files, no `go:embed` for the template itself (the template is a Go string literal so it is part of the binary and always consistent).
- CSS is a `<style>` block inlined in `<head>`. Fonts are system-stack only (`font-family: system-ui, sans-serif`).

**Minimal CSS tile classes:** `tile`, `tile-ok`, `tile-warn`, `tile-error`, `banner-ok`, `banner-violated`, `table-plain`. No framework.

### Determinism and goldens

The scaled bundle is larger than the phase-08 bundle (~1 880 minted tokens vs 15) but remains fully deterministic. We keep byte-equal golden comparison for the scaled bundle and trace — the same rationale applies as in phase-08: any unintentional change to the kernel, op catalog, or serialization is immediately visible as a golden diff. The regeneration workflow is:

```
# Regenerate scaled happy-path goldens
UPDATE_GOLDEN=1 go test ./examples/derivation-graph/scaled/... -run TestScaledFauxbooksEndToEnd -v

# Regenerate broken goldens
UPDATE_GOLDEN=1 go test ./examples/derivation-graph/scaled/broken/... -run TestBrokenFauxbooksEndToEnd -v

# Regenerate HTML goldens
UPDATE_GOLDEN=1 go test ./audit/report/... -run TestRenderHTMLGolden -v
UPDATE_GOLDEN=1 go test ./access/cli/... -run TestAuditHTMLVerb -v
```

The `UPDATE_GOLDEN` env-var pattern is the same convention already used in the phase-08 test harness. Large golden files (the scaled bundle JSON, two HTML files) are stored in the same directory as their test and committed. They are not split across platforms; if any platform produces a different byte sequence, the root cause is fixed in code.

The phase-08 demo golden at `examples/derivation-graph/golden.bundle.json` is not touched by phase-09 work. The two demo subtrees are independent.

### Test harness changes

Phase-09 introduces two new `e2e_test.go` files and one new package test; no changes to the existing `examples/derivation-graph/e2e_test.go`.

**`examples/derivation-graph/scaled/e2e_test.go`:**

```go
func TestScaledFauxbooksEndToEnd(t *testing.T) {
    // drives full CLI sequence via cli.Main with fixed clock
    // asserts bytes.Equal(got, golden.bundle.json)
    // asserts bytes.Equal(traceJSON, golden.trace.json)
    // asserts sum(contributions) == operating_income.ValueCanonical
    // asserts bundle.Verify and bundle.Replay pass
}
func TestScaledFauxbooksOperatingIncomeValue(t *testing.T) {
    // recomputes operating income independently from CSVs; asserts exact canonical decimal
}
func TestScaledFauxbooksLeafCount(t *testing.T) {
    // asserts len(trace.Leaves) == 1980 (sum of all minted rows)
}
func TestScaledFauxbooksReplayDeterminism(t *testing.T) {
    // runs sequence twice in separate temp dirs; asserts identical bundle bytes
}
```

**`examples/derivation-graph/scaled/broken/e2e_test.go`:**

```go
func TestBrokenFauxbooksEndToEnd(t *testing.T) {
    // drives broken CLI sequence
    // asserts bundle.Verify passes (integrity unaffected)
    // runs reconcile/engine.Reconcile(ctx, req, []router.NormalizedFile{happyPayroll, brokenPayroll})
    // asserts exactly 1 OutcomeMissingRight for key employee_id=EMP-0042
    // runs reconcile/engine.Reconcile(ctx, req, []router.NormalizedFile{happyExpenses, brokenExpenses})
    // asserts exactly 1 OutcomeMismatch with len(Differences) > 0
    //   (do NOT assert a literal reason string; capture actual reason in golden)
    // asserts broken operating_income.ValueCanonical != happy operating_income.ValueCanonical
    //   (cross-run value comparison, NOT a trace.Trace ErrContributionMismatch check)
    // asserts reconciliation JSON, mismatches CSV byte-equal goldens
}
// TestBrokenFauxbooksHTMLReport is registered in step 5 (after the HTML renderer lands).
```

**`audit/report/html_test.go`:**

```go
func TestRenderHTMLGolden(t *testing.T) {
    // tiny fixture pack (≤ 5 tokens) → RenderHTML → byte-equal golden HTML
}
func TestRenderHTMLDeterministic(t *testing.T) {
    // calls RenderHTML twice on same input; asserts bytes.Equal
}
func TestRenderHTMLNoExternalAssets(t *testing.T) {
    // parses HTML; asserts no <script> tags, no src= with http, no href= with http
}
func TestRenderHTMLContributionInvariantOK(t *testing.T) {
    // fixture pack where invariant holds; asserts "Contribution invariant: OK" in output
}
func TestRenderHTMLContributionInvariantViolated(t *testing.T) {
    // fixture pack where invariant is violated; asserts "VIOLATED" in output
}
```

**`access/cli/audithtmlverb_test.go`:**

```go
func TestAuditHTMLVerbHappyPath(t *testing.T) {
    // writes scaled happy-path pack to temp file; runs audit-html verb; asserts HTML golden
}
func TestAuditHTMLVerbBrokenPack(t *testing.T) {
    // writes broken pack to temp file; runs audit-html verb; asserts HTML golden
    // asserts "VIOLATED" in output HTML
}
func TestAuditHTMLVerbMissingPack(t *testing.T) {
    // runs audit-html with nonexistent --pack path; asserts exit 2
}
```

## Implementation Steps (sequential PRs — one per step)

### Step 1 — PR 1: Scaled fauxbooks generator

Create `examples/derivation-graph/scaled/gen/gen.go` producing `payroll.csv` (100 rows), `expenses.csv` (500 rows), `revenue.csv` (1 200 rows), `refunds.csv` (80 rows) deterministically from seed 42 + per-table sub-seeds. Amounts as canonical decimal strings. Run the generator once; commit the four CSVs under `examples/derivation-graph/scaled/data/`. Add a brief `examples/derivation-graph/scaled/gen/README.md` runbook (how to regenerate, what `UPDATE_GOLDEN` does). No kernel changes; no test changes to existing phase-08 suite.

### Step 2 — PR 2: Scaled happy-path e2e test

Write request fixtures under `examples/derivation-graph/scaled/requests/` (run-start, read-mint ×4, calc chain ×9, submit-answer, run-finish). Write `examples/derivation-graph/scaled/e2e_test.go` with `TestScaledFauxbooksEndToEnd`, `TestScaledFauxbooksOperatingIncomeValue`, `TestScaledFauxbooksLeafCount`, `TestScaledFauxbooksReplayDeterminism`. Run with `UPDATE_GOLDEN=1` to produce `golden.bundle.json` and `golden.trace.json`; commit goldens. All four tests must pass on Linux and macOS before merge.

### Step 3 — PR 3: Broken sibling dataset

Produce `examples/derivation-graph/scaled/broken/data/` with the two deliberate breakages (EMP-0042 dropped from `payroll.csv`; one cent added to a target expense row in `expenses.csv`). Mirror request fixtures under `broken/requests/`. Write `broken/e2e_test.go` with `TestBrokenFauxbooksEndToEnd` only (do not register `TestBrokenFauxbooksHTMLReport` here — defer that to step 5 when the renderer exists). Use `reconcile/engine.Reconcile(ctx, req, []router.NormalizedFile{...})` to compare the two `payroll.csv` files (happy = left, broken = right) and separately the two `expenses.csv` files. Assert `OutcomeMissingRight` for EMP-0042 (row absent from right/broken), `OutcomeMismatch` with non-empty `Differences` for the tampered expense row, and a cross-run `operating_income.ValueCanonical` inequality check. Do NOT assert `trace.Trace` returns `ErrContributionMismatch`. Run `TestBrokenFauxbooksEndToEnd` with `UPDATE_GOLDEN=1` to produce `broken/golden.bundle.json`, `broken/golden.reconciliation-report.json`, `broken/golden.mismatches.csv`. Commit goldens and confirm the assertions hold.

### Step 4 — PR 4: HTML audit report renderer

Create `audit/report/html.go` exporting `RenderHTML(*pack.Document) ([]byte, error)`. Implement all eight sections described in Technical Design. Use `html/template` with an embedded template string. Sort all map keys. No `time.Now()`. No JS. No external assets. Write `audit/report/html_test.go` with `TestRenderHTMLGolden`, `TestRenderHTMLDeterministic`, `TestRenderHTMLNoExternalAssets`, `TestRenderHTMLContributionInvariantOK`, `TestRenderHTMLContributionInvariantViolated`. Run `UPDATE_GOLDEN=1` to produce `audit/report/testdata/golden.html` from the tiny fixture pack; commit golden. `go test ./audit/report/...` must pass.

### Step 5 — PR 5: CLI verb `audit-html`

Add `access/cli/audithtmlverb.go` implementing `provenant audit-html --pack <pack.json> --out <report.html>`. Register the verb in the CLI's verb table. Write `access/cli/audithtmlverb_test.go` with `TestAuditHTMLVerbHappyPath`, `TestAuditHTMLVerbBrokenPack`, `TestAuditHTMLVerbMissingPack`. Add `TestBrokenFauxbooksHTMLReport` to `broken/e2e_test.go` now that the renderer exists (this is the first time it is registered). Run `UPDATE_GOLDEN=1` for both integration-test HTML goldens (`scaled/golden.report.html`, `scaled/broken/golden.report.html`); commit goldens. `go test ./access/cli/...` and `go test ./examples/derivation-graph/scaled/...` must pass end-to-end.

### Step 6 — PR 6: Demo polish and cross-linking

Write `examples/derivation-graph/scaled/README.md` with: dataset description, derivation graph diagram (ASCII), step-by-step commands for running both demos (`go test -v -run TestScaledFauxbooksEndToEnd ./examples/derivation-graph/scaled/`), commands for generating HTML reports (`provenant audit-html --pack ...`), a note on the broken demo and what each report section surfaces, and the regeneration commands. Add a "Scaled demo (Phase 09)" section to `examples/derivation-graph/README.md` with a one-paragraph description and a link to the new subfolder README. `go test ./...` and `go vet ./...` must pass repo-wide.

## Tests Required

| Test | What it asserts | File |
|---|---|---|
| `TestScaledFauxbooksEndToEnd` | Full scaled CLI sequence runs without error; bundle bytes match golden; trace JSON matches golden; `bundle.Verify` and `bundle.Replay` pass | `examples/derivation-graph/scaled/e2e_test.go` |
| `TestScaledFauxbooksOperatingIncomeValue` | Operating income computed independently from CSVs equals the bundle token's `ValueCanonical` | `examples/derivation-graph/scaled/e2e_test.go` |
| `TestScaledFauxbooksLeafCount` | `len(trace.Leaves) == 1980` (total minted rows across all four CSVs) | `examples/derivation-graph/scaled/e2e_test.go` |
| `TestScaledFauxbooksReplayDeterminism` | Two independent runs in separate temp dirs produce identical bundle bytes | `examples/derivation-graph/scaled/e2e_test.go` |
| `TestBrokenFauxbooksEndToEnd` | Broken bundle still passes `Verify`; row-level CSV reconciliation (happy=left, broken=right) surfaces exactly 1 `OutcomeMissingRight` for `employee_id=EMP-0042` and exactly 1 `OutcomeMismatch` with non-empty `Differences`; broken `operating_income.ValueCanonical` differs from the happy-path value (cross-run assertion); report JSON and mismatches CSV byte-equal their goldens | `examples/derivation-graph/scaled/broken/e2e_test.go` |
| `TestBrokenFauxbooksHTMLReport` | `RenderHTML` on broken pack produces byte-equal golden HTML; HTML contains `"VIOLATED"` banner; HTML contains `"EMP-0042"` in mismatches table | `examples/derivation-graph/scaled/broken/e2e_test.go` |
| `TestRenderHTMLGolden` | Tiny fixture pack → `RenderHTML` produces byte-equal `testdata/golden.html` | `audit/report/html_test.go` |
| `TestRenderHTMLDeterministic` | Two calls to `RenderHTML` on identical input produce identical bytes | `audit/report/html_test.go` |
| `TestRenderHTMLNoExternalAssets` | Parsed HTML contains no `<script>` tags and no `src=` or `href=` attributes with `http` scheme | `audit/report/html_test.go` |
| `TestRenderHTMLContributionInvariantOK` | Fixture pack with valid contribution sum produces HTML containing `"Contribution invariant: OK"` | `audit/report/html_test.go` |
| `TestRenderHTMLContributionInvariantViolated` | Fixture pack with mismatched contribution sum produces HTML containing `"VIOLATED"` | `audit/report/html_test.go` |
| `TestAuditHTMLVerbHappyPath` | `audit-html` verb on scaled happy-path pack writes HTML that byte-equals its golden | `access/cli/audithtmlverb_test.go` |
| `TestAuditHTMLVerbBrokenPack` | `audit-html` verb on broken pack writes HTML containing `"VIOLATED"` and byte-equals its golden | `access/cli/audithtmlverb_test.go` |
| `TestAuditHTMLVerbMissingPack` | `audit-html` with a nonexistent `--pack` path exits with code 2 and an error message | `access/cli/audithtmlverb_test.go` |
| `TestFauxbooksEndToEnd` (existing, phase-08) | Phase-08 demo unchanged; all existing assertions still pass after phase-09 lands | `examples/derivation-graph/e2e_test.go` |

### Amendment: Leaf Count Corrected to 1980 (2026-05-20)

The kernel constraint "one read-mint per (source, outputPath) pair" means that payroll's salary and bonus outputs minted from the same source rows double the payroll leaf count: 100 payroll rows × 2 outputs = 200 leaves. Leaf-count breakdown: 100 (revenue) + 100 (COGS) + 500 (opex) + 200 (payroll) = 900 leaves initially planned, but with the 6-month calculation chain: 100 + 100 + 500 + 1200 (payroll=100×2×6) = 1980 leaves total.

## Definition of Done

- [ ] All six implementation steps merged to `main` via separate PRs, each squash-merged after subagent review approval.
- [ ] `go test ./...` passes on Linux amd64 and macOS arm64 after each step merge.
- [ ] `go vet ./...` passes repo-wide.
- [ ] `go test ./examples/derivation-graph/...` covers both phase-08 (unchanged) and phase-09 scaled + broken tests.
- [ ] Golden bundle bytes are bit-identical across Linux and macOS. If divergence appears, the root cause is fixed in code — no per-platform goldens.
- [ ] Contribution invariant holds for the scaled happy-path run: `sum(LeafContribution.Contribution) == operating_income.ValueCanonical`.
- [ ] Broken run: `bundle.Verify` passes; row-level CSV reconciliation surfaces exactly two anomalies (one `OutcomeMissingRight` for EMP-0042, one `OutcomeMismatch` for the tampered expense row); the broken run's `operating_income.ValueCanonical` differs from the happy-path value (cross-run assertion); `trace.Trace` on the broken bundle returns a valid `Graph` (no `ErrContributionMismatch`).
- [ ] HTML report is self-contained: opens from disk in a browser with no network requests, no JS errors, no broken layout.
- [ ] HTML goldens are byte-identical across platforms and across repeated runs on the same input.
- [ ] `provenant audit-html --help` prints usage; `--pack` and `--out` flags documented.
- [ ] `examples/derivation-graph/scaled/README.md` exists and walks a reader through both demos end-to-end.
- [ ] `examples/derivation-graph/README.md` links to the scaled demo subfolder.
- [ ] Phase-08 demo (`examples/derivation-graph/`) is untouched: its goldens, test, and README are identical to their pre-phase-09 state.
- [ ] No `time.Now()` calls anywhere in `audit/report/html.go` or its test helpers.
- [ ] No `<script>` tags and no external URLs in generated HTML (asserted by `TestRenderHTMLNoExternalAssets`).
- [ ] Regeneration commands documented in both `scaled/README.md` and `scaled/gen/README.md`.

## Risks / Open Questions

- **Bundle size and golden file size.** A ~1 880-token bundle with full JCS serialization may produce a golden file in the range of 10–30 MB. Git handles this fine for a demo fixture, but reviewers should confirm the repo's `.gitattributes` does not exclude large JSON files from LFS rules. If the bundle exceeds 50 MB uncompressed, revisit whether a manifest-hash-only golden is preferable — but the current estimate does not warrant that.
- **Cross-platform decimal formatting.** `shopspring/decimal` is locale-insensitive, but `fmt.Sprintf` with `%v` on a `Decimal` may differ if the library's `String()` method is ever changed. The canonical form should be produced via `d.StringFixed(2)` (or the existing `ValueCanonical` field) to be explicit.
- **HTML template escaping.** `html/template` auto-escapes values inserted into HTML context. Canonical decimal strings and hex digests are ASCII-safe, but any free-text fields (source names, tenant IDs) must pass through the template's contextual escaper rather than `text/template`. Verify the import path is `html/template`, not `text/template`.
- **Contribution invariant on broken run (resolved).** The broken run's `total_payroll` is derived from 99 employees and is internally self-consistent. `trace.Trace` only checks within-bundle invariants (sum of leaf contributions == the bundle's own root token value); it does NOT perform cross-run comparisons. Therefore `trace.Trace` on the broken bundle returns a valid `Graph`, not `ErrContributionMismatch`. The breakage is surfaced via (a) the row-level CSV reconciliation yielding `OutcomeMissingRight` for EMP-0042 and (b) a direct cross-run equality assertion on `operating_income.ValueCanonical`. This is the correct approach; no further resolution needed.
- **`audit/report/` import cycle risk.** `audit/report` imports `audit/pack`. Confirm `audit/pack` does not import any `access/` package (which would import `audit/report` and create a cycle). The existing package graph shows `audit/pack` has no `access/` dependencies, so the cycle risk is low but worth verifying with `go list -f '{{.ImportPath}} {{.Imports}}' ./audit/...` before wiring.
- **CLI verb registration.** The CLI verb table lives in `access/cli/cli.go`. Step 5 adds one entry. If any other in-flight branch also modifies that file, merge conflicts are possible — coordinate with any concurrent work.
- **Broken dataset maintenance.** The broken dataset is a static fixture committed to the repo. If the happy-path generator is ever changed (e.g. to fix a decimal rounding), the broken dataset and its goldens must be regenerated in the same PR. Document this in `scaled/README.md`.

## Dependencies

- Phases 01–08 ([phase-01](./phase-01-token-dag-schema.md) through [phase-08](./phase-08-e2e-fauxbooks.md)) must be merged to `main` before this phase begins. Phase-08's `examples/derivation-graph/` demo is the structural template for the scaled demo.
- `audit/pack` (`audit/pack/pack.go`) must expose `*Document` as a concrete struct (not an interface) for `RenderHTML` to consume. Verify before step 4.
- `reconcile/engine` (`reconcile/engine/engine.go`) must be importable without pulling in CLI or binary dependencies. Its actual signature is `Reconcile(ctx context.Context, req request.Request, files []router.NormalizedFile) (*ResultSet, error)` — it operates on parsed CSV files, not audit packs. Verify importability with `go list -deps ./reconcile/...` before step 3.
- `kernel/public/trace` (`kernel/public/trace/trace.go`) exports `ErrContributionMismatch` as a sentinel error (within-bundle invariant only). The broken-run test does NOT rely on this error being raised; it is listed here only for awareness.
- The module targets Go 1.22 (`go.mod`). Use `sort.Strings` / `sort.Slice` throughout `audit/report/html.go` and all other new code. Do not use `slices.Sorted` or `maps.Keys`.

---

# /goal prompt (single kickoff for all six PRs)

> A condensed copy of this prompt formatted for the **/goal** command (under 4,000 characters) is maintained alongside this plan at `.planning/derivation-graph/goal-prompt.md`. Keep both in sync if the plan changes.

---
description: Execute Phase 09 — Scaled Demo, Broken Dataset, and HTML Audit Report
---

You are autonomously executing Phase 09 of the Derivation Graph plan at `.planning/derivation-graph/phase-09-scaled-demo-and-html-report.md`. You will deliver all six implementation steps sequentially, one PR per step, end-to-end in a single run.

## Operating mode

1. Enter orchestrator mode immediately. Load the `engineering-workflow` skill and follow it for branching, validation, review cycles, PR lifecycle, and merge discipline.
2. Load `conventional-commit-guard` before every commit and PR title.
3. Load `go-implementation-quality` for all Go implementation work. Load `go-testing-fuzzing` for any test files. Load `go-input-validation-security` only for the CLI verb input handling in step 5.
4. Models: `claude-sonnet-4-6` for implementation; `claude-opus-4-7` (thinking on) for architecture decisions and subagent review; `claude-haiku-4-5` for golden-file generation and deterministic mechanical edits.

## Before starting

Read `.planning/derivation-graph/phase-09-scaled-demo-and-html-report.md` in full. Read `examples/derivation-graph/e2e_test.go` and `audit/pack/pack.go` to confirm the structural patterns to mirror. Do not modify any phase plan files unless a discovered constraint forces a documented amendment (make that amendment in a separate `docs(derivation-graph): amend phase-09 plan` PR before resuming).

## Per-step loop

Execute PRs 1 through 6 sequentially, one PR per Implementation Step, stopping after each PR for subagent review approval before opening the next.

For each step NN in 1..6 (in order):

1. Sync local main: `git fetch origin && git checkout main && git pull --ff-only origin main`.
2. Branch: `feat/derivation-graph-phase-09-step-NN-<slug>` from updated main.
3. Read the step's description in the Implementation Steps section of the phase plan. Delegate implementation to a `claude-sonnet-4-6` subagent with the step description, the relevant Technical Design subsections, and the Tests Required rows for that step as its sole spec.
4. On first run (golden files absent), the implementer subagent must run the appropriate `UPDATE_GOLDEN=1 go test ...` command from the phase plan's Determinism and goldens section to generate golden files, then commit them alongside the implementation.
5. Validation gates (all must pass before opening a PR):
   - `go test ./...`
   - `go vet ./...`
   - Any step-specific test commands listed in Tests Required.
   - For steps 2, 3, 5: verify the existing phase-08 golden test `TestFauxbooksEndToEnd` still passes byte-equal (`go test ./examples/derivation-graph/... -run TestFauxbooksEndToEnd`).
6. Run an independent reviewer subagent (`claude-opus-4-7`, thinking on) against the relevant rows of the Tests Required table and the Definition of Done checklist items applicable to the step. The reviewer must not approve if any golden is missing, any test is skipped, or `go test ./...` is failing.
7. If reviewer requests changes, route fixes to a `claude-sonnet-4-6` subagent and re-review. Loop until APPROVE or APPROVE_WITH_FIXES.
8. Open a PR titled `feat(derivation-graph): phase 09 step NN — <short title>` using `conventional-commit-guard`. Include in the PR body: which Tests Required rows are satisfied, which Definition of Done items are checked off, the regeneration command for any new goldens.
9. Wait for CI / checks. On clean checks and reviewer approval, squash-merge with `--delete-branch` and reset local main to `origin/main`.
10. Record the merged squash commit SHA. Proceed to step NN+1.

## Stop conditions

Stop and report if any of: a reviewer returns CHANGES_REQUESTED twice in a row on the same step; validation gates fail after two implementer attempts; a cross-step dependency surfaces that contradicts the plan; the user interrupts.

On a stop: report the last merged step, current branch, what blocked, and the smallest unblocking action.

## Final step (6)

After merging step 6 (demo polish):
- Run `go test ./examples/derivation-graph/...` on `main` and confirm all tests pass — both phase-08 and phase-09.
- Run `provenant audit-html --pack examples/derivation-graph/scaled/golden.bundle.json --out /tmp/scaled-happy.html` and `provenant audit-html --pack examples/derivation-graph/scaled/broken/golden.bundle.json --out /tmp/scaled-broken.html`; confirm both files open in a browser and display the correct invariant banner.
- Report: last merged step commit SHA, full Definition of Done checklist with each item marked, total wall time across all 6 steps.

## Constraints

- One step = one branch = one PR = one squash commit. Never bundle steps.
- Phase-08 demo (`examples/derivation-graph/`) must be untouched throughout. The `TestFauxbooksEndToEnd` golden test is a regression sentinel for every step.
- Golden files are committed; CI does not regenerate. The `UPDATE_GOLDEN=1` flag is only used during initial generation by the implementer subagent and must never appear in CI scripts.
- Determinism is non-negotiable: fixed seeds, sorted map keys, JCS for embedded JSON, no `time.Now()` in any deterministic payload or the HTML renderer.
- HTML is self-contained: no `<script>` tags, no external URLs, openable from disk. `TestRenderHTMLNoExternalAssets` is a hard gate.
- Do not push directly to `main`. All work lands via squash-merged PR.
- Do not skip hooks, signing, or CI.
- Broken-run semantics are fully resolved in this plan: `trace.Trace` does NOT raise `ErrContributionMismatch` on the broken bundle. Use `OutcomeMissingRight` (not `OutcomeMissingLeft`) for EMP-0042, and assert `len(Differences) > 0` (not a literal reason string) for the tampered expense row.
