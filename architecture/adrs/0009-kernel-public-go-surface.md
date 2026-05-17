# ADR-0009: Kernel public Go surface as v0 first-class

Status: **Accepted**
Date: 2026-05-17
Supersedes: the "Out of scope" clause of [ADR-0001](0001-go-as-kernel-language.md) ("the kernel will not provide a Go-only public API to consumers; the public surface is MCP / HTTP / CLI").

## Context

[ADR-0001](0001-go-as-kernel-language.md) was written when "code-level embedding" was explicitly out of scope. Subsequently, the thesis was sharpened (see [`thesis.md`](../../docs/thesis.md) "Dual-mode pluggability is a first-class property") to require that Provenant be usable in two architecturally distinct shapes: runtime-level (MCP/HTTP/CLI) and code-level (embedded library/SDK). [`architecture.md`](../OVERVIEW.md) §3.1 and §3.4 codify a kernel public surface that consumers can call directly as a Go library; Phase 5 of [`implementation-plan.md`](../../plans/implementation-plan.md) commits to a non-Go binding (FFI or sidecar). The ADR-0001 out-of-scope clause is now in direct contradiction with the spec.

## Decision

The Provenant kernel provides a **Go-only public API** as a v0 first-class surface. Go hosts link the kernel directly. The runtime layer (MCP/HTTP/CLI) is built on top of the same public surface, not in addition to a private one.

Non-Go hosts reach the kernel through the runtime layer in v0 (sidecar / MCP / HTTP). A non-Go binding (FFI or strict-latency sidecar) is a Phase 5 deliverable. The choice between FFI and sidecar is gated on Phase 0-4 ergonomics learnings and is itself an ADR concern (Phase 5a).

The kernel's public Go API is the surface that the import-guard check ([`implementation-plan.md`](../../plans/implementation-plan.md) Phase 0) constrains: no `net/*`, `database/sql`, MCP/HTTP/CLI types are permitted in any package transitively reachable from the public API.

## Rationale

- The thesis claim that "the same kernel binary-equivalently underlies both modes" requires a public surface that the runtime layer itself depends on. Two parallel surfaces (a hidden one for the runtime and a public one for embedders) would double the maintenance burden and create silent divergence.
- The "byte-identical audit chain across embedded and runtime modes" property in [`thesis.md`](../../docs/thesis.md) and [`architecture.md`](../OVERVIEW.md) §5 can only hold if both modes drive the same kernel functions; a Go public surface is what makes that mechanically true.
- Forbidding a Go public surface would push every Go-host integration through MCP-over-loopback, paying transport overhead with no architectural benefit — directly contradicting the latency budget in [`architecture.md`](../OVERVIEW.md) §7.3.

## Consequences

**Positive:**
- One surface, one set of contracts, one set of tests.
- Phase 5's binding work is a single additional surface, not a refactor of an existing private one.
- Go fintech hosts can adopt Provenant as a library in v0 without waiting for a binding.

**Negative:**
- The public Go API is a contract. Breaking changes require a kernel-version bump and migration guidance.
- Hosts that misuse the public surface (injecting mismatched `Store`/`AuditSink` substrates, bypassing the `TransactionalScope`) can produce broken runs. The kernel's runtime checks (substrate-marker check at boot, tx-local canary at write) close the worst of these holes; documentation and conformance tests close the rest.

## Alternatives considered

**Keep the ADR-0001 out-of-scope clause; add a private internal Go API for the runtime layer only.** Rejected — see Rationale: two parallel surfaces silently diverge, and the thesis claim depends on them being the same.

**Defer Go public surface to v0.5.** Rejected — Phase 0 of [`implementation-plan.md`](../../plans/implementation-plan.md) already carves out the public surface as the first phase. Deferral would re-do that work and stall every downstream Go-host integration partner.

## References

- [`thesis.md`](../../docs/thesis.md) — dual-mode pluggability requirement
- [`architecture.md`](../OVERVIEW.md) §3.1, §3.4, §5.1 — kernel public surface and runtime/embedded equivalence
- [`implementation-plan.md`](../../plans/implementation-plan.md) Phase 0, Phase 5 — kernel public surface carve-out; non-Go binding
- [ADR-0001](0001-go-as-kernel-language.md) — superseded out-of-scope clause
