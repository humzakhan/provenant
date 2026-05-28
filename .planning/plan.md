# Implementation Plan: Answer-Number Grounding Enforcement

Adopt `codegraph`'s two-tier enforcement pattern (soft MCP instructions + hard release-time hook) to enforce that no answer containing an ungrounded number can be released. Enforcement logic lives in the pure kernel; the Claude Code hook is a thin adapter. See `.planning/goal.md` for objective and definition of done.

## Background & rationale

- codegraph forces harnesses through its index via: (1) MCP `initialize` `serverInstructions` injected into the system prompt (soft), and (2) an optional `PreToolUse` hook that denies `Read` on source files (hard, eval-only). Its installer wires MCP config, permission allowlists, and rules files per agent target.
- **What transfers directly**: the MCP-instructions soft layer and the per-target installer shape.
- **What must change**: the interception point. codegraph blocks an *input* tool call (`Read`). "Computing a number" is not a tool call, so it cannot be intercepted at computation time. We intercept at **release time** (harness `Stop` event) instead — which is exactly where Provenant's `submit_answer` guard (ADR-0002, `architecture/subsystems/guards.md`) already says enforcement belongs.
- **What's already present in Provenant**: the `Token`/`Derivation`/`CapturedRead` grounding model, `verify`/`replay`/`pack`, the `submit-answer` CLI scaffold, and the documented guard pipeline. **Missing**: numeric-extraction-from-answer-text, the `CheckAnswer` guard, and the harness glue (hook + installer).

## Dependency graph

```
PR1 (extraction) ──► PR2 (CheckAnswer + CLI) ──► PR3 (MCP soft layer)
                                              └─► PR4 (Stop hook + install) ──► PR5 (e2e run, results recorded)
```
PR3 and PR4 are independent of each other once PR2 lands.

---

## PR 1 — Numeric-claim extraction (pure kernel)

**Goal**: A pure Go function that extracts numeric claims from answer text, returning each value in canonical 9-decimal form plus its positional span.

**Scope**:
- New pure package (e.g. `kernel/public/grounding`). No I/O, no disallowed imports.
- `ExtractNumericClaims(answer string) []NumericClaim` where `NumericClaim{ ValueCanonical string; Raw string; Start, End int }`.
- MVP in-scope: currency amounts and plain integers/decimals at or above a documented magnitude threshold.
- False-positive handling: define an explicit ignore policy for dates, illustrative percentages, footnote/citation markers, list ordinals, and version strings. Provide an explicit escape convention (e.g. an inline `[[unverified]]` marker) for numbers deliberately exempt. Document every deferred case — no silent caps.

**Tests**: real extractor against a fixture corpus of answers covering true positives and the known false-positive classes.

**Stop condition**: extractor returns canonical decimals consistent with `shopspring/decimal` 9-decimal form, with correct spans; `go test ./...` green.

**Risk**: numeric extraction is the genuinely hard part of this whole effort — harder than the hook. Keep MVP narrow and explicit; list deferred classes in the package doc.

---

## PR 2 — `CheckAnswer` grounding guard + `provenant check-answer` CLI verb

**Goal**: Realize the `submit_answer` numeric-grounding step (ADR-0002) as a pure kernel guard, exposed via CLI.

**Scope**:
- Pure kernel `CheckAnswer(answer string, tokens []Token) (Verdict, error)`. For each extracted claim, require a `Token` whose `ValueCanonical` matches. `Verdict` reports grounded claims, ungrounded claims (value + span), and an overall pass/fail.
- CLI verb `check-answer`: inputs are answer text (file or stdin) and a bundle (the run's minted tokens); load tokens via the existing bundle-loading path used by `verify`/`pack`; print verdict JSON; exit non-zero when any claim is ungrounded.

**Tests**: real kernel + real bundle fixtures via the shared test harness (one common location). Cases: fully grounded → pass/exit 0; one ungrounded value → fail/exit non-zero with the value + span in the reason.

**Stop condition**: `provenant check-answer` exits 0 on a fully-grounded answer and non-zero with structured reasons otherwise; tests green.

---

## PR 3 — MCP `serverInstructions` soft layer

**Goal**: Inject guidance at MCP `initialize` telling the model to ground every number.

**Scope**:
- Add a single source-of-truth `serverInstructions` constant in `access/mcpapi/` (mirror codegraph's one-constant pattern) returned in the `initialize` response.
- Content: every number in a released answer must originate from a Provenant tool (e.g. `provenant.read_mint_bundle` / a calc path); do not compute numbers in prose; ungrounded numbers are blocked at release. Keep it concise.
- Explicitly mark in code comments that this is the *soft* assist layer; teeth live in the PR4 hook.

**Tests**: assert the `initialize` response carries the instructions string.

**Stop condition**: MCP `initialize` includes the instructions; test asserts presence.

**Note**: codegraph's own notes admit soft instructions do not reliably move tool choice — do not rely on this layer alone.

---

## PR 4 — Claude Code `Stop`-hook adapter + `provenant install`

**Goal**: The hard tier. A release-time hook that blocks ungrounded answers, plus an installer that wires everything into the harness.

**Scope**:
- Hook script (e.g. `hack/hooks/answer-grounding-stop-hook.sh`) registered on the Claude Code `Stop` event. It locates the run's bundle and the final assistant answer, calls `provenant check-answer`, and emits Stop-hook JSON with `decision: block` + a reason listing ungrounded values when the check fails; otherwise allows.
- `provenant install` command modeled on codegraph's `targets/`: wires the Provenant MCP server into harness config, adds a permission allowlist for the Provenant MCP tools, and installs the Stop hook into `settings.json`. Implement the `--claude` target now; structure the code so future targets (cursor/codex) can be added without rework. Idempotent writes using marker blocks (`<!-- PROVENANT_START -->` / `<!-- PROVENANT_END -->`).

**Tests**: `install` writes the expected settings JSON into a temp `HOME`; hook script unit-tested against fixture (transcript + bundle): grounded → allow, ungrounded → block.

**Stop condition**: `provenant install --claude` produces valid settings + hook; the hook blocks an ungrounded fixture answer and allows a grounded one.

**Open design question (resolve in this PR)**: how the hook identifies *which* bundle holds this session's tokens. Document the chosen mechanism (e.g. a run-scoped bundle path/env var written at run start).

---

## PR 5 — End-to-end test run (separate PR; results recorded)

**Goal**: Prove the whole loop against the real kernel through the installed hook. This PR is dedicated to running the test and recording results — not new feature code (only e2e harness/fixtures as needed).

**Scope**:
- Set up a workspace with real source files, install via PR4, and run an agent task that must produce a number. Demonstrate two cases:
  - (a) model emits a self-computed number in prose → Stop hook **blocks** → corrected.
  - (b) model routes through the Provenant tool path → grounded → **released**.
- e2e exercises the real kernel/driver via the shared test harness (one common location).

**Results recording**: write `.planning/e2e-results.md` capturing exact commands, transcript excerpts, the block reason output, the final grounded answer, environment, and pass/fail.

**Stop condition**: a documented e2e run in which the block actually fires for an ungrounded number and a grounded answer is released; `.planning/e2e-results.md` committed.

---

## Cross-cutting conventions

- **Kernel purity** enforced by the existing forbidden-import CI check; enforcement core imports nothing from `net/*`, `database/sql`, `pgx`, MCP SDK, or `os/exec`.
- **Decimals**: `shopspring/decimal`, canonical 9-decimal, consistent with `Token.ValueCanonical`.
- **Tests**: real kernel/driver only; one shared harness location; no copy/paste between test files.
- **PR hygiene**: each PR builds + tests green standalone; small and independently reviewable.
- **Reviews**: default to sonnet-tier reviewers; escalate only for genuinely high-stakes changes.

## Risks & open questions

- **Numeric extraction precision** (PR1) is the hardest piece; false positives (dates, ordinals, versions) and false negatives both matter. Mitigation: narrow MVP, explicit escape marker, documented deferred classes.
- **Session→bundle binding** (PR4): the hook must reliably find the current run's tokens. Must be designed explicitly, not assumed.
- **Framework scope**: the Stop hook is Claude Code-specific; keep all enforcement in the kernel so other harness adapters are thin. This preserves Provenant's "regardless of host framework" thesis.
- **Soft-layer reliability**: per codegraph's own findings, MCP instructions alone do not reliably change behavior; the hook is the real enforcer.
