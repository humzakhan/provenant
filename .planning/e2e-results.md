# Answer Grounding E2E Results

Date: 2026-05-29

Branch: `test/answer-grounding-e2e-results`

Commit under test: `bb4b1ac feat: install answer grounding hook`

## Environment

- Repo: `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/hyper-deposit`
- Temp workspace: `/tmp/provenant-answer-grounding-e2e/workspace`
- Test binary: `/tmp/provenant-e2e`
- Hook path under installed workspace: `hack/hooks/answer-grounding-stop-hook.sh`
- Session bundle path: `/tmp/provenant-answer-grounding-e2e/workspace/.provenant/claude-code/session-e2e/current.bundle.json`

## Commands

```sh
go build -o /tmp/provenant-e2e ./cmd/provenant
rm -rf /tmp/provenant-answer-grounding-e2e
mkdir -p /tmp/provenant-answer-grounding-e2e/workspace
cp -R hack /tmp/provenant-answer-grounding-e2e/workspace/hack

cat >/tmp/provenant-answer-grounding-e2e/session-start.json <<'JSON'
{"cwd":"/tmp/provenant-answer-grounding-e2e/workspace","session_id":"session-e2e"}
JSON

cat >/tmp/provenant-answer-grounding-e2e/mcp-request.jsonl <<'JSON'
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"provenant.read_mint_bundle","arguments":{"reader":"extract.json","source":{"source":{"path":"fixture://answer-grounding-e2e.json","sha256":"f26e267ee03331ff5ce10b687a1ba1a9b49012ffb27694c922e17411b4b86e6c"},"bytes":"eyJhbW91bnQiOjQyfQ=="},"run":{"id":"run_answer_grounding_e2e","tenant_id":"tenant_answer_grounding_e2e","status":"running"},"read":{"source_path":"fixture://answer-grounding-e2e.json","selector":{"json_pointer":"/amount"},"output_path":"$.value","unit":"USD"}}}}
JSON

cat >/tmp/provenant-answer-grounding-e2e/stop-ungrounded.json <<'JSON'
{"cwd":"/tmp/provenant-answer-grounding-e2e/workspace","session_id":"session-e2e","hook_event_name":"Stop","last_assistant_message":"The answer is 43."}
JSON

cat >/tmp/provenant-answer-grounding-e2e/stop-grounded.json <<'JSON'
{"cwd":"/tmp/provenant-answer-grounding-e2e/workspace","session_id":"session-e2e","hook_event_name":"Stop","last_assistant_message":"The answer is 42."}
JSON

cd /tmp/provenant-answer-grounding-e2e/workspace
PROVENANT_BIN=/tmp/provenant-e2e /tmp/provenant-e2e install --claude

CLAUDE_ENV_FILE=/tmp/provenant-answer-grounding-e2e/claude.env \
  ./hack/hooks/provenant-session-start.sh \
  < /tmp/provenant-answer-grounding-e2e/session-start.json

. /tmp/provenant-answer-grounding-e2e/claude.env
PROVENANT_BUNDLE_PATH="$PROVENANT_BUNDLE_PATH" \
  /tmp/provenant-e2e mcp \
  < /tmp/provenant-answer-grounding-e2e/mcp-request.jsonl \
  > /tmp/provenant-answer-grounding-e2e/mcp-response.jsonl

PROVENANT_BIN=/tmp/provenant-e2e bash -x ./hack/hooks/answer-grounding-stop-hook.sh \
  < /tmp/provenant-answer-grounding-e2e/stop-ungrounded.json \
  > /tmp/provenant-answer-grounding-e2e/stop-ungrounded.fixed.out \
  2> /tmp/provenant-answer-grounding-e2e/stop-ungrounded.fixed.trace

PROVENANT_BIN=/tmp/provenant-e2e bash -x ./hack/hooks/answer-grounding-stop-hook.sh \
  < /tmp/provenant-answer-grounding-e2e/stop-grounded.json \
  > /tmp/provenant-answer-grounding-e2e/stop-grounded.fixed.out \
  2> /tmp/provenant-answer-grounding-e2e/stop-grounded.fixed.trace

wc -c /tmp/provenant-answer-grounding-e2e/stop-ungrounded.fixed.out \
  /tmp/provenant-answer-grounding-e2e/stop-grounded.fixed.out
```

## Installed Config Excerpts

`.claude/settings.json` included:

```json
{
  "enabledMcpjsonServers": ["provenant"],
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "args": [],
            "command": "${CLAUDE_PROJECT_DIR}/hack/hooks/provenant-session-start.sh",
            "type": "command"
          }
        ],
        "matcher": "startup|resume|clear|compact"
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "args": [],
            "command": "${CLAUDE_PROJECT_DIR}/hack/hooks/answer-grounding-stop-hook.sh",
            "type": "command"
          }
        ]
      }
    ]
  },
  "permissions": {
    "allow": ["mcp__provenant__*"]
  }
}
```

`.mcp.json` included:

```json
{
  "mcpServers": {
    "provenant": {
      "args": ["mcp"],
      "command": "${PROVENANT_BIN:-provenant}",
      "env": {
        "PROVENANT_BUNDLE_PATH": "${PROVENANT_BUNDLE_PATH:-}"
      },
      "type": "stdio"
    }
  }
}
```

## Transcript Excerpts

Session start wrote the bundle binding:

```sh
PROVENANT_BUNDLE_PATH=/tmp/provenant-answer-grounding-e2e/workspace/.provenant/claude-code/session-e2e/current.bundle.json
export PROVENANT_BUNDLE_PATH
```

The source JSON behind the grounded `42` was:

```json
{"amount":42}
```

The MCP request encoded that source as:

```json
{"bytes":"eyJhbW91bnQiOjQyfQ=="}
```

MCP `provenant.read_mint_bundle` minted a token and persisted the session bundle:

```text
surface=mcp bundle_sha256=e9d08a3d1c3ac8d2cd209ffab4f48f5ff4cd6f79a67b992f77d04694883215b9 token_id=tok_0479a6a56a93ca6776510383f7bc56f7
```

```sh
-rw------- ... /tmp/provenant-answer-grounding-e2e/workspace/.provenant/claude-code/session-e2e/current.bundle.json
```

## Case A: Ungrounded Number Blocks

Stop input:

```json
{
  "cwd": "/tmp/provenant-answer-grounding-e2e/workspace",
  "session_id": "session-e2e",
  "hook_event_name": "Stop",
  "last_assistant_message": "The answer is 43."
}
```

Hook exit status:

```text
0
```

`wc -c /tmp/provenant-answer-grounding-e2e/stop-ungrounded.fixed.out /tmp/provenant-answer-grounding-e2e/stop-grounded.fixed.out` returned:

```text
      80 /tmp/provenant-answer-grounding-e2e/stop-ungrounded.fixed.out
       0 /tmp/provenant-answer-grounding-e2e/stop-grounded.fixed.out
      80 total
```

Hook output:

```json
{"decision":"block","reason":"ungrounded value=43.000000000 raw=43 span=14:16"}
```

Result: pass. The answer was blocked at release with a structured reason naming the ungrounded value.

## Case B: Grounded Number Releases

Stop input:

```json
{
  "cwd": "/tmp/provenant-answer-grounding-e2e/workspace",
  "session_id": "session-e2e",
  "hook_event_name": "Stop",
  "last_assistant_message": "The answer is 42."
}
```

Hook exit status:

```text
0
```

Hook output:

```text
```

The trace shows the wrapper calling:

```text
/tmp/provenant-e2e check-answer --request /var/folders/.../request.json
```

and the verdict path differs by case:

```text
ungrounded: status=1
grounded: exit 0
```

The grounded stdout file is `0` bytes.

Result: pass. The answer was released because `42.000000000` traced to the minted token in the session bundle.

## Overall Result

Pass.

The installed hook path exercised the real kernel via `PROVENANT_BIN=/tmp/provenant-e2e check-answer --request ...`, using a bundle produced through the installed MCP command and persisted to the session-scoped bundle path. Ungrounded release was blocked; grounded release was allowed.

## Open Questions

- The e2e run used local hook invocation with Claude-shaped JSON, not a live Claude Code UI session.
- The MCP config relies on Claude's environment propagation for `PROVENANT_BUNDLE_PATH`; the hook also falls back to the session-scoped path written by `SessionStart`.
