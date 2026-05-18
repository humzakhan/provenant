---
name: engineering-workflow
description: Reusable repo workflow for branching, atomic commits, validation, review cycles, and PR lifecycle.
---

# Engineering Workflow

Use this skill for implementation, review, and documentation work when a repo-local workflow should govern branching, validation, and PR behavior.

## Branches

- Create one branch per task from current `main` or `origin/main`.
- Use `feat/*`, `fix/*`, `refactor/*`, `test/*`, `docs/*`, or `chore/*`.
- Never commit directly to `main`.
- Use one PR per branch and rebase instead of merge commits.
- Use squash merge unless a human explicitly approves an exception.

## Commits

- Make atomic commits with Conventional Commit subjects.
- Keep commits small, coherent, and independently understandable.
- Do not use `WIP` or bundle unrelated changes.
- Use the `conventional-commit-guard` skill before writing commit messages or PR titles.

## Context Loading

- Read only the documents and sections needed for the current task.
- Prefer search and targeted reads over broad full-file reads.
- Do not reread already-loaded guidance unless it changed or a fresh check is needed.

## Implementation Loop

1. Start from a task branch.
2. Read only task-relevant context.
3. Implement in small atomic commits.
4. Open a draft PR after the first valid commit.
5. Run relevant validation before marking ready for review.
6. Run at most two automated review cycles by default.
7. If cycle 2 still has major issues, run one final cycle 3 and stop there.

## Validation

- Run the narrowest checks that cover the changed behavior while iterating.
- Run broader validation before declaring work done when the repo expects it.
- Record skipped or unavailable checks plainly.

## PR Description

Every PR description should include:

```markdown
## Changes
## Validation
## Risks & Follow-ups
## Security Considerations
```
