# Packet 02 Implementation Plan - Reconciliation Engine

## Goal

Implement deterministic reconciliation over Packet 01 normalized files. This packet classifies records and field comparisons without provenance minting, bundle generation, CLI wiring, or report packaging.

## Suggested Package Shape

Use the existing harness package if established by earlier packets. Otherwise use:

```text
reconcile/engine/
├── classify.go
├── decimal.go
├── engine.go
├── indexes.go
├── match_exact.go
├── match_grouped.go
├── match_tolerance.go
├── validate_rows.go
└── engine_test.go
```

Suggested responsibilities:

- `engine.go`: public reconciliation entrypoint and mode dispatch
- `indexes.go`: deterministic key construction and duplicate detection
- `decimal.go`: fixed decimal parsing and comparison helpers
- `match_exact.go`: exact match behavior
- `match_tolerance.go`: amount tolerance behavior
- `match_grouped.go`: one-to-many and many-to-one behavior
- `validate_rows.go`: schema/value validation over normalized rows
- `classify.go`: stable statuses, reasons, ordering, and result ids

## Implementation Sequence

1. Inspect Packet 00 request types and Packet 01 normalized output.
2. Define the result model with stable ids, statuses, reasons, and compared fields.
3. Add preflight checks that referenced files and logical fields exist.
4. Implement deterministic key building from configured `match_on` fields.
5. Implement duplicate-key classification before matching.
6. Implement schema/value validation checks required by config.
7. Implement exact match.
8. Implement amount tolerance with decimal handling and currency rules.
9. Implement existence/completeness.
10. Implement grouped one-to-many and many-to-one sum comparison.
11. Add stable result sorting independent of map iteration.
12. Add table-driven tests for all modes and edge cases.

## Result Contract

Expose an entrypoint close to:

```go
func Reconcile(ctx context.Context, req request.Request, files []router.NormalizedFile) (*ResultSet, error)
```

Suggested result model:

```text
ResultSet
  Mode
  Summary
  Rows

ResultSummary
  Matched
  Mismatched
  MissingLeft
  MissingRight
  DuplicateKey
  InvalidRecord

ResultRow
  ResultID
  Status
  Key
  LeftFileID
  RightFileID
  LeftRowKey
  RightRowKey
  GroupRowKeys
  ComparedFields
  Reasons

FieldComparison
  LogicalField
  LeftRaw
  RightRaw
  LeftCanonical
  RightCanonical
  Status
  Delta

ResultReason
  Code
  Message
  Field
```

Statuses should be limited to:

- `matched`
- `mismatch`
- `missing_left`
- `missing_right`
- `duplicate_key`
- `invalid_record`

## Mode Details

`exact_match`:

- match by configured key fields
- compare configured fields when present
- classify absent keys on either side
- classify duplicate keys before comparing values

`amount_tolerance`:

- require amount fields in `compare`
- parse decimals without binary floats
- enforce currency equality before amount comparison
- classify within tolerance as matched and outside tolerance as mismatch

`existence`:

- prove keys from one side exist on the other side
- do not require amount comparison
- reject tolerance config

`one_to_many` and many-to-one shape:

- use configured grouping keys
- sum configured amount fields deterministically
- reject ambiguous grouping
- define deterministic tie-break behavior for duplicate or invalid groups

`schema_validation`:

- report missing required fields, empty required values, invalid numeric/date shapes, duplicate keys, and invalid allowed values if the contract includes allowed values
- do not run reader routing or header discovery

## Determinism Requirements

- never iterate maps without sorting keys before result creation
- create `ResultID` from deterministic mode/file/key/status components
- sort by status priority, key, file id, row position, then result id
- use fixed decimal parsing and comparison
- keep messages stable and concise

## Tests

Add tests for:

- exact match all matched
- exact match value mismatch
- missing left and missing right
- duplicate keys on one side and both sides
- amount exact match
- amount within tolerance
- amount outside tolerance
- currency mismatch
- malformed amount as invalid record
- existence success and absence
- grouped exact sum
- grouped sum mismatch
- grouped duplicate or ambiguous child rows
- stable ordering with shuffled inputs
- invalid config references fail before matching

## Validation

Run:

```bash
rtk go test ./...
rtk proxy git diff --check
```

Add fuzz or property tests only where they are small and deterministic, such as decimal parsing or stable key construction.

## Stop Conditions

Stop if a reconciliation mode requires fuzzy matching, probabilistic search, natural-language interpretation, automatic config repair, persistence, or provenance/kernel semantics.

## Handoff

Packet 03 consumes result rows plus normalized row/value identities. Packet 03 may attach evidence references, but it must not change statuses, reasons, keys, or comparison outcomes.
