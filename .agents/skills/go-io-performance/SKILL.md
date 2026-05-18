---
name: go-io-performance
description: Use for Go I/O code that should stream, buffer, chunk, and avoid unnecessary materialization.
---

# Go I/O Performance

Use this skill for reader/writer-heavy Go code.

## Triggers
`io.Reader`, `io.Writer`, `buffer`, `chunk`, `stream`, `copy`, `I/O`, `memory use`, `throughput`, `latency`

## Rules
- Prefer `io.Reader` and `io.Writer` interfaces at boundaries.
- Buffer and chunk work when it reduces allocations or syscalls.
- Avoid reading the full payload into memory unless it is clearly justified.
- Preserve deterministic ordering where outputs are compared or hashed.
- Measure before micro-optimizing.
