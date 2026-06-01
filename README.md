<div align="center">

<h1>Provenant</h1>

<p>Provenance for finance workflows, with evidence you can verify and replay.</p>

[![CI](https://github.com/humzakhan/provenant/actions/workflows/ci.yml/badge.svg)](https://github.com/humzakhan/provenant/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/humzakhan/provenant.svg)](https://pkg.go.dev/github.com/humzakhan/provenant)
[![Go Report Card](https://goreportcard.com/badge/github.com/humzakhan/provenant)](https://goreportcard.com/report/github.com/humzakhan/provenant)
<br />
<p><strong>Supported MCP hosts</strong></p>
<p>
  <img alt="Claude_Code" src="https://img.shields.io/badge/Claude_Code-supported-5B5BD6?style=flat-square" />
  <img alt="Cursor" src="https://img.shields.io/badge/Cursor-supported-5B5BD6?style=flat-square" />
  <img alt="Codex_CLI" src="https://img.shields.io/badge/Codex_CLI-supported-5B5BD6?style=flat-square" />
  <img alt="opencode" src="https://img.shields.io/badge/opencode-supported-5B5BD6?style=flat-square" />
  <img alt="Hermes_Agent" src="https://img.shields.io/badge/Hermes_Agent-supported-5B5BD6?style=flat-square" />
</p>
<p>
  <img alt="Gemini_CLI" src="https://img.shields.io/badge/Gemini_CLI-supported-5B5BD6?style=flat-square" />
  <img alt="Antigravity_IDE" src="https://img.shields.io/badge/Antigravity_IDE-supported-5B5BD6?style=flat-square" />
  <img alt="Kiro" src="https://img.shields.io/badge/Kiro-supported-5B5BD6?style=flat-square" />
  <img alt="Kimi_CLI" src="https://img.shields.io/badge/Kimi_CLI-supported-5B5BD6?style=flat-square" />
  <img alt="Pi.dev_agent" src="https://img.shields.io/badge/Pi.dev_agent-supported-5B5BD6?style=flat-square" />
</p>
<br />
</div>

Provenant is a provenance layer for finance workflows: it records where every output number came from and packages the evidence for verify/replay. It is not a logger, a database, a workflow engine, or a complete accounting product. It is the enforcement layer that sits between source files, agent/tool reads, numeric claims, and the final answer that a finance team is willing to release.

It enforces four guarantees across every supported read path: non-bypassable exit, fail-closed errors, schema-first validation, and deterministic replay. It is designed for SR 11-7 / FINRA 2026 contexts where finance controllers, audit leads, platform engineers, and compliance officers need a verifiable audit chain from raw bytes through to a replayable audit bundle.

> **Status: v0 / pre-1.0 — early and actively evolving. APIs may change without notice between minor releases.**

---

## The Problem

Finance teams increasingly ask AI systems practical questions over local exports, close packages, reconciliation workbooks, and customer/vendor schedules:

- "What was revenue for Q1?"
- "Why does booked revenue not match cash received?"
- "Which customers still owe us money, and who is the largest balance?"

Those are not hard questions because the arithmetic is exotic. They are hard because the answer is only useful if the organization can later prove where every number came from. A polished paragraph from Claude Code, Codex, or another agent workspace is not enough for a controller, auditor, or model-risk reviewer. They need to know which files were read, which values were selected, which derived values were minted, whether the final answer contains ungrounded numbers, and whether the evidence can be verified offline after the agent session is gone.

The failure mode Provenant is built to prevent is simple: an answer can be directionally correct, well written, and still impossible to audit. If a released answer says revenue was `$395,061.13`, the reviewer should not have to trust the model's prose. The reviewer should be able to inspect the captured read, the token, the hash-chained audit entry, and the bundle that replays the value.

---

## The Solution

Provenant turns numeric claims into grounded, replayable evidence.

The core loop is:

```text
source bytes -> SourceReader.Read -> CapturedRead + ReadResult
                                   |
                                   v
                             kernel.MintToken
                                   |
                                   v
                       AuditSink.AppendAuditEntry (hash-chained)
                                   |
                                   v
                       bundle.Snapshot -> bundle bytes + SHA-256
                                   |
                                   v
                      bundle.Verify / bundle.Replay (offline)
```

In user-facing terms:

1. The agent reads generated or user-provided finance files.
2. Every released numeric value must be minted through Provenant.
3. `provenant check-answer` checks the final answer against the bundle.
4. A Stop hook can block ungrounded numbers before release.
5. Negative controls prove that made-up numbers are rejected.

This is deliberately stricter than normal logging. The kernel owns the path from captured source read to token to audit entry. A numeric value that does not pass validation does not become a token. A final answer with an ungrounded number fails the check-answer path.

---

## What It Looks Like For A User

Imagine a controller has January, February, and March finance exports in a workspace. They open Claude Code, or another agent environment wired to Provenant's kernel and MCP surface, and ask:

```text
I uploaded the January, February, and March 2026 finance exports. What was revenue for Q1 2026? Give me the monthly revenue totals, the quarter total, and the biggest revenue account driving each month.
```

The useful answer is not just:

```text
Q1 revenue was $395,061.13.
```

The useful answer is a grounded answer with machine-checkable values:

```json
{
  "scenario_id": "quarterly_revenue_summary",
  "quarter": "2026-Q1",
  "quarter_total_revenue": "$395,061.13",
  "months": [
    {
      "period": "2026-01",
      "total_revenue": "$185,951.48",
      "top_account_code": "4000",
      "top_account_amount": "$145,633.67"
    },
    {
      "period": "2026-02",
      "total_revenue": "$84,784.44",
      "top_account_code": "4000",
      "top_account_amount": "$62,724.56"
    },
    {
      "period": "2026-03",
      "total_revenue": "$124,325.21",
      "top_account_code": "4000",
      "top_account_amount": "$92,136.28"
    }
  ]
}
```

Behind that answer, Provenant keeps the bundle, the captured reads, the minted tokens, the raw Claude stdout/stderr, the MCP request/response logs, the direct grounded Stop-hook evidence, the negative Stop-hook control, and a scenario verification JSON.

That is the difference Provenant is trying to make: the user still asks a natural finance question, but the released numbers are no longer unsupported prose.

---

## Installation

**As a library:**

```bash
go get github.com/humzakhan/provenant
```

**CLI binary:**

```bash
go install github.com/humzakhan/provenant/cmd/provenant@latest
```

**Build from source:**

```bash
git clone https://github.com/humzakhan/provenant
cd provenant
go build ./...
go test ./...
```

**Install into an MCP host:**

```bash
provenant install --target claude
provenant install --target codex,cursor
provenant install --target all
provenant install --print-config codex
```

That writes the local host config needed to expose the Provenant MCP server. Claude Code also gets project hooks and Stop-hook enforcement. Other MCP hosts get tool access plus official answer attestation through `provenant.submit_answer`; arbitrary host text is not Provenant-attested.

| Host | Compatibility | Notes |
|---|---|---|
| Claude Code | Supported | Project hooks plus Stop-hook enforcement. |
| Cursor | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Codex CLI | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| opencode | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Hermes Agent | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Gemini CLI | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Antigravity IDE | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Kiro | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Kimi CLI | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |
| Pi.dev agent | Supported | MCP tool access; use `provenant.submit_answer` for attested answers. |

---

## What Provenant Does Now

**Exists now:**

- Go kernel with non-bypassable exit, fail-closed errors, schema-first validation, and replay-determinism.
- CLI `read-mint-bundle` with `extract.json` reader.
- MCP surface for `provenant.read_mint_bundle`.
- Claude Code install path with SessionStart and Stop hooks.
- `provenant.submit_answer` as the attestation boundary for official answers.
- `provenant check-answer` for detecting ungrounded numeric claims in final answers.
- CLI `reconcile` fixture with `extract.csv_rows` and `extract.spreadsheet` readers.
- Audit bundle verify and replay through Go APIs and CLI commands.
- Hash-chained tamper-evident audit JSONL.
- Deterministic ZIP report with manifest and sidecar hashes.
- End-to-end Fauxbooks user-scenario runner over generated finance files.

**Does not exist yet:**

- A complete end-user accounting or reconciliation product.
- A planner that infers column mappings from arbitrary customer files.
- Fuzzy or AI-assisted matching.
- HTTP surface for the reconciliation fixture.
- MCP server tool for the reconciliation fixture.
- Host-conformance enforcement for every possible agent environment.

---

## Developer Quick Start

Two useful CLI paths exist today:

- `go run ./cmd/provenant read-mint-bundle` — read one JSON source, mint one token, emit an audit bundle.
- `go run ./cmd/provenant reconcile` — run the deterministic reconciliation fixture against local CSV/XLSX files.

### Example: read-mint-bundle

The supported reader for this surface is `extract.json`. Prepare a request file:

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

The response includes the captured read, minted token, audit entry, base64 canonical bundle bytes, and bundle SHA-256. Decoded, `bundle_bytes` is JSON with format `"phase1.bundle/v1"`, containing `run`, `captured_reads`, `tokens`, and `audit_jsonl`.

The same request contract is exercised across surfaces in [`access/smoke/smoke_test.go`](access/smoke/smoke_test.go).

### Example: reconcile fixture

The reconcile fixture reads two local files (`bank.csv` and `ledger.xlsx`), runs an exact-match comparison on `transaction_id`, and writes a ZIP report. The fixture is deterministic: the same inputs always produce the same output.

```bash
cd examples/reconciliation
go run ../../cmd/provenant reconcile \
  --request reconcile.json \
  --out report.zip \
  --workdir .
```

Re-run with `--overwrite` if `report.zip` already exists.

The output ZIP contains:

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

Note: `extract.csv_rows` and `extract.spreadsheet` are available as kernel readers and are used by the reconcile fixture path. They are not exposed by the `read-mint-bundle` CLI surface today.

---

## Verifying And Replaying Evidence

`bundle.Verify` and `bundle.Replay` are available through Go APIs and the top-level `verify` and `replay` CLI commands in [`access/cli`](access/cli). CLI tests cover both commands.

To verify or replay a bundle in Go:

```go
import "github.com/humzakhan/provenant/kernel/public/bundle"

data, _ := os.ReadFile("bundle.json")
if err := bundle.Verify(data); err != nil {
    log.Fatalf("bundle tampered or invalid: %v", err)
}
entries, _ := bundle.Replay(data)
_ = entries // captured reads and tokens re-derived from bundle bytes
```

Run the fixture tests including verify/replay:

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

## Derivation Graph Demo

The derivation graph demo shows the full pipeline from raw CSV accounting data to a traced, verified operating income figure. Three committed CSV files (`revenue.csv`, `cogs.csv`, `opex.csv`) are minted row-by-row into leaf tokens, composed via `filter.sum_where`, `calc.sum`, and `calc.subtract`, and the final `operating_income` token is traced back to every source row with signed contribution amounts.

The demo uses seed 42 and produces a deterministic bundle and trace:

```text
operating_income = $350,000
  <- gross_margin ($550,000) - total_opex ($200,000)
    <- gross_revenue ($950,000) - total_cogs ($400,000)
      <- (net_revenue $1,000,000 - refunds $50,000)
```

Run the demo tests:

```bash
go test ./examples/derivation-graph/...
```

See [`examples/derivation-graph/README.md`](examples/derivation-graph/README.md) for the full walkthrough, dataset description, and golden regeneration procedure.

---

## Latest End-To-End Scenarios

Note: [Fauxbooks](https://github.com/humzakhan/fauxbooks) is a separate open source project used here to generate deterministic synthetic finance datasets. Provenant uses those generated exports as repeatable source material for end-to-end grounding tests; Fauxbooks is not part of the Provenant kernel.

The current end-to-end suite exercises three natural finance questions over deterministic Fauxbooks `monthly-close` datasets. The suite generates data, verifies the generated datasets, runs Claude Code headlessly with Fauxbooks and Provenant MCP enabled, verifies numeric correctness by recomputing from CSVs, proves Provenant grounding, runs negative controls, and writes one final report.

Run it:

```bash
hack/e2e/fauxbooks-user-scenarios.sh \
  --seed 424200 \
  --out /tmp/provenant-fauxbooks-user-scenarios
```

The latest checked-in report is [`./examples/fauxbooks-user-scenarios/report.md`](examples/fauxbooks-user-scenarios/report.md).

### Scenario Matrix

| Scenario | What the user asks | Verification |
|---|---|---|
| Quarterly revenue summary | "What was revenue for Q1 2026?" | Monthly revenue, Q1 total, and top revenue account are recomputed from `profit-loss.csv`. |
| Cash vs book revenue reconciliation | "Why does cash received not match booked revenue?" | Book revenue, customer cash receipts, delta, and payment cross-check are recomputed from CSVs. |
| Accounts receivable aging | "Which customers owe us money?" | Bucket totals, AR total, largest customer, and statement closing-balance cross-check are recomputed from CSVs. |

The latest run passed all scenario checks:

| Scenario | Key verified result |
|---|---:|
| Q1 revenue total | `$395,061.13` |
| March book revenue | `$147,136.26` |
| March customer cash receipts | `$74,002.66` |
| Book-to-cash delta | `$73,133.60` |
| March AR total | `$73,367.10` |
| Largest AR customer | `cust-005 McKinsey`, `$20,714.83` |

The README renders financial values in user-facing US dollar format. The verification artifacts keep canonical machine values with fixed decimal scale so exact comparisons remain deterministic.

For every scenario, the runner expects:

- `fbx verify` exits `0` for every generated dataset.
- Claude uses Fauxbooks MCP at least once.
- Claude mints every final numeric value through Provenant MCP.
- `provenant check-answer` exits `0`.
- The direct grounded Stop hook does not block.
- The negative Stop-hook control blocks an ungrounded `$999,999,999.00` claim.
- The final JSON matches both generated expected values and direct CSV recomputation.

---

## Solving The Finance File Scenario

A complete local run has three layers:

1. **Data and prompt setup.** The runner builds `provenant` and `fbx`, creates a disposable workspace, generates deterministic monthly-close exports, and writes one prompt per scenario.
2. **Agent execution.** Claude Code runs headlessly with strict MCP config. It can inspect Fauxbooks bundles/documents and must call Provenant to mint the numeric values that appear in the final JSON.
3. **Release verification.** The runner saves answers, stdout, stderr, debug logs, MCP logs, hook evidence, and verification JSON. It then runs `provenant check-answer`, direct grounded Stop-hook control, direct negative Stop-hook control, and CSV recomputation.

The resulting workspace looks like this:

```text
/tmp/provenant-fauxbooks-user-scenarios/
  bin/
    fbx
    provenant
  workspace/
    .claude/settings.json
    .mcp.json
    datasets/
    prompts/
    answers/
    evidence/
    verification/
    report.md
```

The important artifacts are:

| Artifact | Purpose |
|---|---|
| `prompts/<scenario>.md` | The exact user-facing prompt and Provenant minting instructions. |
| `answers/<scenario>.final.md` | The final machine-readable answer block. |
| `evidence/<scenario>.claude.out` | Raw Claude Code output. |
| `evidence/<scenario>.claude-debug.log` | Claude Code debug log with MCP activity. |
| `evidence/<scenario>.provenant-mcp.*.jsonl` | Provenant MCP request/response transcript. |
| `evidence/<scenario>.check-answer.out` | `provenant check-answer` verdict. |
| `evidence/<scenario>.grounded-stop.out` | Direct Stop-hook pass evidence. |
| `evidence/<scenario>.negative-stop.out` | Direct Stop-hook block evidence for the negative control. |
| `verification/<scenario>.json` | Scenario truth, grounding, and control status. |

This is the shape a real integration should preserve even if the host is not Claude Code. The host can be different, but the invariant is the same: released numeric claims must be grounded, checkable, and replayable.

---

## Current Limits

Provenant is intentionally conservative in v0.

- MCP grounding currently uses `extract.json` for the read/mint path.
- CSV and spreadsheet readers exist in the kernel and reconciliation fixture path, but arbitrary customer CSV reasoning is not a complete product surface.
- The Fauxbooks user-scenario runner mints final numeric values from generated expected JSON summaries while separately verifying those summaries against generated CSVs.
- Host-channel text in MCP mode is not Provenant-attested unless it goes through `provenant.submit_answer` or the passing check-answer/Stop-hook path.
- Claude Code gets project hooks and Stop-hook enforcement; other MCP hosts only get tool access plus `provenant.submit_answer` attestation.
- Provenant provides verifiable evidence and replay, not legal advice, regulatory advice, or external tamper-proofing against root compromise.

---

## Project Layout

```text
kernel/                  Pure Go enforcement kernel: readers, minting, audit chain, bundle.
access/cli/              CLI runner, MCP command, install path, check-answer, and tests.
access/mcpapi/           MCP tool contract for Provenant read/mint/bundle.
hack/hooks/              Claude Code SessionStart and Stop hook scripts.
hack/e2e/                End-to-end runners, including Fauxbooks user scenarios.
examples/fauxbooks-user-scenarios/
                         Synthetic finance-file e2e example, runner spec, and latest report.
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
- [`examples/fauxbooks-user-scenarios/README.md`](examples/fauxbooks-user-scenarios/README.md) — synthetic finance-file e2e example with grounded Claude Code answers
- [`examples/reconciliation/README.md`](examples/reconciliation/README.md) — full reconciliation fixture walkthrough, verification guidance, and explicit non-support list
- [`examples/derivation-graph/README.md`](examples/derivation-graph/README.md) — derivation graph end-to-end demo with operating income trace
