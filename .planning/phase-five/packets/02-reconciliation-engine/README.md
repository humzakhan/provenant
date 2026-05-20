# Packet 02 - Reconciliation Engine

## Goal

Implement deterministic reconciliation over the normalized row/value evidence from Packet 01. This packet proves that configured match logic can classify agreement, mismatch, absence, duplication, and validation failures without touching kernel provenance or report packaging.

## Suggested package shape

Use a focused engine package, for example:

```text
reconcile/
├── engine.go
├── match_exact.go
├── match_tolerance.go
├── match_grouped.go
├── validate_rows.go
├── classify.go
└── engine_test.go
```

If a better local package split exists when implementation starts, keep the API boundary small and deterministic.

## Technical tasks

- Accept Packet 00 reconcile config plus Packet 01 normalized files as the only inputs.
- Validate that required logical fields exist in the normalized inputs before matching starts.
- Build deterministic indexes for configured key fields.
- Implement exact match mode.
- Implement amount tolerance mode with explicit decimal handling and currency checks.
- Implement existence/completeness mode.
- Implement one-to-many and many-to-one grouped amount mode with deterministic grouping and sum comparison.
- Implement schema/value validation mode for empty keys, duplicate keys, missing required values, numeric parse failures, date parse failures, and allowed-value checks.
- Classify output records as:
  - `matched`
  - `mismatch`
  - `missing_left`
  - `missing_right`
  - `duplicate_key`
  - `invalid_record`
- Emit deterministic result ordering regardless of map iteration or input file ordering accidents.
- Return structured mismatch reasons that later packets can attach provenance to without reinterpretation.

## Interfaces and contracts

Expose a narrow interface such as:

```go
type Engine interface {
    Reconcile(ctx context.Context, req Request, files []NormalizedFile) (*ResultSet, error)
}
```

Suggested output types:

```go
type ResultSet struct {
    Mode    string
    Summary ResultSummary
    Rows    []ResultRow
}

type ResultRow struct {
    ResultID       string
    Status         string
    LeftRowKey     string
    RightRowKey    string
    GroupRowKeys   []string
    ComparedFields []FieldComparison
    Reasons        []ResultReason
}
```

Contract rules:

- `ResultID` ordering must be reproducible from deterministic sort keys.
- Numeric comparisons must use fixed decimal handling, never binary float comparisons.
- Grouped comparisons must define one grouping algorithm and one tie-break order.
- The engine may explain mismatches, but it must not attach kernel token IDs or audit data.
- The engine must consume the normalized output as-is; no file rereads, no new selectors, no reader calls.

## Input and output boundaries

Input:

- validated Packet 00 reconcile config
- normalized file outputs from Packet 01

Output:

- deterministic `ResultSet`
- result rows with explicit statuses and reasons
- enough row linkage for Packet 03 to attach captured-read and token references

Handoff to Packet 03:

- Packet 03 receives the Packet 01 normalized files plus Packet 02 result rows.
- Packet 02 must preserve row identity and compared field identity so minting can be attached later without recomputing matches.
- Packet 03 should not change statuses or matching decisions.

## Validation and tests

Add focused tests for:

- exact match success and mismatch cases
- amount tolerance exact, within tolerance, and outside tolerance cases
- currency mismatch behavior for amount comparisons
- existence mode with missing left and missing right results
- duplicate key classification
- empty key and invalid record classification
- one-to-many deterministic grouping with exact sum match
- one-to-many deterministic grouping with mismatch
- stable ordering across repeated runs with shuffled fixture insertion order
- schema/value validation failures before match classification where required by config

Use fixed decimal fixtures that exercise sign, scale, and zero handling.

Expected packet validation:

```bash
rtk go test ./reconcile/...
rtk go test ./...
rtk proxy git diff --check
```

Add fuzz or table coverage where parsing and grouping invariants are easy to express.

## Skills/guidance to load

- `engineering-workflow`
- `go-implementation-quality`
- `go-testing-fuzzing`
- `go-input-validation-security`
- `conventional-commit-guard` before commit messages and PR creation

## Non-goals

- No kernel minting or captured-read mutation.
- No bundle generation, verify/replay, CLI wiring, or ZIP report output.
- No fuzzy similarity, NLP, or model-assisted matching.
- No automatic field inference or config repair.
- No persistence or cross-run state.

## Stop conditions

Stop and replan if:

- matching requires fuzzy similarity or natural-language interpretation
- grouped amount logic depends on non-deterministic search or heuristic tie-breaking
- a requested mode cannot be expressed from normalized values alone
- Packet 03 would need Packet 02 to embed kernel semantics instead of plain row/field identity

## Done when

- fixture cases cover all supported deterministic modes
- amount tolerance handles exact, within tolerance, and outside tolerance cases
- duplicate keys and invalid records classify according to config
- one-to-many sums are deterministic and currency-safe
- output is stable across repeated runs
- Packet 03 can attach provenance using row and field identities without re-running reconciliation

## Commit/PR boundary

One PR limited to deterministic reconciliation engine and tests.
