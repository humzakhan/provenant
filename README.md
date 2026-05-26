# Provenant

[![CI](https://github.com/humzakhan/provenant/actions/workflows/ci.yml/badge.svg)](https://github.com/humzakhan/provenant/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/provenant/provenant.svg)](https://pkg.go.dev/github.com/provenant/provenant)
[![Go Report Card](https://goreportcard.com/badge/github.com/provenant/provenant)](https://goreportcard.com/report/github.com/provenant/provenant)

Provenant is a provenance layer for finance workflows: it records where every output number came from and packages the evidence for verify/replay. It is not a logger or a reconciliation product. It enforces four guarantees across every read path: non-bypassable exit, fail-closed errors, schema-first validation, and deterministic replay. It is designed for SR 11-7 / FINRA 2026 contexts where finance controllers, audit leads, platform engineers, and compliance officers need a verifiable audit chain from raw bytes through to a replayable audit bundle.

> **Status: v0 / pre-1.0 — early and actively evolving. APIs may change without notice between minor releases.**

---

## What Provenant Does (and Doesn't, Yet)

**Exists now:**

- Go kernel with non-bypassable exit, fail-closed errors, schema-first validation, and replay-determinism
- CLI `read-mint-bundle` with `extract.json` reader (JSON sources only)
- CLI `reconcile` fixture with `extract.csv_rows` (CSV) and `extract.spreadsheet` (XLSX) readers
- Audit bundle verify and replay (Go API: `bundle.Verify`, `bundle.Replay`)
- Hash-chained tamper-evident audit JSONL
- Deterministic ZIP report with manifest and sidecar hashes

**Does not exist yet:**

- HTTP surface for `reconcile` (no `POST /v1/reconcile` endpoint)
- MCP server tool for `reconcile`
- Reconciliation planner that infers column mappings from arbitrary inputs
- Fuzzy or AI-assisted matching
- A complete end-user reconciliation product

---

## Installation

**As a library:**

```bash
go get github.com/provenant/provenant
```

**CLI binary:**

```bash
go install github.com/provenant/provenant/cmd/provenant@latest
```

**Build from source:**

```bash
git clone https://github.com/humzakhan/provenant
cd provenant
go build ./...
go test ./...
```

---

## Quick Start

Two CLI subcommands exist today:

- `go run ./cmd/provenant read-mint-bundle` — read one JSON source, mint one token, emit an audit bundle.
- `go run ./cmd/provenant reconcile` — run the deterministic reconciliation fixture against local CSV/XLSX files.

---

## Mental Model

```
source bytes → SourceReader.Read → CapturedRead + ReadResult
                                    ↓
                              kernel.MintToken
                                    ↓
                        AuditSink.AppendAuditEntry (hash-chained)
                                    ↓
                        bundle.Snapshot → bundle bytes + SHA-256
                                    ↓
                       bundle.Verify / bundle.Replay (offline)
```

**Source bytes → captured read + ReadResult.** The source reader receives raw bytes with a declared SHA-256 and selector. It produces a captured read (`CapturedRead`: stable identity, origin URI, reader name and version, output hash) plus a `ReadResult` (normalized value, byte span, matched pointer). No value enters the kernel without a captured read.

**kernel.MintToken.** The kernel receives the captured read and issues a provenance token bound to it. The provenance token carries a canonical decimal value, unit, run ID, and tenant ID. Issuing the token is fail-closed: any validation failure returns an error; no partial token escapes.

**AuditSink.AppendAuditEntry.** Every issued token appends one audit entry to the audit chain. Each entry records its sequence number, kind, deterministic payload, previous hash, and current hash. The audit chain is tamper-evident: any edit to an earlier entry breaks the hash linkage.

**bundle.Snapshot → audit bundle bytes + SHA-256.** The snapshot serializes the run descriptor, all captured reads, all provenance tokens, and the full audit JSONL into a single canonical byte sequence. The SHA-256 of that sequence is the audit bundle fingerprint.

**bundle.Verify / bundle.Replay.** Verify checks the audit chain and confirms the audit bundle fingerprint matches. Replay re-derives every provenance token from the captured reads in the audit bundle and confirms the audit chain is consistent. Both are offline operations that require only the audit bundle bytes.

---

## Example 1: read-mint-bundle

The only supported reader for this surface is `extract.json`. Prepare a request file:

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

Run it:

```bash
go run ./cmd/provenant read-mint-bundle --request request.json
```

The response is JSON with four top-level keys: `surface`, `read_and_mint`, `bundle_bytes` (base64 canonical bundle), and `bundle_sha256`.

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

Decoded, `bundle_bytes` is JSON with format `"phase1.bundle/v1"`, containing `run`, `captured_reads`, `tokens`, and `audit_jsonl`.

The same request contract is exercised across surfaces in [`access/smoke/smoke_test.go`](access/smoke/smoke_test.go).

---

## Example 2: reconcile (reconciliation fixture)

The reconcile fixture reads two local files (`bank.csv` and `ledger.xlsx`), runs an exact-match comparison on `transaction_id`, and writes a ZIP report. The fixture is deterministic: the same inputs always produce the same output.

```bash
cd examples/reconciliation
go run ../../cmd/provenant reconcile \
  --request reconcile.json \
  --out report.zip \
  --workdir .
```

Re-run with `--overwrite` if `report.zip` already exists.

### ZIP contents

The output ZIP contains exactly these eight entries:

| Entry | Description |
|---|---|
| `report.md` | Human-readable summary: run ID, mode, result count, outcome counts. |
| `report.json` | Machine-readable version of the same counts plus per-result evidence. |
| `mismatches.csv` | Rows for every non-matched result with result ID, outcome, key, field, left value, right value. |
| `captured_reads.json` | All `CapturedRead` records from the run, including reader name, version, and output hash. |
| `tokens.json` | All minted tokens with canonical values and bound captured read IDs. |
| `audit.jsonl` | Hash-chained audit entries, one JSON object per line, covering every token mint. |
| `bundle.json` | The canonical audit bundle (`phase1.bundle/v1`) containing the full run, captured reads, tokens, and audit chain. |
| `manifest.json` | Verified bundle hash and SHA-256 for each companion artifact; used to confirm ZIP integrity offline. |

### Sample audit.jsonl entry

The first entry is a deterministic token mint for `bank.csv`:

```json
{"at":"1970-01-01T00:00:00Z","hash":"1c3a31b2c15870aa4b205af0263bf882123987affaa55e75222f89ee74203163","kind":"token_mint","payload_deterministic":{"captured_read_id":"read_c3182a42dc24532fcb411f81391f198b","reader_name":"extract.csv_rows","reader_version":"v1","source_sha256":"c430a1f9c8f25c2603459acb5f77b0531f5bddcd2429abafd6e06dc1f59911e0","source_value_raw":"100.00","token_id":"tok_b96a501573ad7fd5edb9185ed93604d5","value_canonical":"100.000000000"},"prev_hash":"4533226321c664c56ea0c277312cb192f27ae2cdad5bb632780ba79a7f9de1ba","run_id":"reconciliation_fixture_run_1","seq":1,"tenant_id":"tenant_demo"}
```

Absolute `source_path` and `origin` fields in the real output depend on the working directory at run time.

### Sample manifest.json shape

```json
{
  "schema_version": "provenant.reconcile.report.manifest.v1",
  "bundle_hash": "43985b8db3be88c9c1b7dbef2fcd5028349eff173302f29f6ad815662983be25",
  "artifacts": [
    {
      "name": "audit.jsonl",
      "size": 14740,
      "sha256": "7d4af8e201608b42c09d41ead8dec89feb5875eaa099a7bf926f1f31be51f8c8"
    },
    {
      "name": "bundle.json",
      "size": 29318,
      "sha256": "43985b8db3be88c9c1b7dbef2fcd5028349eff173302f29f6ad815662983be25"
    }
  ]
}
```

The full manifest also covers `captured_reads.json`, `mismatches.csv`, `report.json`, `report.md`, and `tokens.json`.

Note: `extract.csv_rows` and `extract.spreadsheet` are available as kernel readers and are used by the reconcile fixture path. They are not exposed by the `read-mint-bundle` CLI surface today.

---

## Verifying a Bundle

`bundle.Verify` and `bundle.Replay` are Go API only today. There is no CLI verb for standalone bundle verification. The CLI tests exercise both in [`access/cli`](access/cli).

To verify or replay a bundle in Go:

```go
import "github.com/provenant/provenant/kernel/public/bundle"

data, _ := os.ReadFile("bundle.json")
if err := bundle.Verify(data); err != nil {
    log.Fatalf("bundle tampered or invalid: %v", err)
}
entries, _ := bundle.Replay(data)
_ = entries // captured reads and tokens re-derived from bundle bytes
```

Run the full fixture test including verify/replay:

```bash
go test ./access/cli
```

---

## Failure Fixtures

Seven failure and edge-case fixtures live under `examples/reconciliation/failures/`:

| Fixture | Behavior |
|---|---|
| `missing_required_source` | Missing required source data fails with `selector_invalid`. |
| `malformed_amount` | Invalid numeric evidence fails with `evidence_failed`. |
| `unsupported_format` | Unsupported input format fails with `request_invalid`. |
| `invalid_selector` | Invalid reader selector fails with `request_invalid`. |
| `duplicate_key` | Produces `duplicate_key` outcomes without writing a failure exit. |
| `invalid_record` | Produces `invalid_record` and `missing_left` outcomes. |
| `currency_mismatch` | Produces a deterministic `mismatch` outcome for mismatched currency fields. |

---

## Example 3: Derivation Graph End-to-End Demo

The derivation graph demo shows the full pipeline from raw CSV accounting data to a traced, verified operating income figure. Three committed CSV files (`revenue.csv`, `cogs.csv`, `opex.csv`) are minted row-by-row into leaf tokens, composed via `filter.sum_where`, `calc.sum`, and `calc.subtract`, and the final `operating_income` token is traced back to every source row with signed contribution amounts.

The demo uses seed 42 and produces a deterministic bundle and trace:

```
operating_income = $350,000 USD
  ← gross_margin ($550,000) − total_opex ($200,000)
    ← gross_revenue ($950,000) − total_cogs ($400,000)
      ← (net_revenue $1,000,000 − refunds $50,000)
```

Run the demo tests:

```bash
go test ./examples/derivation-graph/...
```

See [`examples/derivation-graph/README.md`](examples/derivation-graph/README.md) for the full walkthrough, dataset description, and golden regeneration procedure.

---

## Project Layout

```
kernel/                  Pure Go enforcement kernel: readers, minting, audit chain, bundle.
access/cli/              CLI runner, flag parsing, and end-to-end tests including failure fixtures.
reconcile/               Reconciliation engine, request schema, report generation, and provenance binding.
cmd/provenant/           Main entry point for the CLI binary.
examples/reconciliation/ Reconciliation fixture inputs, reconcile.json, and expected outputs.
```

---

## Contributing

Contributions are welcome. Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) before opening a pull request and follow the [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

To report a security vulnerability, see [`SECURITY.md`](SECURITY.md).

---

## License

Provenant is released under the [MIT License](LICENSE).

---

## Further Reading

- [`docs/thesis.md`](docs/thesis.md) — design principles and invariant rationale
- [`architecture/README.md`](architecture/README.md) — repo navigation map
- [`architecture/tools/extract/README.md`](architecture/tools/extract/README.md) — reader contracts and selector reference
- [`ROADMAP.md`](ROADMAP.md) — planned milestones
- [`docs/KNOWN_TRADE_OFFS.md`](docs/KNOWN_TRADE_OFFS.md) — known trade-offs
- [`docs/OPEN_QUESTIONS.md`](docs/OPEN_QUESTIONS.md) — open design questions
- [`examples/reconciliation/README.md`](examples/reconciliation/README.md) — full reconciliation fixture walkthrough, verification guidance, and explicit non-support list
- [`examples/derivation-graph/README.md`](examples/derivation-graph/README.md) — derivation graph end-to-end demo with operating income trace
