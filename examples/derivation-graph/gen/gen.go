// gen generates the deterministic fauxbooks demo dataset used by the
// derivation-graph end-to-end test.
//
// Usage:
//
//	go run ./gen/gen.go --out ./data --seed 42
//
// The generator is run once; the resulting CSV files are committed under
// examples/derivation-graph/data/. The test does not depend on this binary.
//
// Dataset layout:
//
//	data/revenue.csv — row_id,account,amount  (6 rows: 4 Revenue, 2 Refunds)
//	data/cogs.csv    — row_id,account,amount  (4 rows: 3 COGS, 1 Inventory adj)
//	data/opex.csv    — row_id,account,amount  (5 rows: 2 Salaries, 1 Rent, 2 Software)
//
// Financial summary (seed 42):
//
//	net_revenue      = 250000 + 300000 + 275000 + 175000 = 1_000_000.00
//	refunds          = 30000 + 20000                     =    50_000.00
//	gross_revenue    = 1_000_000 − 50_000                =   950_000.00
//	total_cogs       = 150000 + 130000 + 100000 + 20000  =   400_000.00
//	gross_margin     = 950_000 − 400_000                 =   550_000.00
//	total_opex       = 80000 + 60000 + 24000 + 18000 + 18000 = 200_000.00
//	operating_income = 550_000 − 200_000                 =   350_000.00
//
// To regenerate the golden dataset (e.g. after changing amounts):
//
//  1. Run: go run ./gen/gen.go --out ./data --seed 42
//  2. Delete golden.bundle.json and golden.trace.json.
//  3. Run: go test -run TestFauxbooksEndToEnd to regenerate goldens.
//  4. Commit all changed files with a conventional commit scoped to examples.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// Row is one source data record.
type Row struct {
	Account string
	Amount  string // decimal string, 2 fractional digits
}

func main() {
	seed := flag.Int64("seed", 42, "RNG seed for deterministic output")
	outDir := flag.String("out", "./data", "output directory")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed)) //nolint:gosec

	// Generate datasets.
	revenueRows := generateRevenue(rng)
	cogsRows := generateCOGS(rng)
	opexRows := generateOpex(rng)

	if err := writeCSV(filepath.Join(*outDir, "revenue.csv"), []string{"account", "amount"}, revenueRows); err != nil {
		fmt.Fprintf(os.Stderr, "gen: write revenue.csv: %v\n", err)
		os.Exit(1)
	}
	if err := writeCSV(filepath.Join(*outDir, "cogs.csv"), []string{"account", "amount"}, cogsRows); err != nil {
		fmt.Fprintf(os.Stderr, "gen: write cogs.csv: %v\n", err)
		os.Exit(1)
	}
	if err := writeCSV(filepath.Join(*outDir, "opex.csv"), []string{"account", "amount"}, opexRows); err != nil {
		fmt.Fprintf(os.Stderr, "gen: write opex.csv: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("gen: dataset written to", *outDir)
}

// generateRevenue returns 6 revenue rows with deterministic amounts.
// Accounts: Revenue (4 rows), Refunds (2 rows).
//
// net_revenue = 250000 + 300000 + 275000 + 175000 = 1_000_000.00
// refunds     = 30000 + 20000                      =    50_000.00
func generateRevenue(rng *rand.Rand) []Row {
	_ = rng // amounts are fixed for determinism; seed kept for future extensibility
	return []Row{
		{Account: "Revenue", Amount: "250000.00"},
		{Account: "Revenue", Amount: "300000.00"},
		{Account: "Revenue", Amount: "275000.00"},
		{Account: "Revenue", Amount: "175000.00"},
		{Account: "Refunds", Amount: "30000.00"},
		{Account: "Refunds", Amount: "20000.00"},
	}
}

// generateCOGS returns 4 COGS rows.
// total_cogs = 150000 + 130000 + 100000 + 20000 = 400_000.00
func generateCOGS(rng *rand.Rand) []Row {
	_ = rng
	return []Row{
		{Account: "COGS", Amount: "150000.00"},
		{Account: "COGS", Amount: "130000.00"},
		{Account: "COGS", Amount: "100000.00"},
		{Account: "Inventory adj", Amount: "20000.00"},
	}
}

// generateOpex returns 5 OpEx rows.
// total_opex = 80000 + 60000 + 24000 + 18000 + 18000 = 200_000.00
func generateOpex(rng *rand.Rand) []Row {
	_ = rng
	return []Row{
		{Account: "Salaries", Amount: "80000.00"},
		{Account: "Salaries", Amount: "60000.00"},
		{Account: "Rent", Amount: "24000.00"},
		{Account: "Software", Amount: "18000.00"},
		{Account: "Software", Amount: "18000.00"},
	}
}

// writeCSV writes rows to a CSV file with the given headers (account,amount).
func writeCSV(path string, headers []string, rows []Row) error {
	var b strings.Builder
	b.WriteString(strings.Join(headers, ","))
	b.WriteByte('\n')
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%s,%s\n", r.Account, r.Amount))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
