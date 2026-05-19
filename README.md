# Provenant

Provenant is a **Go library you embed** when AI agents need to do finance or compliance work with an auditable trail.

If Claude or Codex are the analysts, Provenant is the **rulebook and evidence layer** around them: it makes sure important numbers come from a tool call, records where they came from, and packages the run so a reviewer can replay it later.

## In plain language

- **Claude/Codex alone** can read a spreadsheet and draft an answer.
- **Provenant** makes that work enforceable: every released answer must pass through `submit_answer`, every numeric claim is tied to provenance tokens, and every run produces an audit bundle.
- The **MCP server** and **CLI** are optional wrappers over the same library. They are convenience surfaces, not the product itself.

## How it works at a high level

1. A host application embeds the Provenant library, or talks to it through MCP/CLI.
2. Tools read source data, normalize it, and compare or calculate from it.
3. Guards and validators check the output before anything is released.
4. Provenant writes a tamper-detectable audit trail and emits an audit bundle for review or export.

## Start here by what you need

| Looking for | Go to |
|---|---|
| Product thesis and who it is for | [`docs/thesis.md`](docs/thesis.md) |
| Big-picture architecture | [`architecture/OVERVIEW.md`](architecture/OVERVIEW.md) |
| Detailed navigation map for the repo | [`architecture/README.md`](architecture/README.md) |
| Tools and source readers | [`architecture/tools/README.md`](architecture/tools/README.md) |
| Audit packs, proof exports, and chain format | [`architecture/subsystems/audit.md`](architecture/subsystems/audit.md) |
| Build phases and implementation plan | [`.planning/implementation-plan.md`](.planning/implementation-plan.md) |
| Known trade-offs and limits | [`docs/KNOWN_TRADE_OFFS.md`](docs/KNOWN_TRADE_OFFS.md) |
| Security model | [`architecture/cross-cutting/security.md`](architecture/cross-cutting/security.md) |
| Deployment options | [`architecture/cross-cutting/deployment.md`](architecture/cross-cutting/deployment.md) |
| Architecture Decision Records (ADRs) | [`architecture/adrs/`](architecture/adrs/) |
| Research lineage and source citations | [`architecture/references.md`](architecture/references.md) |

## Repo layout

```text
provenant/
|-- docs/              thesis, trade-offs, session notes
|-- architecture/      architecture overview, subsystems, tools, ADRs, references
|-- .planning/         phased implementation plan and phase work packets
`-- .agents/           agent role files
```

## Status

Phase 3 planning - Phases 1-2 complete, thin access surfaces and derived export next.
