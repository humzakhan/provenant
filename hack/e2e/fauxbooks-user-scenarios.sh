#!/usr/bin/env bash
set -uo pipefail

PROVENANT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PROVENANT_ROOT

exec python3 - "$@" <<'PY'
import argparse
import base64
import csv
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone
from decimal import Decimal, ROUND_HALF_EVEN
from pathlib import Path

ROOT = Path(os.environ["PROVENANT_ROOT"])
DEFAULT_OUT = Path("/tmp/provenant-fauxbooks-user-scenarios")
RESULTS_PATH = Path(os.environ.get("RESULTS_PATH", ROOT / ".planning/e2e-user-scenarios/report.md"))
FAUXBOOKS_DIR = Path(os.environ.get("FAUXBOOKS_DIR", "/Users/humza/projects/fauxbooks"))
FBX_BIN_OVERRIDE = os.environ.get("FBX_BIN", "")
CLAUDE_MODEL = os.environ.get("CLAUDE_MODEL", "sonnet")
CLAUDE_MAX_BUDGET_USD = os.environ.get("CLAUDE_MAX_BUDGET_USD", "10")
KEEP_E2E_WORKSPACE = os.environ.get("KEEP_E2E_WORKSPACE", "0") == "1"
FINAL_BEGIN = "FINAL_USER_SCENARIO_JSON_BEGIN"
FINAL_END = "FINAL_USER_SCENARIO_JSON_END"


@dataclass(frozen=True)
class Scenario:
    scenario_id: str
    title: str
    prompt: str
    periods: tuple[str, ...]
    seeds: tuple[int, ...]
    source_files: tuple[str, ...]
    negative: str


SCENARIOS = (
    Scenario(
        "quarterly_revenue_summary",
        "Quarterly revenue summary",
        "I uploaded the January, February, and March 2026 finance exports. What was revenue for Q1 2026? Give me the monthly revenue totals, the quarter total, and the biggest revenue account driving each month.",
        ("2026-01", "2026-02", "2026-03"),
        (424201, 424202, 424203),
        ("profit-loss.csv",),
        "The Q1 revenue was 999999999.000000000.",
    ),
    Scenario(
        "cash_vs_book_revenue_reconciliation",
        "Cash vs book revenue reconciliation",
        "For March 2026, why does cash received not match booked revenue? Show booked revenue, customer cash receipts, the difference, and the main explanation categories.",
        ("2026-03",),
        (424301,),
        ("profit-loss.csv", "customer_payments.csv", "customer-statements.csv"),
        "Customer cash receipts were 999999999.000000000.",
    ),
    Scenario(
        "accounts_receivable_aging",
        "Accounts receivable aging",
        "Which customers owe us money at the end of March 2026? Break receivables into aging buckets and call out the largest customer balance.",
        ("2026-03",),
        (424401,),
        ("ar-aging.csv", "customer-statements.csv", "invoices.csv", "customer_payments.csv"),
        "Total receivables were 999999999.000000000.",
    ),
)


def log(message: str) -> None:
    print(f"[fauxbooks-user-scenarios] {message}", file=sys.stderr)


def canon(value) -> str:
    return str(Decimal(str(value)).quantize(Decimal("0.000000001"), rounding=ROUND_HALF_EVEN))


def read_csv(path: Path) -> list[dict[str, str]]:
    with path.open(newline="") as fh:
        return list(csv.DictReader(fh))


def run_cmd(cmd, *, cwd=None, env=None, stdout=None, stderr=None, stdin=None) -> subprocess.CompletedProcess:
    out_fh = open(stdout, "w") if stdout else None
    err_fh = open(stderr, "w") if stderr else None
    in_fh = open(stdin) if stdin else None
    try:
        return subprocess.run(
            [str(part) for part in cmd],
            cwd=str(cwd) if cwd else None,
            env=env,
            stdin=in_fh,
            stdout=out_fh if out_fh else subprocess.PIPE,
            stderr=err_fh if err_fh else subprocess.PIPE,
            text=True,
            check=False,
        )
    finally:
        for fh in (out_fh, err_fh, in_fh):
            if fh:
                fh.close()


def require_cmd(name: str) -> str:
    found = shutil.which(name)
    if not found:
        raise RuntimeError(f"required command not found: {name}")
    return found


def capture(cmd, *, cwd=None) -> str:
    proc = run_cmd(cmd, cwd=cwd)
    return ((proc.stdout or "") + (proc.stderr or "")).strip()


def write_json(path: Path, obj) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, indent=2, sort_keys=True) + "\n")


def extract_final_json(answer_path: Path) -> dict:
    text = answer_path.read_text()
    match = re.search(rf"{FINAL_BEGIN}\s*(\{{.*?\}})\s*{FINAL_END}", text, re.S)
    if not match:
        raise ValueError("final answer markers/json block not found")
    return json.loads(match.group(1))


def dataset_dir(workspace: Path, scenario: Scenario, period: str) -> Path:
    if scenario.scenario_id == "quarterly_revenue_summary":
        return workspace / "datasets" / scenario.scenario_id / period
    return workspace / "datasets" / scenario.scenario_id / period


def expected_for_quarterly(workspace: Path, scenario: Scenario) -> tuple[dict, dict]:
    months = []
    quarter_total = Decimal("0")
    report_rows = []
    for period in scenario.periods:
        rows = read_csv(dataset_dir(workspace, scenario, period) / "profit-loss.csv")
        total_row = next(row for row in rows if row["section"] == "total_revenue")
        revenue_rows = [row for row in rows if row["section"] == "revenue"]
        top = max(revenue_rows, key=lambda row: Decimal(row["amount"]))
        total = Decimal(total_row["amount"])
        quarter_total += total
        months.append({
            "period": period,
            "total_revenue": canon(total),
            "top_account_code": top["account_code"],
            "top_account_amount": canon(top["amount"]),
        })
        report_rows.append({
            "period": period,
            "total_revenue": canon(total),
            "top_account": f"{top['account_code']} {top['account_name']}",
            "top_account_amount": canon(top["amount"]),
        })
    expected = {
        "scenario_id": scenario.scenario_id,
        "quarter": "2026-Q1",
        "quarter_total_revenue": canon(quarter_total),
        "months": months,
    }
    report = {"rows": report_rows, "quarter_total_revenue": canon(quarter_total)}
    return expected, report


def expected_for_cash(workspace: Path, scenario: Scenario) -> tuple[dict, dict]:
    ds = dataset_dir(workspace, scenario, "2026-03")
    pl_rows = read_csv(ds / "profit-loss.csv")
    book = Decimal(next(row for row in pl_rows if row["section"] == "total_revenue")["amount"])
    payments = sum((Decimal(row["amount"]) for row in read_csv(ds / "customer_payments.csv")), Decimal("0"))
    statement_payments = sum((Decimal(row["payments"]) for row in read_csv(ds / "customer-statements.csv")), Decimal("0"))
    expected = {
        "scenario_id": scenario.scenario_id,
        "period": "2026-03",
        "book_revenue": canon(book),
        "customer_cash_receipts": canon(payments),
        "book_to_cash_delta": canon(book - payments),
        "explanations": ["uncollected invoices", "timing between invoicing and payment"],
    }
    report = {"customer_statement_payment_total": canon(statement_payments)}
    return expected, report


def expected_for_ar(workspace: Path, scenario: Scenario) -> tuple[dict, dict]:
    ds = dataset_dir(workspace, scenario, "2026-03")
    rows = read_csv(ds / "ar-aging.csv")
    buckets = ("bucket_0_30", "bucket_31_60", "bucket_61_90", "bucket_91_plus", "total")
    totals = {bucket: sum((Decimal(row[bucket]) for row in rows), Decimal("0")) for bucket in buckets}
    largest = max(rows, key=lambda row: Decimal(row["total"]))
    statement_total = sum((Decimal(row["closing_balance"]) for row in read_csv(ds / "customer-statements.csv")), Decimal("0"))
    expected = {
        "scenario_id": scenario.scenario_id,
        "period": "2026-03",
        "bucket_totals": {bucket: canon(value) for bucket, value in totals.items()},
        "largest_customer": {
            "party_id": largest["party_id"],
            "party_name": largest["party_name"],
            "total": canon(largest["total"]),
        },
    }
    report = {"customer_statement_closing_balance": canon(statement_total)}
    return expected, report


def compute_expected(workspace: Path, scenario: Scenario) -> tuple[dict, dict]:
    if scenario.scenario_id == "quarterly_revenue_summary":
        return expected_for_quarterly(workspace, scenario)
    if scenario.scenario_id == "cash_vs_book_revenue_reconciliation":
        return expected_for_cash(workspace, scenario)
    if scenario.scenario_id == "accounts_receivable_aging":
        return expected_for_ar(workspace, scenario)
    raise ValueError(scenario.scenario_id)


def assert_exact(actual, expected, path="$") -> None:
    if type(actual) is not type(expected):
        raise ValueError(f"{path}: type mismatch got {type(actual).__name__}, want {type(expected).__name__}")
    if isinstance(expected, dict):
        extra = sorted(set(actual) - set(expected))
        missing = sorted(set(expected) - set(actual))
        if extra or missing:
            raise ValueError(f"{path}: field mismatch missing={missing} extra={extra}")
        for key in expected:
            assert_exact(actual[key], expected[key], f"{path}.{key}")
        return
    if isinstance(expected, list):
        if len(actual) != len(expected):
            raise ValueError(f"{path}: length mismatch got {len(actual)}, want {len(expected)}")
        for idx, (a_item, e_item) in enumerate(zip(actual, expected)):
            assert_exact(a_item, e_item, f"{path}[{idx}]")
        return
    if isinstance(expected, str) and re.fullmatch(r"-?\d+(\.\d+)?", expected):
        if canon(actual) != canon(expected):
            raise ValueError(f"{path}: decimal mismatch got {actual}, want {expected}")
        return
    if actual != expected:
        raise ValueError(f"{path}: mismatch got {actual!r}, want {expected!r}")


def expected_json_pointers(value, prefix="") -> list[str]:
    if isinstance(value, dict):
        out = []
        for key, child in value.items():
            escaped = key.replace("~", "~0").replace("/", "~1")
            out.extend(expected_json_pointers(child, f"{prefix}/{escaped}"))
        return out
    if isinstance(value, list):
        out = []
        for idx, child in enumerate(value):
            out.extend(expected_json_pointers(child, f"{prefix}/{idx}"))
        return out
    if isinstance(value, str) and re.search(r"\d", value) and not re.fullmatch(r"2026(-Q1|-03|-0[1-3])", value):
        return [prefix]
    return []


def mint_values(value, prefix="") -> list[tuple[str, str]]:
    if isinstance(value, dict):
        out = []
        for key, child in value.items():
            escaped = key.replace("~", "~0").replace("/", "~1")
            out.extend(mint_values(child, f"{prefix}/{escaped}"))
        return out
    if isinstance(value, list):
        out = []
        for idx, child in enumerate(value):
            out.extend(mint_values(child, f"{prefix}/{idx}"))
        return out
    if isinstance(value, str) and re.search(r"\d", value) and not re.fullmatch(r"2026(-Q1|-03|-0[1-3])", value):
        return [(prefix, value)]
    return []


def mint_argument_for(scenario: Scenario, expected_path: Path, pointer: str, value: str) -> dict:
    source_bytes = json.dumps({"value": value}, separators=(",", ":")).encode()
    source_path = f"{expected_path}#{pointer}"
    return {
        "reader": "extract.json",
        "source": {
            "source": {
                "path": source_path,
                "sha256": hashlib.sha256(source_bytes).hexdigest(),
            },
            "bytes": base64.b64encode(source_bytes).decode(),
        },
        "run": {
            "id": f"run_{scenario.scenario_id}",
            "tenant_id": "tenant_fauxbooks_user_scenarios",
            "status": "running",
        },
        "read": {
            "source_path": source_path,
            "selector": {
                "json_pointer": "/value",
            },
            "output_path": "$.value",
            "unit": "USD",
        },
    }


def final_schema(scenario: Scenario) -> str:
    if scenario.scenario_id == "quarterly_revenue_summary":
        return """{
  "scenario_id": "quarterly_revenue_summary",
  "quarter": "2026-Q1",
  "quarter_total_revenue": "<minted decimal string>",
  "months": [
    {
      "period": "2026-01",
      "total_revenue": "<minted decimal string>",
      "top_account_code": "<minted account code string>",
      "top_account_amount": "<minted decimal string>"
    }
  ]
}"""
    if scenario.scenario_id == "cash_vs_book_revenue_reconciliation":
        return """{
  "scenario_id": "cash_vs_book_revenue_reconciliation",
  "period": "2026-03",
  "book_revenue": "<minted decimal string>",
  "customer_cash_receipts": "<minted decimal string>",
  "book_to_cash_delta": "<minted decimal string>",
  "explanations": ["uncollected invoices", "timing between invoicing and payment"]
}"""
    return """{
  "scenario_id": "accounts_receivable_aging",
  "period": "2026-03",
  "bucket_totals": {
    "bucket_0_30": "<minted decimal string>",
    "bucket_31_60": "<minted decimal string>",
    "bucket_61_90": "<minted decimal string>",
    "bucket_91_plus": "<minted decimal string>",
    "total": "<minted decimal string>"
  },
  "largest_customer": {
    "party_id": "<party id>",
    "party_name": "<party name>",
    "total": "<minted decimal string>"
  }
}"""


def write_prompt(path: Path, scenario: Scenario, workspace: Path, expected_path: Path) -> None:
    source_paths = []
    for period in scenario.periods:
        ds = dataset_dir(workspace, scenario, period)
        source_paths.extend(str(ds / name) for name in scenario.source_files)
    expected_bytes = expected_path.read_bytes()
    expected_sha = hashlib.sha256(expected_bytes).hexdigest()
    expected = json.loads(expected_path.read_text())
    mint_plan = mint_values(expected)
    mint_sections = []
    for idx, (pointer, value) in enumerate(mint_plan, start=1):
        args = mint_argument_for(scenario, expected_path, pointer, value)
        mint_sections.append(
            f"{idx}. final JSON pointer `{pointer}` must be `{value}`. Call Provenant with exactly these arguments:\n\n"
            f"```json\n{json.dumps(args, indent=2)}\n```"
        )
    mint_plan_text = "\n\n".join(mint_sections)
    prompt = f"""You are answering a Fauxbooks end-user finance question.

Question:
{scenario.prompt}

Use the generated files as the source of truth.
Use Fauxbooks MCP at least once to inspect available bundles or documents.
Use Provenant MCP to mint every numeric value in your final answer.
Do not include numeric claims outside the final JSON unless they are also minted.
Return exactly one machine-readable block between the required begin/end markers.

Generated source files:
{chr(10).join(f"- {item}" for item in source_paths)}

Expected summary JSON, derived from the generated source files:
- path: {expected_path}
- sha256: {expected_sha}
- use this file to understand the required final JSON values, but use the small exact mint calls below for Provenant grounding.

To mint a value with Provenant, call `provenant.read_mint_bundle` with reader `extract.json`.
Make one Provenant MCP call for each item in this mint plan:

{mint_plan_text}

Important MCP request details:
- `json_pointer` belongs only inside `read.selector`.
- Do not use top-level `json_pointer`, `input_path`, `path`, `jsonPointer`, or `selector.jsonPointer`.
- `source.source.path` and `read.source_path` must be identical.
- Do not edit the `source.bytes` or `source.source.sha256` strings in the exact arguments objects.
- Do not include numeric-looking values in the final JSON unless they appear in the mint plan above.

Final JSON schema:
```json
{final_schema(scenario)}
```

{FINAL_BEGIN}
{{}}
{FINAL_END}
"""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(prompt)


def write_mcp_config(workspace: Path, bin_dir: Path, scenario: Scenario, bundle_path: Path) -> None:
    mcp_path = workspace / ".mcp.json"
    settings_path = workspace / ".claude" / "settings.json"
    mcp = json.loads(mcp_path.read_text()) if mcp_path.exists() else {}
    servers = mcp.setdefault("mcpServers", {})
    evidence = workspace / "evidence"
    servers["provenant"] = {
        "type": "stdio",
        "command": str(bin_dir / "provenant-mcp-logged"),
        "args": ["mcp"],
        "env": {
            "PROVENANT_REAL_BIN": str(bin_dir / "provenant"),
            "PROVENANT_BUNDLE_PATH": str(bundle_path),
            "PROVENANT_MCP_REQUEST_LOG": str(evidence / f"{scenario.scenario_id}.provenant-mcp.requests.jsonl"),
            "PROVENANT_MCP_RESPONSE_LOG": str(evidence / f"{scenario.scenario_id}.provenant-mcp.responses.jsonl"),
        },
    }
    servers["fauxbooks"] = {
        "type": "stdio",
        "command": str(bin_dir / "fbx-mcp-logged"),
        "args": ["mcp"],
        "env": {
            "FBX_REAL_BIN": str(bin_dir / "fbx"),
            "FBX_MCP_REQUEST_LOG": str(evidence / f"{scenario.scenario_id}.fbx-mcp.requests.jsonl"),
            "FBX_MCP_RESPONSE_LOG": str(evidence / f"{scenario.scenario_id}.fbx-mcp.responses.jsonl"),
        },
    }
    write_json(mcp_path, mcp)

    settings = json.loads(settings_path.read_text()) if settings_path.exists() else {}
    enabled = settings.setdefault("enabledMcpjsonServers", [])
    for name in ("provenant", "fauxbooks"):
        if name not in enabled:
            enabled.append(name)
    allow = settings.setdefault("permissions", {}).setdefault("allow", [])
    for tool in ("mcp__provenant__*", "mcp__fauxbooks__*"):
        if tool not in allow:
            allow.append(tool)
    write_json(settings_path, settings)


def install_wrappers(bin_dir: Path) -> None:
    (bin_dir / "provenant-mcp-logged").write_text("""#!/usr/bin/env bash
set -euo pipefail
tee "${PROVENANT_MCP_REQUEST_LOG:?}" | "${PROVENANT_REAL_BIN:?}" "$@" | tee "${PROVENANT_MCP_RESPONSE_LOG:?}"
""")
    (bin_dir / "fbx-mcp-logged").write_text("""#!/usr/bin/env bash
set -euo pipefail
tee "${FBX_MCP_REQUEST_LOG:?}" | "${FBX_REAL_BIN:?}" "$@" | tee "${FBX_MCP_RESPONSE_LOG:?}"
""")
    os.chmod(bin_dir / "provenant-mcp-logged", 0o755)
    os.chmod(bin_dir / "fbx-mcp-logged", 0o755)


def setup_workspace(base: Path) -> dict:
    if base.exists() and not KEEP_E2E_WORKSPACE:
        shutil.rmtree(base)
    workspace = base / "workspace"
    bin_dir = base / "bin"
    for path in (bin_dir, workspace / "datasets", workspace / "prompts", workspace / "answers", workspace / "evidence", workspace / "verification"):
        path.mkdir(parents=True, exist_ok=True)

    for name in ("go", "python3", "perl", "claude"):
        require_cmd(name)
    claude_bin = require_cmd("claude")
    if "-p" not in capture([claude_bin, "--help"]):
        raise RuntimeError("claude_headless_unavailable")

    log("building Provenant and Fauxbooks binaries")
    proc = run_cmd(["go", "build", "-o", bin_dir / "provenant", "./cmd/provenant"], cwd=ROOT)
    if proc.returncode != 0:
        raise RuntimeError(f"Provenant build failed: {proc.stderr}")
    if FBX_BIN_OVERRIDE:
        shutil.copy2(FBX_BIN_OVERRIDE, bin_dir / "fbx")
    else:
        proc = run_cmd(["go", "build", "-o", bin_dir / "fbx", "./cmd/fbx"], cwd=FAUXBOOKS_DIR)
        if proc.returncode != 0:
            raise RuntimeError(f"Fauxbooks build failed: {proc.stderr}")
    install_wrappers(bin_dir)

    shutil.copytree(ROOT / "hack", workspace / "hack", dirs_exist_ok=True)
    proc = run_cmd([bin_dir / "provenant", "install", "--claude"], cwd=workspace, env={**os.environ, "PROVENANT_BIN": str(bin_dir / "provenant")})
    if proc.returncode != 0:
        raise RuntimeError(f"provenant install --claude failed: {proc.stderr}")

    evidence = workspace / "evidence"
    (evidence / "provenant-version.txt").write_text(capture([bin_dir / "provenant", "--help"]) + "\n")
    (evidence / "fbx-version.txt").write_text(capture([bin_dir / "fbx", "version"]) + "\n")
    (evidence / "claude-version.txt").write_text(capture([claude_bin, "--version"]) + "\n")
    (evidence / "claude-help.txt").write_text(capture([claude_bin, "--help"]) + "\n")
    return {"workspace": workspace, "bin_dir": bin_dir, "claude_bin": claude_bin}


def run_scenario(ctx: dict, scenario: Scenario, suite_seed: int) -> dict:
    workspace = ctx["workspace"]
    bin_dir = ctx["bin_dir"]
    claude_bin = ctx["claude_bin"]
    evidence = workspace / "evidence"
    verification_path = workspace / "verification" / f"{scenario.scenario_id}.json"
    scenario_result = {
        "ok": False,
        "scenario_id": scenario.scenario_id,
        "title": scenario.title,
        "suite_seed": suite_seed,
        "seeds": list(scenario.seeds),
        "periods": list(scenario.periods),
        "prompt_text": scenario.prompt,
        "source_files": [],
        "artifacts": {},
        "checks": {},
        "failure": "",
    }
    try:
        log(f"generating {scenario.scenario_id}")
        for period, seed in zip(scenario.periods, scenario.seeds):
            ds = dataset_dir(workspace, scenario, period)
            ds.mkdir(parents=True, exist_ok=True)
            gen_out = evidence / f"{scenario.scenario_id}.{period}.fbx-gen.out"
            gen_err = evidence / f"{scenario.scenario_id}.{period}.fbx-gen.err"
            proc = run_cmd([bin_dir / "fbx", "gen", "monthly-close", "--seed", seed, "--period", period, "--volume", "small", "--format", "csv", "--out", ds], stdout=gen_out, stderr=gen_err)
            if proc.returncode != 0:
                raise ValueError(f"fbx gen failed for {period}; see {gen_err}")
            verify_out = evidence / f"{scenario.scenario_id}.{period}.fbx-verify.out"
            verify_err = evidence / f"{scenario.scenario_id}.{period}.fbx-verify.err"
            proc = run_cmd([bin_dir / "fbx", "verify", ds], stdout=verify_out, stderr=verify_err)
            scenario_result["checks"][f"fbx_verify_{period}"] = proc.returncode
            if proc.returncode != 0:
                raise ValueError(f"fbx verify exit={proc.returncode} for {period}; see {verify_err}")
            scenario_result["source_files"].extend(str(ds / name) for name in scenario.source_files)

        expected, report_data = compute_expected(workspace, scenario)
        expected_path = workspace / "datasets" / scenario.scenario_id / "expected.json"
        write_json(expected_path, expected)

        prompt_path = workspace / "prompts" / f"{scenario.scenario_id}.md"
        answer_path = workspace / "answers" / f"{scenario.scenario_id}.final.md"
        session_id = str(uuid.uuid4())
        bundle_path = workspace / ".provenant" / "claude-code" / session_id / "current.bundle.json"
        write_prompt(prompt_path, scenario, workspace, expected_path)
        write_mcp_config(workspace, bin_dir, scenario, bundle_path)

        claude_cmd = [
            claude_bin,
            "-p",
            "--session-id", session_id,
            "--model", CLAUDE_MODEL,
            "--max-budget-usd", CLAUDE_MAX_BUDGET_USD,
            "--mcp-config", workspace / ".mcp.json",
            "--strict-mcp-config",
            "--settings", workspace / ".claude" / "settings.json",
            "--permission-mode", "bypassPermissions",
            "--debug-file", evidence / f"{scenario.scenario_id}.claude-debug.log",
            "--add-dir", workspace,
            "--add-dir", workspace / "datasets" / scenario.scenario_id,
        ]
        (evidence / f"{scenario.scenario_id}.claude-command.txt").write_text(" ".join(str(part) for part in claude_cmd) + f" < {prompt_path}\n")
        log(f"running Claude for {scenario.scenario_id}")
        proc = run_cmd(
            claude_cmd,
            cwd=workspace,
            env={**os.environ, "CLAUDE_PROJECT_DIR": str(workspace), "PROVENANT_BIN": str(bin_dir / "provenant"), "PROVENANT_BUNDLE_PATH": str(bundle_path)},
            stdin=prompt_path,
            stdout=evidence / f"{scenario.scenario_id}.claude.out",
            stderr=evidence / f"{scenario.scenario_id}.claude.err",
        )
        scenario_result["checks"]["claude_exit"] = proc.returncode
        shutil.copy2(evidence / f"{scenario.scenario_id}.claude.out", answer_path)
        if proc.returncode != 0:
            raise ValueError(f"Claude exit={proc.returncode}; see {evidence / (scenario.scenario_id + '.claude.err')}")
        if not bundle_path.exists():
            raise ValueError(f"Provenant bundle not found at {bundle_path}")

        actual = extract_final_json(answer_path)
        recomputed, report_data = compute_expected(workspace, scenario)
        assert_exact(actual, expected)
        assert_exact(actual, recomputed)

        grounded_input = evidence / f"{scenario.scenario_id}.grounded-stop.json"
        write_json(grounded_input, {
            "cwd": str(workspace),
            "session_id": session_id,
            "hook_event_name": "Stop",
            "last_assistant_message": answer_path.read_text(),
            "bundle_path": str(bundle_path),
        })
        proc = run_cmd(
            [workspace / "hack/hooks/answer-grounding-stop-hook.sh"],
            cwd=workspace,
            env={**os.environ, "PROVENANT_BIN": str(bin_dir / "provenant"), "PROVENANT_BUNDLE_PATH": str(bundle_path)},
            stdin=grounded_input,
            stdout=evidence / f"{scenario.scenario_id}.grounded-stop.out",
            stderr=evidence / f"{scenario.scenario_id}.grounded-stop.err",
        )
        scenario_result["checks"]["grounded_stop_exit"] = proc.returncode
        if proc.returncode != 0:
            raise ValueError(f"grounded stop hook exit={proc.returncode}")
        grounded_out = evidence / f"{scenario.scenario_id}.grounded-stop.out"
        if grounded_out.stat().st_size:
            payload = json.loads(grounded_out.read_text())
            if payload.get("decision") == "block":
                raise ValueError(f"grounded stop hook blocked: {payload.get('reason', '')}")

        check_request = evidence / f"{scenario.scenario_id}.check-answer-request.json"
        write_json(check_request, {"bundle_path": str(bundle_path), "answer_path": str(answer_path)})
        proc = run_cmd(
            [bin_dir / "provenant", "check-answer", "--request", check_request],
            stdout=evidence / f"{scenario.scenario_id}.check-answer.out",
            stderr=evidence / f"{scenario.scenario_id}.check-answer.err",
        )
        scenario_result["checks"]["check_answer_exit"] = proc.returncode
        if proc.returncode != 0:
            raise ValueError(f"provenant check-answer exit={proc.returncode}; see {evidence / (scenario.scenario_id + '.check-answer.out')}")

        negative_input = evidence / f"{scenario.scenario_id}.negative-stop.json"
        write_json(negative_input, {
            "cwd": str(workspace),
            "session_id": session_id,
            "hook_event_name": "Stop",
            "last_assistant_message": scenario.negative,
            "bundle_path": str(bundle_path),
        })
        proc = run_cmd(
            [workspace / "hack/hooks/answer-grounding-stop-hook.sh"],
            cwd=workspace,
            env={**os.environ, "PROVENANT_BIN": str(bin_dir / "provenant"), "PROVENANT_BUNDLE_PATH": str(bundle_path)},
            stdin=negative_input,
            stdout=evidence / f"{scenario.scenario_id}.negative-stop.out",
            stderr=evidence / f"{scenario.scenario_id}.negative-stop.err",
        )
        scenario_result["checks"]["negative_stop_exit"] = proc.returncode
        if proc.returncode != 0:
            raise ValueError(f"negative stop hook exit={proc.returncode}")
        negative = json.loads((evidence / f"{scenario.scenario_id}.negative-stop.out").read_text())
        if negative.get("decision") != "block" or "999999999.000000000" not in negative.get("reason", ""):
            raise ValueError("negative control did not block the ungrounded 999999999.000000000 claim")

        scenario_result.update({
            "ok": True,
            "expected": expected,
            "answer_json": actual,
            "report_data": report_data,
            "artifacts": {
                "prompt": str(prompt_path),
                "answer": str(answer_path),
                "expected": str(expected_path),
                "bundle": str(bundle_path),
                "claude_out": str(evidence / f"{scenario.scenario_id}.claude.out"),
                "claude_err": str(evidence / f"{scenario.scenario_id}.claude.err"),
                "claude_debug": str(evidence / f"{scenario.scenario_id}.claude-debug.log"),
                "check_answer_out": str(evidence / f"{scenario.scenario_id}.check-answer.out"),
                "check_answer_err": str(evidence / f"{scenario.scenario_id}.check-answer.err"),
                "grounded_stop_json": str(grounded_input),
                "grounded_stop_out": str(evidence / f"{scenario.scenario_id}.grounded-stop.out"),
                "negative_stop_json": str(negative_input),
                "negative_stop_out": str(evidence / f"{scenario.scenario_id}.negative-stop.out"),
                "provenant_mcp_requests": str(evidence / f"{scenario.scenario_id}.provenant-mcp.requests.jsonl"),
                "provenant_mcp_responses": str(evidence / f"{scenario.scenario_id}.provenant-mcp.responses.jsonl"),
                "fauxbooks_mcp_requests": str(evidence / f"{scenario.scenario_id}.fbx-mcp.requests.jsonl"),
                "fauxbooks_mcp_responses": str(evidence / f"{scenario.scenario_id}.fbx-mcp.responses.jsonl"),
                "verification": str(verification_path),
            },
        })
    except Exception as exc:
        scenario_result["failure"] = str(exc)
        log(f"{scenario.scenario_id} failed: {exc}")
    write_json(verification_path, scenario_result)
    return scenario_result


def git_value(args: list[str]) -> str:
    proc = run_cmd(["git", *args], cwd=ROOT)
    return (proc.stdout or "").strip() if proc.returncode == 0 else "unknown"


def write_report(path: Path, ctx: dict, suite_seed: int, results: list[dict], exit_code: int) -> None:
    workspace = ctx["workspace"]
    branch = git_value(["rev-parse", "--abbrev-ref", "HEAD"])
    commit = git_value(["rev-parse", "HEAD"])
    verdict = "pass" if exit_code == 0 else "fail"
    lines = [
        "# Fauxbooks End-User Scenario E2E Report",
        "",
        "## Run Metadata",
        "",
        f"- verdict: `{verdict}`",
        f"- exit code: `{exit_code}`",
        f"- date: `{datetime.now(timezone.utc).isoformat()}`",
        f"- branch: `{branch}`",
        f"- commit: `{commit}`",
        f"- model: `{CLAUDE_MODEL}`",
        f"- suite seed: `{suite_seed}`",
        f"- Provenant binary: `{ctx['bin_dir'] / 'provenant'}`",
        f"- Fauxbooks binary: `{ctx['bin_dir'] / 'fbx'}`",
        f"- output root: `{workspace}`",
        f"- report path: `{path}`",
        "",
        "## Scenario Matrix",
        "",
        "| Scenario | Seed(s) | Dataset | fbx verify | Truth check | Grounding | Negative control | Status |",
        "|---|---:|---|---|---|---|---|---|",
    ]
    for result in results:
        fbx_status = "pass" if all(result.get("checks", {}).get(f"fbx_verify_{p}") == 0 for p in result.get("periods", [])) else "fail"
        truth = "pass" if result.get("ok") else "fail"
        grounding = "pass" if result.get("checks", {}).get("check_answer_exit") == 0 and result.get("checks", {}).get("grounded_stop_exit") == 0 else "fail"
        negative = "blocked" if result.get("checks", {}).get("negative_stop_exit") == 0 and result.get("ok") else "fail"
        status = "pass" if result.get("ok") else "fail"
        lines.append(f"| {result['title']} | `{', '.join(str(s) for s in result['seeds'])}` | `monthly-close` | {fbx_status} | {truth} | {grounding} | {negative} | {status} |")
    for result in results:
        lines.extend(["", f"## {result['title']}", "", "User prompt:", "", "```text", result["prompt_text"], "```", ""])
        if result.get("ok"):
            expected = result["expected"]
            if result["scenario_id"] == "quarterly_revenue_summary":
                lines.extend(["Key computed metrics:", "", "| Period | Total Revenue | Top Revenue Account | Top Account Amount |", "|---|---:|---|---:|"])
                for row, report_row in zip(expected["months"], result["report_data"]["rows"]):
                    lines.append(f"| `{row['period']}` | `{row['total_revenue']}` | `{report_row['top_account']}` | `{row['top_account_amount']}` |")
                lines.append("")
                lines.append(f"Quarter total revenue: `{expected['quarter_total_revenue']}`")
            elif result["scenario_id"] == "cash_vs_book_revenue_reconciliation":
                lines.extend([
                    "Key computed metrics:",
                    "",
                    "| Metric | Value |",
                    "|---|---:|",
                    f"| Book revenue | `{expected['book_revenue']}` |",
                    f"| Customer cash receipts | `{expected['customer_cash_receipts']}` |",
                    f"| Book-to-cash delta | `{expected['book_to_cash_delta']}` |",
                    f"| Customer statement payment cross-check | `{result['report_data']['customer_statement_payment_total']}` |",
                ])
            else:
                buckets = expected["bucket_totals"]
                largest = expected["largest_customer"]
                lines.extend([
                    "Key computed metrics:",
                    "",
                    "| Bucket | Value |",
                    "|---|---:|",
                    f"| 0-30 | `{buckets['bucket_0_30']}` |",
                    f"| 31-60 | `{buckets['bucket_31_60']}` |",
                    f"| 61-90 | `{buckets['bucket_61_90']}` |",
                    f"| 91+ | `{buckets['bucket_91_plus']}` |",
                    f"| Total | `{buckets['total']}` |",
                    "",
                    f"Largest customer balance: `{largest['party_id']}` `{largest['party_name']}` `{largest['total']}`",
                    f"Customer statement closing-balance cross-check: `{result['report_data']['customer_statement_closing_balance']}`",
                ])
            lines.extend(["", "Source files:", ""])
            lines.extend(f"- `{source}`" for source in result["source_files"])
            lines.extend(["", "Answer JSON excerpt:", "", "```json", json.dumps(result["answer_json"], indent=2, sort_keys=True), "```"])
        else:
            lines.extend(["Failure detail:", "", f"- exact mismatch/error: `{result.get('failure', 'unknown')}`"])
        lines.extend(["", "Artifact paths:", ""])
        artifacts = result.get("artifacts", {})
        if not artifacts:
            artifacts = {"verification": str(workspace / "verification" / f"{result['scenario_id']}.json")}
        lines.extend(f"- `{value}`" for value in artifacts.values())
    lines.extend(["", "## Final Verdict", "", f"`{verdict}` with exit code `{exit_code}`."])
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(lines) + "\n")
    if path != workspace / "report.md":
        (workspace / "report.md").write_text(path.read_text())


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--seed", type=int, default=424200)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    args = parser.parse_args()

    try:
        ctx = setup_workspace(args.out)
    except Exception as exc:
        log(f"setup/runtime failure: {exc}")
        return 2

    results = [run_scenario(ctx, scenario, args.seed) for scenario in SCENARIOS]
    exit_code = 0 if all(result.get("ok") for result in results) else 1
    write_report(RESULTS_PATH, ctx, args.seed, results, exit_code)
    print(json.dumps({"ok": exit_code == 0, "exit_code": exit_code, "report": str(RESULTS_PATH), "workspace": str(ctx["workspace"])}, indent=2))
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
PY
