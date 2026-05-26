package derivation

import (
	"errors"
	"fmt"

	kernel "github.com/humzakhan/provenant/kernel/public"
)

// rowShapedReaders is the closed set of reader names whose output is row-shaped
// and whose row metadata can be used by filter.sum_where.
var rowShapedReaders = map[string]bool{
	"extract.csv_rows":    true,
	"extract.spreadsheet": true,
	"extract.jsonl":       true,
}

// ResolveLeafCapturedRead walks the token DAG from tok back to its single
// leaf-level captured read. If tok is a leaf token it returns the referenced
// captured read directly. If tok is a derived token it recursively resolves
// its inputs; the set of reachable leaf captured reads must be exactly one —
// any deviation returns ErrAmbiguousLineage.
//
// The function signature accepts two separate look-up helpers so callers can
// provide pre-built maps instead of re-scanning slices on every call.
func ResolveLeafCapturedRead(
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
		return kernel.CapturedRead{}, errors.New("derivation: token has no leaf captured reads")
	case 1:
		read, ok := readsByID[leaves[0]]
		if !ok {
			return kernel.CapturedRead{}, fmt.Errorf("derivation: captured read %s not found", leaves[0])
		}
		return read, nil
	default:
		return kernel.CapturedRead{}, fmt.Errorf("%w: token %s has %d leaf sources",
			ErrAmbiguousLineage, tok.ID, len(leaves))
	}
}

// ErrAmbiguousLineage is returned when a token traces to more than one leaf
// captured read, making predicate evaluation against a single row impossible.
var ErrAmbiguousLineage = errors.New("derivation: ambiguous lineage")

// collectLeaves recursively gathers the unique set of leaf captured read IDs
// reachable from tok.
func collectLeaves(tok kernel.Token, tokensByID map[string]kernel.Token) ([]string, error) {
	if tok.Derivation == nil {
		// Leaf token.
		return []string{tok.Source.CapturedReadID}, nil
	}
	seen := make(map[string]struct{})
	var result []string
	for _, inputID := range tok.Derivation.Inputs {
		input, ok := tokensByID[inputID]
		if !ok {
			return nil, fmt.Errorf("derivation: input token %s not found during lineage resolution", inputID)
		}
		leaves, err := collectLeaves(input, tokensByID)
		if err != nil {
			return nil, err
		}
		for _, id := range leaves {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	return result, nil
}

// IsRowShaped reports whether the captured read originates from a reader whose
// output is row-shaped and whose row metadata can be tested by a RowPredicate.
func IsRowShaped(read kernel.CapturedRead) bool {
	return rowShapedReaders[read.ReaderName]
}
