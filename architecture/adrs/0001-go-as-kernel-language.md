# ADR-0001: Go as the kernel language

Status: **Accepted, scope-amended by [ADR-0009](0009-kernel-public-go-surface.md) (2026-05-17)**
Date: 2026-05-17

> **Amendment note (cycle 4, 2026-05-17).** The "Out of scope" clause below - "the kernel will not provide a Go-only public API to consumers; the public surface is MCP / HTTP / CLI" - was correct at the time it was written but contradicted the subsequent elevation of code-level embeddability to a first-class thesis requirement (see [`thesis.md`](../../docs/thesis.md), [`architecture.md`](../OVERVIEW.md) sections 3.1 and 3.4). [ADR-0009](0009-kernel-public-go-surface.md) supersedes that specific out-of-scope clause: the kernel **does** provide a Go-only public API as a v0 first-class surface, and a non-Go binding is a future-phase deliverable. The rest of this ADR (Go as the kernel language choice itself) remains accepted.

## Context

The Provenant kernel is the substrate for an open-source enforcement layer for financial agents. The language choice constrains the next several years of contributor experience, deployment shape, and operational characteristics. Constraints:

- Single static binary deployment (air-gapped customers exist).
- Concurrent tool dispatch and audit log appends, on the order of hundreds of operations per second per tenant.
- A type system strong enough to enforce token-typed flow at compile time.
- Mature ecosystem for HTTP, MCP, Postgres, JSON schema, decimal arithmetic.
- Wide hireability for the contributor base — financial-engineering-adjacent backend developers.

## Decision

The Provenant kernel is implemented in Go. All adapters that run in the kernel process (MCP, HTTP, CLI) are Go. Non-Go adapters (LangChain Python, OpenAI Agents TypeScript) are out-of-process clients.

## Rationale

**Single binary deployment.** Go produces a single static binary, which is the simplest possible deployment shape. This matters disproportionately for customers running on-prem or air-gapped — common in regulated finance. Rust would offer the same property; Python and Node would not.

**Concurrency model.** Goroutines and channels fit our workload: many independent tool calls, audit log appends serialized per tenant, kernel-level fan-out to validators. The model is well-understood by mid-level backend developers; the contributor onboarding cost is low compared to Rust.

**Maturity of the ecosystem.** `pgx`, `sqlc`, `atlas` (Postgres path); `shopspring/decimal` (monetary arithmetic); `chi` (HTTP); the official MCP Go SDK — all are production-mature. The xx-grade libraries we need exist.

**Hireability.** Backend Go engineers are common in fintech infrastructure. The contributor pool for an OSS project is broader than Rust; the cost of correctness review for community PRs is lower than dynamic languages.

**Strong-enough type system.** Go's type system enforces what we need (interfaces for tool contracts, typed schemas, decimal types) without the cognitive overhead of dependent types or borrow checking. We trade some safety we could get from Rust for productivity and contributor accessibility.

## Consequences

**Positive:**
- Fast iteration on the kernel.
- Easy operational story: one binary, one config file, one Postgres instance.
- Standard testing, profiling, and tracing tooling.
- The Go MCP SDK becomes our primary integration interface effectively for free.

**Negative:**
- Generic programming is less ergonomic than in Rust or Haskell. Our tool registry leans on `interface{}` and runtime type assertions in places where dependent typing would prevent the bug.
- We do not get Rust's memory-safety guarantees. We mitigate via standard practices: no `unsafe`, careful concurrency review.
- Cross-language adapters (Python, TS) run out-of-process. Not a real loss — embedding cross-language was never a goal.

**Out of scope (superseded by [ADR-0009](0009-kernel-public-go-surface.md)):**
- ~~The kernel will not provide a Go-only public API to consumers; the public surface is MCP / HTTP / CLI. Consumers can use any language they prefer.~~ **Superseded.** The kernel provides a Go-only public API as a v0 first-class surface for code-level embedding; non-Go consumers reach the kernel through the runtime layer (MCP/HTTP/CLI) until Phase 5 ships a non-Go binding. See [ADR-0009](0009-kernel-public-go-surface.md).

## Alternatives considered

**Rust.** Stronger safety guarantees and equally fast. Rejected because: the contributor cohort is narrower in fintech ops; build times slow contributor iteration; the marginal safety benefit doesn't outweigh the productivity cost for our domain (we are not writing a kernel for a real-time OS).

**Python.** Familiar to the data-science community that builds LLM agents. Rejected because: GIL constrains concurrent tool dispatch under load; single-binary deployment is awkward; decimal-arithmetic discipline is harder to enforce in a dynamic language; ecosystem for Postgres-backed services is weaker.

**TypeScript / Node.** Async story is fine. Rejected because: less mature for the Postgres-heavy data path; single-binary deployment via `pkg`/`nexe` is awkward; the contributor cohort in fintech ops is smaller than Go's.

**Java / Kotlin.** Strong type system, mature ecosystem. Rejected because: JVM operational overhead (heap tuning, GC) is unnecessary for our workload; air-gapped deployments are more complex than a single static binary.

## References

- No academic paper is load-bearing here; this is an engineering decision.
- See [`subsystems/storage.md`](../subsystems/storage.md) for the Postgres-binding choice that Go ergonomics shape.
