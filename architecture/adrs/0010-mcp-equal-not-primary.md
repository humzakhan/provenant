# ADR-0010: MCP, HTTP, and CLI are equal first-class adapters

Status: **Accepted**
Date: 2026-05-17
Supersedes: [ADR-0007](0007-mcp-as-primary-adapter.md) ("MCP is the primary adapter; HTTP is secondary; CLI is full-enforcement").

## Context

[ADR-0007](0007-mcp-as-primary-adapter.md) was written under the framing that distributing engineering effort across multiple "primary" adapters would produce two half-polished surfaces in v0. Subsequently, [`architecture.md`](../OVERVIEW.md) §1's diagram and §2.1.3's open question reframed the three adapters (MCP, HTTP, CLI) as architectural equals — each backed by the same runtime-layer translation of a single kernel public surface ([ADR-0009](0009-kernel-public-go-surface.md)). The cycle-3 critic flagged this contradiction; the cycle-4 critic raised it again as an ADR-vs-architecture inconsistency that requires explicit resolution.

## Decision

MCP, HTTP, and CLI are **equal first-class adapters** in v0. None of the three has architectural primacy. All three are thin runtime-layer shells over the kernel public Go surface; all three are subject to the same conformance tests; all three must produce byte-identical deterministic-portion audit chains on the same input (the Phase 4 done-when in [`implementation-plan.md`](../../plans/implementation-plan.md)).

The engineering sequence in which the three adapters ship is a project-management concern, not an architectural one. v0 plan-of-record: CLI in Phase 3 (it embeds the Mode A harness directly); MCP and HTTP in Phase 4 (they are runtime-layer surfacings of the same kernel). Marketing emphasis remains on MCP because of the host-ecosystem reach; that is a go-to-market choice, not an architectural privileging.

## Rationale

- The dual-mode pluggability promise in [`thesis.md`](../../docs/thesis.md) is mode-agnostic: a regulated finance buyer running CLI in air-gapped mode and a Claude Code user running MCP must receive identical enforcement and identical audit packs. Calling MCP "primary" implied that HTTP and CLI would receive less testing, less polish, and weaker enforcement guarantees — none of which is true and none of which is what we want to commit to.
- The cycle-3 critic correctly observed that ADR-0007's framing contradicted [`architecture.md`](../OVERVIEW.md) §1 (HTTP/CLI/MCP as adapter-shim equals). The cycle-4 critic correctly observed that the contradiction was still unresolved. Internal consistency of the spec set is a hard requirement of the Hone loop; this ADR is the resolution.
- "MCP-first because Anthropic and Moody's ship through it" is a real co-marketing benefit but does not require architectural primacy. The same kernel and runtime are available through HTTP and CLI on day one.

## Consequences

**Positive:**
- The ADR set is internally consistent with [`architecture.md`](../OVERVIEW.md) §1 and §2.1.3.
- Customers who must use HTTP (e.g., a regulated finance buyer whose host vendor does not speak MCP) are not second-class adopters.
- The Phase 4 done-when ("the same fixture run via HTTP API directly produces the same byte-identical deterministic-portion audit chain") gains explicit ADR backing.

**Negative:**
- Three adapters at "equal first-class" polish is more conformance-suite work than one primary plus two secondary. The mitigation is that the runtime-layer shells are deliberately thin and share most code; the conformance suite is shared across them.
- Some marketing copy and developer documentation written under the MCP-primary framing must be revisited.

## Alternatives considered

**Keep ADR-0007 as written; mark [`architecture.md`](../OVERVIEW.md) §1 as illustrative-only.** Rejected — [`architecture.md`](../OVERVIEW.md) §1 is the synthesis view, not a sketch. The diagram is load-bearing for the dual-mode promise.

**Amend ADR-0007 in place with a "scope-corrected" header rather than full supersession.** Rejected — the original decision text ("MCP is primary, HTTP is secondary, CLI is full-enforcement") is what is being reversed, not a side clause. Full supersession is cleaner.

**Reduce v0 to MCP-only (true MCP primary).** Rejected — air-gapped customers and HTTP-first customers exist now, not in v0.5.

## References

- [`thesis.md`](../../docs/thesis.md) — dual-mode pluggability
- [`architecture.md`](../OVERVIEW.md) §1, §2.1.3, §3.3 — adapter equality and the now-resolved open question
- [`implementation-plan.md`](../../plans/implementation-plan.md) Phase 3, Phase 4 — CLI in Phase 3, MCP and HTTP in Phase 4
- [ADR-0007](0007-mcp-as-primary-adapter.md) — superseded ADR
- [ADR-0009](0009-kernel-public-go-surface.md) — kernel public Go surface that all three adapters rest on
