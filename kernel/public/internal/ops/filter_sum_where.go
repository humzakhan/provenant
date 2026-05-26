package ops

import (
	"encoding/json"
	"fmt"
	"strings"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/shopspring/decimal"
)

// FilterSumWhere implements the filter.sum_where op.
//
// It sums the subset of input tokens selected by a row-metadata predicate.
// The predicate is evaluated against each input's leaf captured-read row
// metadata. Inputs whose leaf does not match are excluded from the sum.
//
// This op uses a richer evaluation method (EvaluateFilter) than the standard
// Op interface because it requires resolved captured reads and token maps.
type FilterSumWhere struct{}

func (FilterSumWhere) Name() string { return "filter.sum_where" }

// Evaluate satisfies the Op interface but always returns ErrNoInputs because
// filter.sum_where requires captured-read context. Callers must use
// EvaluateFilter instead; the derive engine dispatches appropriately.
func (FilterSumWhere) Evaluate(_ []kernel.Token, _ kernel.RoundingPolicy) (Evaluation, error) {
	return Evaluation{}, fmt.Errorf("%w: filter.sum_where requires captured-read context; use EvaluateFilter", ErrNoInputs)
}

// FilterEvaluation extends Evaluation with the list of matched token IDs,
// in input-position order (not sorted), for audit-payload embedding.
type FilterEvaluation struct {
	Evaluation
	Matched []string // input IDs of tokens that passed the predicate, in original order
}

// EvaluateFilter evaluates the predicate against each input's leaf row
// metadata and sums the matching subset.
//
// tokensByID and readsByID are pre-built from the current run's tokens and
// captured reads respectively; they must contain every token reachable by
// lineage traversal from inputs.
func (FilterSumWhere) EvaluateFilter(
	inputs []kernel.Token,
	rounding kernel.RoundingPolicy,
	predicate kernel.RowPredicate,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
) (FilterEvaluation, error) {
	if len(inputs) == 0 {
		return FilterEvaluation{}, fmt.Errorf("%w: filter.sum_where requires at least 1 input", ErrNoInputs)
	}

	if err := validatePredicate(predicate); err != nil {
		return FilterEvaluation{}, err
	}

	// Evaluate predicate for each input.
	var matched []string
	var matchedTokens []kernel.Token
	for _, tok := range inputs {
		passes, err := evaluateTokenPredicate(tok, predicate, tokensByID, readsByID)
		if err != nil {
			return FilterEvaluation{}, err
		}
		if passes {
			matched = append(matched, tok.ID)
			matchedTokens = append(matchedTokens, tok)
		}
	}

	if len(matchedTokens) == 0 {
		return FilterEvaluation{}, ErrNoMatchingInputs
	}

	// Unit safety across matched inputs.
	unit, err := RequireSameUnit(matchedTokens)
	if err != nil {
		return FilterEvaluation{}, err
	}

	// Sum matched values.
	total := decimal.Zero
	for _, tok := range matchedTokens {
		d, err := ParseCanonical(tok.ValueCanonical)
		if err != nil {
			return FilterEvaluation{}, err
		}
		total = total.Add(d)
	}

	value := ApplyRounding(total, rounding)
	return FilterEvaluation{
		Evaluation: Evaluation{Value: value, Unit: unit},
		Matched:    matched,
	}, nil
}

// validatePredicate ensures the predicate op is in the allowed set.
func validatePredicate(p kernel.RowPredicate) error {
	switch p.Op {
	case "eq", "in":
		return nil
	default:
		return fmt.Errorf("%w: %q (must be \"eq\" or \"in\")", ErrPredicateOp, p.Op)
	}
}

// evaluateTokenPredicate resolves a token's leaf captured read, checks it is
// row-shaped, then tests the predicate against the row fields.
func evaluateTokenPredicate(
	tok kernel.Token,
	predicate kernel.RowPredicate,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
) (bool, error) {
	// Resolve leaf captured read.
	read, err := resolveLeafForFilter(tok, tokensByID, readsByID)
	if err != nil {
		return false, err
	}

	// Verify row-shaped reader.
	if !isRowShaped(read) {
		return false, fmt.Errorf("%w: reader %q for token %s", ErrPredicateNotRowShaped, read.ReaderName, tok.ID)
	}

	// Extract row fields from the captured-read output.
	fields, err := extractRowFields(read.Output, tok.Source.OutputPath)
	if err != nil {
		return false, fmt.Errorf("filter.sum_where: extract row fields for token %s: %w", tok.ID, err)
	}

	return testPredicate(fields, predicate), nil
}

// rowShapedReaders mirrors the set in internal/derivation/lineage.go to avoid
// a circular import. Both sets must be kept in sync.
var rowShapedReaders = map[string]bool{
	"extract.csv_rows":    true,
	"extract.spreadsheet": true,
	"extract.jsonl":       true,
}

func isRowShaped(read kernel.CapturedRead) bool {
	return rowShapedReaders[read.ReaderName]
}

// resolveLeafForFilter is a local copy of lineage resolution to avoid importing
// the derivation package (which would create a cycle). It returns
// ErrAmbiguousLineage when the token has more than one leaf.
func resolveLeafForFilter(
	tok kernel.Token,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
) (kernel.CapturedRead, error) {
	leaves, err := collectLeaves(tok, tokensByID)
	if err != nil {
		return kernel.CapturedRead{}, err
	}
	switch len(leaves) {
	case 0:
		return kernel.CapturedRead{}, fmt.Errorf("filter.sum_where: token %s has no leaf captured reads", tok.ID)
	case 1:
		read, ok := readsByID[leaves[0]]
		if !ok {
			return kernel.CapturedRead{}, fmt.Errorf("filter.sum_where: captured read %s not found", leaves[0])
		}
		return read, nil
	default:
		return kernel.CapturedRead{}, fmt.Errorf("%w: token %s has %d leaf sources", ErrAmbiguousLineage, tok.ID, len(leaves))
	}
}

// collectLeaves gathers the unique leaf captured-read IDs reachable from tok.
func collectLeaves(tok kernel.Token, tokensByID map[string]kernel.Token) ([]string, error) {
	if tok.Derivation == nil {
		return []string{tok.Source.CapturedReadID}, nil
	}
	seen := make(map[string]struct{})
	var result []string
	for _, inputID := range tok.Derivation.Inputs {
		input, ok := tokensByID[inputID]
		if !ok {
			return nil, fmt.Errorf("filter.sum_where: input token %s not found during lineage resolution", inputID)
		}
		sub, err := collectLeaves(input, tokensByID)
		if err != nil {
			return nil, err
		}
		for _, id := range sub {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	return result, nil
}

// minimalOutput is the subset of the CSV-rows output envelope we need for
// predicate evaluation. We only decode rows and their cells.
type minimalOutput struct {
	Rows []struct {
		Row   int `json:"row"`
		Cells []struct {
			Header string `json:"header"`
			Value  string `json:"value"`
			Row    int    `json:"row"`
		} `json:"cells"`
	} `json:"rows"`
}

// extractRowFields parses the captured-read output JSON and returns a
// map of header→value for the row that the token's outputPath references.
//
// The outputPath is a JSONPath like "$.rows[0].cells[?(@.header==\"amount\")].value".
// We extract the row index from it; if we can't determine the row, we fall
// back to row 0.
func extractRowFields(output json.RawMessage, outputPath string) (map[string]string, error) {
	var out minimalOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse output: %w", err)
	}
	if len(out.Rows) == 0 {
		return nil, fmt.Errorf("output contains no rows")
	}

	// Determine which row index the outputPath targets.
	rowIdx := extractRowIndex(outputPath, out.Rows)

	row := out.Rows[rowIdx]
	fields := make(map[string]string, len(row.Cells))
	for _, cell := range row.Cells {
		if cell.Header != "" {
			fields[cell.Header] = cell.Value
		}
	}
	return fields, nil
}

// extractRowIndex attempts to parse the row index from an outputPath in one of
// these forms:
//
//	"$.rows[N].*"    (JSONPath style)
//	"/rows/N/*"      (JSON pointer style)
//
// Returns 0 on failure, clamped to len(rows)-1.
func extractRowIndex(outputPath string, rows []struct {
	Row   int `json:"row"`
	Cells []struct {
		Header string `json:"header"`
		Value  string `json:"value"`
		Row    int    `json:"row"`
	} `json:"cells"`
}) int {
	var digits string

	// JSON pointer: "/rows/N/..."
	const ptrPrefix = "/rows/"
	if strings.HasPrefix(outputPath, ptrPrefix) {
		rest := outputPath[len(ptrPrefix):]
		end := strings.IndexByte(rest, '/')
		if end < 0 {
			end = len(rest)
		}
		digits = rest[:end]
	} else {
		// JSONPath: "$.rows[N]..."
		const jsonPathPrefix = "$.rows["
		idx := strings.Index(outputPath, jsonPathPrefix)
		if idx < 0 {
			return 0
		}
		rest := outputPath[idx+len(jsonPathPrefix):]
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return 0
		}
		digits = rest[:end]
	}

	n := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	if n >= len(rows) {
		return len(rows) - 1
	}
	return n
}

// testPredicate evaluates the predicate against a row's fields.
// The field path "row.account" maps to the "account" header in the row.
func testPredicate(fields map[string]string, p kernel.RowPredicate) bool {
	// Strip "row." prefix if present.
	fieldName := strings.TrimPrefix(p.Field, "row.")
	val, ok := fields[fieldName]
	if !ok {
		return false
	}
	switch p.Op {
	case "eq":
		return val == p.Value
	case "in":
		for _, v := range p.Values {
			if val == v {
				return true
			}
		}
		return false
	default:
		return false
	}
}
