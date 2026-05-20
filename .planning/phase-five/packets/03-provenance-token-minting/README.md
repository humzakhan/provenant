# Packet 03 - Provenance and Token Minting

## Goal

Attach Packet 01 source evidence and Packet 02 reconciliation facts to kernel provenance so every reportable value is traceable to captured reads, tokens, bundle bytes, and deterministic audit output.

## Suggested package shape

Use a narrow provenance integration layer, for example:

```text
reconcile/
├── provenance.go
├── mint.go
├── bundle.go
├── replay.go
└── provenance_test.go
```

Keep reconciliation logic outside this layer. This packet integrates with existing kernel APIs only.

## Technical tasks

- Set up an in-process run using `memio` store and audit sink for fixture and harness execution paths.
- For each normalized file from Packet 01, connect the normalized row/value fields to the captured-read identities produced by the underlying reader flow.
- For each configured evidence field, call `kernel.ReadAndMint` or the equivalent existing kernel primitive once with deterministic paths and canonical values.
- Mint tokens for keys, amounts, currency, dates, references, and any configured reportable facts from Packet 02 result rows.
- Embed captured-read IDs, token IDs, and any required locator metadata into the reconciliation result rows without changing Packet 02 statuses or reasons.
- Snapshot canonical bundle bytes after minting and audit capture.
- Verify the bundle in tests.
- Replay the bundle in tests when needed to prove that captured reads, tokens, and audit entries reconstruct the same evidence chain.
- Fail closed when any reportable value lacks a valid captured-read path, mintable path, token reference, or bundle-backed proof.

## Interfaces and contracts

Expose a narrow API such as:

```go
type ProvenanceBinder interface {
    Bind(ctx context.Context, req Request, files []NormalizedFile, results *ResultSet) (*ProvenancedResultSet, error)
}
```

Suggested output types:

```go
type ProvenancedResultSet struct {
    Results      *ResultSet
    CapturedReads []CapturedReadRef
    Tokens       []TokenRef
    BundleHash   string
}
```

Contract rules:

- Every reportable field in `Results` must have either a captured-read reference, a token reference, or both, depending on the field type.
- Token mint inputs must be canonical and deterministic before they cross the kernel boundary.
- Bundle snapshot bytes and verify results must correspond to the exact token/captured-read set attached to the result rows.
- This packet may enrich result rows with provenance fields, but it must not change reconciliation outcomes.

## Input and output boundaries

Input:

- validated Packet 00 request
- normalized file outputs from Packet 01
- deterministic result rows from Packet 02

Output:

- reconciliation results enriched with provenance references
- bundle snapshot and verification evidence for later packets
- stable token and captured-read linkage for report generation

Handoff to later packets:

- Packet 04 and beyond consume the enriched results plus bundle-backed evidence only.
- Later report packaging must not infer provenance that this packet did not explicitly attach.
- Any value that cannot be proven here is a stop condition, not a report-layer problem.

## Validation and tests

Add focused tests for:

- token minting for key, amount, currency, date, and reference fields
- deterministic token attachment across repeated runs
- captured-read references preserved for every source-backed field
- bundle snapshot succeeds and verify passes
- replay reconstructs the same captured reads, tokens, and audit entries for fixtures
- missing provenance path fails closed
- non-canonical mint inputs fail with stable errors
- Packet 02 result statuses remain unchanged after provenance binding

Expected packet validation:

```bash
rtk go test ./reconcile/...
rtk go test ./...
rtk proxy git diff --check
```

If replay tests are expensive, keep one focused fixture for replay and the rest as unit tests around binding logic.

## Skills/guidance to load

- `engineering-workflow`
- `provenant-source-reader-provenance`
- `provenant-audit-determinism`
- `provenant-kernel-boundary`
- `go-testing-fuzzing`
- `conventional-commit-guard` before commit messages and PR creation

## Non-goals

- Do not move reconciliation logic into the kernel.
- Do not add new audit semantics, new token classes, or persistent storage.
- Do not generate the final ZIP report package in this packet.
- Do not widen kernel public APIs unless an existing boundary is demonstrably insufficient.

## Stop conditions

Stop and replan if:

- a desired report value cannot be backed by captured reads, tokens, bundle bytes, or manifest-ready evidence
- minting requires new kernel audit semantics
- required provenance attachment would force Packet 02 to change match outcomes
- bundle verification or replay exposes nondeterministic evidence attachment

## Done when

- every reportable configured field has a captured-read or token reference
- bundle verification passes
- replay reconstructs captured reads, tokens, and audit entries for fixture runs
- invalid or missing provenance fails closed
- later report packaging can consume provenance-rich results without recomputing evidence

## Commit/PR boundary

One PR limited to provenance connection and bundle verification evidence.
