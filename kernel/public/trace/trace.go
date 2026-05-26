package trace

import (
	"errors"
	"fmt"

	"github.com/gowebpki/jcs"
	"github.com/shopspring/decimal"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/bundle"
)

// Sentinel errors returned by Trace.
var (
	// ErrTokenNotFound is returned when tokenID does not exist in the bundle.
	ErrTokenNotFound = errors.New("trace: token not found")

	// ErrContributionMismatch is returned when the contribution invariant
	// sum(contributions) + sum(roundingResiduals) == rootValue is violated.
	// This indicates a kernel-integrity failure.
	ErrContributionMismatch = errors.New("trace: contribution invariant violated")
)

// Graph is the contribution graph rooted at the queried token.
// Nodes are in topological order (parents before children, root first).
// Leaves are all distinct leaf tokens reachable from the root.
type Graph struct {
	Root      string             `json:"root"`       // queried token ID
	RootValue string             `json:"root_value"` // canonical decimal
	RootUnit  string             `json:"root_unit"`
	Nodes     []Node             `json:"nodes"`  // topological order, root first
	Edges     []Edge             `json:"edges"`  // parent -> child (input)
	Leaves    []LeafContribution `json:"leaves"` // one per distinct leaf token in the lineage
}

// Node represents one token in the lineage graph.
type Node struct {
	TokenID        string `json:"token_id"`
	ValueCanonical string `json:"value_canonical"`
	Unit           string `json:"unit"`
	Kind           string `json:"kind"`                // "leaf" or "derived"
	Operation      string `json:"operation,omitempty"` // only for derived
	FormulaID      string `json:"formula_id,omitempty"`
	AuditSeq       uint64 `json:"audit_seq"` // seq of the minting/deriving audit entry
}

// Edge connects a parent (derived) token to one of its input (child) tokens.
type Edge struct {
	From     string `json:"from"`     // parent (derived) token ID
	To       string `json:"to"`       // child (input) token ID
	Position int    `json:"position"` // ordinal position in parent's Derivation.Inputs
}

// LeafContribution records the contribution one leaf token makes to the root value.
// For synthetic rounding residuals, TokenID is "" and SourceName is "<rounding>".
type LeafContribution struct {
	TokenID        string                `json:"token_id"`
	CapturedReadID string                `json:"captured_read_id,omitempty"`
	SourceName     string                `json:"source_name,omitempty"`
	SourcePath     string                `json:"source_path,omitempty"`
	SourceSHA256   string                `json:"source_sha256,omitempty"`
	Selector       kernel.SourceSelector `json:"selector"`
	LeafValue      string                `json:"leaf_value,omitempty"` // canonical decimal
	Unit           string                `json:"unit,omitempty"`
	Contribution   string                `json:"contribution"` // signed canonical decimal
}

// Trace reconstructs the full contribution graph from the token identified by
// tokenID within bundleBytes.
//
// Trace first calls bundle.Replay to verify bundle integrity; if verification
// fails, the tamper error is returned and no tracing is attempted.
//
// The returned Graph is fully deterministic: two calls on the same bundle
// bytes and tokenID return byte-identical JSON (when marshalled with
// encoding/json without indentation).
//
// Contribution semantics — see package doc.go for the full specification.
// In brief:
//   - Additive ops: contributions sum exactly to root value.
//   - Multiplicative ops (calc.multiply, calc.divide): the scalar side reports
//     Contribution = "0"; the currency side carries the scaled amount.
//   - Rounding residuals are recorded as synthetic LeafContribution entries
//     with TokenID = "" and SourceName = "<rounding>".
//
// The contribution invariant sum(contributions) + sum(rounding residuals) ==
// root value is asserted before return; ErrContributionMismatch is returned if
// it is violated.
func Trace(bundleBytes []byte, tokenID string) (*Graph, error) {
	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		return nil, err
	}

	// Index tokens and captured reads.
	tokensByID := make(map[string]kernel.Token, len(state.Tokens))
	for _, tok := range state.Tokens {
		tokensByID[tok.ID] = tok
	}
	readsByID := make(map[string]kernel.CapturedRead, len(state.CapturedReads))
	for _, r := range state.CapturedReads {
		readsByID[r.ID] = r
	}

	// Build seq index: token ID -> audit seq, and matched index for
	// filter.sum_where derivations: token ID -> matched input IDs.
	seqByTokenID := make(map[string]uint64, len(state.AuditEntries))
	matchedByTokenID := make(map[string][]string)
	for _, entry := range state.AuditEntries {
		if entry.Kind == kernel.EntryKindTokenMint || entry.Kind == kernel.EntryKindTokenDerive {
			// Extract token_id (and matched list, when present) from the
			// deterministic payload bytes.
			id, matched, err2 := extractDeriveMetaFromPayload(entry.Payload.Deterministic)
			if err2 == nil && id != "" {
				seqByTokenID[id] = entry.Seq
				if matched != nil {
					matchedByTokenID[id] = matched
				}
			}
		}
	}

	root, ok := tokensByID[tokenID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTokenNotFound, tokenID)
	}

	// DFS to collect nodes in topological order (parents before children).
	var nodes []Node
	var edges []Edge
	visited := make(map[string]bool)
	if err := collectNodes(root, tokensByID, seqByTokenID, &nodes, &edges, visited); err != nil {
		return nil, err
	}

	// Compute contributions for each distinct leaf.
	leafContribs, err := computeContributions(root, tokensByID, readsByID, matchedByTokenID, decimal.New(1, 0))
	if err != nil {
		return nil, err
	}

	// Assert contribution invariant.
	rootVal, err := parseCanonical(root.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse root value: %w", err)
	}
	sum := decimal.Zero
	for _, lc := range leafContribs {
		c, err2 := parseCanonical(lc.Contribution)
		if err2 != nil {
			return nil, fmt.Errorf("trace: parse contribution: %w", err2)
		}
		sum = sum.Add(c)
	}
	if !sum.Equal(rootVal) {
		return nil, fmt.Errorf("%w: sum(%s) != root(%s)", ErrContributionMismatch, formatCanonical(sum), root.ValueCanonical)
	}

	g := &Graph{
		Root:      root.ID,
		RootValue: root.ValueCanonical,
		RootUnit:  root.Unit,
		Nodes:     nodes,
		Edges:     edges,
		Leaves:    leafContribs,
	}
	return g, nil
}

// MarshalCanonical serialises the graph to JCS-canonical JSON bytes.
// This ensures byte-deterministic output across calls.
func MarshalCanonical(g *Graph) ([]byte, error) {
	raw, err := marshalJSON(g)
	if err != nil {
		return nil, fmt.Errorf("trace: marshal graph: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("trace: canonicalize graph: %w", err)
	}
	return canonical, nil
}

// collectNodes performs a DFS from tok and records nodes + edges in topological
// order (parents before children). It is idempotent: already-visited tokens are
// skipped.
func collectNodes(
	tok kernel.Token,
	tokensByID map[string]kernel.Token,
	seqByTokenID map[string]uint64,
	nodes *[]Node,
	edges *[]Edge,
	visited map[string]bool,
) error {
	if visited[tok.ID] {
		return nil
	}
	visited[tok.ID] = true

	kind := "leaf"
	var op, formulaID string
	if tok.Derivation != nil {
		kind = "derived"
		op = tok.Derivation.Operation
		formulaID = tok.Derivation.FormulaID
	}

	*nodes = append(*nodes, Node{
		TokenID:        tok.ID,
		ValueCanonical: tok.ValueCanonical,
		Unit:           tok.Unit,
		Kind:           kind,
		Operation:      op,
		FormulaID:      formulaID,
		AuditSeq:       seqByTokenID[tok.ID],
	})

	if tok.Derivation == nil {
		return nil
	}

	for pos, inputID := range tok.Derivation.Inputs {
		*edges = append(*edges, Edge{
			From:     tok.ID,
			To:       inputID,
			Position: pos,
		})
		child, ok := tokensByID[inputID]
		if !ok {
			return fmt.Errorf("trace: input token %s not found", inputID)
		}
		if err := collectNodes(child, tokensByID, seqByTokenID, nodes, edges, visited); err != nil {
			return err
		}
	}
	return nil
}

// computeContributions recursively walks the derivation tree and returns the
// flat list of LeafContribution values for all leaves reachable from tok,
// with multiplier applied. Synthetic rounding residuals are appended inline.
func computeContributions(
	tok kernel.Token,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	if tok.Derivation == nil {
		// This is a leaf token.
		read, ok := readsByID[tok.Source.CapturedReadID]
		if !ok {
			return nil, fmt.Errorf("trace: captured read %s not found for leaf %s", tok.Source.CapturedReadID, tok.ID)
		}
		leafVal, err := parseCanonical(tok.ValueCanonical)
		if err != nil {
			return nil, fmt.Errorf("trace: parse leaf value: %w", err)
		}
		contrib := leafVal.Mul(multiplier)
		return []LeafContribution{
			{
				TokenID:        tok.ID,
				CapturedReadID: read.ID,
				SourceName:     read.SourceName,
				SourcePath:     read.SourcePath,
				SourceSHA256:   read.SourceSHA256,
				Selector:       read.Selector,
				LeafValue:      tok.ValueCanonical,
				Unit:           tok.Unit,
				Contribution:   formatCanonical(contrib),
			},
		}, nil
	}

	op := tok.Derivation.Operation
	inputs := tok.Derivation.Inputs

	switch op {
	case "calc.add", "calc.sum", "select.passthrough":
		return passThroughContribs(inputs, tokensByID, readsByID, matchedByTokenID, multiplier)

	case "calc.subtract":
		return subtractContribs(inputs, tokensByID, readsByID, matchedByTokenID, multiplier)

	case "round.half_even":
		return roundContribs(tok, inputs, tokensByID, readsByID, matchedByTokenID, multiplier)

	case "calc.multiply":
		return multiplyContribs(tok, inputs, tokensByID, readsByID, matchedByTokenID, multiplier)

	case "calc.divide":
		return divideContribs(tok, inputs, tokensByID, readsByID, matchedByTokenID, multiplier)

	case "filter.sum_where":
		// The Derivation on filter.sum_where has a Matched field embedded in the
		// audit payload. We need it to know which inputs are matched. However, the
		// kernel.Derivation struct only stores Operation, Inputs, FormulaID,
		// Rounding, Evaluated — the Matched list is stored only in the bundle's
		// token payload. We retrieve it by examining the token structure.
		//
		// For tracing purposes we reconstruct matched from the derivation by
		// looking at which inputs share a unit with the derived token. But a
		// cleaner approach is: the bundle.Replay result already carries token
		// objects — however, kernel.Derivation does not expose Matched.
		//
		// Instead, we use the convention: any input whose contribution we can
		// determine to be nonzero from the evaluated sum. The spec says to use
		// the Matched list from the audit payload. Since bundle.Replay validates
		// the payload but doesn't surface Matched on kernel.Derivation, we must
		// reconstruct it. We do this by summing all inputs and comparing to
		// evaluated — but for a stable implementation, we walk the inputs and
		// include those that match the unit. This is the same logic filter.sum_where uses.
		//
		// The Matched list is authoritative: we read it from the deterministic
		// audit payload via matchedByTokenID (extracted in Trace).
		return filterSumWhereContribs(tok, inputs, tokensByID, readsByID, matchedByTokenID, multiplier)

	default:
		return nil, fmt.Errorf("trace: unsupported operation %q", op)
	}
}

// passThroughContribs handles calc.add, calc.sum, select.passthrough.
// Each input's contributions pass through with the same multiplier.
func passThroughContribs(
	inputs []string,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	var out []LeafContribution
	for _, id := range inputs {
		tok, ok := tokensByID[id]
		if !ok {
			return nil, fmt.Errorf("trace: input token %s not found", id)
		}
		contribs, err := computeContributions(tok, tokensByID, readsByID, matchedByTokenID, multiplier)
		if err != nil {
			return nil, err
		}
		out = append(out, contribs...)
	}
	return out, nil
}

// subtractContribs handles calc.subtract(a, b).
// Position 0 is positive, position 1 is negated.
func subtractContribs(
	inputs []string,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("trace: calc.subtract expects 2 inputs, got %d", len(inputs))
	}
	tokA, ok := tokensByID[inputs[0]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[0])
	}
	tokB, ok := tokensByID[inputs[1]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[1])
	}
	contribsA, err := computeContributions(tokA, tokensByID, readsByID, matchedByTokenID, multiplier)
	if err != nil {
		return nil, err
	}
	// Negate b's multiplier.
	negMul := multiplier.Neg()
	contribsB, err := computeContributions(tokB, tokensByID, readsByID, matchedByTokenID, negMul)
	if err != nil {
		return nil, err
	}
	return append(contribsA, contribsB...), nil
}

// roundContribs handles round.half_even.
// Contributions pass through; a synthetic <rounding> residual is added when
// the rounded value differs from the unrounded sum of inputs.
func roundContribs(
	tok kernel.Token,
	inputs []string,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("trace: round.half_even expects 1 input, got %d", len(inputs))
	}
	input, ok := tokensByID[inputs[0]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[0])
	}
	contribs, err := computeContributions(input, tokensByID, readsByID, matchedByTokenID, multiplier)
	if err != nil {
		return nil, err
	}

	// Compute rounding residual.
	// inputSum = input.ValueCanonical * multiplier (unrounded).
	// roundedSum = tok.ValueCanonical * multiplier.
	inputVal, err := parseCanonical(input.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse round input value: %w", err)
	}
	roundedVal, err := parseCanonical(tok.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse rounded value: %w", err)
	}
	unroundedContrib := inputVal.Mul(multiplier)
	roundedContrib := roundedVal.Mul(multiplier)
	residual := roundedContrib.Sub(unroundedContrib)

	if !residual.IsZero() {
		contribs = append(contribs, LeafContribution{
			TokenID:      "",
			SourceName:   "<rounding>",
			Contribution: formatCanonical(residual),
		})
	}
	return contribs, nil
}

// multiplyContribs handles calc.multiply(a, b).
// If one operand is dimensionless, its leaves contribute 0 and the other
// side's leaves are multiplied by the dimensionless value.
func multiplyContribs(
	tok kernel.Token,
	inputs []string,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("trace: calc.multiply expects 2 inputs, got %d", len(inputs))
	}
	tokA, ok := tokensByID[inputs[0]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[0])
	}
	tokB, ok := tokensByID[inputs[1]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[1])
	}

	aVal, err := parseCanonical(tokA.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse multiply input a: %w", err)
	}
	bVal, err := parseCanonical(tokB.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse multiply input b: %w", err)
	}

	// Determine which side is the scalar (dimensionless).
	// currency × dimensionless: a has unit, b is dimensionless → a is currency side.
	// dimensionless × currency: a is dimensionless, b has unit → b is currency side.
	// Both dimensionless: a is "currency side" by convention (both contribute).
	switch {
	case tokA.Unit == "" && tokB.Unit != "":
		// a is scalar, b is currency.
		// b's leaves: contribution = b_leaf_val * a_val * multiplier.
		// a's leaves: contribution = 0.
		scaledMul := multiplier.Mul(aVal)
		contribsB, err := computeContributions(tokB, tokensByID, readsByID, matchedByTokenID, scaledMul)
		if err != nil {
			return nil, err
		}
		contribsA, err := zeroContribs(tokA, tokensByID, readsByID, matchedByTokenID)
		if err != nil {
			return nil, err
		}
		return append(contribsB, contribsA...), nil

	case tokB.Unit == "" && tokA.Unit != "":
		// b is scalar, a is currency.
		// a's leaves: contribution = a_leaf_val * b_val * multiplier.
		// b's leaves: contribution = 0.
		scaledMul := multiplier.Mul(bVal)
		contribsA, err := computeContributions(tokA, tokensByID, readsByID, matchedByTokenID, scaledMul)
		if err != nil {
			return nil, err
		}
		contribsB, err := zeroContribs(tokB, tokensByID, readsByID, matchedByTokenID)
		if err != nil {
			return nil, err
		}
		return append(contribsA, contribsB...), nil

	default:
		// Both dimensionless (or unit×unit, which the kernel would have rejected).
		// Treat as: result = a * b; a's leaves contribute a_leaf * b_val * multiplier,
		// b's leaves contribute 0.
		scaledMul := multiplier.Mul(bVal)
		contribsA, err := computeContributions(tokA, tokensByID, readsByID, matchedByTokenID, scaledMul)
		if err != nil {
			return nil, err
		}
		contribsB, err := zeroContribs(tokB, tokensByID, readsByID, matchedByTokenID)
		if err != nil {
			return nil, err
		}
		_ = tok
		return append(contribsA, contribsB...), nil
	}
}

// divideContribs handles calc.divide(numerator, denominator).
// Numerator's (position 0) leaves are divided by denominator's canonical value.
// Denominator's (position 1) leaves contribute 0.
// A synthetic <rounding> residual captures rounding loss.
func divideContribs(
	tok kernel.Token,
	inputs []string,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("trace: calc.divide expects 2 inputs, got %d", len(inputs))
	}
	numerator, ok := tokensByID[inputs[0]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[0])
	}
	denominator, ok := tokensByID[inputs[1]]
	if !ok {
		return nil, fmt.Errorf("trace: input token %s not found", inputs[1])
	}

	denomVal, err := parseCanonical(denominator.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse divide denominator: %w", err)
	}
	if denomVal.IsZero() {
		return nil, fmt.Errorf("trace: calc.divide denominator is zero")
	}

	// Scale multiplier by 1/denominator.
	scaledMul := multiplier.Div(denomVal)

	contribsNum, err := computeContributions(numerator, tokensByID, readsByID, matchedByTokenID, scaledMul)
	if err != nil {
		return nil, err
	}
	contribsDen, err := zeroContribs(denominator, tokensByID, readsByID, matchedByTokenID)
	if err != nil {
		return nil, err
	}

	all := append(contribsNum, contribsDen...)

	// Compute rounding residual: tok.ValueCanonical * multiplier - sum(num_contribs).
	rootVal, err := parseCanonical(tok.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse divide result: %w", err)
	}
	numeratorVal, err := parseCanonical(numerator.ValueCanonical)
	if err != nil {
		return nil, fmt.Errorf("trace: parse divide numerator: %w", err)
	}
	// unrounded contribution = (numerator / denominator) * multiplier, full precision.
	unroundedContrib := numeratorVal.Div(denomVal).Mul(multiplier)
	roundedContrib := rootVal.Mul(multiplier)
	residual := roundedContrib.Sub(unroundedContrib)

	if !residual.IsZero() {
		all = append(all, LeafContribution{
			TokenID:      "",
			SourceName:   "<rounding>",
			Contribution: formatCanonical(residual),
		})
	}
	return all, nil
}

// filterSumWhereContribs handles filter.sum_where.
// Matched inputs contribute their values; unmatched inputs contribute 0.
// The matched set is read authoritatively from the deterministic audit payload
// of the derived token (which bundle.Replay has already validated as a subset
// of inputs).
func filterSumWhereContribs(
	tok kernel.Token,
	inputs []string,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
	multiplier decimal.Decimal,
) ([]LeafContribution, error) {
	matched, ok := matchedByTokenID[tok.ID]
	if !ok {
		return nil, fmt.Errorf("trace: filter.sum_where token %s has no audit-payload matched list", tok.ID)
	}
	matchedSet := make(map[string]bool, len(matched))
	for _, id := range matched {
		matchedSet[id] = true
	}

	var out []LeafContribution
	for _, id := range inputs {
		inp, ok := tokensByID[id]
		if !ok {
			return nil, fmt.Errorf("trace: input token %s not found", id)
		}
		if matchedSet[id] {
			contribs, err := computeContributions(inp, tokensByID, readsByID, matchedByTokenID, multiplier)
			if err != nil {
				return nil, err
			}
			out = append(out, contribs...)
		} else {
			// Unmatched: contribute 0.
			zeros, err := zeroContribs(inp, tokensByID, readsByID, matchedByTokenID)
			if err != nil {
				return nil, err
			}
			out = append(out, zeros...)
		}
	}
	return out, nil
}

// zeroContribs returns LeafContribution records for all leaves of tok with
// Contribution = "0". Used for the scalar side of multiply/divide and for
// unmatched filter inputs.
func zeroContribs(
	tok kernel.Token,
	tokensByID map[string]kernel.Token,
	readsByID map[string]kernel.CapturedRead,
	matchedByTokenID map[string][]string,
) ([]LeafContribution, error) {
	return computeContributions(tok, tokensByID, readsByID, matchedByTokenID, decimal.Zero)
}

// extractDeriveMetaFromPayload extracts the token_id and (for
// filter.sum_where derivations) the matched input-ID list from a deterministic
// audit payload. Other ops have no matched field and the returned slice is nil.
func extractDeriveMetaFromPayload(det []byte) (string, []string, error) {
	var v struct {
		TokenID string   `json:"token_id"`
		Matched []string `json:"matched"`
	}
	if err := unmarshalLoose(det, &v); err != nil {
		return "", nil, err
	}
	return v.TokenID, v.Matched, nil
}
