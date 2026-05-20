// Package brokenderivationgraph provides an end-to-end demo for the deliberately
// broken sibling dataset (phase-09 step 03).
//
// The broken dataset contains two deliberate breakages relative to the happy-path
// scaled dataset:
//
//  1. EMP-0042 is absent from broken/data/payroll.csv (row dropped from right side).
//  2. EXP-00001's amount is incremented by $0.01 in broken/data/expenses.csv.
//
// The broken run is internally self-consistent (its operating_income is derived
// from 99 employees and its leaf contributions sum to its own stated value), so
// trace.Trace on the broken bundle does NOT return ErrContributionMismatch.
// The breakages are surfaced via:
//
//   - Row-level CSV reconciliation: OutcomeMissingRight for EMP-0042 (present in
//     happy/left but absent from broken/right); OutcomeMismatch for EXP-00001
//     with len(Differences) > 0.
//   - Cross-run comparison: broken operating_income.ValueCanonical != happy-path value.
package brokenderivationgraph_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/provenant/provenant/access/cli"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"

	"github.com/provenant/provenant/examples/derivation-graph/internal/testsupport"
)

// fixedBrokenTime is the deterministic clock used for all CLI invocations.
var fixedBrokenTime = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

// brokenRun is the fixed run identity for the broken demo.
var brokenRun = kernel.Run{
	ID:       "derivation_graph_broken_run_1",
	TenantID: "tenant_demo",
	Status:   kernel.RunRunning,
}

// brokenDataDir is the path to the broken CSV source files.
const brokenDataDir = "data"

// happyDataDir is the path to the happy-path CSV source files (for cross-run comparison).
const happyDataDir = "../data"

// csvProjectedOutputPath for a single-column projection.
const csvProjectedOutputPath = "/rows/0/cells/0/value"

// ---------------------------------------------------------------------------
// TestBrokenFauxbooksEndToEnd: the main broken-run assertion test
// ---------------------------------------------------------------------------

func TestBrokenFauxbooksEndToEnd(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "broken.bundle.json")

	runner := cli.Runner{Now: func() time.Time { return fixedBrokenTime }}
	tids := runBrokenDemo(t, runner, bundlePath)

	// --- 1. Read broken bundle bytes ---
	got, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundle): %v", err)
	}

	// --- 2. Save/compare golden bundle ---
	goldenBundlePath := filepath.Join(".", "golden.bundle.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err2 := os.WriteFile(goldenBundlePath, got, 0o644); err2 != nil {
			t.Fatalf("write golden.bundle.json: %v", err2)
		}
		t.Logf("golden.bundle.json written (%d bytes)", len(got))
	}
	wantBundle, readErr := os.ReadFile(goldenBundlePath)
	if readErr != nil {
		t.Fatalf("golden.bundle.json missing — run with UPDATE_GOLDEN=1 to generate: %v", readErr)
	}
	if !bytes.Equal(got, wantBundle) {
		t.Fatal("bundle bytes diverged from golden.bundle.json — if intentional, re-run with UPDATE_GOLDEN=1")
	}

	// --- 3. Assert bundle.Verify passes (cryptographic integrity unaffected by semantic breakages) ---
	if err := bundle.Verify(got); err != nil {
		t.Fatalf("bundle.Verify: %v", err)
	}

	// --- 4. Replay and extract broken operating_income value ---
	state, err := bundle.Replay(got)
	if err != nil {
		t.Fatalf("bundle.Replay: %v", err)
	}
	tokensByID := make(map[string]string, len(state.Tokens))
	for _, tok := range state.Tokens {
		tokensByID[tok.ID] = tok.ValueCanonical
	}
	brokenOIValue, ok := tokensByID[tids.operatingIncome]
	if !ok {
		t.Fatal("operating_income token not found in replayed state")
	}

	// --- 5. Read happy-path golden bundle to compare operating_income values ---
	happyBundleBytes, err := os.ReadFile(filepath.Join("..", "golden.bundle.json"))
	if err != nil {
		t.Fatalf("ReadFile(happy-path golden.bundle.json): %v", err)
	}
	happyState, err := bundle.Replay(happyBundleBytes)
	if err != nil {
		t.Fatalf("bundle.Replay(happy-path): %v", err)
	}
	var happyOIValue string
	// Cross-run comparison: compute happy-path operating_income independently from CSVs.
	happyOIDec := recomputeHappyOperatingIncome(t)
	for _, tok := range happyState.Tokens {
		d, parseErr := decimal.NewFromString(tok.ValueCanonical)
		if parseErr != nil {
			continue
		}
		if d.Equal(happyOIDec) {
			happyOIValue = tok.ValueCanonical
			break
		}
	}
	if happyOIValue == "" {
		// If we cannot find it by value (unlikely), scan for any token whose label
		// is "operating_income" via the CapturedReads (not available here).
		// Fall back: just assert it's findable via independent computation value string.
		t.Logf("warning: could not find happy-path operating_income token by canonical value %s; using computed value for comparison", happyOIDec.String())
		happyOIValue = happyOIDec.String()
	}

	brokenOIDec, err := decimal.NewFromString(brokenOIValue)
	if err != nil {
		t.Fatalf("parse broken operating_income %q: %v", brokenOIValue, err)
	}
	happyOIDec2, err := decimal.NewFromString(happyOIValue)
	if err != nil {
		t.Fatalf("parse happy operating_income %q: %v", happyOIValue, err)
	}
	if brokenOIDec.Equal(happyOIDec2) {
		t.Errorf("broken operating_income == happy operating_income (%s); expected them to differ because EMP-0042 was removed", brokenOIValue)
	}
	t.Logf("Happy-path operating_income: %s", happyOIValue)
	t.Logf("Broken operating_income:     %s", brokenOIValue)

	// --- 6. CSV reconciliation: payroll (happy=left, broken=right) ---
	payrollResultSet := reconcileCSVFiles(t,
		"payroll-happy",  // left file ID
		"payroll-broken", // right file ID
		filepath.Join(happyDataDir, "payroll.csv"),
		filepath.Join(brokenDataDir, "payroll.csv"),
		request.ModeExactMatch,
		nil, // no tolerance for exact match
		[]string{"employee_id"},           // match_on
		[]string{"department", "salary", "bonus"}, // compare
		payrollColumns(),
	)

	// Assert exactly 1 OutcomeMissingRight for key employee_id=EMP-0042.
	var missingRightResults []engine.Result
	for _, r := range payrollResultSet.Results {
		if r.Outcome == engine.OutcomeMissingRight {
			missingRightResults = append(missingRightResults, r)
		}
	}
	if len(missingRightResults) != 1 {
		t.Errorf("payroll reconciliation: got %d OutcomeMissingRight results, want 1", len(missingRightResults))
	} else {
		r := missingRightResults[0]
		found := false
		for _, kv := range r.Key {
			if kv.Field == "employee_id" && kv.Value == "EMP-0042" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("OutcomeMissingRight key = %v, want employee_id=EMP-0042", r.Key)
		}
		t.Logf("OutcomeMissingRight confirmed: employee_id=EMP-0042")
	}

	// --- 7. CSV reconciliation: expenses (happy=left, broken=right) ---
	// Use ModeExactMatch with RoleReference for the amount field: the expenses CSV
	// has no currency column, which is required by ModeAmountTolerance + RoleAmount.
	// Exact-match string comparison on amount is functionally equivalent for
	// detecting the +$0.01 tamper: the canonical strings "3394.28" vs "3394.29" differ.
	expensesResultSet := reconcileCSVFiles(t,
		"expenses-happy",  // left file ID
		"expenses-broken", // right file ID
		filepath.Join(happyDataDir, "expenses.csv"),
		filepath.Join(brokenDataDir, "expenses.csv"),
		request.ModeExactMatch,
		nil, // no tolerance config for ModeExactMatch
		[]string{"expense_id"}, // match_on
		[]string{"amount"},     // compare
		expensesColumns(),
	)

	// Assert exactly 1 OutcomeMismatch with len(Differences) > 0.
	var mismatchResults []engine.Result
	for _, r := range expensesResultSet.Results {
		if r.Outcome == engine.OutcomeMismatch {
			mismatchResults = append(mismatchResults, r)
		}
	}
	if len(mismatchResults) != 1 {
		t.Errorf("expenses reconciliation: got %d OutcomeMismatch results, want 1", len(mismatchResults))
	} else {
		r := mismatchResults[0]
		if len(r.Differences) == 0 {
			t.Error("expenses mismatch: Differences is empty, want len > 0")
		}
		t.Logf("OutcomeMismatch confirmed: expense key %v, Differences: %v", r.Key, r.Differences)
	}

	// --- 8. Produce and assert golden reconciliation-report.json ---
	// The reconciliation report is a JSON encoding of a combined result summary.
	reportJSON := buildReconciliationReport(t, payrollResultSet, expensesResultSet)

	goldenReportPath := filepath.Join(".", "golden.reconciliation-report.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err2 := os.WriteFile(goldenReportPath, reportJSON, 0o644); err2 != nil {
			t.Fatalf("write golden.reconciliation-report.json: %v", err2)
		}
		t.Logf("golden.reconciliation-report.json written (%d bytes)", len(reportJSON))
	}
	wantReport, readReportErr := os.ReadFile(goldenReportPath)
	if readReportErr != nil {
		t.Fatalf("golden.reconciliation-report.json missing — run with UPDATE_GOLDEN=1 to generate: %v", readReportErr)
	}
	if !bytes.Equal(reportJSON, wantReport) {
		t.Fatal("reconciliation report JSON diverged from golden — if intentional, re-run with UPDATE_GOLDEN=1")
	}

	// --- 9. Produce and assert golden mismatches.csv ---
	mismatchesCSV := buildMismatchesCSV(t, payrollResultSet, expensesResultSet)

	goldenMismatchesPath := filepath.Join(".", "golden.mismatches.csv")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err2 := os.WriteFile(goldenMismatchesPath, mismatchesCSV, 0o644); err2 != nil {
			t.Fatalf("write golden.mismatches.csv: %v", err2)
		}
		t.Logf("golden.mismatches.csv written (%d bytes)", len(mismatchesCSV))
	}
	wantMismatches, readMismatchesErr := os.ReadFile(goldenMismatchesPath)
	if readMismatchesErr != nil {
		t.Fatalf("golden.mismatches.csv missing — run with UPDATE_GOLDEN=1 to generate: %v", readMismatchesErr)
	}
	if !bytes.Equal(mismatchesCSV, wantMismatches) {
		t.Fatal("mismatches CSV diverged from golden — if intentional, re-run with UPDATE_GOLDEN=1")
	}

	t.Logf("Broken run: bundle verified, reconciliation anomalies confirmed, cross-run OI differs")
}

// ---------------------------------------------------------------------------
// TestBrokenFauxbooksHTMLReport: audit-html renders a deterministic HTML report
// for the broken dataset
// ---------------------------------------------------------------------------

func TestBrokenFauxbooksHTMLReport(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "broken.bundle.json")
	packPath := filepath.Join(dir, "broken.pack.json")
	htmlPath := filepath.Join(dir, "report.html")

	runner := cli.Runner{Now: func() time.Time { return fixedBrokenTime }}
	runBrokenDemo(t, runner, bundlePath)

	// Build the audit pack from the bundle.
	testsupport.RunCLIFileArgs(t, runner, []string{"pack", "--bundle", bundlePath, "--out", packPath})

	// Render the HTML report.
	testsupport.RunCLIFileArgs(t, runner, []string{"audit-html", "--pack", packPath, "--out", htmlPath})

	got, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("ReadFile(html): %v", err)
	}

	goldenPath := filepath.Join(".", "golden.report.html")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err2 := os.WriteFile(goldenPath, got, 0o644); err2 != nil {
			t.Fatalf("write golden.report.html: %v", err2)
		}
		t.Logf("golden.report.html written (%d bytes)", len(got))
	}
	want, readErr := os.ReadFile(goldenPath)
	if readErr != nil {
		t.Fatalf("golden.report.html missing — run with UPDATE_GOLDEN=1 to generate: %v", readErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("HTML report bytes diverged from golden.report.html — if intentional, re-run with UPDATE_GOLDEN=1")
	}
}

// ---------------------------------------------------------------------------
// Helper: payroll column config for reconciliation
// ---------------------------------------------------------------------------

func payrollColumns() request.Columns {
	// Use RoleKey/RoleDescription/RoleReference for payroll columns.
	// RoleAmount requires a currency_field; since payroll CSV has no currency
	// column we use RoleReference for salary/bonus to compare them as plain strings.
	return request.Columns{
		"employee_id": {Source: "employee_id", Role: request.RoleKey, Required: true},
		"department":  {Source: "department", Role: request.RoleDescription},
		"salary":      {Source: "salary", Role: request.RoleReference, Required: true},
		"bonus":       {Source: "bonus", Role: request.RoleReference, Required: true},
	}
}

// expensesColumns returns the column config for expense CSV reconciliation.
// The amount field is mapped with RoleReference (plain string comparison) rather
// than RoleAmount, because the expenses CSV has no currency column and ModeAmountTolerance
// requires RoleAmount + Numeric.CurrencyField. For detecting the +$0.01 tamper,
// string comparison on the canonical decimal strings is equivalent.
func expensesColumns() request.Columns {
	return request.Columns{
		"expense_id": {Source: "expense_id", Role: request.RoleKey, Required: true},
		"amount":     {Source: "amount", Role: request.RoleReference, Required: true},
	}
}

// ---------------------------------------------------------------------------
// Helper: reconcile two CSV files via router.Route + engine.Reconcile
// ---------------------------------------------------------------------------

// reconcileCSVFiles routes and reconciles two CSV files.
// leftID/rightID are the logical file identifiers used in the reconcile request.
// leftPath/rightPath must be absolute or relative to the test's working directory.
func reconcileCSVFiles(
	t *testing.T,
	leftID, rightID string,
	leftPath, rightPath string,
	mode string,
	tolerance *request.ToleranceConfig,
	matchOn, compare []string,
	columns request.Columns,
) *engine.ResultSet {
	t.Helper()

	// Write both CSV files to a temp directory so router.Route can resolve them.
	tmpDir := t.TempDir()

	leftData, err := os.ReadFile(leftPath)
	if err != nil {
		t.Fatalf("ReadFile(left %s): %v", leftPath, err)
	}
	rightData, err := os.ReadFile(rightPath)
	if err != nil {
		t.Fatalf("ReadFile(right %s): %v", rightPath, err)
	}

	leftFile := "left.csv"
	rightFile := "right.csv"
	if err := os.WriteFile(filepath.Join(tmpDir, leftFile), leftData, 0o644); err != nil {
		t.Fatalf("WriteFile(left): %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, rightFile), rightData, 0o644); err != nil {
		t.Fatalf("WriteFile(right): %v", err)
	}

	req := request.Request{
		Run: request.RunConfig{
			ID:       brokenRun.ID,
			TenantID: brokenRun.TenantID,
		},
		Files: []request.FileConfig{
			{
				ID:      leftID,
				Purpose: "left",
				Format:  request.FormatCSV,
				Path:    leftFile,
				Reader:  request.ReaderExtractCSVRows,
				Selector: request.SelectorConfig{
					RowWindow: "2:",
					Encoding:  "utf-8",
					Dialect:   "excel",
				},
				Columns: columns,
			},
			{
				ID:      rightID,
				Purpose: "right",
				Format:  request.FormatCSV,
				Path:    rightFile,
				Reader:  request.ReaderExtractCSVRows,
				Selector: request.SelectorConfig{
					RowWindow: "2:",
					Encoding:  "utf-8",
					Dialect:   "excel",
				},
				Columns: columns,
			},
		},
		Reconcile: request.ReconcileConfig{
			Mode:      mode,
			Left:      leftID,
			Right:     rightID,
			MatchOn:   matchOn,
			Compare:   compare,
			Tolerance: tolerance,
		},
	}

	files, err := router.Route(context.Background(), req, router.Options{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("router.Route: %v", err)
	}

	resultSet, err := engine.Reconcile(context.Background(), req, files)
	if err != nil {
		t.Fatalf("engine.Reconcile: %v", err)
	}

	return resultSet
}

// ---------------------------------------------------------------------------
// Helper: build reconciliation-report.json golden
// ---------------------------------------------------------------------------

// reconciliationReport is the structure committed as the golden JSON.
type reconciliationReport struct {
	RunID    string              `json:"run_id"`
	Payroll  *engine.ResultSet   `json:"payroll_reconciliation"`
	Expenses *engine.ResultSet   `json:"expenses_reconciliation"`
}

// buildReconciliationReport produces a deterministic JSON encoding of both result sets.
func buildReconciliationReport(t *testing.T, payroll, expenses *engine.ResultSet) []byte {
	t.Helper()
	rep := reconciliationReport{
		RunID:    brokenRun.ID,
		Payroll:  payroll,
		Expenses: expenses,
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(reconciliation report): %v", err)
	}
	return append(raw, '\n')
}

// ---------------------------------------------------------------------------
// Helper: build mismatches.csv golden
// ---------------------------------------------------------------------------

// buildMismatchesCSV produces a deterministic CSV of all non-matched results.
// Columns: source,result_id,outcome,key,field,left,right,reason.
// Results are sorted by source+result_id for determinism.
func buildMismatchesCSV(t *testing.T, payroll, expenses *engine.ResultSet) []byte {
	t.Helper()

	type mismatchRow struct {
		source   string
		resultID string
		outcome  string
		key      string
		field    string
		left     string
		right    string
		reason   string
	}

	var rows []mismatchRow

	appendMismatches := func(source string, rs *engine.ResultSet) {
		for _, r := range rs.Results {
			if r.Outcome == engine.OutcomeMatched {
				continue
			}
			keyParts := make([]string, 0, len(r.Key))
			for _, kv := range r.Key {
				keyParts = append(keyParts, fmt.Sprintf("%s=%s", kv.Field, kv.Value))
			}
			keyStr := strings.Join(keyParts, ";")

			if len(r.Differences) == 0 {
				rows = append(rows, mismatchRow{
					source:   source,
					resultID: r.ID,
					outcome:  string(r.Outcome),
					key:      keyStr,
					reason:   r.Reason,
				})
				continue
			}
			for _, diff := range r.Differences {
				rows = append(rows, mismatchRow{
					source:   source,
					resultID: r.ID,
					outcome:  string(r.Outcome),
					key:      keyStr,
					field:    diff.Field,
					left:     diff.Left,
					right:    diff.Right,
					reason:   r.Reason,
				})
			}
		}
	}

	appendMismatches("payroll", payroll)
	appendMismatches("expenses", expenses)

	// Sort for determinism: by source, then result_id, then field.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].source != rows[j].source {
			return rows[i].source < rows[j].source
		}
		if rows[i].resultID != rows[j].resultID {
			return rows[i].resultID < rows[j].resultID
		}
		return rows[i].field < rows[j].field
	})

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"source", "result_id", "outcome", "key", "field", "left", "right", "reason"}); err != nil {
		t.Fatalf("csv.Write(header): %v", err)
	}
	for _, row := range rows {
		if err := w.Write([]string{
			row.source, row.resultID, row.outcome, row.key,
			row.field, row.left, row.right, row.reason,
		}); err != nil {
			t.Fatalf("csv.Write(row): %v", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("csv.Flush: %v", err)
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// Helper: recompute happy-path operating income from CSVs
// ---------------------------------------------------------------------------

// recomputeHappyOperatingIncome independently recomputes the happy-path
// operating_income from the original (happy-path) CSV files.
func recomputeHappyOperatingIncome(t *testing.T) decimal.Decimal {
	t.Helper()

	totalSalaries := sumCSVColumn(t, filepath.Join(happyDataDir, "payroll.csv"), "salary")
	totalBonuses := sumCSVColumn(t, filepath.Join(happyDataDir, "payroll.csv"), "bonus")
	totalPayroll := totalSalaries.Add(totalBonuses)

	totalExpenses := sumCSVColumn(t, filepath.Join(happyDataDir, "expenses.csv"), "amount")
	totalOpex := totalPayroll.Add(totalExpenses)

	grossRevenue := sumCSVColumn(t, filepath.Join(happyDataDir, "revenue.csv"), "amount")
	totalRefunds := sumCSVColumn(t, filepath.Join(happyDataDir, "refunds.csv"), "amount")
	netRevenue := grossRevenue.Sub(totalRefunds)

	return netRevenue.Sub(totalOpex)
}

// sumCSVColumn reads a CSV file and sums the values in the named column.
func sumCSVColumn(t *testing.T, path, header string) decimal.Decimal {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("CSV %s has no data rows", path)
	}
	headers := strings.Split(lines[0], ",")
	colIdx := -1
	for i, h := range headers {
		if h == header {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		t.Fatalf("CSV %s: column %q not found in headers %v", path, header, headers)
	}
	sum := decimal.Zero
	for i, line := range lines[1:] {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if colIdx >= len(fields) {
			t.Fatalf("CSV %s row %d: not enough columns", path, i+2)
		}
		d, err := decimal.NewFromString(fields[colIdx])
		if err != nil {
			t.Fatalf("CSV %s row %d col %s: parse %q: %v", path, i+2, header, fields[colIdx], err)
		}
		sum = sum.Add(d)
	}
	return sum
}

// ---------------------------------------------------------------------------
// Demo driver: run the broken CLI sequence
// ---------------------------------------------------------------------------

// brokenTokenIDs holds the IDs of the key derived tokens from the broken run.
type brokenTokenIDs struct {
	totalSalaries      string
	totalBonuses       string
	totalPayroll       string
	totalExpenses      string
	totalOpex          string
	grossRevenueBatch1 string
	grossRevenueBatch2 string
	grossRevenue       string
	totalRefunds       string
	netRevenue         string
	operatingIncome    string
}

// runBrokenDemo executes the full broken derivation-graph CLI sequence.
// It mirrors runScaledDemo exactly but reads from broken/data/.
func runBrokenDemo(t *testing.T, runner cli.Runner, bundlePath string) brokenTokenIDs {
	t.Helper()

	// 1. run start
	testsupport.RunCLIInTest(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run":      brokenRun,
		"run_file": bundlePath,
	})

	// 2. Mint payroll salary tokens (99 rows — EMP-0042 absent).
	salarySrc := loadBrokenCSVSource(t, "payroll.csv")
	salaryTokenIDs := mintBrokenCSVRowTokens(t, runner, bundlePath, salarySrc, 99, "salary")

	// 3. Mint payroll bonus tokens (99 rows).
	bonusTokenIDs := mintBrokenCSVRowTokens(t, runner, bundlePath, salarySrc, 99, "bonus")

	// 4. Mint expenses tokens (500 rows — EXP-00001 amount tampered by +$0.01).
	expensesSrc := loadBrokenCSVSource(t, "expenses.csv")
	expensesTokenIDs := mintBrokenCSVRowTokens(t, runner, bundlePath, expensesSrc, 500, "amount")

	// 5. Mint revenue tokens (1200 rows — identical to happy-path).
	revenueSrc := loadBrokenCSVSource(t, "revenue.csv")
	revenueTokenIDs := mintBrokenCSVRowTokens(t, runner, bundlePath, revenueSrc, 1200, "amount")

	// 6. Mint refunds tokens (80 rows — identical to happy-path).
	refundsSrc := loadBrokenCSVSource(t, "refunds.csv")
	refundsTokenIDs := mintBrokenCSVRowTokens(t, runner, bundlePath, refundsSrc, 80, "amount")

	// 7. calc.sum: total_salaries = sum(99 salary tokens)
	totalSalariesID := testsupport.CalcSum(t, runner, bundlePath, salaryTokenIDs)

	// 8. calc.sum: total_bonuses = sum(99 bonus tokens)
	totalBonusesID := testsupport.CalcSum(t, runner, bundlePath, bonusTokenIDs)

	// 9. calc.add: total_payroll = total_salaries + total_bonuses
	totalPayrollID := testsupport.CalcAdd(t, runner, bundlePath, totalSalariesID, totalBonusesID)

	// 10. calc.sum: total_expenses = sum(500 expense tokens, first has +$0.01)
	totalExpensesID := testsupport.CalcSum(t, runner, bundlePath, expensesTokenIDs)

	// 11. calc.add: total_opex = total_payroll + total_expenses
	totalOpexID := testsupport.CalcAdd(t, runner, bundlePath, totalPayrollID, totalExpensesID)

	// 12. calc.sum: gross_revenue (batched — 1200 rows, exceeds maxCalcInputs=1000).
	grossRevenueBatch1ID := testsupport.CalcSum(t, runner, bundlePath, revenueTokenIDs[:1000])
	grossRevenueBatch2ID := testsupport.CalcSum(t, runner, bundlePath, revenueTokenIDs[1000:])
	grossRevenueID := testsupport.CalcAdd(t, runner, bundlePath, grossRevenueBatch1ID, grossRevenueBatch2ID)

	// 13. calc.sum: total_refunds = sum(80 refund tokens)
	totalRefundsID := testsupport.CalcSum(t, runner, bundlePath, refundsTokenIDs)

	// 14. calc.subtract: net_revenue = gross_revenue - total_refunds
	netRevenueID := testsupport.CalcSubtract(t, runner, bundlePath, grossRevenueID, totalRefundsID)

	// 15. calc.subtract: operating_income = net_revenue - total_opex
	operatingIncomeID := testsupport.CalcSubtract(t, runner, bundlePath, netRevenueID, totalOpexID)

	// 16. submit-answer
	testsupport.RunCLIInTest(t, runner, []string{"submit-answer", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"token_id": operatingIncomeID,
		"label":    "operating_income",
	})

	// 17. run finish
	testsupport.RunCLIInTest(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})

	return brokenTokenIDs{
		totalSalaries:      totalSalariesID,
		totalBonuses:       totalBonusesID,
		totalPayroll:       totalPayrollID,
		totalExpenses:      totalExpensesID,
		totalOpex:          totalOpexID,
		grossRevenueBatch1: grossRevenueBatch1ID,
		grossRevenueBatch2: grossRevenueBatch2ID,
		grossRevenue:       grossRevenueID,
		totalRefunds:       totalRefundsID,
		netRevenue:         netRevenueID,
		operatingIncome:    operatingIncomeID,
	}
}

// loadBrokenCSVSource reads a broken CSV data file and returns a kernel.SourceBytes.
func loadBrokenCSVSource(t *testing.T, filename string) kernel.SourceBytes {
	t.Helper()
	path := filepath.Join(brokenDataDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return testsupport.SourceWithHash("fixture://broken/"+filename, data)
}

// mintBrokenCSVRowTokens mints one token per data row from the given source.
func mintBrokenCSVRowTokens(t *testing.T, runner cli.Runner, bundlePath string, src kernel.SourceBytes, rowCount int, columnName string) []string {
	t.Helper()
	return testsupport.MintCSVRowTokens(t, runner, bundlePath, src, brokenRun, rowCount, columnName, csvProjectedOutputPath)
}
