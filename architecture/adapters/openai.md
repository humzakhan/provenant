# Adapter: OpenAI Agents SDK

Status: **v0.5 / deferred**

> A TypeScript adapter for the OpenAI Agents SDK. Planned for v0.5. This document establishes the slot and the design constraints; implementation is deferred.

## Why deferred

Same rationale as the LangChain adapter: MCP and CLI cover the v0 surface area; OpenAI Agents SDK users can reach for the HTTP adapter first.

The OpenAI Agents SDK has a stronger native tool-use story than older frameworks, which makes the adapter both more valuable (better integration possible) and less urgent (HTTP adapter is more usable in the interim).

## Design constraints when we build it

1. **TypeScript package**: `npm install @provenant/openai-agents`. Separate repo.
2. **Adapter shape**: a `ProvenantAgent` wrapper around `Agent` from `@openai/agents`, intercepting tool calls and the `submit_answer` exit.
3. **Token-level type safety**: provenance tokens surface as branded TypeScript types so they cannot be passed to non-token-accepting parameters at compile time. *This is a TS-specific value-add that MCP cannot provide.*
4. **Mode B harness**: agent SDK owns the loop; we enforce I/O.
5. **Transport**: HTTP to the Provenant server.
6. **Schemas**: Zod schemas generated from the same JSON schemas the kernel uses.

## What we will not do

- Re-implement the kernel in TypeScript.
- Ship a Node-hosted Provenant server. The server stays Go.

## Differentiator vs MCP

The Agents SDK is more opinionated than MCP, which lets the adapter offer:

- Compile-time type safety on token-flow paths.
- Native streaming response support tied to our audit stream.
- Direct integration with the SDK's tracing surface — every tool call shows up in OpenAI's tracing UI alongside Provenant's audit log.

These are the reasons the adapter is worth building despite HTTP being available.

## Schedule

v0.5 timeline depends on v0 production signal and on the OpenAI Agents SDK API stabilizing. Earliest realistic: 6 months after v0 GA.

## References

- See [`adapters/mcp.md`](mcp.md) and [`adapters/http.md`](http.md) for the v0 surfaces this defers in favor of
