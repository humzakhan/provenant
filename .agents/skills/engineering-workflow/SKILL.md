---
name: engineering-workflow
description: Reusable repo workflow for branching, atomic commits, validation, review cycles, and PR lifecycle.
---

# Engineering Workflow

Use this skill for implementation, review, and documentation work when a repo-local workflow should govern branching, validation, and PR behavior.

## Modes

- **Mode A — Orchestrator sequence:** one peer/PR at a time. Finish a peer slice, review it, merge it once auto-review clears, then branch the next peer from updated `origin/main`.
- **Mode B — Single feature:** one branch, one PR, one feature delivered normally.

## Branches

- Create one branch per task from current `main` or `origin/main`.
- Use `feat/*`, `fix/*`, `refactor/*`, `test/*`, `docs/*`, or `chore/*`.
- Never commit directly to `main`.
- Use one PR per branch and rebase instead of merge commits.
- Use squash merge unless a human explicitly approves an exception.
- In a Git worktree, do not switch this worktree back to `main` just to start the next task. After merge, fetch and branch the next feature from updated `origin/main`.
- In a normal repo checkout, switching back to `main` is fine before branching the next task.

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

1. Start from a task branch and choose Mode A or Mode B.
2. Read only task-relevant context.
3. As soon as the first file or first coherent change is ready, commit it.
4. Open a draft PR immediately after that first valid commit.
5. Continue in small atomic commits.
6. Run relevant validation before marking ready for review.
7. Run at most two automated review cycles by default.
8. If cycle 2 still has major issues, run one final cycle 3 and stop there.

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
