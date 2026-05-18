# Provenant CLAUDE

Use repo-local guidance from `.claude/skills/` and `.agents/` when it matches. Prefer the smallest applicable guidance set.

## Sources

- Prefer repo-local skills for orchestration, architecture review, workflow, and commit/PR hygiene.
- Keep skill usage additive and narrow. For implementation-specific work, load only the extra skills required for that task.

## Personas

- Implementation: `.agents/engineer.md`
- Review: `.agents/reviewer.md`

## Skills

- `conventional-commit-guard`: before creating, amending, squashing, or suggesting any commit message, and before opening or updating a PR title.
- `orchestrator-mode`: when the user asks Claude to coordinate through subagents only or to enter orchestrator mode.
- `engineering-workflow`: default process for branching, commits, validation, review cycles, and PR lifecycle.

## Claude Models

- Orchestration and task sequencing: `claude-sonnet-4-6`, thinking off.
- Architecture and cross-surface decisions: `claude-opus-4-7`, thinking on.
- Broad repo study and docs synthesis: `claude-haiku-4-5`, thinking off.
- Normal implementation: `claude-sonnet-4-6`, thinking off.
- High-risk implementation or review: `claude-opus-4-7`, thinking on.
- Small mechanical edits: `claude-haiku-4-5`, thinking off.

## Claude Delegation

- In orchestrator mode, delegate repo reading, implementation, validation, and review.
- Outside orchestrator mode, use subagents only when parallel study or independent review materially reduces risk or time.
- For broad architecture review, prefer 2-4 narrow read-only readers plus one independent reviewer.
