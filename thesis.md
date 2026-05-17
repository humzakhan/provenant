# Thesis: Provenant

## The Problem

LLM agents writing finance work product — reconciliations, AP coding, variance commentary, sanctions dispositions, SAR drafts — fail in a specific, regulator-disqualifying way: they emit numbers, citations, and policy claims that look correct but have no traceable lineage to source data. When a regulator asks "where did this $1,247,322.18 come from, and who computed the $237.42 residual?", the team cannot answer without re-doing the work by hand. The agent's text is plausible; the underlying chain of evidence is not reconstructable, not tamper-evident, and not reproducible on replay.

Existing agent frameworks (LangChain, OpenAI Agents SDK, raw MCP clients) treat provenance as a logging concern: capture what happened. That is insufficient under [FINRA 2026](architecture/references.md#finra-2026) and [SR 11-7](architecture/references.md#sr-11-7), which require that every consequential decision be reconstructable, that the chain be tamper-evident, and that an independent reviewer be able to replay the reasoning. Logging-after-the-fact does not provide non-bypassability, fail-closed semantics, or the per-number grounding finance teams need to certify model outputs.

A finance team that adopts an agent today either accepts unreviewable output (and the regulatory risk), wraps it in a brittle layer of hand-written validators that the agent can route around, or refuses to deploy at all. None of these is acceptable.

## The Core Thesis

Provenance is an enforcement boundary, not a logging concern: if the substrate makes it architecturally impossible for an agent to release a Provenant-signed answer without verifiable lineage to a tool call, the agent becomes a usable financial analyst regardless of the host framework or model.

The enforcement boundary is the `submit_answer` exit. In Mode A (CLI / embedded Go harness) the substrate owns the loop and `submit_answer` is the only output channel: the non-bypassable invariant is architectural. In Mode B (MCP host driving its own loop) the substrate does not own the host's text channel; the host can in principle emit text that never went through `submit_answer`. Such output is **not** Provenant-signed: it does not appear in the audit pack as a released answer, the audit chain records the absence, and Mode B skill packs must instruct the host that only `submit_answer` output carries Provenant attestations. For deployments where unsigned host-channel output is unacceptable, Mode A is the required deployment shape. See [`architecture/subsystems/harness.md`](architecture/subsystems/harness.md#mode-b-specifics-adapter-side-harness) and [ADR-0007](architecture/adrs/0007-mcp-as-primary-adapter.md).

## The Solution

Provenant is an open-source enforcement substrate for LLM agents in finance. It is not an agent, not a model, and not a workflow — it is the layer that turns whatever agent loop you already have into one whose every numeric claim is grounded, every step audited in a hash-chained log, and every run replay-deterministic on the portions that should be deterministic.

Four kernel invariants are load-bearing and non-configurable (see [`architecture/README.md`](architecture/README.md#core-invariants)): non-bypassable (the only exit is `submit_answer`), fail-closed (errors default to denial), schema-first (every tool, input, and output is typed against a published schema), replay-deterministic (the deterministic portion of every run is bit-identical on replay).

**Dual-mode pluggability is a first-class property.** Provenant must be usable in two architecturally distinct shapes, with identical invariants in each:

1. **Code-level (embedded library/SDK).** A host application that already owns its agent loop links Provenant as a dependency. The kernel exposes a pure, I/O-free public API; the host drives tool dispatch, guard application, and `submit_answer` enforcement via function calls. The invariants hold because they are encoded in the kernel's types and pure functions, not in a process boundary. **In v0, the embedded path is Go-only**: Go hosts link the kernel directly; non-Go hosts (Python, TypeScript) reach the same kernel through the runtime/sidecar shape below until Phase 5 of [`implementation-plan.md`](implementation-plan.md) delivers a non-Go binding (FFI or sub-50ms loopback sidecar). The "identical invariants" claim is a Phase-5 deliverable for non-Go hosts; v0 ships the Go embedded mode and the runtime mode and proves byte-identity between them.

2. **Runtime-level (sidecar / MCP server / daemon).** A host that prefers process isolation, a non-Go runtime, or zero-code integration points at a Provenant runtime — an MCP server, an HTTP API, or a CLI harness — over the wire. The invariants hold because the runtime layer is a thin shell that delegates every enforcement decision to the same kernel that the embedded mode exposes.

These two modes share the kernel binary-equivalently. The audit chain produced by a run in embedded mode and the same run in MCP mode is byte-identical on its deterministic portions. This is the property that makes Provenant a substrate rather than a product.

The detailed architecture, including subsystem decomposition, ADRs, and the dual-mode module boundary, lives in [`architecture.md`](architecture.md) and under [`architecture/`](architecture/).

## What This Is Not

- **Not an agent.** Provenant does not ship a finance analyst. It ships the substrate that makes someone else's agent a usable finance analyst. The reconciliation skill pack in v0 is a demonstration, not the product.
- **Not a model.** Provenant is model-pluggable. v0 verifies one model class (Claude) end-to-end; the harness abstraction supports any chat-completion or tool-use-capable model.
- **Not a workflow engine.** Skill packs are configuration plus prompts; the substrate does not encode business processes beyond what skills declare.
- **Not a replacement for the GL, the ERP, or the document extractor.** Provenant wraps these as tools; it does not own them. See [`architecture/tools/README.md`](architecture/tools/README.md).
- **Not a hosted SaaS in v0.** Multi-tenant SaaS is deferred to v0.5; v0 ships single-tenant and air-gapped.
- **Not a guardrail vendor for chat assistants.** The provenance and audit story is overkill for general-purpose chat; it is calibrated for regulated finance work product.
- **Not framework-coupled.** Provenant does not require LangChain, OpenAI Agents, or any specific framework. Integrations are adapter shims; the kernel is independent.

## Who This Is For

- **A controller or audit lead** at a mid-market or enterprise finance organization who has been told to "deploy AI" and is responsible for the quality of the output. They currently either reject agent output entirely or absorb unreviewable risk; they need a substrate they can show their regulator.
- **A platform engineer at a fintech infrastructure company** building an agent product (a recon SaaS, an AP automation tool, a SAR-drafting copilot). They currently maintain their own validators, audit logging, and replay infrastructure as undifferentiated heavy lifting; they would adopt a substrate that does this for them if it did not constrain their agent loop choices.
- **An MCP host vendor or agent framework author** who wants to make their host viable for regulated finance use cases without rebuilding the enforcement layer themselves. They need a drop-in MCP server (zero code on their side) or an embeddable library (if their host is Python or TypeScript) that brings the enforcement with it.
- **A compliance officer or model risk reviewer** asked to sign off on agent-produced work product under SR 11-7. They need the hash-chained audit log, the replay tooling, and the four DFAH-style metrics in their hand before they will approve a deployment.

## Why Existing Solutions Fail

- **Agent frameworks (LangChain, OpenAI Agents SDK, vanilla MCP).** Treat tool calls as opaque function invocations. No notion of provenance tokens, no enforcement boundary at output emission, no hash-chained audit log, no replay-determinism guarantee. The output is the agent's text; whether the numbers in that text correspond to real data is the integrator's problem.
- **General-purpose guardrail products (Arthur, Lakera, NeMo Guardrails).** Useful for chat assistants. Do not provide token-level numeric grounding, do not enforce a single-exit pattern, do not produce a regulator-grade audit pack, and do not give you replay-determinism on the deterministic portion of a run. See [`architecture/references.md`](architecture/references.md#arthur-guardrails); the pre/post split is the right pattern, but the implementations are not calibrated for finance work product.
- **In-house validator layers.** Every serious finance-agent team builds one. They are brittle: the agent can route around them (no architectural non-bypassability), they fail-open by default (a missing validator silently passes), they are not hash-chained (a regulator cannot certify tamper-evidence), and they are not replayable (the team cannot answer "what would the same agent do on the same input tomorrow?"). Building this layer correctly is months of work that does not differentiate the product on top.
- **Vendor-specific compliance modes (Moody's modular AI, Brex internal frameworks).** Do this well for one vendor's products. Not available as substrate; not usable by a competing team. See [`architecture/references.md`](architecture/references.md#moodys-modular).
- **Logging-and-review platforms (LangSmith, LangFuse, Helicone).** Excellent observability. Not enforcement. The unreviewable output already shipped to the user before the trace was written.

The common failure mode: the enforcement is in the prompt or in optional middleware. Provenant moves the enforcement into the architecture, where the agent cannot ignore it and the integrator cannot accidentally disable it.

## What Success Looks Like

- A finance team adopts Provenant and stops hand-checking agent output for ungrounded numbers, because ungrounded numbers cannot leave the substrate. The team's review effort shifts from "did the agent hallucinate?" to "did the agent reason well over real data?".
- A regulator asks "how did this answer come to be?" and the team runs `provenant audit pack <run_id>` and hands over a tamper-evident bundle that reconstructs every tool call, every token mint, every guard outcome, and the final answer with its claim-to-token mapping. The conversation is about the reasoning, not about whether the evidence exists.
- A platform engineer building a new finance-agent product imports the Provenant Go kernel as a library, wires it into their existing agent loop, and ships with full enforcement in days rather than building it themselves over months. The same engineer with a Python codebase points their agent at a Provenant sidecar over MCP and gets the same enforcement without rewriting their stack.
- An MCP host vendor adds Provenant as a connector in their marketplace and immediately ships a regulated-finance-capable surface to their customers, without writing enforcement code.
- The same reconciliation case, run on the same skill pack and inputs, produces byte-identical deterministic-portion audit chains across embedded-library mode, MCP-server mode, and CLI mode. This is the dual-mode pluggability promise made concrete.
- A model risk reviewer signs off on a Provenant-backed deployment under SR 11-7 by reviewing the four DFAH-style metrics (trajectory determinism, decision determinism, faithfulness, accuracy) from the case bank, the threat-model documentation, and a sampled audit pack. The substrate carries the evidentiary burden; the reviewer focuses on the skill, not the plumbing.
