# Provenant Public Documentation Writing Plan

## Purpose

This plan governs the public Provenant documentation writing effort after the
initial `docs-site/` scaffold. The canonical planning artifacts are:

- `.planning/documentation/documentation-plan.md`
- `.planning/documentation/goal-prompt.md`

Do not create a parallel `.planning/documents/` tree or a separate
Google-branded prompt. Do not edit public docs content in this planning PR.

The launch goal is a compact, conservative public docs site for Provenant: a
provenance enforcement layer/library for regulated finance work. Provenant is
not SaaS, not an agent, not a database, and not a workflow engine.

## Dedicated Writing Agent

Use one dedicated public-docs author:

- Agent name: `docs writer`
- Model: `gpt-5.5`
- Role: author public docs content only
- Scope: write one docs PR at a time, using reader briefs and canonical repo
  sources
- Boundary: do not implement runtime, CLI, MCP, HTTP, kernel, storage, or
  source-reader behavior

The `docs writer` owns final wording, structure, source citations, caveats,
and page acceptance for public docs. It must not delegate final prose to
low-tier readers.

## Reader Feeders

Before each docs PR, spawn low-tier reader agents to collect concise briefs.
Readers feed the `docs writer`; they do not write final public docs.

Required reader lanes:

- Product and audience reader: product framing, target audiences, terms to
  avoid, what readers need to decide after the page
- Implementation surfaces reader: current CLI, Go library, docs-site structure,
  package scripts, and shipped access surfaces
- Examples and tests reader: runnable fixtures, proof artifacts, test-backed
  behavior, command output, and current failure cases
- Architecture and tradeoffs reader: invariants, Mode A/Mode B limits, storage,
  adapters, ADRs, determinism, and caveats
- Link and validation reader: route inventory, internal links, status labels,
  source-reference coverage, and build/check commands

Each reader brief must be short and source-backed:

- Files read
- Claims safe to make
- Claims to avoid
- Commands or examples verified, if any
- Page-specific caveats
- Open questions or stop conditions

## Product Message

Every page should stay aligned with this public message:

Provenant is a provenance enforcement layer/library for regulated finance
work. It turns source reads, derived facts, and final answers into verifiable
evidence through schema-first validation, token-level grounding,
replay-deterministic outputs, and hash-chained audit bundles.

Primary audiences:

- Finance controllers and audit leads
- Platform engineers
- MCP host vendors and framework authors
- Compliance and model-risk reviewers

Repeated public claims must remain conservative:

- Non-bypassable `submit_answer` only in the controlled Mode A embedded/CLI
  path
- Fail-closed behavior when validation, grounding, bounds, or verification
  fails
- Schema-first validation
- Replay-deterministic outputs for captured execution artifacts
- Token-level grounding
- Hash-chained audit bundles

Critical caveats:

- Mode A embedded/CLI has stronger enforcement.
- Mode B/MCP host-channel text is not official unless it comes through passing
  `submit_answer` and verification.
- Host conformance is self-declared in v0.
- Do not claim external notary, external tamper-proofing, or root-compromise
  resistance.
- Legal and regulatory material is context, not advice.
- Examples are proof fixtures, not full product scope.

## Launch Information Architecture

Keep launch public docs capped at 15 pages. Do not exceed this hierarchy for
launch without explicit human approval:

1. `index`
2. `guarantees-and-limits`
3. `quickstart/current-artifacts`
4. `concepts/provenance-model`
5. `concepts/modes`
6. `guides/review-audit-evidence`
7. `guides/integrate-go-library`
8. `guides/extend-source-tools`
9. `reference/status-matrix`
10. `reference/current-surfaces`
11. `architecture/overview`
12. `architecture/kernel-runtime-storage`
13. `architecture/security-determinism-tradeoffs`
14. `architecture/adrs`
15. `ops/adoption-caveats`

Current public docs-site state:

- Top-level docs app: `docs-site/`
- Framework: Astro + Starlight
- Existing public pages: home, `quickstart/current-artifacts`, and
  `reference/status-matrix`

Do not move public docs into existing repo-facing `docs/`. Do not move or
replace `architecture/` or `examples/`.

## Page Standards

Every capability page must include:

- One status label: `Implemented now`, `Architecture contract`, `Planned`, or
  `Deferred`
- Source references to canonical repo inputs
- Explicit Mode A/Mode B caveats where relevant
- Conservative current-vs-planned wording
- No invented commands, APIs, screenshots, hosted flows, marketplace flows, or
  product surfaces

Preferred page shape:

- Plain-language purpose
- Audience and decision this page supports
- Current behavior or architecture contract
- Evidence readers can inspect
- Limits and caveats
- Source references
- Next page links

## Sequential PR Strategy

Use Mode A sequencing for docs implementation after this planning PR:

1. Branch from updated `origin/main`.
2. Spawn low-tier readers for the next public docs page.
3. Collect reader briefs.
4. Spawn or reuse the GPT-5.5 `docs writer`.
5. Write only the single public docs page in scope.
6. Run validation.
7. Run automated review.
8. Open a PR with a docs title.
9. Stop for human review only where the orchestrator goal prompt says to stop,
   or continue Mode A after automated approval when later authorized.
10. After merge, fetch `origin/main` and branch the next docs slice from it.

Each docs PR must include:

- Source-reading packet
- Public docs page to edit
- Validation commands and results
- Review notes
- Stop condition
- Next branch handoff

Each docs implementation PR may write exactly one public docs page. A PR may
update shared sidebar, navigation, or status-matrix files only when needed to
support that single page. If more than one public docs page needs content
changes, split the work into separate PRs.

PR title convention:

- `docs: update public docs plan`
- `docs: write public docs home`
- `docs: write current artifacts quickstart`
- `docs: write provenance model concept`
- `docs: write modes concept`
- `docs: write audit evidence guide`
- `docs: write go library integration guide`
- `docs: write source tools extension guide`
- `docs: write current surfaces reference`
- `docs: write architecture overview`
- `docs: write kernel runtime storage architecture`
- `docs: write security determinism tradeoffs`
- `docs: write adr architecture reference`
- `docs: write adoption caveats`

## Launch PR Slices

### PR 0: Planning Refresh

- Source-reading packet: current `.planning/documentation/` files and provided
  reader context
- Pages to edit: `.planning/documentation/documentation-plan.md`,
  `.planning/documentation/goal-prompt.md`
- Validation: `git diff --check`, `git status --short`
- Review: check that no `docs-site/` content or source docs changed
- Stop condition: open planning PR and stop for human review
- Next branch handoff: docs implementation starts from updated `origin/main`

### PR 1: Home

- Source-reading packet: product/audience, implementation surfaces,
  architecture/tradeoffs, link/validation
- Pages to edit: `docs-site/src/content/docs/index.mdx`
- Validation: docs build/check plus link/status checks available in `docs-site/`
- Review: verify product framing and no overclaiming
- Stop condition: PR opened with home page only
- Next branch handoff: `quickstart/current-artifacts`

### PR 2: Current Artifacts Quickstart

- Source-reading packet: implementation surfaces, examples/tests,
  link/validation
- Pages to edit: `docs-site/src/content/docs/quickstart/current-artifacts.mdx`
- Validation: docs build/check; run only current commands needed to verify page
  claims
- Review: commands are current and copy-pasteable; examples are labeled proof
  fixtures
- Stop condition: PR opened with quickstart only
- Next branch handoff: `guarantees-and-limits`

### PR 3: Guarantees And Limits

- Source-reading packet: product/audience, architecture/tradeoffs,
  link/validation
- Pages to edit: `docs-site/src/content/docs/guarantees-and-limits.mdx`
- Validation: docs build/check and link/status checks
- Review: Mode A/Mode B caveats, no tamper-proof or legal-advice claims
- Stop condition: PR opened with guarantees page only
- Next branch handoff: `concepts/provenance-model`

### PR 4: Provenance Model

- Source-reading packet: implementation surfaces, examples/tests,
  architecture/tradeoffs
- Pages to edit: `docs-site/src/content/docs/concepts/provenance-model.mdx`
- Validation: docs build/check and page links
- Review: token, captured-read, audit-chain, bundle, replay, and
  `submit_answer` wording is source-backed
- Stop condition: PR opened with provenance model only
- Next branch handoff: `concepts/modes`

### PR 5: Modes

- Source-reading packet: architecture/tradeoffs, product/audience,
  link/validation
- Pages to edit: `docs-site/src/content/docs/concepts/modes.mdx`
- Validation: docs build/check and page links
- Review: Mode B warning is prominent; MCP is not treated as primary or
  non-bypassable
- Stop condition: PR opened with modes page only
- Next branch handoff: `guides/review-audit-evidence`

### PR 6: Review Audit Evidence

- Source-reading packet: examples/tests, product/audience,
  architecture/tradeoffs, link/validation
- Pages to edit: `docs-site/src/content/docs/guides/review-audit-evidence.mdx`
- Validation: docs build/check and example artifact references
- Review: practical reviewer path, no legal advice, no product-scope inflation
- Stop condition: PR opened with audit evidence guide only
- Next branch handoff: `guides/integrate-go-library`

### PR 7: Integrate Go Library

- Source-reading packet: implementation surfaces, architecture/tradeoffs,
  link/validation
- Pages to edit: `docs-site/src/content/docs/guides/integrate-go-library.mdx`
- Validation: docs build/check; Go command only if page claims require it
- Review: preserves kernel/runtime/storage boundaries
- Stop condition: PR opened with Go library guide only
- Next branch handoff: `guides/extend-source-tools`

### PR 8: Extend Source Tools

- Source-reading packet: implementation surfaces, examples/tests,
  architecture/tradeoffs, link/validation
- Pages to edit: `docs-site/src/content/docs/guides/extend-source-tools.mdx`
- Validation: docs build/check and source-reference links
- Review: reader/tool/skill claims distinguish current, architecture, planned,
  and deferred surfaces
- Stop condition: PR opened with extension guide only
- Next branch handoff: `reference/current-surfaces`

### PR 9: Current Surfaces Reference

- Source-reading packet: implementation surfaces, examples/tests,
  link/validation
- Pages to edit: `docs-site/src/content/docs/reference/current-surfaces.mdx`;
  update `reference/status-matrix.mdx` only if necessary to keep shared status
  labels consistent with this single reference page
- Validation: docs build/check and status-label check
- Review: compact current/planned/deferred inventory, no duplicate
  architecture spec
- Stop condition: PR opened with reference page only, plus any narrow shared
  status-matrix consistency update required by that page
- Next branch handoff: architecture pages

### PR 10: Architecture Overview

- Source-reading packet: architecture/tradeoffs, product/audience,
  link/validation
- Pages to edit: `docs-site/src/content/docs/architecture/overview.mdx`
- Validation: docs build/check and architecture links
- Review: public summary cites architecture sources and does not replace them
- Stop condition: PR opened with architecture overview only
- Next branch handoff: `architecture/kernel-runtime-storage`

### PR 11: Kernel Runtime Storage

- Source-reading packet: architecture/tradeoffs, implementation surfaces,
  link/validation
- Pages to edit:
  `docs-site/src/content/docs/architecture/kernel-runtime-storage.mdx`
- Validation: docs build/check and architecture links
- Review: pure kernel boundary, runtime responsibilities, and storage caveats
  are source-backed
- Stop condition: PR opened with page only
- Next branch handoff: `architecture/security-determinism-tradeoffs`

### PR 12: Security Determinism Tradeoffs

- Source-reading packet: architecture/tradeoffs, product/audience,
  link/validation
- Pages to edit:
  `docs-site/src/content/docs/architecture/security-determinism-tradeoffs.mdx`
- Validation: docs build/check and architecture links
- Review: conservative security, determinism, tamper-evidence, and regulatory
  language
- Stop condition: PR opened with page only
- Next branch handoff: `architecture/adrs`

### PR 13: ADRs

- Source-reading packet: architecture/tradeoffs and link/validation
- Pages to edit: `docs-site/src/content/docs/architecture/adrs.mdx`
- Validation: docs build/check and ADR links
- Review: concise ADR index, no rewritten decisions
- Stop condition: PR opened with ADR page only
- Next branch handoff: `ops/adoption-caveats`

### PR 14: Adoption Caveats

- Source-reading packet: product/audience, architecture/tradeoffs,
  implementation surfaces, link/validation
- Pages to edit: `docs-site/src/content/docs/ops/adoption-caveats.mdx`
- Validation: docs build/check and link/status checks
- Review: adoption caveats are explicit, practical, and not legal advice
- Stop condition: PR opened with final launch page
- Next branch handoff: launch polish only if approved

### PR 15: Launch Polish, Only If Needed

- Source-reading packet: link/validation plus any stale page-specific reader
  lane
- Pages to edit: one existing public docs page; shared navigation or status
  matrix only if needed for that single page
- Validation: full docs build/check, link/status checks, `git diff --check`
- Review: no new page count, no new claims, no multi-page content edits
- Stop condition: the single polish page and its required shared references are
  internally consistent
- Next branch handoff: none unless human approves post-launch docs work

## Source Material

Readers and the `docs writer` should prefer these canonical inputs:

- `README.md`
- `docs/thesis.md`
- `docs/KNOWN_TRADE_OFFS.md`
- `architecture/OVERVIEW.md`
- `architecture/README.md`
- `architecture/subsystems/kernel.md`
- `architecture/subsystems/audit.md`
- `architecture/subsystems/guards.md`
- `architecture/subsystems/harness.md`
- `architecture/subsystems/tools.md`
- `architecture/tools/**`
- `architecture/subsystems/skills.md`
- `architecture/subsystems/storage.md`
- `architecture/adapters/mcp.md`
- `architecture/adapters/cli.md`
- `architecture/adapters/http.md`
- `architecture/adapters/langchain.md`
- `architecture/adapters/openai.md`
- `architecture/adrs/*.md`
- `examples/reconciliation/README.md`
- `examples/derivation-graph/README.md`
- Current CLI, access-surface, reader, bundle, and docs-site tests

## Validation Commands

For this planning PR:

```bash
rtk git diff --check
rtk git status --short
```

For each docs implementation PR, run the narrowest applicable set:

```bash
cd docs-site && rtk npm run build
cd docs-site && rtk npm run check
rtk git diff --check
rtk git status --short
```

Run Go commands only when a page makes or changes a claim that needs current
code verification.

## Non-Goals

- Do not edit `docs-site/` content in this planning PR.
- Do not create more than 15 launch pages.
- Do not create `.planning/documents/`.
- Do not add a separate Google-branded prompt.
- Do not move public docs into existing `docs/`.
- Do not replace `architecture/` or `examples/`.
- Do not implement product code.
- Do not invent current commands, APIs, packages, hosted workflows,
  screenshots, marketplace flows, or legal interpretations.
- Do not let low-tier readers author final public docs prose.

## Stop Conditions

Stop and report if:

- Reader briefs conflict on a public claim.
- Current code and source docs disagree on a user-facing behavior.
- A page requires describing a planned/deferred feature as implemented.
- A docs PR would exceed the 15-page launch IA.
- Validation cannot run because dependencies or network access are unavailable.
- A requested edit would touch runtime, CLI, MCP, HTTP, kernel, storage,
  examples, or source docs outside the docs-writing scope.
