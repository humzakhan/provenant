@/Users/humza/.codex/RTK.md

# Provenant AGENTS

Use repo-local guidance from `.codex/skills/` and `.agents/` when it matches. Prefer the smallest applicable guidance set.

## Sources

- First message in this repo: read `CLAUDE.md` only when Claude-specific guidance is needed.
- Prefer repo-local skills for orchestration, architecture review, workflow, and commit/PR hygiene.
- Keep skill usage additive and narrow. For implementation-specific work, load only the extra skills required for that task.

## Personas

- Implementation: `.agents/engineer.md`
- Review: `.agents/reviewer.md`

## Skills

- `conventional-commit-guard`: before creating, amending, squashing, or suggesting any commit message, and before opening or updating a PR title.
- `orchestrator-mode`: when the user asks Codex to coordinate through sub-agents only or to enter orchestrator mode.
- `engineering-workflow`: default process for branching, commits, validation, review cycles, and PR lifecycle.
- Go skills are additive and should be loaded narrowly:
  - `go-implementation-quality`: general Go implementation, Effective Go style, small APIs, package boundaries, and error handling.
  - `go-testing-fuzzing`: table/golden tests, fuzzing, and invariant checks for parsers, serialization, and path handling.
  - `go-input-validation-security`: strict decoding, bounded reads, size/depth limits, allowlists, and fail-closed input handling.
  - `go-io-performance`: `io.Reader`/`io.Writer`, buffering, chunking, and avoiding full materialization.
  - `go-concurrency-memory-model`: only for goroutines, channels, mutexes, atomics, or shared state; use happens-before and the race detector.
- For active phase scope, read `.planning/phases/implementation-sequence.md` and the current phase README before using `plans/implementation-plan.md` as background.

## Codex Models

- Orchestration and task sequencing: `gpt-5.5`, medium reasoning.
- Architecture and cross-surface decisions: `gpt-5.5`, high reasoning.
- Broad repo study and docs synthesis: `gpt-5.4-mini`, medium reasoning.
- Normal implementation and refactoring: `gpt-5.4`, medium reasoning.
- High-risk implementation or review: `gpt-5.5`, high reasoning.
- Small mechanical edits: fastest small coding model available.

## Codex Delegation

- In orchestrator mode, delegate repo reading, implementation, validation, and review.
- Outside orchestrator mode, use subagents only when parallel study or independent review materially reduces risk or time.
- For broad architecture review, prefer 2-4 narrow read-only readers plus one independent reviewer.
