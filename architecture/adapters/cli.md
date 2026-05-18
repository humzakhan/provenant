# Adapter: CLI Harness

Status: **v0**

> The `provenant` CLI is the full-enforcement deployment surface. It embeds the kernel and harness, owns the agent loop end-to-end (Mode A), and is the recommended path for production-critical workflows where MCP's host-driven loop introduces too much variance.

## Depends on
- [`subsystems/harness.md`](../subsystems/harness.md) — embedded harness (Mode A)
- All other subsystems

## Depended on by
- Operators running `provenant run`, `provenant replay`, `provenant audit pack`
- CI pipelines invoking `provenant eval`

## Why a CLI matters

MCP is great for adoption; CLI is great for control. The CLI is the deployment shape for: scheduled batch jobs (nightly recon), CI/CD-integrated eval gates, air-gapped on-prem deployments, and any workflow where the customer needs guaranteed loop bounds, prompt control, and the full kernel header in every run.

## Subcommands

```
provenant run <skill> [flags]                  execute a run
provenant replay <case_id|run_id> [flags]      replay a captured case or run
provenant cases [list|show|export] [flags]     case bank operations
provenant audit [show|verify|pack] <run_id>    audit operations
provenant skill [install|list|show|publish]    skill pack operations
provenant tools [list|show]                    tool registry inspection
provenant eval [--skill <id>] [--against <ref>] run regression suite
provenant token [issue|revoke|list]            tenant token management
provenant version                              version info
```

### `provenant run`

```bash
$ provenant run recon \
    --input ./inputs/may-2026-recon.json \
    --model claude-opus-4-7 \
    --tenant acme \
    --output ./outputs/run-may-2026.json

Loading skill: recon@0.3.0 (verified)
Initializing run run_01HXYZ... for tenant acme...
Step 1/30: extract.bank_statement
  Confidence: 0.92 (above threshold)
  Tokens minted: 47
Step 2/30: ledger.get_transactions
  Returned 248 rows for account 1010, period 2026-05
  Tokens minted: 248
Step 3/30: reconcile.match
  Matched: 233 pairs
  Unmatched: 15 (bank) + 12 (ledger) = 27 residuals
Step 4/30: calc.subtract (× 27)
  Variance tokens minted: 27
Step 5/30: submit_answer
  Validators: schema.output ✓, arith.crossfoot ✓, arith.tie ✓, arith.sign ✓, confidence.extraction_min ✓
  Result: completed, awaiting_review (12 residuals below confidence threshold)
Run complete in 47.2s, $0.31 cost.
Review URL: http://localhost:8080/reviews/rev_01HXYZ...
```

Flags:

- `--input`: JSON file conforming to the skill's input schema.
- `--model`: override the skill's default model.
- `--tenant`: required for multi-tenant deployments.
- `--output`: writes the final answer to disk; optional.
- `--temperature`: override (defaults to skill recommendation; refuses non-zero on compliance-marked skills).
- `--max-steps`, `--max-wall-clock`, `--max-cost`: override skill bounds (capped at skill maximums).
- `--bon-mav`: explicit on/off override of the skill's BoN-MAV configuration.
- `--dry-run`: load the skill, validate the input, exit before any model call.

### `provenant replay`

Replay a captured case or a prior run:

```bash
$ provenant replay case_01HXYZ --n 20 --skill recon@0.4.0-rc.1
Replaying case_01HXYZ 20 times against recon@0.4.0-rc.1...
  20/20 trajectories match baseline (1.000)
  20/20 decisions match baseline (1.000)
  Faithfulness: 1.000
  Accuracy: 1.000 (vs ground truth from human correction)
PASS
```

Replay uses captured reads for `CapturedDeterministic` tools — see [`subsystems/tools.md`](../subsystems/tools.md). External state is reproduced bit-identical; only the LLM-portion variance is measured.

### `provenant cases`

```bash
$ provenant cases list --skill recon --status active
247 active cases for recon:
  case_01HXYZ  2026-05-16  bank-fees                edited (1 field)
  case_01HABC  2026-05-15  multi-currency           edited (3 fields)
  ...

$ provenant cases show case_01HXYZ
[full case JSON]

$ provenant cases export --skill recon --output recon-cases.zip
Exported 247 cases (12 MB).
```

### `provenant audit`

```bash
$ provenant audit show run_01HXYZ
Run run_01HXYZ (tenant acme, recon@0.3.0)
  Started: 2026-05-16T14:30:00Z
  Ended:   2026-05-16T14:30:47Z
  Status:  completed (awaiting_review)
  Audit entries: 247 (chain verified ✓)
  Tool calls: 12
  Tokens minted: 322
  Guard runs: 14 (all passed)
  Cost: $0.31

$ provenant audit verify --tenant acme
Verifying audit chain for tenant acme...
  4,827 entries from genesis to current head
  Chain hash: 8a2f...
  Result: VERIFIED

$ provenant audit pack run_01HXYZ --output ./pack-may-2026.zip
[as documented in subsystems/audit.md]
```

### `provenant skill`

```bash
$ provenant skill install ./packs/recon
Validating pack...
  manifest.yaml ✓
  tools.yaml: 7 tools all registered at compatible versions ✓
  guards.yaml: 7 guards all registered ✓
  prompts/: 1 system, 1 skill, 14 examples ✓
  eval/: 78 cases ✓
Pack hash: a3f9...
Installed as recon@0.3.0.

$ provenant skill list
recon@0.3.0           bank ↔ GL reconciliation
ap_coding@0.2.0       AP invoice coding
expense_review@0.1.0  expense compliance review

$ provenant skill show recon@0.3.0
[full manifest + tools + guards + thresholds]
```

### `provenant tools`

```bash
$ provenant tools list
Built-in tools:
  calc.subtract                 v1.2.0  PureDeterministic
  calc.sum                      v1.2.0  PureDeterministic
  ...

Registered backends:
  ledger.get_balance            v1.0.0  CapturedDeterministic  (sql backend)
  extract.bank_statement        v1.1.0  NonDeterministic       (textract backend)
  policy.lookup                 v1.0.0  CapturedDeterministic  (keyword backend)
  external.fx_rate              v1.0.0  CapturedDeterministic  (openfx backend)

$ provenant tools show extract.bank_statement
[input schema, output schema, mintable paths, determinism class, latency budget]
```

### `provenant eval`

```bash
$ provenant eval --skill recon --against HEAD
[as documented in subsystems/eval.md]
```

CI-friendly: exit code 0 on pass, non-zero on regression. Output also available as JSON via `--format json`.

### `provenant token`

```bash
$ provenant token issue --tenant acme --skills recon,ap_coding --expires 2027-01-01
prv_acme_8f4d...
(save this token; it will not be shown again)

$ provenant token list --tenant acme
prv_acme_8f4d...   skills: recon, ap_coding   expires: 2027-01-01
prv_acme_3a91...   skills: *                  expires: 2026-12-01 (rotation pending)

$ provenant token revoke prv_acme_3a91...
Revoked. Token is no longer valid.
```

## Configuration

CLI reads from:

1. CLI flags (highest precedence).
2. Environment variables (`PROVENANT_*`).
3. `~/.provenant/config.yaml` (per-user).
4. `/etc/provenant/config.yaml` (system).

Multi-tenant deployments typically use a system config with the persistent-binding choice and tenant defaults; per-user config holds personal tokens.

## Embedded mode vs server-attached mode

The CLI has two operating modes:

- **Embedded** (default): the CLI process owns the kernel, runs the harness in-process against `kernel/public/memio/`, and emits bundle bytes. Used for local development, CI, batch jobs.
- **Server-attached**: the CLI talks to a running Provenant server over HTTP (`--server=http://...`). Used for production deployments where the kernel runs on a server tier and the CLI is a thin client for ops; SQLite is the default persistent binding when HITL pause/resume is needed, Postgres is optional for multi-process/multi-tenant deployments.

Embedded mode is the default. Server-attached requires `--server` and a token.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic failure |
| 2 | Configuration error |
| 3 | Skill load failure |
| 10 | Run failed with structured errors (see stderr) |
| 11 | Run exhausted (max_steps / max_wall_clock / max_cost) |
| 12 | Run aborted by agent |
| 20 | Eval regression below threshold |
| 30 | Audit chain verification failure |

Stable across versions; CI integrations rely on them.

## Tests

- Integration: end-to-end `run` against fixture inputs produces expected audit pack contents.
- Replay: cases captured by `run` replay deterministically via `replay`.
- CI: `eval` correctly blocks a deliberately regressed skill version.
- Cross-platform: macOS, Linux, Windows binaries produce byte-identical audit packs given identical inputs.

## Open questions

1. **Interactive mode.** Should `provenant run` support an interactive REPL where the operator approves each tool call? Useful for skill development; defer to v0.5.
2. **Plugin discovery.** When tools are loaded via plugin (Go plugins or out-of-process), should the CLI auto-discover them on PATH? Proposal: explicit registration only; no PATH magic.

## References

- [DFAH](../references.md#dfah) — the CLI is the surface where bit-identical replay is enforceable
- See [`subsystems/audit.md`](../subsystems/audit.md) for `audit pack` contents
- See [`subsystems/eval.md`](../subsystems/eval.md) for `eval` semantics
