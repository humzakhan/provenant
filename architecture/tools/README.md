# Tool Layer

Status: **v0 / load-bearing**

> This directory contains tool category specs and per-backend implementation specs. Every tool that ships with Provenant — or that customers contribute — must conform to the template defined here. This document is the source of truth for tool authoring.

## How the tool layer is organized

```
tools/
├── README.md                       <- you are here: the template + conformance
│
├── <category>/                     one directory per tool category
│   ├── README.md                   the category contract (interface, schemas, semantics)
│   └── backends/                   implementations (if the category has them)
│       └── <backend>.md            connection details, mapping, quirks
```

Categories are *what* the tool does (`ledger`, `extract`, `reconcile`). Backends are *how* it's done (`sql`, `quickbooks`, `textract`). The agent only sees the category-level tool name (`ledger.get_balance`); the backend is a configuration concern.

## v0 categories

| Category | Purpose | Has backends? | Doc |
|---|---|---|---|
| Ledger | Read GL / sub-ledger / TB | Yes | [ledger/README.md](ledger/README.md) |
| Extract | Document extraction with confidence | Yes | [extract/README.md](extract/README.md) |
| Reconcile | Set matching with tolerances | No (pure Go) | [reconcile/README.md](reconcile/README.md) |
| Calc | Decimal-safe arithmetic + derived tokens | No (pure Go) | [calc/README.md](calc/README.md) |
| Policy | Cited passage retrieval | Yes | [policy/README.md](policy/README.md) |
| External | Market / FX / sanctions data, versioned | Yes | [external/README.md](external/README.md) |

## The category-spec template

Every category README must include these sections, in this order:

```
# Tool Category: <Name>

Status: <v0 | v0.5 deferred | v1 deferred>

> One-sentence purpose.

## Depends on
- subsystems this category depends on (kernel, tools registry)

## Depended on by
- skill packs that require this category

## Purpose
2-4 paragraphs. What problem this category solves. Why it's a category (not just one tool).

## Tools provided
Table of the tools in this category.

| Tool name | Purpose | Determinism class |
|---|---|---|

## Schemas
For each tool: input schema, output schema, mintable paths.
Use JSON Schema syntax. Show realistic examples.

## Token semantics
What gets minted, with what units, what `as_of`, what `source.path`.

## Determinism and replay
For each tool: PureDeterministic, CapturedDeterministic, or NonDeterministic? What needs to be captured for replay?

## Confidence (if applicable)
For tools that emit confidence scores (extraction especially), how is it computed? What's the threshold convention?

## Failure modes
Enumerate. For each: error code, when it fires, recommended skill-pack handling.

## Performance budget
Per-tool p50/p95 latency budget. The registry enforces a per-call timeout.

## Backend list
Table of backends (or "none — pure Go implementation in this directory").

## Tests
What the conformance suite covers.

## References
Links to references.md anchors and relevant external docs.
```

## The backend-spec template

Backend docs live in `<category>/backends/<backend>.md`:

```
# <Category> Backend: <Name>

Status: <v0 | v0.5 deferred | partial v0>

> One-sentence summary of what this backend does.

## Connection
Auth, transport, endpoint configuration.

## Schema mapping
How the upstream schema maps to the category's canonical schema.
Include a side-by-side table.

## Captured reads (if CapturedDeterministic)
What's persisted to S3 for replay. Format and identifier conventions.

## Rate limits and retries
Upstream limits, our retry policy.

## Failure modes
Backend-specific failures (e.g., QuickBooks-specific OAuth refresh, Textract-specific job-pending states).

## Tests
Conformance tests + backend-specific integration tests.

## Configuration
YAML example showing the backend's config block.

## References
```

## Conformance checklist

Every tool implementation must pass:

| Check | Owner | Enforced where |
|---|---|---|
| Input/output JSON schemas published | Tool author | Tool registry refuses to register without them |
| ≥5 positive + ≥5 negative schema tests | Tool author | CI |
| `DeterminismClass()` is correct | Tool author | Replay test verifies |
| Every numeric output is in `MintablePaths` or has documented exclusion | Tool author | Code review |
| Decimal-safe (no `float64` on monetary paths) | Tool author | Static linter in CI |
| Captured reads enable bit-identical replay (CapturedDeterministic only) | Tool author | Replay test in CI |
| Documented failure modes with stable error codes | Tool author | Doc review |
| p95 latency budget documented; per-call timeout configured | Tool author | Doc review + runtime enforcement |
| Spec doc following the template above | Tool author | Doc review |

## Naming conventions

- Tool name: `<category>.<verb_or_noun>`, lowercase, dot-separated, snake_case within segments. Example: `ledger.get_balance`, `extract.bank_statement`.
- Category directory: lowercase, no underscores. Example: `ledger/`, `extract/`.
- Backend directory: lowercase, optional hyphens. Example: `quickbooks/`, `azure-di/`.
- Schema files: kebab-case JSON, e.g., `get-balance.input.json`.

## Adding a new tool category

1. Open an RFC issue describing the category, its tools, why it's needed, and which skill packs would use it.
2. Get sign-off from a kernel maintainer (the category schema becomes part of the public surface).
3. Implement the category-level package under `internal/tools/<category>/`.
4. Implement at least one backend (or, for pure-Go categories like reconcile, the category implementation itself).
5. Write the category README and at least one backend spec following the templates above.
6. Add conformance tests.
7. Submit a PR; CI runs conformance + replay + lint.

## Adding a new backend to an existing category

1. Open an issue describing the backend (upstream system, auth model, schema mapping concerns).
2. Implement under `internal/tools/<category>/backends/<backend>/`.
3. Write the backend spec following the template above.
4. Add backend-specific integration tests (recorded HTTP fixtures or against a sandbox).
5. Submit a PR.

## Open questions

1. **Out-of-process tools.** Should custom tools run in a separate process (sandbox)? Defer to post-v0; see [FinVault](../references.md#finvault). v0 supports only in-process tools written in Go.
2. **Schema deprecation policy.** When a category's schema bumps major version, how long do we support the old version? Proposal: at least 12 months overlap.

## References

- [`subsystems/tools.md`](../subsystems/tools.md) — registry contract
- [`subsystems/kernel.md`](../subsystems/kernel.md) — token minting
- [DFAH](../references.md#dfah) — schema-first determinism foundation
- [FinVault](../references.md#finvault) — sandboxed execution pattern (future)
