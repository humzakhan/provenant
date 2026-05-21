# Provenant Broken Fauxbooks — Audit Report Artifact

This directory contains a self-contained audit report for the broken Fauxbooks
derivation-graph demo (phase-09 step-03, with injected mismatches).

## How to open

Open `report.html` in any modern browser:

```
open report.html          # macOS
xdg-open report.html      # Linux
```

Or navigate to the file directly in your browser address bar:
```
file:///path/to/this/artifact/report.html
```

## Clickable source file links

The report contains `file://` links that open the CSV source files directly in
your browser. These links work because the report and the CSV files sit in the
same directory. Click any file card or "view file →" button in the Plain view
tab to open the corresponding source file locally.

**Note:** browsers may require you to allow local file access. On macOS, Chrome
and Firefox support `file://` links when the report itself is opened via a
`file://` URL (not `localhost`).

## Report structure

The report has two tabs:

- **Plain view** (default) — visual file cards for each source, the
  contribution invariant OK banner, reconciliation mismatch details, the
  submitted answer hero callout, and the full derivation audit chain.
- **Technical detail** — derivation graph table, reconciliation mismatches,
  captured reads, audit chain sequence, and manifest.

## Source files

The four CSV data files are included in this directory so that `file://` links
in the report resolve when opened locally:

| File | Description |
|------|-------------|
| `revenue.csv` | 1200-row revenue transactions |
| `payroll.csv` | 99-row payroll records (EMP-0042 removed) |
| `expenses.csv` | 500-row operating expense records (EXP-00001 tampered +$0.01) |
| `refunds.csv` | 80-row refund records |

## Notes

- The contribution invariant remains OK for this broken fixture: the bundle and
  report are internally self-consistent, and verify/replay integrity is not
  broken. The injected issues appear as reconciliation/business mismatches.
- The report is fully self-contained (no external assets, offline-capable).
- Source file paths in the report are absolute `file://` paths pointing at the
  CSVs in this directory. Moving the directory requires regenerating the report
  with `BUILD_ARTIFACT=1 go test ./examples/derivation-graph/scaled/broken/ -run TestBrokenFauxbooksArtifact`.
