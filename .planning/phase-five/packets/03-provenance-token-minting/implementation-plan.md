# Packet 03 Implementation Plan - Provenance and Token Minting

## Goal

Bind Packet 01 source evidence and Packet 02 reconciliation results to kernel provenance. This packet makes every reportable value traceable to captured reads, minted tokens, audit entries, and a verified bundle.

## Suggested Package Shape

Use the established reconciliation package location. A focused layout could be:

```text
reconcile/provenance/
├── bind.go
├── bundle.go
├── evidence.go
├── mint.go
├── replay.go
└── provenance_test.go
```

Suggested responsibilities:

- `bind.go`: public API and result enrichment
- `evidence.go`: mapping normalized rows/fields to evidence paths
- `mint.go`: calls to existing kernel minting primitives
- `bundle.go`: snapshot and verify integration
- `replay.go`: replay test helpers if existing repo APIs support it

## Implementation Sequence

1. Read existing kernel, `memio`, audit sink, token minting, bundle snapshot, verify, and replay APIs.
2. Define provenance-enriched result types without changing Packet 02 outcomes.
3. Create an in-process execution setup using existing `memio` store and audit sink.
4. Map each normalized file, row, logical field, and source selector to a mintable evidence path.
5. Mint configured evidence fields through existing kernel primitives.
6. Attach token and captured-read references to result rows and field comparisons.
7. Snapshot canonical bundle bytes after all configured evidence is attached.
8. Verify the bundle.
9. Add a replay test for at least one full fixture path if repo APIs support replay.
10. Fail closed when evidence is missing, non-canonical, or not bundle-backed.

## Provenance Contract

Expose an entrypoint close to:

```go
func Bind(ctx context.Context, req request.Request, files []router.NormalizedFile, results *engine.ResultSet) (*ProvenancedResultSet, error)
```

Suggested output:

```text
ProvenancedResultSet
  Results
  CapturedReads
  Tokens
  BundleBytes
  BundleHash
  AuditRefs

ProvenanceRef
  FileID
  RowKey
  LogicalField
  CapturedReadID
  TokenID
  SourceHash
  SelectorPath
```

Rules:

- result status and reason fields from Packet 02 are immutable in this packet
- reportable values must have captured-read or token evidence
- token inputs must be canonical before crossing the kernel boundary
- bundle bytes must correspond to the exact evidence attached to the result set
- no report layer should need to infer evidence later

## Evidence Mapping

For each configured evidence field, preserve:

- file id
- source hash
- selector path or row locator
- logical field name
- source field/header name
- raw value
- canonical value
- role
- token id, if minted
- captured-read id, if exposed by the reader/kernel path

For result facts, mint only deterministic facts that are configured and needed for reporting, such as matched key, mismatch status, compared amount, currency, and result reason.

## Bundle and Audit Requirements

- use existing canonical bundle snapshot APIs
- run bundle verify in tests
- add replay coverage where practical
- preserve audit order
- avoid new audit event classes unless explicitly replanned
- avoid wall-clock time or random identifiers unless injected deterministically by existing kernel facilities

## Tests

Add tests for:

- key token minting
- amount token minting
- currency token minting
- date or reference token minting
- result fact token attachment when configured
- captured-read refs attached for source-backed fields
- missing evidence path fails closed
- non-canonical value fails or canonicalizes deterministically before minting
- bundle verify passes
- replay reconstructs the same evidence chain for one fixture
- Packet 02 statuses and reasons are unchanged after binding
- repeated runs attach stable evidence references when inputs are identical

## Validation

Run:

```bash
rtk go test ./...
rtk proxy git diff --check
```

If replay validation is expensive, keep replay to one focused fixture and keep other tests as unit-level evidence binding checks.

## Stop Conditions

Stop if evidence attachment requires new kernel audit semantics, public kernel surface expansion, report-only fabricated evidence, changed reconciliation outcomes, or nondeterministic bundle/replay behavior.

## Handoff

Packets 04 and 05 consume provenance-enriched results and bundle evidence. They must not mint new evidence or invent report claims that are not backed by this packet.
