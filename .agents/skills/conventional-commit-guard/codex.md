---
name: conventional-commit-guard
description: Use before Codex creates, amends, squashes, or suggests a git commit message, and before Codex opens or updates a pull request title. Enforces Conventional Commit subjects for every commit and PR title without adding hooks or CI checks.
---

# Conventional Commit Guard

Use this skill immediately before writing any commit message or PR title.

## Required Format

Every commit subject and PR title must use:

```text
<type>(optional-scope): <summary>
```

Allowed types:

- `feat`
- `fix`
- `docs`
- `test`
- `refactor`
- `chore`
- `perf`
- `ci`
- `build`

## Rules

- Use a lowercase type.
- Use an optional lowercase scope only when it improves clarity.
- Keep the summary concise, specific, and accurate to the staged change.
- Do not end the subject with a period.
- Do not use `WIP`, `temp`, `savepoint`, `misc`, or vague subjects.
- PR titles must follow the same format as commit subjects.
- Prefer a PR title that would also work as the squash commit title.

## Before Committing

1. Identify the narrowest accurate type.
2. Add a scope only if it helps.
3. Check that the subject matches the actual staged diff.
4. Rewrite any vague or non-conforming message before committing.
