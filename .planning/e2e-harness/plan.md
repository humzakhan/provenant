# Fauxbooks Claude/Provenant E2E Harness Plan

## Objective

Build an end-to-end harness on this machine that proves a Claude Code session can solve a Fauxbooks reconciliation task while every numeric answer released by Claude is grounded through Provenant.

The harness uses:

- Provenant repo under test: `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/hyper-deposit`
- Fauxbooks source/tooling: `/Users/humza/projects/fauxbooks`
- Fauxbooks MCP server: `fbx mcp`
- Provenant MCP server and hooks: `provenant mcp`, `provenant install --claude`, `answer-grounding-stop-hook.sh`
- Claude Code headless session, with a transcript and hook output captured as test evidence

Treat `MCV` in the request as `MCP`.

## Success Criteria

The harness passes only when all of these are true:

1. Provenant is started as a real MCP server and its Claude Stop hook is installed in the test workspace.
2. A Claude Code session is launched against a temporary workspace configured with both Provenant and Fauxbooks MCP servers.
3. Fauxbooks generates randomized reconciliation input files during the run.
4. Claude is prompted to solve the reconciliation task from those files.
5. Claude's final answer is released only after the Provenant Stop hook checks it.
6. The released answer's numeric claims match tokens minted through Provenant from the Fauxbooks output files.
7. Independent verification compares Claude's answer to Fauxbooks ground truth or deterministic local calculations and fails the harness on mismatch.
8. A negative control proves an ungrounded numeric answer is blocked.

## Branch And Artifact Plan

Use a dedicated branch:

```sh
git fetch origin main --prune
git switch -c test/fauxbooks-claude-provenant-e2e origin/main
```

Add these artifacts:

```text
.planning/e2e-harness/plan.md
.planning/e2e-harness/runbook.md
.planning/e2e-harness/results.md
```

Implementation can later add a script, for example:

```text
hack/e2e/fauxbooks-claude-provenant.sh
```

Keep the first PR as a planning/runbook PR unless the implementation is also requested.

## Test Workspace Layout

Create a disposable workspace per run:

```text
/tmp/provenant-fauxbooks-e2e/
  bin/
    provenant
    fbx
  workspace/
    .claude/settings.json
    .mcp.json
    hack/hooks/
    data/
      source/
      generated/
      answers/
    evidence/
      claude-transcript.jsonl
      provenant-hook.log
      fbx-mcp.log
      provenant-mcp.log
      verification.json
```

The harness must delete and recreate this directory each run unless a `KEEP_E2E_WORKSPACE=1` flag is set.

## Build Tools

Build local binaries from the checked-out sources:

```sh
go build -o /tmp/provenant-fauxbooks-e2e/bin/provenant ./cmd/provenant
go -C /Users/humza/projects/fauxbooks build -o /tmp/provenant-fauxbooks-e2e/bin/fbx ./cmd/fbx
```

Record versions:

```sh
/tmp/provenant-fauxbooks-e2e/bin/provenant --help
/tmp/provenant-fauxbooks-e2e/bin/fbx version
```

## MCP And Hook Setup

Copy Provenant hooks into the temporary workspace and install Claude integration:

```sh
cp -R hack /tmp/provenant-fauxbooks-e2e/workspace/hack
cd /tmp/provenant-fauxbooks-e2e/workspace
PROVENANT_BIN=/tmp/provenant-fauxbooks-e2e/bin/provenant \
  /tmp/provenant-fauxbooks-e2e/bin/provenant install --claude
```

Then merge the Fauxbooks MCP server into `.mcp.json`:

```json
{
  "mcpServers": {
    "provenant": {
      "type": "stdio",
      "command": "${PROVENANT_BIN:-provenant}",
      "args": ["mcp"],
      "env": {
        "PROVENANT_BUNDLE_PATH": "${PROVENANT_BUNDLE_PATH:-}"
      }
    },
    "fauxbooks": {
      "type": "stdio",
      "command": "/tmp/provenant-fauxbooks-e2e/bin/fbx",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

Update `.claude/settings.json` to allow both MCP servers:

```json
{
  "enabledMcpjsonServers": ["provenant", "fauxbooks"],
  "permissions": {
    "allow": ["mcp__provenant__*", "mcp__fauxbooks__*"]
  }
}
```

Do not hand-edit the Provenant hook entries produced by `provenant install --claude`.

## Dataset Generation

Use Fauxbooks to generate random but reproducible reconciliation problems. The harness chooses a seed at runtime and records it.

Recommended first scenario:

```sh
SEED="$(jot -r 1 100000 999999 2>/dev/null || shuf -i 100000-999999 -n 1)"
PERIOD="2026-01"
OUT="/tmp/provenant-fauxbooks-e2e/workspace/data/generated/recon-$SEED"

/tmp/provenant-fauxbooks-e2e/bin/fbx gen recon-stress-test \
  --seed "$SEED" \
  --period "$PERIOD" \
  --volume small \
  --format csv \
  --out "$OUT"
```

Also run:

```sh
/tmp/provenant-fauxbooks-e2e/bin/fbx verify "$OUT"
```

For `recon-stress-test`, exit code `2` is expected and must be captured because injected anomalies deliberately violate invariant 3. `truth.json` is the ground truth for the expected anomalies.

## Claude Code Session Design

Launch Claude Code in the temporary workspace after MCP and hooks are configured. The exact command should be discovered at runtime from the installed CLI:

```sh
command -v claude
claude --help
```

Required launch fallback order:

1. If `claude --help` shows `--print` or `-p`, run Claude in print/headless mode with stdin prompt capture.
2. Else if `claude --help` shows a non-interactive prompt flag with another name, use that and record the detected flag.
3. Else fail the harness before any model work with `claude_headless_unavailable`.

The harness must record:

- the exact `claude` binary path from `command -v claude`
- `claude --version` output when available
- the exact launch command
- the full prompt written to Claude
- stdout/stderr paths
- transcript path if Claude reports one
- `.claude/settings.json` and `.mcp.json` as used by that run

The initial implementation should use this command shape when supported:

```sh
cd /tmp/provenant-fauxbooks-e2e/workspace
CLAUDE_PROJECT_DIR="$PWD" \
PROVENANT_BIN=/tmp/provenant-fauxbooks-e2e/bin/provenant \
claude -p < /tmp/provenant-fauxbooks-e2e/workspace/evidence/prompt.txt \
  > /tmp/provenant-fauxbooks-e2e/workspace/evidence/claude.out \
  2> /tmp/provenant-fauxbooks-e2e/workspace/evidence/claude.err
```

If Claude writes a transcript elsewhere, copy it into `evidence/claude-transcript.jsonl`. If no transcript is available in print mode, `claude.out`, `claude.err`, hook logs, MCP request/response logs, and the final answer file become the required transcript surrogate.

The prompt should require Claude to:

1. Use Fauxbooks MCP to inspect available tools.
2. Use the generated files under `data/generated/recon-$SEED`.
3. Use Provenant MCP to read/mint every numeric value it includes in the final answer.
4. Identify the reconciliation anomalies.
5. Produce a final answer with:
   - anomaly count
   - anomaly types
   - affected entry or row identifiers
   - exact amounts when relevant
   - short explanation of how each value was grounded

Prompt skeleton:

```text
You are solving a reconciliation task. Use the Fauxbooks MCP server for dataset context and Provenant MCP for every numeric value that will appear in your final answer.

Dataset directory: /tmp/provenant-fauxbooks-e2e/workspace/data/generated/recon-$SEED

Rules:
- Do not compute or copy numeric values into the final answer unless they have been read/minted through Provenant.
- Use Fauxbooks truth data only as source material; final numeric claims must be grounded by Provenant.
- Return a concise reconciliation report listing anomaly count, anomaly types, affected ids, and amounts.
```

## Provenant Pass-Through Verification

After Claude exits, the harness must verify both content correctness and Provenant grounding.

Grounding evidence:

- Claude Stop hook exit code is `0`.
- Grounded answer produces no block JSON.
- Session bundle exists at `.provenant/claude-code/<session_id>/current.bundle.json`.
- `provenant check-answer` run manually against the final answer and bundle exits `0`.
- Bundle replay/verification succeeds through existing Provenant bundle path.

Content correctness:

- Parse `truth.json`.
- Extract Claude's final answer into `data/answers/final-answer.md`.
- Parse a required machine-readable block from the final answer:

  ```json
  {
    "anomalies": [
      {
        "type": "duplicate_bank_row",
        "id": "bank_row_...",
        "amount": "123.000000000",
        "source_file": "bank_statement.csv",
        "source_row_id": "..."
      }
    ]
  }
  ```

- Compare anomaly count to `len(truth.anomalies)` or the equivalent top-level truth array/object present in the generated `truth.json`.
- Compare each `(type, id/source_row_id)` pair to `truth.json`.
- For every numeric `amount` in the machine-readable answer block, locate the referenced `source_file` and `source_row_id`, read that row from the generated Fauxbooks CSV, and compare the decimal string exactly after canonical 9-decimal normalization.
- Run `provenant check-answer` against `final-answer.md` and the session bundle; require exit `0`.
- Reject the run if `provenant check-answer` passes but any answer amount does not match its referenced Fauxbooks row or truth entry.
- Run `fbx verify "$OUT"` and confirm expected exit `2` for stress bundle.

The harness should produce:

```json
{
  "ok": true,
  "seed": 123456,
  "period": "2026-01",
  "dataset_dir": "...",
  "grounding": {
    "check_answer_exit": 0,
    "bundle_path": "...",
    "blocked_negative_control": true
  },
  "reconciliation": {
    "truth_matched": true,
    "fbx_verify_exit": 2
  }
}
```

## Negative Control

Run a direct Claude-shaped Stop hook invocation after the main Claude run. This keeps the control deterministic while still exercising the installed hook command.

Input file:

```sh
cat > /tmp/provenant-fauxbooks-e2e/workspace/evidence/negative-stop.json <<JSON
{
  "cwd": "/tmp/provenant-fauxbooks-e2e/workspace",
  "session_id": "$SESSION_ID",
  "hook_event_name": "Stop",
  "last_assistant_message": "There are 999 reconciliation problems."
}
JSON
```

Command:

```sh
cd /tmp/provenant-fauxbooks-e2e/workspace
PROVENANT_BIN=/tmp/provenant-fauxbooks-e2e/bin/provenant \
  ./hack/hooks/answer-grounding-stop-hook.sh \
  < evidence/negative-stop.json \
  > evidence/negative-stop.out
NEGATIVE_EXIT=$?
```

Expected exit code:

```text
0
```

Expected output shape:

```json
{
  "decision": "block",
  "reason": "ungrounded value=999.000000000 ..."
}
```

Assertions:

- `NEGATIVE_EXIT == 0`
- `evidence/negative-stop.out` is valid JSON
- `.decision == "block"`
- `.reason` contains `999.000000000`

This proves the Stop hook is active and fail-closed for ungrounded numeric claims.

## Evidence To Record

The final results artifact must include:

- exact Provenant and Fauxbooks commits or binary build paths
- exact seed, period, bundle, volume, and output directory
- exact `.mcp.json` and `.claude/settings.json` excerpts
- Claude launch command
- Claude prompt
- transcript excerpt showing MCP tool usage
- final Claude answer
- Provenant Stop hook output and exit status
- manual `provenant check-answer` output and exit status
- `fbx verify` output and exit status
- parsed comparison against `truth.json`
- negative-control block output

## Risks And Open Questions

- Claude Code headless CLI flags may differ across local installs; the harness must discover and record the actual command.
- A live Claude Code UI/session may add timing and transcript differences not visible in direct hook tests.
- Fauxbooks `recon-stress-test` is intentionally invariant-violating; `fbx verify` exit `2` is expected and must not be treated as harness failure.
- Provenant currently grounds numeric claims by exact canonical value. If two source values share the same amount, the harness should record enough row/id context to avoid falsely accepting the wrong source.
- The first implementation should use CSV outputs only. XLSX/PDF can be added after the CSV harness is stable.

## Ready For Implementation Checklist

- [ ] Confirm `claude` CLI exists on this machine.
- [ ] Confirm Claude Code can run non-interactively with project `.mcp.json`.
- [ ] Build local `provenant` and `fbx` binaries.
- [ ] Generate a `recon-stress-test` dataset with a recorded random seed.
- [ ] Run Claude against the generated dataset.
- [ ] Verify final answer against `truth.json`.
- [ ] Verify final answer with `provenant check-answer`.
- [ ] Run the negative control and confirm block JSON.
- [ ] Save results under `.planning/e2e-harness/results.md`.
