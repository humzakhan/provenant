---
name: engineering-workflow
description: Reusable repo workflow for branching, atomic commits, validation, review cycles, and PR lifecycle.
---

# Engineering Workflow

Use this skill for implementation, review, and documentation work when repo-local workflow should govern branching, validation, and PR behavior.

## Modes

- **Mode A — Orchestrator sequence:** one PR at a time. Finish a slice, review it, merge it once auto-review clears, then branch the next slice from updated `origin/main`.
- **Mode B — Single feature:** one branch, one PR, one feature delivered normally.

`pr-gate` applies only to Mode B. In Mode A, do not stop for human approval at
push, PR, merge, phase-transition, or done boundaries. Instead, run automated
sub-agent review; once review approves and merge criteria pass, squash-merge the
PR, update from `origin/main`, and continue to the next branch.

## Branches

- Create one branch per task from current `main` or `origin/main`.
- Use `feat/*`, `fix/*`, `refactor/*`, `test/*`, `docs/*`, or `chore/*`.
- Never commit directly to `main`.
- Use one PR per branch and rebase instead of merge commits.
- Use squash merge unless a human explicitly approves an exception.
- In a Git worktree, do not switch this worktree back to `main` just to start the next task. After merge, fetch and branch the next feature from updated `origin/main`.
- In a normal repo checkout, switching back to `main` is fine before branching the next task.

## Provenant context loading

- Always load this workflow for implementation, review, and docs work.
- Load the smallest matching Provenant domain skill(s); most tasks need one, occasionally two.
- If a task seems to need all three domain skills, narrow the slice before continuing.
- For current phase work, read `.planning/implementation-plan.md` and the current phase README before treating phase work-packet docs as execution guidance.

## Commits

- Make atomic commits with Conventional Commit subjects.
- Keep commits small, coherent, and independently understandable.
- Do not use `WIP` or bundle unrelated changes.
- Use the `conventional-commit-guard` skill before writing commit messages or PR titles.

## Phase scope guard

- Do not pre-implement later surfaces while doing active phase work: no CLI, HTTP, MCP, runtime persistence, S3/SQLite/Postgres, model harness, PDF/XLSX export, broad reader coverage, or custom validator/plugin infrastructure unless the current phase docs explicitly allow it.
- Keep the active phase slice independently buildable and reviewable.

## Validation

- Prefer the narrowest Go tests that cover the touched packages.
- Phase acceptance should use observable evidence named by the current phase docs, with focused tests for the touched packages.
- Run broader validation only when the repo or task explicitly needs it.
- Record skipped or unavailable checks plainly.

## Review Model Selection

- Default automated code review should use `gpt-5.4`; increase reasoning effort to high when the diff or failure mode warrants it.
- Most code reviews can be handled by `gpt-5.4` with high reasoning.
- Escalate review work to `gpt-5.5`, medium reasoning, when the review is unclear, critical, business-logic-heavy, or security-sensitive.
- Reserve `gpt-5.5`, high reasoning, for decision-making, genuinely high-stakes ambiguity, or cases where repo-level guidance classifies the review as high-risk and requires high reasoning.
- Repo-level model routing in `AGENTS.md` wins if this skill and repo guidance ever differ, especially for high-risk reviews.

## Implementation Loop

1. Start from a task branch and choose Mode A or Mode B.
2. Read only task-relevant context.
3. As soon as the first file or first coherent change is ready, commit it.
4. Open a draft PR immediately after that first valid commit.
5. Continue in small atomic commits.
6. Run relevant validation before marking ready for review.
7. Run at most two automated review cycles by default.
8. If cycle 2 still has major issues, run one final cycle 3 and stop there.
9. In Mode A, merge after automated review approval and passing merge criteria;
   in Mode B, stop at PR boundaries for explicit human approval.

## PR Description

Every PR description should include:

```markdown
## Changes
## Validation
## Risks & Follow-ups
## Security Considerations
```
