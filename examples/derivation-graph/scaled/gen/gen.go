// gen generates the deterministic scaled fauxbooks dataset for the
// derivation-graph phase-09 demo.
//
// Usage:
//
//	go run ./examples/derivation-graph/scaled/gen [--out ./examples/derivation-graph/scaled/data]
//
// The generator is run once; the resulting CSV files are committed under
// examples/derivation-graph/scaled/data/. Tests do NOT re-run this binary;
// they consume the committed CSVs directly.
//
// Dataset layout:
//
//	data/payroll.csv  — employee_id,department,salary,bonus   (100 rows)
//	data/expenses.csv — expense_id,category,department,amount,date  (500 rows)
//	data/revenue.csv  — txn_id,product,region,amount          (1200 rows)
//	data/refunds.csv  — refund_id,product,region,amount       (80 rows)
//
// Seed strategy:
//
//	baseSeed = 42
//	payrollRand  = rand.New(rand.NewSource(baseSeed + 1))
//	expensesRand = rand.New(rand.NewSource(baseSeed + 2))
//	revenueRand  = rand.New(rand.NewSource(baseSeed + 3))
//	refundsRand  = rand.New(rand.NewSource(baseSeed + 4))
//
// Amounts are generated as integer cents and formatted as "NNNN.NN" canonical
// decimal strings (e.g. "12345.67") using shopspring/decimal for locale-neutral
// output. Output files are UTF-8 with Unix line endings.
//
// To regenerate (if the generator logic changes):
//
//	go run ./examples/derivation-graph/scaled/gen --out ./examples/derivation-graph/scaled/data
//
// After regenerating CSVs, the phase-09 e2e goldens must be regenerated too:
//
//	UPDATE_GOLDEN=1 go test ./examples/derivation-graph/scaled/... -run TestScaledFauxbooksEndToEnd -v
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/shopspring/decimal"
)

const baseSeed = 42

// departments is the fixed set of departments used for payroll and expense rows.
// Sorted to guarantee stable output across Go versions.
var departments = []string{
	"Engineering",
	"Finance",
	"Marketing",
	"Operations",
	"Sales",
}

// expenseCategories is the fixed set of expense categories.
// Sorted for stable output.
var expenseCategories = []string{
	"Equipment",
	"Meals",
	"Software",
	"Training",
	"Travel",
}

// products is the set of product lines for revenue and refund rows.
// Sorted for stable output.
var products = []string{
	"Alpha",
	"Beta",
	"Gamma",
	"Delta",
}

// regions is the set of sales regions for revenue and refund rows.
// Sorted for stable output.
var regions = []string{
	"APAC",
	"EMEA",
	"LATAM",
}

func main() {
	outDir := flag.String("out", "./examples/derivation-graph/scaled/data", "output directory for CSV files")
	flag.Parse()

	// Per-table RNGs with deterministic sub-seeds.
	payrollRand := rand.New(rand.NewSource(baseSeed + 1))  //nolint:gosec
	expensesRand := rand.New(rand.NewSource(baseSeed + 2)) //nolint:gosec
	revenueRand := rand.New(rand.NewSource(baseSeed + 3))  //nolint:gosec
	refundsRand := rand.New(rand.NewSource(baseSeed + 4))  //nolint:gosec

	files := []struct {
		name    string
		headers []string
		rows    [][]string
	}{
		{
			name:    "payroll.csv",
			headers: []string{"employee_id", "department", "salary", "bonus"},
			rows:    generatePayroll(payrollRand, 100),
		},
		{
			name:    "expenses.csv",
			headers: []string{"expense_id", "category", "department", "amount", "date"},
			rows:    generateExpenses(expensesRand, 500),
		},
		{
			name:    "revenue.csv",
			headers: []string{"txn_id", "product", "region", "amount"},
			rows:    generateRevenue(revenueRand, 1200),
		},
		{
			name:    "refunds.csv",
			headers: []string{"refund_id", "product", "region", "amount"},
			rows:    generateRefunds(refundsRand, 80),
		},
	}

	for _, f := range files {
		path := filepath.Join(*outDir, f.name)
		if err := writeCSV(path, f.headers, f.rows); err != nil {
			fmt.Fprintf(os.Stderr, "gen: write %s: %v\n", f.name, err)
			os.Exit(1)
		}
		fmt.Printf("gen: wrote %s (%d data rows)\n", path, len(f.rows))
	}
}

// centsToDecimal converts an integer cent amount to a canonical decimal string
// with exactly 2 fractional digits, e.g. 123456 → "1234.56".
func centsToDecimal(cents int64) string {
	d := decimal.NewFromInt(cents).Div(decimal.NewFromInt(100))
	return d.StringFixed(2)
}

// generatePayroll returns n payroll rows, each with a deterministic
// employee_id (EMP-NNNN, zero-padded to 4 digits), a department drawn from
// the fixed departments slice, a salary (30 000.00–120 000.00), and a bonus
// (1 000.00–20 000.00).
func generatePayroll(rng *rand.Rand, n int) [][]string {
	rows := make([][]string, n)
	for i := 0; i < n; i++ {
		// Salary: 3 000 000–12 000 000 cents (30 000.00–120 000.00).
		salaryCents := int64(rng.Intn(9_000_001) + 3_000_000)
		// Bonus: 100 000–2 000 000 cents (1 000.00–20 000.00).
		bonusCents := int64(rng.Intn(1_900_001) + 100_000)

		rows[i] = []string{
			fmt.Sprintf("EMP-%04d", i+1),
			departments[i%len(departments)],
			centsToDecimal(salaryCents),
			centsToDecimal(bonusCents),
		}
	}
	return rows
}

// generateExpenses returns n expense rows. Each row has an expense_id
// (EXP-NNNNN, zero-padded to 5 digits), a category, a department, an amount
// (10.00–5 000.00), and a date (2024-01-01 through 2024-12-31, deterministic).
func generateExpenses(rng *rand.Rand, n int) [][]string {
	rows := make([][]string, n)
	for i := 0; i < n; i++ {
		// Amount: 1 000–500 000 cents (10.00–5 000.00).
		amountCents := int64(rng.Intn(499_001) + 1_000)
		// Date: day-of-year 1–365 within 2024 (non-leap projection mapped to month/day).
		dayOfYear := rng.Intn(365) // 0..364
		month := dayOfYear/30 + 1
		if month > 12 {
			month = 12
		}
		day := dayOfYear%30 + 1

		rows[i] = []string{
			fmt.Sprintf("EXP-%05d", i+1),
			expenseCategories[rng.Intn(len(expenseCategories))],
			departments[rng.Intn(len(departments))],
			centsToDecimal(amountCents),
			fmt.Sprintf("2024-%02d-%02d", month, day),
		}
	}
	return rows
}

// generateRevenue returns n revenue transaction rows. Each row has a txn_id
// (REV-NNNNNN, zero-padded to 6 digits), a product, a region, and an amount
// (100.00–50 000.00).
func generateRevenue(rng *rand.Rand, n int) [][]string {
	rows := make([][]string, n)
	for i := 0; i < n; i++ {
		// Amount: 10 000–5 000 000 cents (100.00–50 000.00).
		amountCents := int64(rng.Intn(4_990_001) + 10_000)

		rows[i] = []string{
			fmt.Sprintf("REV-%06d", i+1),
			products[rng.Intn(len(products))],
			regions[rng.Intn(len(regions))],
			centsToDecimal(amountCents),
		}
	}
	return rows
}

// generateRefunds returns n refund rows. Each row has a refund_id
// (RFD-NNNN, zero-padded to 4 digits), a product, a region, and an amount
// (10.00–2 000.00).
func generateRefunds(rng *rand.Rand, n int) [][]string {
	rows := make([][]string, n)
	for i := 0; i < n; i++ {
		// Amount: 1 000–200 000 cents (10.00–2 000.00).
		amountCents := int64(rng.Intn(199_001) + 1_000)

		rows[i] = []string{
			fmt.Sprintf("RFD-%04d", i+1),
			products[rng.Intn(len(products))],
			regions[rng.Intn(len(regions))],
			centsToDecimal(amountCents),
		}
	}
	return rows
}

// writeCSV writes headers and rows to path as a UTF-8 CSV file with Unix line
// endings. Existing files are overwritten.
func writeCSV(path string, headers []string, rows [][]string) error {
	var b strings.Builder
	b.WriteString(strings.Join(headers, ","))
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString(strings.Join(row, ","))
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
