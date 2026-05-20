# Phase 06 — Named-Source Mapping and Flat JSON Audit Pack

## Objective

Add two cosmetic but load-bearing affordances on top of the now-complete kernel surface: (1) a **named-source layer** that maps logical source names (e.g. `ledger.get_balance`, `payroll.export`) to concrete `(reader, path, selector)` tuples, recorded in captured reads and audit entries alongside the underlying reader name; and (2) a **flat single-file JSON audit pack** (`audit.pack.json`) that contains the same content as today's multi-file ZIP but as one schema-versioned JSON document for callers that prefer streaming a single artifact.

## Scope (in)

- New package `access/sources` providing a named-source registry decoded from a config JSON file.
- Augment `read-mint` CLI verb to accept `"source_name": "..."` as an alternative to `"source_path"` + raw selector. When a source name is used, the registry resolves it to a concrete reader and selector.
- Augment `kernel.CapturedRead` with optional `SourceName string` field. Augment the `token_mint` audit payload with optional `source_name`.
- New package `audit/pack` producing `audit.pack.json` from a finalized bundle + the same sidecar artifacts the ZIP already contains.
- New CLI verb `pack`: `provenant pack --bundle <path> --out audit.pack.json`. The existing `reconcile` ZIP output is unchanged.

## Scope (out)

- No removal of the ZIP path. ZIP and JSON pack coexist.
- No new ops.
- No new audit kinds.
- No HTTP / MCP.

## Technical Design

### Named-source registry

Config file shape (`sources.json`):

```json
{
  "schema_version": "provenant.sources/v1",
  "sources": {
    "ledger.get_balance": {
      "reader": "extract.csv_rows",
      "path": "./ledger.csv",
      "selector": {"row_window": "2:", "encoding": "utf-8", "dialect": "excel"},
      "output_path": "$.amount",
      "unit": "USD"
    },
    "payroll.export": {
      "reader": "extract.spreadsheet",
      "path": "./payroll.xlsx",
      "selector": {"sheet": "Q1", "range": "A1:F200"},
      "output_path": "$.gross_pay",
      "unit": "USD"
    }
  }
}
```

Registry surface:

```go
// access/sources/sources.go
type Registry struct { entries map[string]Entry }

type Entry struct {
    Reader     string
    Path       string
    Selector   kernel.SourceSelector
    OutputPath string
    Unit       string
}

func Load(r io.Reader) (*Registry, error)
func (r *Registry) Resolve(name string) (Entry, bool)
```

Resolution failure is fail-closed: an unknown source name returns a user error from the `read-mint` verb.

### Captured-read change

```go
// kernel/public/types.go
type CapturedRead struct {
    // ...existing fields...
    SourceName string `json:"source_name,omitempty"`
}
```

`source_name` is propagated into the deterministic `token_mint` payload:

```json
{
  "token_id": "...",
  "captured_read_id": "...",
  "source_name": "ledger.get_balance",
  "reader_name": "extract.csv_rows",
  ...
}
```

`source_name` is optional. Pre-Phase-06 bundles continue to verify because the field is `omitempty` and the bundle decoder treats absence as empty string. New verification logic: if `source_name` is present, it must be non-empty and must match the captured read.

### Flat JSON audit pack

`audit.pack.json` is a single canonical JSON document combining everything the ZIP currently carries:

```json
{
  "schema_version": "provenant.audit.pack/v1",
  "bundle_sha256": "<hex>",
  "bundle": { ... `phase1.bundle/v1` document, embedded inline ... },
  "captured_reads": [ ... ],
  "tokens": [ ... ],
  "audit_jsonl": "...\n...\n",
  "report": {
    "markdown": "...",
    "json": { ... },
    "mismatches_csv": "..."
  },
  "manifest": {
    "schema_version": "provenant.audit.pack.manifest/v1",
    "bundle_hash": "<hex>",
    "artifacts": [
      {"name": "audit.jsonl", "sha256": "..."},
      {"name": "bundle", "sha256": "..."}
    ]
  }
}
```

`report` is present only for runs that came from `reconcile`. For Derivation-Graph runs, `report` is absent (or, in v1, an object with `markdown: ""` and no mismatches). Decoders must accept both.

Canonicalization: the whole document is JCS-transformed. The pack file is therefore byte-deterministic for a given bundle.

### `pack` CLI verb

```bash
provenant pack --bundle ./run.bundle.json --out ./audit.pack.json
```

Behavior:

- Load `bundle` bytes, call `bundle.Verify`.
- Load any sidecar artifacts adjacent to the bundle (`report.md`, `report.json`, `mismatches.csv`) if present; otherwise omit them.
- Build the pack document, JCS-canonicalize, write atomically.

The verb is read-only with respect to the original bundle.

## Implementation Steps

1. Create `access/sources/sources.go` with `Load`, `Resolve`. Add table-driven tests for malformed registries.
2. Extend `kernel.CapturedRead` with `SourceName`. Update `kernel/public/bundle/bundle.go`:
   - `tokenMintPayload` adds `SourceName string json:"source_name,omitempty"`.
   - `validateCapturedReadAgainstPayload` checks the field equals the captured read's `SourceName`.
3. Extend the Phase 05 `read-mint` verb:
   - Accept `source_name` as an alternative to raw `read.source_path` + `read.selector`.
   - When `source_name` is provided, look it up in the registry (loaded from `--sources <path>` flag), construct the `ReadAndMintRequest` from the entry, and record `source_name` on the resulting captured read.
4. Create `audit/pack/pack.go`:
   - `Build(bundle []byte, opts Options) ([]byte, error)`.
   - JCS canonicalization.
5. Add CLI verb `pack` wiring to `access/cli/cli.go`.
6. Update `examples/reconciliation/README.md` and the top-level `README.md` to describe `audit.pack.json` as an alternative to the ZIP.

## Tests Required

| Test | Asserts |
|---|---|
| Registry decode | Valid registry parses; unknown reader name rejected; relative paths resolved against registry file dir |
| Resolve unknown name | Returns `false`; CLI returns user error |
| `read-mint` with source name | Captured read carries `SourceName`; token_mint payload includes it; bundle replay validates equality |
| Backward compat — bundle without `source_name` | Pre-Phase-06 bundles in `examples/reconciliation` still verify |
| Pack determinism | Building the pack twice from the same bundle yields byte-identical output |
| Pack verifies bundle | A pack built from a tampered bundle returns `ErrTamperDetected` at build time |
| `pack` CLI happy path | Produces a valid JSON document parsable as `provenant.audit.pack/v1` |
| Pack omits `report` section when sidecars absent | Document still decodes |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] Adding `SourceName` to `CapturedRead` does not break replay for any existing fixture.
- [ ] `audit.pack.json` is byte-deterministic for fixed inputs across Linux and macOS.
- [ ] The ZIP path remains unchanged and is still the default for `reconcile`.

## Risks / Open Questions

- Should the registry be required, or is the raw `source_path` path always available? Optional. Either path works. The registry is a convenience for skill packs that want consistent named handles.
- `audit.pack.json` could grow large for runs with many captured reads. JCS canonicalization is O(n log n) on key sorting; acceptable for runs up to ~10k entries. For larger runs, callers should keep using the ZIP, which streams its sidecars.
- Schema versioning: `provenant.audit.pack/v1` is the first version. Pack decoders must reject unknown schema versions with a typed error rather than silently ignoring fields.
- Do we make `pack` also accept a `.part` file (mid-run)? No. Pack operates only on finalized bundles. Mid-run inspection is what `replay` provides on the partfile.

## Dependencies

- [Phase 01](./phase-01-token-dag-schema.md), [Phase 04](./phase-04-run-lifecycle.md), [Phase 05](./phase-05-cli-surface.md) all merged.
