# Packet 00 Implementation Plan - Reconciliation Request Contract

## Goal

Implement the request/config contract for the deterministic reconciliation harness. This packet makes reconciliation requests parseable, validatable, and fail-closed before any file is read or any kernel reader is called.

## Suggested Package Shape

Use a small package owned by the harness layer, for example:

```text
reconcile/request/
├── decode.go
├── errors.go
├── request.go
├── validate.go
├── validate_modes.go
└── request_test.go
```

If the repo already has a better harness package location when implementation starts, use the existing local pattern.

Suggested responsibilities:

- `request.go`: type definitions and supported enum constants
- `decode.go`: strict decode entrypoint from `io.Reader` or `[]byte`
- `errors.go`: stable typed validation errors or error codes
- `validate.go`: common validation and cross-reference checks
- `validate_modes.go`: mode-specific checks only
- `request_test.go`: table-driven positive and negative coverage

## Request Types

Define types for:

- `Request`
- `RunConfig`
- `FileConfig`
- `SelectorConfig`
- `ColumnConfig`
- `NumericConfig`
- `ReconcileConfig`
- `ToleranceConfig`
- `GroupConfig`
- `ValidationError`

The top-level request should include:

```json
{
  "run": {
    "id": "run_recon_001",
    "tenant_id": "tenant_1"
  },
  "files": [],
  "reconcile": {}
}
```

Prefer explicit structs over `map[string]any`. Avoid unbounded free-form config sections.

## Suggested Data Model

Use a structure close to:

```text
Request
  Run RunConfig
  Files []FileConfig
  Reconcile ReconcileConfig

RunConfig
  ID string
  TenantID string

FileConfig
  ID string
  Purpose string
  Format string
  Path string
  Reader string
  Selector SelectorConfig
  Columns map[string]ColumnConfig

SelectorConfig
  RowWindow string
  Sheet string
  Table string
  HeaderRow int
  Encoding string
  Dialect string

ColumnConfig
  Source string
  Role string
  Required bool
  Unique bool
  Format string
  Numeric *NumericConfig

NumericConfig
  Scale int
  CurrencyField string

ReconcileConfig
  Mode string
  Left string
  Right string
  MatchOn []string
  Compare []string
  Tolerance *ToleranceConfig
  Group *GroupConfig

ToleranceConfig
  Amount string

GroupConfig
  LeftKey []string
  RightKey []string
```

Notes:

- `Columns` keys are logical field names such as `transaction_id`, `amount`, or `currency`.
- `ColumnConfig.Source` is the raw source column or field name.
- Cross-file references in `match_on`, `compare`, and grouping config should point to logical field names, not source headers.
- `ToleranceConfig.Amount` should stay string-based in the request model so decode does not introduce float ambiguity. Numeric parsing can validate decimal shape without executing reconciliation.

## File Config

Each file must declare:

- `id`: stable local identifier, unique within the request
- `purpose`: business role such as `bank_statement`, `ledger`, or `invoices`
- `format`: one of `csv`, `xlsx`, `json`, `jsonl`
- `path`: input path for later packets; validate presence only in this packet
- `reader`: one of `extract.csv_rows`, `extract.spreadsheet`, `extract.json`, `extract.jsonl`
- `selector`: row/range/table/window metadata for later reader routing
- `columns`: named logical fields mapped to source columns

Selector guidance:

- CSV and JSONL requests should normally require row/window selection metadata.
- Spreadsheet requests should normally require sheet or table targeting metadata.
- JSON requests may allow a simpler selector, but it must still be typed and deterministic.
- Do not add selector keys that imply execution-time discovery.

Supported format and reader pairs should be explicitly allowlisted:

- `csv` -> `extract.csv_rows`
- `xlsx` -> `extract.spreadsheet`
- `json` -> `extract.json`
- `jsonl` -> `extract.jsonl`

## Column Roles

Support these roles:

- `key`
- `amount`
- `currency`
- `date`
- `description`
- `entity`
- `status`
- `reference`
- `ignored`

Each column config should support:

- `source`: source column/header name
- `role`
- `required`
- `unique`
- `format` for dates
- `numeric` options for amounts

Amount columns may point to a currency field:

```json
{
  "amount": {
    "source": "Amount",
    "role": "amount",
    "required": true,
    "numeric": {
      "scale": 2,
      "currency_field": "currency"
    }
  }
}
```

Column validation rules:

- every column entry must have a non-empty logical field name
- every column entry must declare non-empty `source` and `role`
- logical field names must be unique within a file
- `ignored` fields cannot also be marked `required` or `unique`
- `numeric` is allowed only on `amount`
- `format` is allowed only on `date`
- `currency_field` must reference another declared logical field whose role is `currency`
- at most one logical field per file should claim each singleton semantic role unless the mode explicitly allows otherwise:
  - `amount`
  - `currency`
  - `date`

## Reconcile Config

Support mode declarations for:

- `exact_match`
- `amount_tolerance`
- `existence`
- `one_to_many`
- `schema_validation`

The config should support:

- `left`
- `right`
- `match_on`
- `compare`
- `tolerance`
- grouping fields for one-to-many and many-to-one modes

This packet validates the shape only. It does not execute reconciliation.

Mode expectations:

- `exact_match`
  - requires `left`, `right`, `match_on`
  - may require `compare` when value equality beyond key existence is intended
- `amount_tolerance`
  - requires `left`, `right`, `match_on`, `compare`, `tolerance`
  - `compare` must include at least one `amount` logical field
- `existence`
  - requires `left`, `right`, `match_on`
  - should reject `tolerance`
- `one_to_many`
  - requires `left`, `right`, `match_on`, `compare`, `group`
- `schema_validation`
  - requires `left`, `right`
  - compares declared logical schema compatibility across two configured files

## Validation Rules

Fail closed when:

- request has unknown JSON fields
- request exceeds the configured size bound
- request contains trailing non-whitespace JSON
- `run.id` or `run.tenant_id` is missing
- `files` has fewer than two entries
- a file has missing `id`, `purpose`, `format`, `path`, `reader`, or `columns`
- file IDs are duplicated
- file format is unsupported
- reader is unsupported
- format and reader disagree
- required selector fields are absent for the format family
- columns are missing source names or roles
- a column role is unsupported
- logical field references in `match_on`, `compare`, or group config do not exist on both referenced files
- the same logical field is assigned conflicting role semantics
- singleton semantic roles are duplicated in one file
- `numeric` appears on a non-amount field
- date `format` appears on a non-date field
- an amount column references a missing currency field
- an amount column references a field that is not declared as `currency`
- required key fields are missing from `match_on`
- tolerance is invalid or used with a non-amount mode
- reconciliation mode is unsupported
- `left` or `right` references a missing file ID
- `left` and `right` point to the same file where the mode expects two distinct files
- mode-specific grouping config is missing, partial, or used with a non-grouped mode

## Decoder

Use a strict JSON decoder:

- limit request size with a fixed byte cap owned by this package
- reject unknown fields
- reject trailing JSON
- return stable errors suitable for table tests

Prefer one entrypoint such as:

```go
func DecodeRequest(r io.Reader) (*Request, error)
```

Implementation detail:

- wrap the reader with `io.LimitReader`
- use `json.Decoder`
- call `DisallowUnknownFields`
- decode once into `Request`
- require EOF after optional whitespace
- run semantic validation after structural decode

## Error Handling

Keep errors deterministic and machine-checkable.

Suggested shape:

- sentinel or typed error for decode failures
- sentinel or typed error for validation failures
- per-field error code strings for assertions, for example:
  - `missing_required`
  - `unsupported_enum`
  - `duplicate_id`
  - `invalid_reference`
  - `invalid_mode_config`
  - `incompatible_reader`
  - `ambiguous_role`

Error requirements:

- include the request field path when possible
- do not include nondeterministic values
- do not depend on map iteration order
- sort collected validation errors before returning if more than one error is reported

## Implementation Sequence

1. Define enums, request structs, and small helper lookups.
2. Implement strict decode with bounded input and trailing-data rejection.
3. Implement per-struct required-field validation.
4. Implement file-level enum and selector validation.
5. Implement column role and logical-field validation.
6. Implement cross-file reference validation.
7. Implement mode-specific validation.
8. Add table tests for valid and invalid cases.
9. Run focused and repo-wide validation commands.

## Test Matrix

Add table tests for:

- valid exact match request
- valid amount tolerance request
- valid existence request
- valid one-to-many request
- valid schema validation request
- strict decode rejects trailing JSON
- strict decode rejects oversized input
- unknown top-level field
- unknown nested field
- duplicate file ID
- missing run
- fewer than two files
- missing file path
- unsupported format
- unsupported reader
- mismatched format/reader
- missing selector for CSV
- missing selector for XLSX
- unsupported column role
- duplicate singleton amount role
- `numeric` on non-amount field
- date format on non-date field
- amount column with missing currency field
- amount column with non-currency reference
- unsupported reconciliation mode
- missing left/right file reference
- `match_on` references missing logical field
- `compare` references missing logical field
- grouped mode missing group keys
- tolerance supplied to existence mode
- invalid tolerance

Test style:

- use table-driven tests with fixture JSON embedded as string literals or testdata files
- assert stable error codes or stable error substrings
- include at least one fully valid fixture per supported mode
- keep tests contract-focused; do not read files or call readers

## Validation Commands

Expected packet validation:

```bash
rtk go test ./reconcile
rtk go test ./...
rtk proxy git diff --check
```

Adjust the focused package test if the implementation chooses a different package path.

## Exact Non-Goals

- No request-to-reader execution path
- No file existence or contents checks
- No automatic header normalization or alias inference
- No reconciliation output classification types unless validation strictly needs them
- No CLI request loading helper outside the chosen request package
- No provenance minting hooks
- No report or manifest model

## Stop Conditions

- Stop if the request shape cannot stay fully explicit and typed.
- Stop if a mode needs business-rule inference instead of declared config.
- Stop if validation depends on opening files, inspecting headers, or sampling rows.
- Stop if the package needs dynamic expressions, regex-based column matching, or arbitrary callback hooks.

## Done When

- request parsing is strict
- validation is deterministic and fail-closed
- all supported modes and roles are represented in typed config
- tests cover positive and negative config cases
- no file reading, reader routing, matching, minting, CLI, or report generation is implemented
