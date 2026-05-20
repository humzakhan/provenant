# Packet 00 - Reconciliation Request Contract

## What the PR should do

Define the deterministic request contract for multi-file reconciliation and implement strict decoding plus fail-closed validation around that contract.

## What it should build

- A harness-owned request package for config types and validation only.
- Strict JSON request decoding with:
  - bounded body size
  - unknown-field rejection
  - trailing-data rejection
  - deterministic error shaping
- Typed request structs for:
  - `run`
  - `files`
  - `reconcile`
  - `selector`
  - `columns`
  - `numeric`
  - `tolerance`
  - optional mode-specific grouping config
- File declarations with explicit:
  - `id`
  - `purpose`
  - `format`
  - `path`
  - `reader`
  - `selector`
  - `columns`
- Column role declarations limited to:
  - `key`
  - `amount`
  - `currency`
  - `date`
  - `description`
  - `entity`
  - `status`
  - `reference`
  - `ignored`
- Reconciliation config with explicit:
  - `mode`
  - `left`
  - `right`
  - `match_on`
  - `compare`
  - `tolerance`
  - grouping rules for grouped modes only
- Deterministic validation errors for:
  - missing required fields
  - duplicate IDs or duplicate logical declarations
  - unsupported enums
  - incompatible format and reader combinations
  - ambiguous role wiring
  - invalid mode-specific config

## Required implementation detail

- Keep the contract config-driven. The request must fully describe file intent, reader choice, selector shape, and business field mapping.
- Model selectors as explicit per-format metadata, not generic free-form maps.
- Treat column aliases as logical field names owned by the request, with `source` pointing to the raw file header or field name.
- Validate semantic references by logical field name, not by raw source header.
- Keep validation split into:
  - shape validation
  - enum and compatibility validation
  - cross-reference validation
  - mode-specific validation

## Data model boundary

- `run.id` and `run.tenant_id` are required opaque strings.
- `files` must contain at least two file declarations.
- `file.id` must be unique within the request.
- `columns` should be keyed by logical field name and each entry must declare exactly one supported role.
- `match_on` and `compare` must reference declared logical field names from both compared files.
- `tolerance` is allowed only for tolerance-based modes.
- Grouping config is allowed only for grouped reconciliation modes.

## Skills/guidance to load

- `engineering-workflow`
- `go-input-validation-security`
- `go-implementation-quality`
- `go-testing-fuzzing`
- `conventional-commit-guard` before commit messages and PR creation

## Implementation boundary

No file reading, source hashing, reader routing, matching execution, token minting, CLI command, report generation, fixture packaging, or kernel API expansion. This packet defines the request contract and validation only.

## Pass criteria / validation

- Valid config fixtures for each supported reconciliation family decode and validate successfully.
- Unknown fields fail at every nesting level.
- Trailing JSON fails.
- Missing required run, file, selector, column, or reconcile fields fail.
- Duplicate file IDs fail.
- Duplicate or ambiguous logical field declarations fail.
- Unsupported formats, readers, roles, and modes fail.
- Incompatible format and reader pairings fail.
- Bad cross-file references in `left`, `right`, `match_on`, `compare`, and grouping config fail.
- Tolerance on unsupported modes fails.
- Amount declarations with missing or invalid currency linkage fail.
- Grouped-mode config missing group keys or compare fields fails.
- Tests are table-driven and cover both valid and invalid config paths.

## Stop conditions

- Stop if the contract requires automatic column guessing, fuzzy matching, inferred business meaning, or reader-specific behavior that cannot be described explicitly in config.
- Stop if validation needs runtime file inspection to determine whether the request is well-formed.
- Stop if the request format would need open-ended expression languages, user code hooks, or arbitrary maps to represent reconciliation rules.

## Exact non-goals

- No reader execution.
- No file existence checks.
- No header discovery from real files.
- No automatic schema inference.
- No defaulting based on file purpose.
- No reconciliation result model beyond what validation needs.
- No audit entries, bundles, or report artifacts.
- No CLI flags or command surface.

## Commit/PR boundary

One PR limited to request contract and validation tests.
