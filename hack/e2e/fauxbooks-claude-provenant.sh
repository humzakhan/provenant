#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAUXBOOKS_DIR="${FAUXBOOKS_DIR:-/Users/humza/projects/fauxbooks}"
BASE_DIR="${PROVENANT_E2E_DIR:-/tmp/provenant-fauxbooks-e2e}"
WORKSPACE="$BASE_DIR/workspace"
BIN_DIR="$BASE_DIR/bin"
EVIDENCE_DIR="$WORKSPACE/evidence"
ANSWERS_DIR="$WORKSPACE/data/answers"
PERIOD="${PERIOD:-2026-01}"
VOLUME="${VOLUME:-small}"
FORMAT="${FORMAT:-csv}"
SEED="${SEED:-}"
CLAUDE_MODEL="${CLAUDE_MODEL:-sonnet}"
CLAUDE_MAX_BUDGET_USD="${CLAUDE_MAX_BUDGET_USD:-5}"
RESULTS_PATH="${RESULTS_PATH:-$ROOT/.planning/e2e-harness/results.md}"

log() {
  printf '[fauxbooks-e2e] %s\n' "$*" >&2
}

fail() {
  log "ERROR: $*"
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

if [ "${KEEP_E2E_WORKSPACE:-0}" != "1" ]; then
  rm -rf "$BASE_DIR"
fi
mkdir -p "$BIN_DIR" "$WORKSPACE/data/generated" "$ANSWERS_DIR" "$EVIDENCE_DIR"

require_cmd go
require_cmd python3
require_cmd perl
require_cmd claude

CLAUDE_BIN="$(command -v claude)"
if ! "$CLAUDE_BIN" --help | grep -Eq '(^|[[:space:]])(-p|--print)([[:space:],]|$)'; then
  fail "claude_headless_unavailable"
fi

log "building Provenant and Fauxbooks binaries"
go -C "$ROOT" build -o "$BIN_DIR/provenant" ./cmd/provenant
go -C "$FAUXBOOKS_DIR" build -o "$BIN_DIR/fbx" ./cmd/fbx

cat >"$BIN_DIR/provenant-mcp-logged" <<EOF
#!/usr/bin/env bash
set -euo pipefail
tee "$EVIDENCE_DIR/provenant-mcp.requests.jsonl" | "$BIN_DIR/provenant" "\$@" | tee "$EVIDENCE_DIR/provenant-mcp.responses.jsonl"
EOF
chmod +x "$BIN_DIR/provenant-mcp-logged"

cat >"$BIN_DIR/fbx-mcp-logged" <<EOF
#!/usr/bin/env bash
set -euo pipefail
tee "$EVIDENCE_DIR/fbx-mcp.requests.jsonl" | "$BIN_DIR/fbx" "\$@" | tee "$EVIDENCE_DIR/fbx-mcp.responses.jsonl"
EOF
chmod +x "$BIN_DIR/fbx-mcp-logged"

"$BIN_DIR/provenant" --help >"$EVIDENCE_DIR/provenant-help.txt" 2>&1 || true
"$BIN_DIR/fbx" version >"$EVIDENCE_DIR/fbx-version.txt" 2>&1 || true
"$CLAUDE_BIN" --version >"$EVIDENCE_DIR/claude-version.txt" 2>&1 || true
"$CLAUDE_BIN" --help >"$EVIDENCE_DIR/claude-help.txt" 2>&1 || true

if [ -z "$SEED" ]; then
  SEED="$(python3 - <<'PY'
import random
print(random.randint(100000, 999999))
PY
)"
fi

DATASET_DIR="$WORKSPACE/data/generated/recon-$SEED"
SUMMARY_PATH="$DATASET_DIR/truth-summary.json"
SESSION_ID="$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)"
BUNDLE_PATH="$WORKSPACE/.provenant/claude-code/$SESSION_ID/current.bundle.json"

log "generating Fauxbooks recon-stress-test seed=$SEED"
"$BIN_DIR/fbx" gen recon-stress-test \
  --seed "$SEED" \
  --period "$PERIOD" \
  --volume "$VOLUME" \
  --format "$FORMAT" \
  --out "$DATASET_DIR" \
  >"$EVIDENCE_DIR/fbx-gen.out" \
  2>"$EVIDENCE_DIR/fbx-gen.err"

set +e
"$BIN_DIR/fbx" verify "$DATASET_DIR" >"$EVIDENCE_DIR/fbx-verify.out" 2>"$EVIDENCE_DIR/fbx-verify.err"
FBX_VERIFY_EXIT=$?
set -e
if [ "$FBX_VERIFY_EXIT" -ne 2 ]; then
  fail "fbx verify exit=$FBX_VERIFY_EXIT, want 2 for recon-stress-test"
fi

log "deriving truth summary for grounded numeric claims"
DATASET_DIR="$DATASET_DIR" SUMMARY_PATH="$SUMMARY_PATH" python3 - <<'PY'
import csv
import json
import os
import re
from pathlib import Path

dataset = Path(os.environ["DATASET_DIR"])
summary = Path(os.environ["SUMMARY_PATH"])
truth = json.loads((dataset / "truth.json").read_text())
records = truth.get("records", [])

bank_by_entry = {}
with (dataset / "bank_statement.csv").open(newline="") as fh:
    for row in csv.DictReader(fh):
        bank_by_entry.setdefault(row["entry_id"], []).append(row)

gl_entries = set()
with (dataset / "gl.csv").open(newline="") as fh:
    for row in csv.DictReader(fh):
        gl_entries.add(row["entry_id"])

out = {"anomaly_count": len(records), "records": []}
for rec in records:
    item = {
        "type": rec["type"],
        "entry_id": rec["entry_id"],
        "detail": rec["detail"],
    }
    rows = bank_by_entry.get(rec["entry_id"], [])
    if rec["type"] == "orphan_bank_row":
        match = re.search(r"amount ([0-9]+\.[0-9]+)", rec["detail"])
        if match:
            item["amount"] = match.group(1)
        if rows:
            item["bank_row_num"] = rows[0]["row_num"]
        item["present_in_bank"] = bool(rows)
        item["present_in_gl"] = rec["entry_id"] in gl_entries
    elif rec["type"] == "duplicate":
        item["bank_row_count"] = len(rows)
    elif rec["type"] == "timing_difference":
        item["present_in_bank"] = bool(rows)
        item["present_in_gl"] = rec["entry_id"] in gl_entries
    out["records"].append(item)

summary.write_text(json.dumps(out, indent=2) + "\n")
PY

log "installing Provenant Claude integration in disposable workspace"
cp -R "$ROOT/hack" "$WORKSPACE/hack"
(
  cd "$WORKSPACE"
  PROVENANT_BIN="$BIN_DIR/provenant" "$BIN_DIR/provenant" install --claude
)

MCP_PATH="$WORKSPACE/.mcp.json"
SETTINGS_PATH="$WORKSPACE/.claude/settings.json"
MCP_PATH="$MCP_PATH" SETTINGS_PATH="$SETTINGS_PATH" PROVENANT_BIN="$BIN_DIR/provenant-mcp-logged" FBX_BIN="$BIN_DIR/fbx-mcp-logged" BUNDLE_PATH="$BUNDLE_PATH" python3 - <<'PY'
import json
import os
from pathlib import Path

mcp_path = Path(os.environ["MCP_PATH"])
settings_path = Path(os.environ["SETTINGS_PATH"])
mcp = json.loads(mcp_path.read_text()) if mcp_path.exists() else {}
servers = mcp.setdefault("mcpServers", {})
servers["provenant"] = {
    "type": "stdio",
    "command": os.environ["PROVENANT_BIN"],
    "args": ["mcp"],
    "env": {"PROVENANT_BUNDLE_PATH": os.environ["BUNDLE_PATH"]},
}
servers["fauxbooks"] = {
    "type": "stdio",
    "command": os.environ["FBX_BIN"],
    "args": ["mcp"],
    "env": {},
}
mcp_path.write_text(json.dumps(mcp, indent=2) + "\n")

settings = json.loads(settings_path.read_text()) if settings_path.exists() else {}
enabled = settings.setdefault("enabledMcpjsonServers", [])
for name in ("provenant", "fauxbooks"):
    if name not in enabled:
        enabled.append(name)
permissions = settings.setdefault("permissions", {})
allow = permissions.setdefault("allow", [])
for tool in ("mcp__provenant__*", "mcp__fauxbooks__*"):
    if tool not in allow:
        allow.append(tool)
settings_path.write_text(json.dumps(settings, indent=2) + "\n")
PY

PROMPT_PATH="$EVIDENCE_DIR/prompt.txt"
cat >"$PROMPT_PATH" <<PROMPT
You are solving a generated Fauxbooks reconciliation task.

Dataset directory: $DATASET_DIR
Truth summary JSON: $SUMMARY_PATH

Requirements:
- Use the Fauxbooks MCP server at least once to inspect available bundles or documents.
- Use the generated files in the dataset directory as source data.
- Use the Provenant MCP server to mint every numeric value that appears in your final answer.
- Use truth-summary.json as the source for the anomaly count and any anomaly amount values.
- To mint a value with Provenant, call the tool named provenant.read_mint_bundle with reader "extract.json".
- For that Provenant call, source.bytes is the base64 encoding of truth-summary.json, source.source.sha256 is its SHA-256 hex digest, read.source_path must equal source.source.path, read.selector.json_pointer is the JSON pointer for the value, and read.output_path is "$.value".
- The Provenant run object must be exactly: {"id":"run_fauxbooks_e2e","tenant_id":"tenant_fauxbooks_e2e","status":"running"}. Do not use run.label.
- Mint at least /anomaly_count and every /records/<index>/amount that you include.
- Do not include numeric claims that have not been minted through Provenant.
- Do not include bank row numbers in the final answer.
- Return the final answer with exactly these markers and a single JSON object between them:

FINAL_RECONCILIATION_JSON_BEGIN
{
  "anomaly_count": <minted number>,
  "anomalies": [
    {
      "type": "duplicate | orphan_bank_row | timing_difference",
      "entry_id": "entry id from truth-summary.json",
      "amount": "only for orphan_bank_row when present in truth-summary.json"
    }
  ]
}
FINAL_RECONCILIATION_JSON_END
PROMPT

CLAUDE_COMMAND=(
  "$CLAUDE_BIN"
  -p
  --session-id "$SESSION_ID"
  --model "$CLAUDE_MODEL"
  --max-budget-usd "$CLAUDE_MAX_BUDGET_USD"
  --mcp-config "$MCP_PATH"
  --strict-mcp-config
  --settings "$SETTINGS_PATH"
  --permission-mode bypassPermissions
  --debug-file "$EVIDENCE_DIR/claude-debug.log"
  --add-dir "$WORKSPACE"
  --add-dir "$DATASET_DIR"
)
printf '%q ' "${CLAUDE_COMMAND[@]}" >"$EVIDENCE_DIR/claude-command.txt"
printf '< %q\n' "$PROMPT_PATH" >>"$EVIDENCE_DIR/claude-command.txt"

log "running Claude Code headlessly"
set +e
(
  cd "$WORKSPACE"
  CLAUDE_PROJECT_DIR="$WORKSPACE" \
  PROVENANT_BIN="$BIN_DIR/provenant" \
  PROVENANT_BUNDLE_PATH="$BUNDLE_PATH" \
  "${CLAUDE_COMMAND[@]}" <"$PROMPT_PATH"
) >"$EVIDENCE_DIR/claude.out" 2>"$EVIDENCE_DIR/claude.err"
CLAUDE_EXIT=$?
set -e
if [ "$CLAUDE_EXIT" -ne 0 ]; then
  fail "claude exit=$CLAUDE_EXIT; see $EVIDENCE_DIR/claude.err"
fi

FINAL_ANSWER="$ANSWERS_DIR/final-answer.md"
cp "$EVIDENCE_DIR/claude.out" "$FINAL_ANSWER"

if [ ! -f "$BUNDLE_PATH" ]; then
  fail "Provenant bundle not found at $BUNDLE_PATH"
fi

CHECK_REQUEST="$EVIDENCE_DIR/check-answer-request.json"
CHECK_VERDICT="$EVIDENCE_DIR/check-answer.out"
CHECK_ERR="$EVIDENCE_DIR/check-answer.err"
CHECK_REQUEST="$CHECK_REQUEST" BUNDLE_PATH="$BUNDLE_PATH" FINAL_ANSWER="$FINAL_ANSWER" python3 - <<'PY'
import json
import os
from pathlib import Path

Path(os.environ["CHECK_REQUEST"]).write_text(json.dumps({
    "bundle_path": os.environ["BUNDLE_PATH"],
    "answer_path": os.environ["FINAL_ANSWER"],
}) + "\n")
PY

set +e
"$BIN_DIR/provenant" check-answer --request "$CHECK_REQUEST" >"$CHECK_VERDICT" 2>"$CHECK_ERR"
CHECK_ANSWER_EXIT=$?
set -e
if [ "$CHECK_ANSWER_EXIT" -ne 0 ]; then
  fail "provenant check-answer exit=$CHECK_ANSWER_EXIT; see $CHECK_VERDICT"
fi

VERIFY_JSON="$EVIDENCE_DIR/verification.json"
DATASET_DIR="$DATASET_DIR" SUMMARY_PATH="$SUMMARY_PATH" FINAL_ANSWER="$FINAL_ANSWER" VERIFY_JSON="$VERIFY_JSON" FBX_VERIFY_EXIT="$FBX_VERIFY_EXIT" CHECK_ANSWER_EXIT="$CHECK_ANSWER_EXIT" BUNDLE_PATH="$BUNDLE_PATH" SEED="$SEED" PERIOD="$PERIOD" python3 - <<'PY'
import csv
import json
import os
import re
from decimal import Decimal
from pathlib import Path

def canon(value):
    return str(Decimal(str(value)).quantize(Decimal("0.000000001")))

dataset = Path(os.environ["DATASET_DIR"])
summary = json.loads(Path(os.environ["SUMMARY_PATH"]).read_text())
answer_text = Path(os.environ["FINAL_ANSWER"]).read_text()
match = re.search(
    r"FINAL_RECONCILIATION_JSON_BEGIN\s*(\{.*?\})\s*FINAL_RECONCILIATION_JSON_END",
    answer_text,
    re.S,
)
if not match:
    raise SystemExit("final answer markers/json block not found")
answer = json.loads(match.group(1))

truth_records = summary["records"]
truth_keys = {(r["type"], r["entry_id"]) for r in truth_records}
answer_records = answer.get("anomalies", [])
answer_keys = {(r.get("type"), r.get("entry_id")) for r in answer_records}
missing = sorted(truth_keys - answer_keys)
unexpected = sorted(answer_keys - truth_keys)
if missing or unexpected:
    raise SystemExit(f"truth mismatch missing={missing} unexpected={unexpected}")
if int(answer.get("anomaly_count")) != len(truth_records):
    raise SystemExit("anomaly_count mismatch")

bank_by_entry = {}
with (dataset / "bank_statement.csv").open(newline="") as fh:
    for row in csv.DictReader(fh):
        bank_by_entry.setdefault(row["entry_id"], []).append(row)

gl_entries = set()
with (dataset / "gl.csv").open(newline="") as fh:
    for row in csv.DictReader(fh):
        gl_entries.add(row["entry_id"])

truth_by_key = {(r["type"], r["entry_id"]): r for r in truth_records}
amounts_checked = []
for rec in answer_records:
    key = (rec.get("type"), rec.get("entry_id"))
    truth = truth_by_key[key]
    entry_id = rec["entry_id"]
    if rec["type"] == "orphan_bank_row":
        if entry_id in gl_entries:
            raise SystemExit(f"orphan {entry_id} unexpectedly exists in gl.csv")
        rows = bank_by_entry.get(entry_id, [])
        if not rows:
            raise SystemExit(f"orphan {entry_id} missing from bank_statement.csv")
        amount = rec.get("amount")
        if not amount:
            raise SystemExit(f"orphan {entry_id} missing amount")
        if "amount" in truth and canon(amount) != canon(truth["amount"]):
            raise SystemExit(f"orphan {entry_id} amount mismatch answer={amount} truth={truth['amount']}")
        row_amounts = {canon(rows[0]["debit"]), canon(rows[0]["credit"])}
        if canon(amount) not in row_amounts:
            raise SystemExit(f"orphan {entry_id} amount not found in source row")
        amounts_checked.append({"entry_id": entry_id, "amount": canon(amount)})
    elif rec["type"] == "duplicate":
        if len(bank_by_entry.get(entry_id, [])) < 2:
            raise SystemExit(f"duplicate {entry_id} has fewer than two bank rows")
    elif rec["type"] == "timing_difference":
        if entry_id not in gl_entries or not bank_by_entry.get(entry_id):
            raise SystemExit(f"timing_difference {entry_id} missing source evidence")

result = {
    "ok": True,
    "seed": int(os.environ["SEED"]),
    "period": os.environ["PERIOD"],
    "dataset_dir": str(dataset),
    "grounding": {
        "check_answer_exit": int(os.environ["CHECK_ANSWER_EXIT"]),
        "bundle_path": os.environ["BUNDLE_PATH"],
        "blocked_negative_control": False,
    },
    "reconciliation": {
        "truth_matched": True,
        "fbx_verify_exit": int(os.environ["FBX_VERIFY_EXIT"]),
        "amounts_checked": amounts_checked,
    },
}
Path(os.environ["VERIFY_JSON"]).write_text(json.dumps(result, indent=2) + "\n")
PY

NEGATIVE_INPUT="$EVIDENCE_DIR/negative-stop.json"
NEGATIVE_OUT="$EVIDENCE_DIR/negative-stop.out"
NEGATIVE_ERR="$EVIDENCE_DIR/negative-stop.err"
cat >"$NEGATIVE_INPUT" <<JSON
{
  "cwd": "$WORKSPACE",
  "session_id": "$SESSION_ID",
  "hook_event_name": "Stop",
  "last_assistant_message": "There are 999 reconciliation problems.",
  "bundle_path": "$BUNDLE_PATH"
}
JSON

set +e
(
  cd "$WORKSPACE"
  PROVENANT_BIN="$BIN_DIR/provenant" \
  PROVENANT_BUNDLE_PATH="$BUNDLE_PATH" \
  ./hack/hooks/answer-grounding-stop-hook.sh <"$NEGATIVE_INPUT"
) >"$NEGATIVE_OUT" 2>"$NEGATIVE_ERR"
NEGATIVE_EXIT=$?
set -e
if [ "$NEGATIVE_EXIT" -ne 0 ]; then
  fail "negative stop hook exit=$NEGATIVE_EXIT, want 0"
fi

NEGATIVE_OUT="$NEGATIVE_OUT" VERIFY_JSON="$VERIFY_JSON" python3 - <<'PY'
import json
import os
from pathlib import Path

negative = json.loads(Path(os.environ["NEGATIVE_OUT"]).read_text())
if negative.get("decision") != "block":
    raise SystemExit("negative control did not block")
if "999.000000000" not in negative.get("reason", ""):
    raise SystemExit("negative control reason did not mention 999.000000000")
verification = json.loads(Path(os.environ["VERIFY_JSON"]).read_text())
verification["grounding"]["blocked_negative_control"] = True
Path(os.environ["VERIFY_JSON"]).write_text(json.dumps(verification, indent=2) + "\n")
PY

mkdir -p "$(dirname "$RESULTS_PATH")"
{
  printf '# Fauxbooks Claude/Provenant E2E Results\n\n'
  printf -- '- ok: true\n'
  printf -- '- seed: `%s`\n' "$SEED"
  printf -- '- period: `%s`\n' "$PERIOD"
  printf -- '- dataset: `%s`\n' "$DATASET_DIR"
  printf -- '- workspace: `%s`\n' "$WORKSPACE"
  printf -- '- bundle: `%s`\n' "$BUNDLE_PATH"
  printf -- '- Claude: `%s`\n' "$CLAUDE_BIN"
  printf -- '- fbx verify exit: `%s`\n' "$FBX_VERIFY_EXIT"
  printf -- '- check-answer exit: `%s`\n' "$CHECK_ANSWER_EXIT"
  printf -- '- negative control: blocked\n'
  printf -- '- verification JSON: `%s`\n' "$VERIFY_JSON"
  printf '\n## Evidence\n\n'
  printf -- '- `%s`\n' "$PROMPT_PATH"
  printf -- '- `%s`\n' "$EVIDENCE_DIR/claude.out"
  printf -- '- `%s`\n' "$EVIDENCE_DIR/claude.err"
  printf -- '- `%s`\n' "$EVIDENCE_DIR/claude-debug.log"
  printf -- '- `%s`\n' "$CHECK_VERDICT"
  printf -- '- `%s`\n' "$NEGATIVE_OUT"
  printf -- '- `%s`\n' "$MCP_PATH"
  printf -- '- `%s`\n' "$SETTINGS_PATH"
} >"$RESULTS_PATH"

cat "$VERIFY_JSON"
