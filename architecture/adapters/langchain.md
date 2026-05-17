# Adapter: LangChain

Status: **v0.5 / deferred**

> A Python adapter for LangChain agents. Planned for v0.5. This document establishes the slot and the design constraints; implementation is deferred until v0 ships and we have production signal that LangChain integrations are demanded.

## Why deferred

The v0 priorities are MCP and CLI. MCP captures the largest share of new agent development (Claude Code, Cursor, Claude.ai connectors); CLI covers full-enforcement on-prem deployments. LangChain has a sizable installed base but is a Python-first ecosystem, and adding a second-language adapter before v0 stability would increase the maintenance surface prematurely.

We also believe many LangChain users will reach for the HTTP adapter first (it's already general-purpose) and graduate to a native LangChain adapter only if the friction warrants it.

## Design constraints when we build it

1. **Python package**: `pip install provenant-langchain`. Source in a separate repo (`provenant/provenant-langchain`) to keep the Go monorepo Go-only.
2. **Adapter shape**: a `ProvenantMiddleware` (or `ProvenantCallbacks`) that wraps a LangChain agent's tool and LLM calls.
3. **`submit_answer` enforcement**: provided as a `ProvenantTool` that the LangChain agent must call as its final step. Documented prominently in the package README.
4. **Mode B harness**: same limitations as MCP — LangChain owns the loop; Provenant enforces I/O.
5. **Transport**: gRPC or HTTP to a Provenant server. No in-process Go-Python embedding.
6. **Schemas**: tool schemas exposed as Pydantic models for LangChain's typed tool surface.

## What we will not do

- Re-implement the kernel in Python. The kernel is Go and stays Go.
- Embed a Python interpreter in the Provenant server.
- Ship LangChain-specific guards or validators; all enforcement remains in the kernel.

## Migration path for early adopters

Before v0.5 ships, LangChain users should:

1. Use the HTTP adapter via the `requests`-based client we ship as a one-page example in the docs.
2. Implement `submit_answer` as a LangChain tool that calls `POST /v1/runs/.../submit_answer`.
3. File issues with their LangChain integration friction; these become the v0.5 backlog.

## Schedule

v0.5 timeline depends on v0 production signal. Earliest realistic: 6 months after v0 GA.

## References

- See [`adapters/mcp.md`](mcp.md) and [`adapters/http.md`](http.md) for the v0 surfaces this defers in favor of
- [Arthur AI guardrails](../references.md#arthur-guardrails) — comparable pattern for how Python guardrail libraries integrate with LangChain
