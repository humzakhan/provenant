# Phase 05 — CLI Surface and Ungrounded-Number Gate

## Objective

Expose the Derivation Graph through the CLI as a coherent set of verbs that share one long-lived run file: `run start`, `run finish`, `run fail`, `read-mint`, `calc`, `submit-answer`, `verify`, `replay`. Enforce the ungrounded-number invariant structurally at `submit-answer`: only a registered token ID is accepted. Existing `read-mint-bundle` and `reconcile` verbs are preserved.

## Scope (in)

- Extend `access/cli/cli.go` to add new verbs without breaking existing ones.
- Add `access/cli/runverbs.go`, `access/cli/calcverb.go`, `access/cli/submitverb.go`, `access/cli/verifyverb.go`, `access/cli/replayverb.go`.
- JSON-in, JSON-out for all new verbs. Machine-readable.
- Wire the verbs to `kernel/public/runfile` (Phase 04) and `kernel/public/derive` (Phases 02–03).
- Failure fixtures for the ungrounded-number gate.

## Scope (out)

- No HTTP or MCP surface.
- No named-source mapping (Phase 06).
- No JSON pack format (Phase 06).
- No trace-back tool (Phase 07).

## Technical Design

### Verbs

All verbs share one positional `--run-file <path>` flag that names the long-lived `.part` file (and, after finish, the finalized bundle). Verbs read JSON requests from `--request <path>` or stdin (`--request -`).

| Verb | Request | Response | Effect |
|---|---|---|---|
| `run start` | `{"run":{"id","tenant_id"}, "run_file":"..."}` | `{"run_file":"...","seq":1}` | Creates partfile, writes `run_started` |
| `run finish` | `{"run_file":"..."}` | `{"bundle_sha256":"...","bundle_path":"..."}` | Writes `run_completed`, finalizes |
| `run fail` | `{"run_file":"...","reason":"..."}` | `{"bundle_sha256":"...","bundle_path":"..."}` | Writes `run_failed`, finalizes |
| `read-mint` | existing `ReadMintBundleRequest` + `"run_file":"..."` | `{"token":{...},"audit_entry":{...},"seq":N}` | Appends mint to the run |
| `calc` | `{"run_file":"...","op":"calc.add","inputs":["tok_a","tok_b"],"rounding":{...},"predicate":{...}}` | `{"token":{...},"audit_entry":{...},"seq":N}` | Appends derive |
| `submit-answer` | `{"run_file":"...","token_id":"tok_x","label":"operating_income"}` | `{"token":{...},"audit_entry":{...},"answer":{"value_canonical":"...","unit":"USD"}}` | Validates token exists in the run, writes `answer_submitted` audit entry |
| `verify` | `{"bundle_path":"..."}` | `{"ok":true}` or typed error | Calls `bundle.Verify` on a finalized bundle |
| `replay` | `{"bundle_path":"..."}` | `{"tokens":[...],"captured_reads":[...]}` | Calls `bundle.Replay`, prints reconstructed state |

### New audit entry kind

`submit-answer` introduces:

```go
// kernel/public/types.go
const EntryKindAnswerSubmitted EntryKind = "answer_submitted"
```

Deterministic payload:

```json
{
  "token_id": "tok_x",
  "label": "operating_income",
  "value_canonical": "1234.560000000",
  "unit": "USD"
}
```

Bundle replay must validate that the referenced token exists earlier in the chain (by seq). Phase 01's bundle invariant `audit count == token count` must be relaxed for `answer_submitted` — it produces no new token. Update `normalizeState` accordingly.

### Ungrounded-number gate

The `submit-answer` request schema accepts only `token_id`. There is no `value` field. Attempting to add one fails JSON decoding with `DisallowUnknownFields`. The CLI rejects:

- Missing `token_id`
- `token_id` not present in the partfile's token set
- A `token_id` from a different run

This is the structural guarantee. There is no separate "is this a number" check — the CLI's request shape simply cannot express a literal.

### Calc verb request

```json
{
  "run_file": "./run.json.part",
  "op": "calc.subtract",
  "inputs": ["tok_revenue", "tok_cogs"],
  "rounding": {"mode": "half_even", "scale": 2}
}
```

For `filter.sum_where`, the request also accepts `predicate`:

```json
{
  "run_file": "./run.json.part",
  "op": "filter.sum_where",
  "inputs": ["tok_row_1","tok_row_2","tok_row_3"],
  "predicate": {"field": "row.account", "op": "eq", "value": "4000"}
}
```

The verb fails closed on:

- Unknown `op`
- Inputs not present in the partfile
- Predicate supplied for an op other than `filter.sum_where`
- `rounding.scale > 18`

### Existing verbs

`read-mint-bundle` and `reconcile` are not removed and not modified semantically. They become thin wrappers: `read-mint-bundle` is equivalent to `run start` + `read-mint` + `run finish` against a temporary run file; `reconcile` continues to use its current internal flow. Both are documented as "convenience verbs over the lifecycle" in `README.md` updates landed within this phase.

### Exit codes

- `0`: success
- `2`: user error (bad request, unknown op, unknown token, ungrounded literal)
- `1`: internal error (file system, hash chain corruption, op evaluator panic)

These match the existing `cli.Error`/`ErrorCategoryUser` taxonomy.

## Implementation Steps

1. Extend the verb dispatch in `(Runner).Run` to recognize: `run`, `read-mint`, `calc`, `submit-answer`, `verify`, `replay`. `run` is a two-word verb: `run start`, `run finish`, `run fail`.
2. Add `access/cli/runverbs.go` for `run start|finish|fail`. Each opens (or creates) the run file via `runfile.Start` or `runfile.Recover`.
3. Add `access/cli/readmintverb.go` for `read-mint`: decodes the existing `ReadMintBundleRequest` shape with an added `run_file` field, executes the read+mint, appends the audit entry to the runfile.
4. Add `access/cli/calcverb.go`. Decode request, look up input tokens from the runfile, call `derive.Derive`, append.
5. Add `access/cli/submitverb.go`. Decode request with `DisallowUnknownFields`. Verify `token_id` exists. Append `answer_submitted` entry. The verb does not finalize the run.
6. Add `access/cli/verifyverb.go` and `replayverb.go` — thin wrappers over `bundle.Verify` and `bundle.Replay`.
7. Add `EntryKindAnswerSubmitted` and bundle decoder support for it in `kernel/public/bundle/bundle.go`.
8. Extend `bundle.normalizeState` to allow `answer_submitted` entries that produce no token. The invariant becomes `len(tokens) == count(entries where kind in {token_mint, token_derive})`.
9. Failure fixtures under `examples/derivation-graph/failures/`:
   - `submit_literal/`: request with `"value": 123` instead of `"token_id"` → user error.
   - `submit_unknown_token/`: token ID not present in runfile → user error.
   - `calc_unknown_op/`: op not in registry → user error.
   - `calc_unit_mismatch/`: USD + EUR → user error.

## Tests Required

| Test | Asserts |
|---|---|
| `run start` then `read-mint` then `run finish` | Bundle parses; one mint entry; `bundle.Verify` ok |
| `calc` happy path | After mint + mint + `calc.add`, finalized bundle has 1 derive entry, replay re-derives identical value |
| `submit-answer` happy | `answer_submitted` entry appended; finalized bundle verifies |
| `submit-answer` rejects literal | Request with `value` field returns exit 2 and `request_invalid` |
| `submit-answer` rejects unknown token | Returns exit 2 and `token_not_found` |
| `calc` rejects predicate on non-filter op | Returns exit 2 |
| `verify` ok | Returns `{"ok":true}` |
| `verify` tampered | Returns exit 1 and `ErrTamperDetected` |
| `replay` round-trip | Reconstructed state matches the runfile contents |
| Concurrent `calc` on same runfile | Second invocation returns `ErrConcurrentAppender` |
| Existing `read-mint-bundle` regression | Unchanged behavior; fixture output bit-identical |
| Existing `reconcile` regression | Same |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] `submit-answer` has no JSON field that accepts a literal value — confirmed by code review and a test that asserts the request struct has only `RunFile`, `TokenID`, `Label`.
- [ ] All failure fixtures produce the documented exit codes and error categories.
- [ ] `examples/reconciliation/report.zip` still verifies and replays unchanged.
- [ ] README updated with the new verb table and a short walkthrough.

## Risks / Open Questions

- Should `submit-answer` be allowed to fire multiple times in one run (multiple "answers")? Yes — there is no kernel reason to forbid it. Each is its own audit entry. The Phase 08 demo uses one.
- Should `run start` accept `--run-file -` (stdin/stdout streaming bundle)? No. The run file is on disk by design — that is what makes append-then-crash recovery tractable.
- Backward compat with `read-mint-bundle`: its request struct adds `run_file` as optional. When absent, behavior is unchanged (single-shot temp runfile). Document the new field as optional.
- Are there security implications to opening a user-supplied path? Yes — limit to absolute or workdir-relative paths, reject `..` traversal beyond `--workdir`. Mirror the `reconcile` verb's existing `ensureOutputWritable`.

## Dependencies

- [Phase 01](./phase-01-token-dag-schema.md), [Phase 02](./phase-02-op-catalog-core.md), [Phase 03](./phase-03-op-catalog-rest.md), [Phase 04](./phase-04-run-lifecycle.md) all merged.
