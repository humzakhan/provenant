@/Users/humza/.codex/RTK.md

# Provenant AGENTS

Use repo-local guidance from `.codex/skills/` and `.agents/` when it matches. Prefer the smallest applicable guidance set.

## Sources

- First message in this repo: read `CLAUDE.md` only when Claude-specific guidance is needed.
- Prefer repo-local skills for orchestration, architecture review, workflow, and commit/PR hygiene.
- Keep skill usage additive and narrow. For implementation-specific work, load only the extra skills required for that task.

## Provenant MCP Attestation

- MCP local-agent mode cannot force arbitrary host-channel model output through Provenant.
- Treat regulated conclusions as official only when they come from a passing `submit_answer` and the `[PROVENANT-ATTESTED v1 ...]` marker is matched against the audit chain/audit pack; marker text alone is not sufficient.
- Restricting regulated files to Provenant-controlled tools mitigates direct raw-file bypass, but host-channel output still requires `submit_answer` and verification. Hard non-bypassable enforcement requires Mode A CLI/embedded harness or a Provenant-owned harness.

## Personas

- Implementation: `.agents/engineer.md`
- Review: `.agents/reviewer.md`

## Skills

- `conventional-commit-guard`: before creating, amending, squashing, or suggesting any commit message, and before opening, creating, or updating a PR.
- `orchestrator-mode`: when the user asks Codex to coordinate through sub-agents only or to enter orchestrator mode.
- `phase-planning`: when the user asks to plan the next phase or create/update `.planning/phases/<phase-slug>/` docs in the repo's phase format.
- `engineering-workflow`: default process for branching, commits, validation, review cycles, and PR lifecycle.
- `pr-gate`: only in single-feature mode when a push, PR, merge, milestone transition, or done boundary requires explicit human approval. Do not use when orchestrator mode is active.
- Go skills are additive and should be loaded narrowly:
  - `go-implementation-quality`: general Go implementation, Effective Go style, small APIs, package boundaries, and error handling.
  - `go-testing-fuzzing`: table/golden tests, fuzzing, and invariant checks for parsers, serialization, and path handling.
  - `go-input-validation-security`: strict decoding, bounded reads, size/depth limits, allowlists, and fail-closed input handling.
  - `go-io-performance`: `io.Reader`/`io.Writer`, buffering, chunking, and avoiding full materialization.
  - `go-concurrency-memory-model`: only for goroutines, channels, mutexes, atomics, or shared state; use happens-before and the race detector.

## Codex Models

- Orchestration and task sequencing: `gpt-5.5`, medium reasoning.
- Architecture and cross-surface decisions: `gpt-5.5`, high reasoning.
- Broad repo study and docs synthesis: `gpt-5.4-mini`, medium reasoning.
- Normal implementation and refactoring: `gpt-5.4`, medium reasoning.
- High-risk implementation or review: `gpt-5.5`, high reasoning.
- Deterministic operator work: `gpt-5.3-codex-spark`; fall back to `gpt-5.4-mini` when Spark is unavailable.
- Small mechanical edits: `gpt-5.3-codex-spark`; fall back to `gpt-5.4-mini` when Spark is unavailable.

## Deterministic Operators

Use Spark-class models for fully deterministic, bounded operations that do not
need judgment beyond following already-selected commands or text edits. Good
fits include merge/fetch/branch state checks, PR comment or status reads,
scope scans, format checks, simple renames, and already-selected validation
commands.

Do not use Spark-class models for review, architecture, security, reliability,
tenant-boundary decisions, synthesis, recommendations, or deciding whether a
test matrix is sufficient.

## Codex Delegation

- In orchestrator mode, delegate repo reading, implementation, validation, and review.
- In orchestrator mode, `pr-gate` is not applicable: after automated sub-agent review approves a PR and merge criteria pass, merge it and branch the next slice from updated `origin/main`.
- Outside orchestrator mode, use subagents only when parallel study or independent review materially reduces risk or time.
- For broad architecture review, prefer 2-4 narrow read-only readers plus one independent reviewer.
