# Provenant

Provenant is a Go provenance kernel plus thin access surfaces for auditable source reads. Today it can read injected sources, mint provenance tokens from selected values, emit canonical audit bundles that can be verified and replayed later, and run a deterministic config-driven local reconciliation fixture through `provenant reconcile`.

## Start Here

If you are new to the repo, read this README first, then:

- [`architecture/README.md`](architecture/README.md) for repo navigation
- [`architecture/tools/extract/README.md`](architecture/tools/extract/README.md) for reader contracts
- [`examples/reconciliation/README.md`](examples/reconciliation/README.md) for the current deterministic `provenant reconcile` fixture
- [`.planning/implementation-plan.md`](.planning/implementation-plan.md) for shipped phase boundaries

## What Exists Today

### Core path

The shipped path is:

1. Resolve one source's bytes in memory.
2. Read a selected slice from that source.
3. Mint one token from the selected output path.
4. Snapshot a canonical bundle.
5. Verify or replay that bundle from bytes alone.

The thin access surfaces all call the same core path:

- CLI: `read-mint-bundle`
- HTTP: `POST /v1/read-mint-bundle`
- MCP: `provenant.read_mint_bundle`

### Readers

At the kernel level, structured readers exist for:

- `extract.json`
- `extract.jsonl`
- `extract.csv_rows`
- `extract.spreadsheet` for `.xlsx`

The current thin CLI, HTTP, and MCP wrappers are narrower: they accept one source at a time and currently only expose `reader: "extract.json"` in their request contract.

### Bundle and audit output

The top-level result is JSON around canonical bundle bytes and hash:

- `surface`
- `read_and_mint`
- `bundle_bytes`
- `bundle_sha256`

The decoded canonical bundle inside `bundle_bytes` is JSON with:

- `format`
- `run`
- `captured_reads`
- `tokens`
- `audit_jsonl`

`audit_jsonl` is the tamper-evident audit chain for the run.

## Reconciliation Fixture Path

Phase 5 adds a local, deterministic, config-driven reconciliation path for fixture-scale CSV/XLSX inputs:

```bash
go run ./cmd/provenant reconcile --request examples/reconciliation/reconcile.json --out /tmp/provenant-reconcile-report.zip --workdir examples/reconciliation
```

See [`examples/reconciliation/README.md`](examples/reconciliation/README.md) for the exact fixture inputs, request shape, ZIP artifacts, report excerpts, verification guidance, and failure fixtures.

For the broader user scenario "give Provenant arbitrary Excel files or CSVs and ask for reconciliation plus a complete audited chain":

### Exists now

- Kernel readers for JSON, JSONL, CSV row windows, and XLSX ranges/tables
- Deterministic captured reads
- Token minting tied to captured reads
- Canonical bundle emission, verification, replay, and derived export from bundle bytes
- Thin access surfaces proving the same single-source path across CLI, HTTP, and MCP
- Local `provenant reconcile` CLI path for an explicit request mapping two configured local sources

### Does not exist yet

- No thin HTTP/MCP reconciliation flow that accepts multiple files in one request
- No shipped reconciliation planner that infers mappings from arbitrary CSV/XLSX inputs
- No access-surface contract today for `extract.csv_rows` or `extract.spreadsheet`
- No complete end-user reconciliation product in this repo

So the accurate claim today is: Provenant ships the auditable read/mint/bundle path, structured readers, and a deterministic local reconciliation fixture command, but not a finished end-user reconciliation product.

## Run The Supported Path Today

The easiest supported path is the CLI:

```bash
rtk go run ./cmd/provenant read-mint-bundle --request /path/to/request.json
```

Request shape:

```json
{
  "reader": "extract.json",
  "source": {
    "source": {
      "path": "fixture://invoice.json",
      "sha256": "00a31c1d108c9a5045542250c7551107be5355a4ef6f5dad81a1ada6c4b894c9"
    },
    "bytes": "eyJpbnZvaWNlIjp7InRvdGFsIjoxMjMuNDV9fQ=="
  },
  "run": {
    "id": "run_1",
    "tenant_id": "tenant_1",
    "status": "running"
  },
  "read": {
    "source_path": "fixture://invoice.json",
    "selector": {
      "json_pointer": "/invoice/total",
      "jsonpath": "$.invoice.total",
      "byte_span": "20-26"
    },
    "output_path": "$.value",
    "unit": "USD"
  }
}
```

The same request contract is exercised across surfaces in [`access/smoke/smoke_test.go`](access/smoke/smoke_test.go).

## Example In Repo

Use [`access/smoke/smoke_test.go`](access/smoke/smoke_test.go) as the canonical end-to-end example for first-time readers. It shows:

- one JSON source
- one `extract.json` read
- one minted token
- one canonical bundle
- parity across CLI, HTTP, and MCP

For reader coverage beyond the current thin surfaces, see [`architecture/tools/extract/README.md`](architecture/tools/extract/README.md).

## Sample Output Shape

Representative CLI result:

```json
{
  "surface": "cli",
  "read_and_mint": {
    "read_result": {
      "output": {
        "byte_end": 26,
        "byte_start": 20,
        "json_pointer": "/invoice/total",
        "jsonpath": "$.invoice.total",
        "normalized_value": 123.45,
        "raw_value": "123.45",
        "source_hash": "00a31c1d108c9a5045542250c7551107be5355a4ef6f5dad81a1ada6c4b894c9",
        "value": 123.45
      },
      "captured_read": {
        "id": "read_5038967ffe96e2e546aaf16a042c0e33",
        "run_id": "run_1",
        "source_path": "fixture://invoice.json",
        "origin": "fixture://invoice.json#ptr=/invoice/total&byte=20-26",
        "selector": {
          "json_pointer": "/invoice/total",
          "jsonpath": "$.invoice.total",
          "byte_span": "20-26"
        },
        "reader_name": "extract.json",
        "reader_version": "v1",
        "source_sha256": "00a31c1d108c9a5045542250c7551107be5355a4ef6f5dad81a1ada6c4b894c9",
        "output_sha256": "7da2ca3e36de54031d97fcd9752e405ef2ca71311c0799b2ac75a62730a0e5d9"
      }
    },
    "token": {
      "id": "tok_12d6f0a3ea0b8ee41c02ee9cdeb145ff",
      "run_id": "run_1",
      "tenant_id": "tenant_1",
      "value_canonical": "123.450000000",
      "unit": "USD"
    },
    "audit_entry": {
      "seq": 1,
      "kind": "token_mint"
    }
  },
  "bundle_bytes": "<base64 canonical bundle bytes>",
  "bundle_sha256": "4c59478548e47bc1b47b874372135aaf6ed61bdedcabc648f8c638b44ad5a5b8"
}
```

Decoded bundle shape:

```json
{
  "format": "phase1.bundle/v1",
  "run": {
    "id": "run_1",
    "tenant_id": "tenant_1",
    "status": "running"
  },
  "captured_reads": [
    {
      "id": "read_5038967ffe96e2e546aaf16a042c0e33",
      "reader_name": "extract.json",
      "source_path": "fixture://invoice.json"
    }
  ],
  "tokens": [
    {
      "id": "tok_12d6f0a3ea0b8ee41c02ee9cdeb145ff",
      "value_canonical": "123.450000000"
    }
  ],
  "audit_jsonl": "{\"seq\":1,\"kind\":\"token_mint\",...}\n"
}
```

## Current Status

Phases 0-4 in the current release sequence are complete. The handoff boundary is recorded in [`.planning/phases/phase-four/handoff.md`](.planning/phases/phase-four/handoff.md). That closeout did not add a broader reconciliation product; it closed hardening and doc alignment for the shipped single-source path.
