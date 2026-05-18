---
name: go-implementation-quality
description: Use for general Go implementation work that should follow Effective Go and Go Code Review Comments.
---

# Go Implementation Quality

Use this skill for ordinary Go coding work.

## Triggers
`Go`, `Effective Go`, `API design`, `package boundaries`, `errors`, `small API`, `abstraction`, `idiomatic Go`, `code review`

## Rules
- Prefer small, focused APIs and packages.
- Keep abstractions concrete until there is a clear repeated need.
- Return errors clearly; wrap context without hiding the cause.
- Keep ownership and boundaries obvious; avoid broad package coupling.
- Choose simple code paths that are easy to test and review.
