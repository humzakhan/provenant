---
name: go-input-validation-security
description: Use for strict Go input handling, decoding, validation, and security-sensitive parsing.
---

# Go Input Validation and Security

Use this skill when Go code accepts or interprets input.

## Triggers
`validation`, `decode`, `parser`, `input`, `security`, `allowlist`, `size limit`, `depth limit`, `bounded read`, `coercion`, `strict`

## Rules
- Decode strictly and fail closed.
- Bound reads, sizes, recursion, and collection growth.
- Prefer allowlists over denylists.
- Reject ambiguous or unsupported forms instead of coercing silently.
- Validate at the boundary before data reaches deeper logic.
