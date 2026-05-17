# ADR-0007: MCP as the primary adapter

Status: **Superseded by [ADR-0010](0010-mcp-equal-not-primary.md) (2026-05-17)**
Date: 2026-05-17

> **Supersession note (cycle 4, 2026-05-17).** This ADR's "MCP is primary; HTTP is secondary; CLI is full-enforcement" framing contradicts [`architecture.md`](../../architecture.md) §1 (which depicts HTTP, CLI, and MCP as adapter-shim equals) and §2.1.3 (which explicitly flags MCP-first-vs-MCP-equal as an open question, since resolved). [ADR-0010](0010-mcp-equal-not-primary.md) supersedes the "MCP-primary" framing: MCP, HTTP, and CLI are equal first-class adapters in v0; the order of shipping is now an engineering-sequencing question, not an architectural privileging. The rationale for MCP support (universal agent protocol, low-friction adoption, etc.) remains valid; the change is that the same rationale also applies, on its own terms, to HTTP and CLI.

## Context

We must decide which integration surface gets the most engineering investment, the most polish, and the cleanest contract for v0. Candidates:

- A custom HTTP API.
- A custom RPC protocol.
- MCP (Model Context Protocol).
- LangChain Python adapter.
- OpenAI Agents SDK TypeScript adapter.
- Direct Go SDK.

The choice shapes which customers adopt first, which integrations require the least work, and how the product distributes.

## Decision

MCP is the primary adapter. The Provenant MCP server is the surface most engineering effort goes into, the surface that gets shipped first, and the surface used in all marketing examples. HTTP is the secondary general-purpose adapter. CLI is the full-enforcement adapter. LangChain and OpenAI Agents adapters are v0.5+.

## Rationale

**MCP has converged into the universal agent protocol.** [Anthropic ships finance agents through MCP](../references.md#anthropic-finance-agents); [Moody's launched their agentic suite via MCP](../references.md#moodys-agentic); Claude Code, Cursor, Claude.ai connectors are all MCP-native. Shipping as an MCP server places Provenant inside every MCP-capable surface for free.

**Tool-protocol focus matches our architecture.** MCP is a protocol for exposing tools and prompts. Provenant's value proposition *is* a typed tool surface plus a guarded exit. The match is near-perfect; we get the protocol's investment in transport, discovery, and auth for free.

**Low-friction adoption.** A customer with Claude Code points it at the Provenant MCP server with one config change and gains a finance-tools surface plus validator-gated `submit_answer`. No SDK install, no code change, no library version pinning.

**Defers cross-language adapter complexity.** LangChain Python and OpenAI Agents TypeScript are real adoption targets, but each is a separate runtime and ecosystem. MCP lets us punt the cross-language adapters to v0.5 without leaving Python or TS users without an integration path — they can use the MCP server, or the HTTP adapter, until the native adapters ship.

## Consequences

**Positive:**
- Single adapter to polish for v0 launch.
- Marketing demos work on Claude Code out of the box.
- The MCP server is portable: any MCP-capable host gains Provenant for free.
- Two co-marketing partners (Anthropic, Moody's) implicitly validate the strategy.

**Negative:**
- Mode B harness limitations (no kernel header guaranteed in host prompts; host can emit non-`submit_answer` outputs) are most visible through MCP. We mitigate via documentation and the CLI adapter for full enforcement.
- MCP versioning is real complexity. We commit to supporting N-1 protocol versions.
- Some customers want HTTP-first or SDK-first integration. They use the HTTP adapter; we're polite but not flexible on which is "primary."

**Out of scope:**
- We don't try to ship a custom RPC protocol. MCP is good enough; inventing a new one would compete with our integration surface.

## Alternatives considered

**HTTP-first.** General-purpose and language-agnostic. Rejected as primary because integration with existing MCP-capable hosts requires the customer to write glue code, defeating the low-friction goal.

**LangChain Python first.** Largest existing-agent installed base. Rejected because Python is a separate ecosystem and the Go kernel cannot embed Python; the adapter would be a separate maintenance burden ahead of v0 stability.

**OpenAI Agents SDK first.** Strong type-safety story; TS-native. Rejected because the Agents SDK is newer and less broadly adopted than MCP for tool integration. Defer to v0.5.

**Multiple primary adapters.** Considered. Rejected — distributing engineering effort across two "primary" surfaces produces two half-polished adapters in v0.

## References

- [Anthropic finance agents](../references.md#anthropic-finance-agents)
- [Moody's MCP launch](../references.md#moodys-agentic)
- See [`adapters/mcp.md`](../adapters/mcp.md) for the implementation
