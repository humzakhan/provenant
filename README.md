# Provenant

**Provenant is a Go library you embed.** The MCP server and the CLI are optional access surfaces — thin shells over the same library — for hosts that can't or won't embed it directly.

Provenant is an open-source enforcement layer for LLM agents in regulated finance. LLM agents writing reconciliations, AP coding, variance commentary, sanctions dispositions, and SAR drafts fail in a specific, regulator-disqualifying way: they emit numbers with no traceable lineage to source data. Provenant moves enforcement into the architecture — the `submit_answer` exit is the only output channel, every numeric claim is grounded to a kernel-minted provenance token, and every run produces a hash-chained audit log that is replay-deterministic on its deterministic portions. The library makes it architecturally impossible for an agent to release a Provenant-signed answer without verifiable lineage to a tool call. **The kernel itself has no storage requirement**: it takes typed inputs and emits a self-contained audit bundle as bytes; the caller decides where the bytes live. See [ADR-0011](architecture/adrs/0011-storage-is-a-deployment-concern.md).

## Documents

- [docs/thesis.md](docs/thesis.md) — product claim, dual-mode pluggability model, who it is for
- [architecture/OVERVIEW.md](architecture/OVERVIEW.md) — synthesis view: system diagram, technology stack, module-boundary contract, deployment model
- [architecture/README.md](architecture/README.md) — navigation index for the detailed architecture tree (subsystems, adapters, tools, ADRs, cross-cutting)
- [plans/implementation-plan.md](plans/implementation-plan.md) — phased build plan, Phase 0 through Phase 6, done-when criteria
- [docs/CRITIC.md](docs/CRITIC.md) — structured prompt for running blocking-issues critique sessions
- [docs/KNOWN_TRADE_OFFS.md](docs/KNOWN_TRADE_OFFS.md) — accepted trade-offs with mitigations documented
- [docs/SESSION.md](docs/SESSION.md) — running log of Hone critique/defender cycles and outstanding items
- [.agents/](.agents/) — agent role files (engineer, reviewer)

## Repo layout

```
provenant/
├── README.md                    <- you are here
├── docs/
│   ├── thesis.md                product claim and dual-mode model
│   ├── CRITIC.md                blocking-issues critique prompt
│   ├── KNOWN_TRADE_OFFS.md      accepted trade-offs
│   └── SESSION.md               Hone cycle log
├── architecture/
│   ├── README.md                navigation index and core invariants
│   ├── OVERVIEW.md              synthesis view (tech stack, module boundary)
│   ├── references.md            paper and blog citations
│   ├── subsystems/              one doc per kernel module
│   ├── adapters/                MCP, HTTP, CLI, LangChain, OpenAI
│   ├── tools/                   tool category specs and per-backend specs
│   ├── adrs/                    Architecture Decision Records
│   └── cross-cutting/           security, observability, deployment
├── plans/
│   └── implementation-plan.md   Phase 0 – Phase 6 build plan
└── .agents/                     agent role files
```

## Status

Spec phase — Hone-hardened, ready for Phase 0 implementation.
