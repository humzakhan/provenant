---
name: go-concurrency-memory-model
description: Use only for Go concurrency, shared state, synchronization, and memory-model-sensitive code.
---

# Go Concurrency and Memory Model

Use this skill only when goroutines, channels, mutexes, atomics, or shared state are involved.

## Triggers
`goroutine`, `channel`, `mutex`, `atomic`, `race`, `shared state`, `sync`, `context cancellation`, `happens-before`

## Rules
- Rely on clear happens-before edges; do not assume visibility without synchronization.
- Avoid unsynchronized shared memory.
- Use the race detector as part of validation when concurrency changes.
- Prefer explicit cancellation and bounded lifetimes with `context.Context`.
- Keep coordination simple and testable.
