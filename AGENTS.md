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
