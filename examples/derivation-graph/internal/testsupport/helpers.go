// Package testsupport provides shared test helpers for the derivation-graph
// e2e tests. It is intentionally placed under internal/ to prevent use outside
// the examples/derivation-graph subtree.
//
// All exported functions are designed to be called only from _test.go files.
// They accept *testing.T and call t.Fatal on any error, matching the
// convention used by the phase-08 and scaled e2e tests.
package testsupport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/provenant/provenant/access/cli"
	kernel "github.com/provenant/provenant/kernel/public"
)

// RunCLIInTest invokes runner.Run with a JSON-encoded body on stdin and returns
// the stdout bytes. It fails the test immediately if the CLI call returns an
// error.
func RunCLIInTest(t *testing.T, runner cli.Runner, args []string, body any) []byte {
	t.Helper()
	reqBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	var stdout bytes.Buffer
	if err := runner.Run(context.Background(), args, bytes.NewReader(reqBytes), &stdout); err != nil {
		t.Fatalf("CLI %v error: %v\nbody: %s", args, err, reqBytes)
	}
	return stdout.Bytes()
}

// ExtractTokenID parses a calc or read-mint JSON response and returns the token
// ID. It fails the test if the response cannot be parsed or the ID is empty.
func ExtractTokenID(t *testing.T, data []byte) string {
	t.Helper()
	var resp struct {
		Token kernel.Token `json:"token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal token response: %v\ndata: %s", err, data)
	}
	if resp.Token.ID == "" {
		t.Fatalf("empty token ID in response: %s", data)
	}
	return resp.Token.ID
}

// SourceWithHash builds a kernel.SourceBytes with the computed SHA256 for the
// given path and raw bytes. A copy of the data is stored so the caller's slice
// is not aliased.
func SourceWithHash(path string, data []byte) kernel.SourceBytes {
	sum := sha256.Sum256(data)
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:   path,
			SHA256: hex.EncodeToString(sum[:]),
		},
		Bytes: append([]byte(nil), data...),
	}
}

// CSVSelector builds a SourceSelector for one data row in a CSV file.
// dataRow is 1-indexed relative to the first data row (header is row 1,
// so data row i maps to CSV row i+1).
//
// columnProjection is optional: pass "" for the phase-08 two-column schema
// (account,amount) where the caller specifies the output path manually; pass a
// column name (e.g. "salary", "amount") for the scaled schema where
// ColumnProjection is needed.
func CSVSelector(dataRow int, columnProjection string) kernel.SourceSelector {
	row := dataRow + 1 // skip header
	sel := kernel.SourceSelector{
		RowWindow: fmt.Sprintf("%d-%d", row, row),
		HeaderRow: 1,
		Dialect:   "excel",
		Encoding:  "utf-8",
	}
	if columnProjection != "" {
		sel.ColumnProjection = columnProjection
	}
	return sel
}

// CalcSum calls calc.sum on the given input IDs and returns the derived token
// ID.
func CalcSum(t *testing.T, runner cli.Runner, bundlePath string, inputs []string) string {
	t.Helper()
	out := RunCLIInTest(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.sum",
		"inputs":   inputs,
	})
	return ExtractTokenID(t, out)
}

// CalcSubtract calls calc.subtract(a, b) and returns the derived token ID.
func CalcSubtract(t *testing.T, runner cli.Runner, bundlePath, aID, bID string) string {
	t.Helper()
	out := RunCLIInTest(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.subtract",
		"inputs":   []string{aID, bID},
	})
	return ExtractTokenID(t, out)
}

// CalcAdd calls calc.add(a, b) and returns the derived token ID.
func CalcAdd(t *testing.T, runner cli.Runner, bundlePath, aID, bID string) string {
	t.Helper()
	out := RunCLIInTest(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.add",
		"inputs":   []string{aID, bID},
	})
	return ExtractTokenID(t, out)
}

// MintCSVRowTokens mints one token per data row from the given source.
//
//   - src: the SourceBytes prepared by SourceWithHash.
//   - run: the kernel.Run identity (used in each read-mint request).
//   - rowCount: how many data rows to mint.
//   - columnProjection: passed to CSVSelector. Pass "" for the two-column
//     (account,amount) schema; pass the column name for multi-column CSVs.
//   - outputPath: the JSON path to extract the value from the reader envelope.
//     For the two-column schema use "/rows/0/cells/1/value"; for single-column
//     projections use "/rows/0/cells/0/value".
//
// Returns token IDs in row order.
func MintCSVRowTokens(
	t *testing.T,
	runner cli.Runner,
	bundlePath string,
	src kernel.SourceBytes,
	run kernel.Run,
	rowCount int,
	columnProjection string,
	outputPath string,
) []string {
	t.Helper()
	ids := make([]string, rowCount)
	for i := 1; i <= rowCount; i++ {
		out := RunCLIInTest(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
			"run_file": bundlePath,
			"reader":   "extract.csv_rows",
			"source":   src,
			"run":      run,
			"read": kernel.ReadAndMintRequest{
				SourcePath: src.Source.Path,
				Selector:   CSVSelector(i, columnProjection),
				OutputPath: outputPath,
				Unit:       "USD",
			},
		})
		var resp struct {
			Token kernel.Token `json:"token"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal read-mint response (row %d, col %q): %v", i, columnProjection, err)
		}
		if resp.Token.ID == "" {
			t.Fatalf("read-mint %s row %d col %q: empty token ID", src.Source.Path, i, columnProjection)
		}
		ids[i-1] = resp.Token.ID
	}
	return ids
}
