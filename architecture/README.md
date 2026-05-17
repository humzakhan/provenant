# Provenant Architecture

This is the architecture documentation for Provenant — an open-source enforcement substrate that turns any LLM agent into a financial analyst with full provenance over everything it produces. Start with [`docs/thesis.md`](../docs/thesis.md) and [`product.md`](../product.md) before this if you have not read them.

## What lives where

```
architecture/
├── README.md              <- you are here
├── references.md          canonical paper + blog citation list
│
├── subsystems/            one doc per kernel module
├── adapters/              integration surfaces (MCP, HTTP, CLI, LangChain, OpenAI)
├── tools/                 tool category specs + per-backend specs
├── adrs/                  Architecture Decision Records — the *why*
└── cross-cutting/         security, observability, deployment
```

A new contributor should read this README, then [ADR-0001](adrs/0001-go-as-kernel-language.md) through [ADR-0005](adrs/0005-determinism-faithfulness-independent.md), then whichever subsystem they're about to touch.

## Reading paths

**Joining the project:** [`docs/thesis.md`](../docs/thesis.md) → `product.md` → this README → ADRs 0001–0005 → the subsystem you're touching.

**Integrating Provenant:** [`docs/thesis.md`](../docs/thesis.md) → `product.md` → [`adapters/mcp.md`](adapters/mcp.md) (or your adapter) → the [`tools/<category>/README.md`](tools/README.md) for the tools you'll use.

**Adding a new tool:** [`tools/README.md`](tools/README.md) (the conformance checklist) → the closest existing tool spec → relevant ADR if your tool challenges a design assumption.

## High-level architecture

```
                          ┌────────────────────────────────┐
                          │           Adapters             │
       ┌──────────┬───────┴───┬──────────┬──────────┬─────┴─────────┐
       │   MCP    │   HTTP    │ LangChain│ OpenAI   │  CLI Harness  │
       │  Server  │   API     │  Adapter │ Adapter  │               │
       └────┬─────┴─────┬─────┴────┬─────┴────┬─────┴───────┬───────┘
            │           │          │          │             │
            └───────────┴──────────┼──────────┴─────────────┘
                                   │
                  ┌────────────────▼────────────────┐
                  │       Provenant Kernel          │
                  │ ┌─────────────────────────────┐ │
                  │ │ Provenance Engine           │ │
                  │ │ Harness (agent loop)        │ │
                  │ │ Guard Pipeline              │ │
                  │ │   ├─ Pre-LLM input guards   │ │
                  │ │   └─ Post-LLM validators    │ │
                  │ │ Tool Registry + Audit MW    │ │
                  │ │ Skill Loader                │ │
                  │ │ Case Bank + Eval Harness    │ │
                  │ │ Audit Logger (hash-chained) │ │
                  │ │ HITL Router                 │ │
                  │ └─────────────────────────────┘ │
                  └────────────────┬────────────────┘
                                   │
                  ┌────────────────▼────────────────┐
                  │           Storage               │
                  │  Postgres  +  Object Store (S3) │
                  └─────────────────────────────────┘
```

The kernel exposes a single internal API surface. Every adapter is a translation between that surface and the host framework's idioms.

## Core invariants

Four kernel-level invariants. Every component preserves them; no configuration can disable them.

1. **Non-bypassable.** The only exit for an agent is `submit_answer`. There is no other path that returns data to the user. See [ADR-0002](adrs/0002-submit-answer-as-single-exit.md). *(From [Faramesh, 2026](references.md#faramesh).)*
2. **Fail-closed.** Validator error, tool error, schema mismatch, token verification failure — all default to denial. Silent pass is never the default. See [ADR-0003](adrs/0003-fail-closed-default.md). *(From [Faramesh, 2026](references.md#faramesh).)*
3. **Schema-first.** Every tool, validator, input guard, and output is typed against a published JSON schema. The kernel rejects malformed traffic before LLM exposure and before downstream consumption. *(From [IBM DFAH, 2026](references.md#dfah): schema-first architectures dominate determinism under stress.)*
4. **Replay-deterministic.** Every run's deterministic portion (tool calls, validator outcomes, audit trail, tokens minted) is bit-identical on replay. LLM-portion variance is measured and bounded by guards. See [ADR-0005](adrs/0005-determinism-faithfulness-independent.md). *(Per [DFAH](references.md#dfah), which establishes determinism and accuracy as independent metrics.)*

These invariants are the value proposition. Everything else is in service of them.

## Subsystems

| Subsystem | Purpose | Doc |
|---|---|---|
| Kernel (provenance engine) | Tokens, propagation, enforcement | [kernel.md](subsystems/kernel.md) |
| Harness | Agent runtime loop, model abstraction, agent state | [harness.md](subsystems/harness.md) |
| Guard pipeline | Pre-LLM input guards, post-LLM output validators | [guards.md](subsystems/guards.md) |
| Tool registry | Tool registration contract, audit middleware | [tools.md](subsystems/tools.md) |
| Skill loader | Skill pack format and lifecycle | [skills.md](subsystems/skills.md) |
| Storage | Postgres schema, object store, retention | [storage.md](subsystems/storage.md) |
| Audit logger | Hash-chained log, replay, export | [subsystems/audit.md](subsystems/audit.md) |
| Eval harness | Case bank, DFAH-style metrics, benchmarks | [eval.md](subsystems/eval.md) |
| HITL router | Routing, approvals, diff/redline | [hitl.md](subsystems/hitl.md) |

## Adapters

| Adapter | Status | Doc |
|---|---|---|
| MCP server | v0 | [mcp.md](adapters/mcp.md) |
| CLI harness | v0 | [cli.md](adapters/cli.md) |
| HTTP API | v0 | [http.md](adapters/http.md) |
| LangChain | deferred (v0.5) | [langchain.md](adapters/langchain.md) |
| OpenAI Agents SDK | deferred (v0.5) | [openai.md](adapters/openai.md) |

## Tools

The tool layer is the largest surface and the one that grows fastest. See [`tools/README.md`](tools/README.md) for the conformance template every tool must follow.

| Category | Purpose | Doc | v0 backends |
|---|---|---|---|
| Ledger | Read GL / sub-ledger / TB | [ledger/README.md](tools/ledger/README.md) | SQL |
| Extract | Document extraction with confidence | [extract/README.md](tools/extract/README.md) | Textract |
| Reconcile | Set matching with tolerances | [reconcile/README.md](tools/reconcile/README.md) | pure Go |
| Calc | Decimal-safe arithmetic + derived tokens | [calc/README.md](tools/calc/README.md) | pure Go |
| Policy | Cited passage retrieval | [policy/README.md](tools/policy/README.md) | keyword |
| External | Market / FX / sanctions data, versioned | [external/README.md](tools/external/README.md) | OpenFX, OFAC (partial) |

## ADRs

Architecture Decision Records capture the *why* with paper citations. Read these to understand the foundational choices.

| ADR | Decision | Paper backing |
|---|---|---|
| [0001](adrs/0001-go-as-kernel-language.md) | Go as the kernel language | — |
| [0002](adrs/0002-submit-answer-as-single-exit.md) | `submit_answer` as the single exit | [DCCD](references.md#dccd) |
| [0003](adrs/0003-fail-closed-default.md) | Fail-closed by default | [Faramesh](references.md#faramesh) |
| [0004](adrs/0004-token-level-grounding.md) | Token-level grounding (not document-level citation) | — |
| [0005](adrs/0005-determinism-faithfulness-independent.md) | Determinism and faithfulness as independent metrics | [DFAH](references.md#dfah) |
| [0006](adrs/0006-skill-pack-format.md) | Skill pack as the unit of workflow | [Moody's modular AI](references.md#moodys-modular) |
| [0007](adrs/0007-mcp-as-primary-adapter.md) | MCP as the primary adapter | — |
| [0008](adrs/0008-postgres-as-canonical-store.md) | Postgres as the canonical transactional store | — |

## Cross-cutting concerns

| Concern | Doc |
|---|---|
| Security: threat model, prompt injection, isolation | [security.md](cross-cutting/security.md) |
| Observability: metrics, tracing, logs | [observability.md](cross-cutting/observability.md) |
| Deployment: local, single-tenant, multi-tenant, air-gapped | [deployment.md](cross-cutting/deployment.md) |

## Conventions

**Doc structure.** Every subsystem, adapter, and tool doc starts with `Depends on:` and `Depended on by:` link-only headers. Other docs link laterally, but only this README maintains a full navigation table.

**Paper citations.** Live in [`references.md`](references.md) under stable anchors. Other docs link there rather than restating URLs.

**Decision rationale.** Lives in ADRs. Subsystem docs describe what is; ADRs describe why. If you find yourself writing rationale in a subsystem doc, you probably want an ADR.

**Status labels.** Docs are tagged at the top with one of: `v0`, `v0.5 (deferred)`, `v1 (deferred)`, `Planning`. Deferred docs ship as short specs that establish the slot.

## Open questions tracker

Open architectural questions across all docs are aggregated in subsystem-level "Open questions" sections. Resolution moves to an ADR.

## v0 scope summary

- Kernel: provenance + guard pipeline (pre + post) + tool registry + audit log + case bank + eval harness.
- Kernel invariants enforced: non-bypassable, fail-closed, schema-first, replay-deterministic.
- Adapters: MCP, HTTP, CLI.
- One skill pack: reconciliation.
- Tool backends shipped: SQL ledger, Textract extract, keyword policy, OpenFX external, OFAC partial.
- One model class verified: Claude.
- Eval: four-metric reporting (trajectory determinism, decision determinism, faithfulness, accuracy).
- CLI: `run`, `replay`, `cases`, `audit`, `audit pack`, `skill`, `tools`.

Deferred: LangChain and OpenAI adapters, BoN-MAV mode, web HITL UI, multi-tenant SaaS, hosted skill registry, additional skill packs, additional tool backends.
