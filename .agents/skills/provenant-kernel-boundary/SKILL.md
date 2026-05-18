---
name: provenant-kernel-boundary
description: Use when changing Provenant kernel public API, memio, TransactionalScope, injected clock/rand, or any code that could widen the pure-Go kernel surface.
---

# Provenant Kernel Boundary

Use this skill for `kernel/**`, `kernel/public/memio/`, `TransactionalScope`, or any change that could add I/O or runtime/storage coupling.

## Triggers
`kernel`, `kernel/public`, `memio`, `Store`, `AuditSink`, `BlobStore`, `TransactionalScope`, `RunState`, `pure Go`, `no I/O`, `import guard`, `storage concern`

## Rules
- Keep kernel code pure Go and I/O-free: no socket, file, DB, transport, or runtime/storage imports.
- `kernel/public/memio/` is the first-class library-mode binding.
- Persistent bindings live outside `kernel/`.
- Atomic `Store` + `AuditSink` writes go through `TransactionalScope`.
- Inject clock and rand; do not read wall clock or random sources directly in kernel code.
- Any new kernel-public type must match the spec/snapshot/godoc contract.
