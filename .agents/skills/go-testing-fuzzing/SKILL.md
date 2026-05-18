---
name: go-testing-fuzzing
description: Use for Go tests, table tests, golden tests, fuzz targets, and invariant checks.
---

# Go Testing and Fuzzing

Use this skill for Go test design.

## Triggers
`test`, `table test`, `golden`, `fuzz`, `invariant`, `parser`, `serialization`, `path handling`, `external input`, `boundary case`

## Rules
- Prefer table-driven tests for behavior matrices.
- Use golden tests when output shape matters and should stay readable.
- Add fuzz targets for external input, parsers, serialization, and path handling.
- Test invariants and failure modes, not only happy paths.
- Keep fixtures small and deterministic.
