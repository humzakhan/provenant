# Provenant Documentation Plan

## Purpose

Build a public documentation site that explains Provenant to both technical and non-technical users without overstating what exists today. The site should make the core idea clear first: Provenant is a Go provenance enforcement layer for regulated finance workflows, not a logger, SaaS product, agent, database, or generic guardrail tool. The docs must separate current implemented examples and APIs from architecture commitments and deferred surfaces.

## Audiences

- Finance controllers, audit leads, compliance officers, and model-risk reviewers who need to understand what evidence Provenant produces and what it does not prove.
- Platform engineers evaluating whether to embed the Go library or run an optional access surface.
- Agent-framework, MCP host, and integration authors who need clear mode boundaries and conformance expectations.
- Contributors who need the kernel invariants, module boundaries, validation rules, and examples before changing code.

## Recommended Tooling

Use Astro + Starlight unless a later implementer finds a concrete blocker in repository constraints.

Rationale:

- Starlight is optimized for documentation information architecture, sidebars, search, versioned navigation patterns, Markdown and MDX, code blocks, and static deployment.
- Provenant needs chronological onboarding plus deep architecture reference. Starlight handles both without building a custom site shell.
- Astro can import generated reference pages later from Go docs, schemas, or CLI output without committing to a heavy framework.
- The docs should live as source-controlled static files and build in CI without a hosted docs dependency.

Do not choose GitBook as the default. GitBook is acceptable only if the project explicitly prioritizes hosted editing workflow over local CI validation and source-controlled build reproducibility. That is not the current requirement.

## Docs Site Technical Specification

Build the documentation site as a small, source-controlled Astro + Starlight project in `docs-site/`.

Repository documentation boundaries:

- Public documentation pages and public-docs tooling belong in top-level `docs-site/`, not in the existing `docs/` directory.
- The existing `docs/` directory remains repo-facing narrative, governance, thesis, and trade-off material. Do not move or replace those source documents as part of the public docs site.
- The existing `architecture/` directory remains the canonical architecture reference. Public architecture pages should summarize, cite, and link to those files rather than becoming a competing source of truth.
- The existing `examples/` directory remains runnable example evidence. Public quickstarts and guides should cite, summarize, and link to committed examples instead of relocating or duplicating their evidence artifacts.
- Public docs should make these source relationships visible through `Source references` sections, status labels, and links back to canonical repository inputs.

Recommended structure:

- `docs-site/package.json`: docs-local package manifest with scripts and pinned docs dependencies.
- `docs-site/astro.config.mjs`: Starlight integration, sidebar groups, site title, social links if known, custom CSS registration, and Markdown/MDX settings.
- `docs-site/src/content/docs/`: source pages organized by the information architecture in this plan.
- `docs-site/src/components/`: small Astro components for status badges, source-reference cards, caveat banners, audit-artifact summaries, and mode-warning callouts.
- `docs-site/src/styles/provenant.css`: Provenant theme overrides, component classes, and any custom layout CSS.
- `docs-site/public/`: static assets only when generated or explicitly provided; do not invent screenshots or branding assets.

Package manager and scripts:

- Prefer the repository's existing package manager if one is already established before implementation begins. If no JavaScript package manager convention exists, use `npm` because it requires no extra workspace tooling.
- Keep docs scripts local to `docs-site/package.json`: `dev`, `build`, `check`, and `preview`.
- Add root-level forwarding scripts only if the repo already has a root package manifest or an explicit contributor workflow for root scripts. Do not restructure the repository to host docs.
- Expected commands after scaffolding: `npm install` from `docs-site/`, `npm run build`, `npm run check`, and optionally `npm run preview` for manual inspection.

Theming and custom HTML/CSS:

- The docs site should follow the Provenant landing-page design language and remain highly customizable with ordinary HTML/CSS, not a locked hosted-docs theme.
- The actual landing-page file is not present in this repository. The discoverable design-token source is `audit/report/html.go`, especially the embedded report template and CSS variables marked as extracted from `provenant-landing-v2.html`; generated audit report HTML and `audit/report/testdata/golden.html` can be used as secondary evidence for the same visual language.
- Start from the visible tokens and patterns in `audit/report/html.go`: white and warm-gray surfaces, deep Provenant green, muted gray text, mono metadata, compact cards, bordered panels, green-tinted success banners, tab-like navigation, and audit/source-card visual motifs.
- Implement Starlight theme overrides in `src/styles/provenant.css` with CSS custom properties and component classes. Prefer Astro components and semantic HTML for custom sections instead of forking Starlight internals.
- If the actual landing-page asset is later provided, align colors, typography, spacing, hero treatment, and components against that asset and update the docs token file deliberately.

Content source strategy:

- Author pages in Markdown or MDX under `docs-site/src/content/docs/` and treat this repository's existing docs, architecture files, examples, tests, and source tables as canonical inputs.
- Prefer linking to canonical architecture docs and code for detailed contracts instead of duplicating long specifications.
- For reference material that is table-like or source-derived, generate it only when a stable source exists; otherwise keep it manual and label it as manually synchronized.
- Every page that makes a capability claim must include a status label and a `Source references` section.

Components and callouts:

- Create a reusable status badge component for `Implemented now`, `Architecture contract`, `Planned`, and `Deferred`.
- Create warning callouts for Mode B host-channel limits, internal-only tamper evidence, generated-reference staleness, and regulatory-context-not-legal-advice.
- Create small artifact cards for audit bundle contents, captured reads, tokens, audit entries, and replay output.
- Use Starlight's built-in asides where sufficient; use custom Astro components when Provenant-specific status and evidence patterns need consistent styling.

Build, link, and content validation:

- `npm run build` should perform the static Astro/Starlight build.
- `npm run check` should run Astro checks and any Markdown/content checks introduced for the docs project.
- Add internal link validation for Markdown routes and built static output. Use a package such as `lychee`, `markdown-link-check`, or another small checker only after confirming it works in local and CI constraints.
- Add a lightweight content check that fails when docs pages under the main content tree omit one of the required content status labels.
- Keep generated-reference checks separate from page authoring. If generation scripts are added, CI should fail on stale generated pages.

## Source Material

Use these as canonical inputs, with labels that distinguish implemented behavior from planned or deferred architecture:

- `README.md`: current mental model, implemented CLI examples, reconciliation ZIP contents, Go-only `bundle.Verify` and `bundle.Replay` note, and failure fixtures.
- `docs/thesis.md`: product framing, audience, non-goals, Mode A versus Mode B caveat, and market/problem narrative.
- `docs/KNOWN_TRADE_OFFS.md`: accepted caveats, especially internal-only tamper evidence, Mode B self-declared conformance, streaming and split-turn deferrals, locale handling, custom validator deferral, and HITL persistence responsibilities.
- `architecture/OVERVIEW.md`: integrated architecture, technology choices, module boundary contract, deployment model, and equal adapter stance.
- `architecture/README.md`: architecture navigation and core invariants.
- `architecture/subsystems/kernel.md`: public API, token model, `TransactionalScope`, and pure-kernel boundary.
- `architecture/subsystems/audit.md`: audit entry schema, hash construction, deterministic payload rules, bundle semantics, error codes, and marker verification.
- `architecture/subsystems/guards.md`: `submit_answer`, token grounding, validators, and fail-closed pipeline.
- `architecture/subsystems/harness.md`: Mode A loop, Mode B caveats, bounds, termination states, signature marker, and streaming limits.
- `architecture/subsystems/tools.md` and `architecture/tools/**`: tool registry, mintable paths, source readers, conformance template, and tool category status.
- `architecture/subsystems/skills.md`: skill-pack format, built-in normalizers, signing status, and deferred extensibility.
- `architecture/subsystems/storage.md`: library-mode storage stance, runtime persistence projections, and HITL pause/resume limits.
- `architecture/adapters/mcp.md`: MCP capabilities and hard limits. Do not imply MCP can force arbitrary host-channel output through Provenant.
- `architecture/adapters/cli.md`, `architecture/adapters/http.md`, `architecture/adapters/langchain.md`, and `architecture/adapters/openai.md`: adapter status and deferred surfaces.
- `architecture/adrs/*.md`: decision records, especially ADR-0010 and ADR-0011.
- `examples/reconciliation/README.md` and `examples/derivation-graph/README.md`: user-facing walkthroughs and evidence artifacts.

## Content Status Labels

Every page that describes capabilities should use one of these labels near the top:

- `Implemented now`: verified by current code, examples, tests, or CLI commands in this repository.
- `Architecture contract`: specified in architecture docs but not necessarily exposed as current user-facing code.
- `Planned`: expected implementation work, not current behavior.
- `Deferred`: explicitly out of scope for the current phase or v0.

Current implemented docs/code to describe conservatively:

- `go test ./...` as the repository-wide verification command from `README.md`.
- CLI `read-mint-bundle` with the `extract.json` reader.
- CLI `reconcile` fixture using local CSV/XLSX inputs and producing the documented ZIP artifacts.
- Go API-only bundle verification and replay through `kernel/public/bundle`.
- Smoke tests across access surfaces where referenced by current tests.

Planned or architecture-level surfaces that must not be presented as shipped user workflows unless implementation confirms them:

- Production MCP server packaging, marketplace connector flows, token issuance CLI, and full host conformance workflows.
- HTTP API and non-Go bindings.
- Runtime SQLite/Postgres/S3 packaging beyond what current code exposes.
- Hosted registry, skill-pack signing, custom validators, WASM/plugin loading, and multi-tenant SaaS.
- Full audit-pack CLI commands if they are only architecture-level and not present in current code.

## Information Architecture

Use chronological IA, but keep the launch site concise. The goal is a focused docs site people will actually read, not a 39-page reference site on day one. Target 12-15 public pages at launch, with an upper bound of 15 launch pages. Details should live as sections within these pages unless reader behavior, support load, or implementation growth proves they deserve standalone navigation later.

The site should support two visible reader paths: a non-technical evidence path for controllers, audit leads, compliance officers, and model-risk reviewers; and a technical implementation path for platform engineers, integration authors, and contributors. The same sidebar serves both paths by starting with plain-language framing, moving through runnable artifacts, then branching into task guides, compact reference, architecture, and adoption caveats.

Avoid a redundant split between many Start Here pages and many Quickstart pages. The home page should orient readers, and one current-artifacts quickstart should cover install/test, `read-mint-bundle`, the reconciliation fixture, derivation graph demo, and Go API-only verify/replay as sections. Add separate pages only when the topic has a distinct job, audience, or maintenance cadence.

Every planned page below uses this repeated schema:

- `Route/path`: Starlight source path under `docs-site/src/content/docs/`.
- `Audience`: Primary readers for the page.
- `Purpose`: The job the page does in the reader journey.
- `Content outline`: The sections or article beats that belong on the page, in order.
- `Source material`: Canonical repository inputs to use before drafting.
- `Status label`: One of `Implemented now`, `Architecture contract`, `Planned`, or `Deferred`.
- `Acceptance notes`: Checks that keep the page accurate and useful.

### Reader Path Summary

Non-technical path:

1. `index.md`
2. `guarantees-and-limits.md`
3. `quickstart/current-artifacts.md`
4. `guides/review-audit-evidence.md`
5. `ops/adoption-caveats.md`

Technical path:

1. `index.md`
2. `quickstart/current-artifacts.md`
3. `concepts/provenance-model.md`
4. `concepts/modes.md`
5. `guides/integrate-go-library.md`
6. `guides/extend-source-tools.md`
7. `reference/status-matrix.md`
8. `reference/current-surfaces.md`
9. Architecture pages as needed.

### 1. Orientation

#### Page: Home

- `Route/path`: `index.md`
- `Audience`: All readers; especially first-time finance, compliance, and platform evaluators.
- `Purpose`: Explain Provenant in one sitting without assuming Go, MCP, or agent-framework knowledge.
- `Content outline`: Plain-language one-paragraph explanation; regulated-finance problem statement; what Provenant is; what Provenant is not; current mental model from source bytes to audit bundle; who should read which path next; short glossary for captured read, token, audit entry, audit bundle, replay, `submit_answer`, Mode A, and Mode B; status callout for current repository examples.
- `Source material`: `README.md`, `docs/thesis.md`, `architecture/OVERVIEW.md`.
- `Status label`: `Implemented now` for the repository mental model and examples; `Architecture contract` for broader deployment framing.
- `Acceptance notes`: A non-engineer can explain what evidence Provenant produces after reading this page; the page does not call Provenant a SaaS, database, agent, logger, or generic guardrail product; every current command claim links to the current-artifacts quickstart.

#### Page: Guarantees And Limits

- `Route/path`: `guarantees-and-limits.md`
- `Audience`: Compliance, audit, model-risk, and engineering evaluators.
- `Purpose`: State what the four invariants guarantee and where the guarantees stop.
- `Content outline`: Four kernel invariants; internally tamper-evident audit chains; captured replay versus fresh-run equivalence; Mode A non-bypassability; Mode B host-channel caveat; self-declared host conformance caveat; root-compromise scope; legal-advice disclaimer for regulatory interpretation; links to deeper operations pages.
- `Source material`: `README.md`, `docs/thesis.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/README.md`, `architecture/subsystems/harness.md`, `architecture/adapters/mcp.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page says `internally tamper-evident`, not `tamper-proof`; it says arbitrary Mode B host text is not Provenant-attested unless it came from passing `submit_answer` and the marker verifies against the audit chain or audit pack.

### 2. Current Artifact Quickstart

#### Page: Run Current Artifacts

- `Route/path`: `quickstart/current-artifacts.md`
- `Audience`: Finance reviewers, compliance reviewers, engineers, and contributors who want a tangible artifact quickly.
- `Purpose`: Provide the single launch quickstart for current, runnable repository behavior.
- `Content outline`: Prerequisites; clone command; repository layout snapshot; `go test ./...`; `read-mint-bundle` with `extract.json`; reconciliation fixture command and ZIP entries; derivation graph demo and deterministic trace; Go API-only `bundle.Verify` and `bundle.Replay`; common failure modes; what each artifact is not.
- `Source material`: `README.md`, `go.mod`, `cmd/provenant`, `access/smoke/smoke_test.go`, `access/cli` tests, `kernel/public/bundle`, `examples/reconciliation/README.md`, `examples/derivation-graph/README.md`, current repository tests.
- `Status label`: `Implemented now`.
- `Acceptance notes`: Commands are copy-pasteable from repository root; last verified command is recorded; `extract.json` is labeled as the only current reader for `read-mint-bundle`; the page lists exactly the current reconciliation ZIP artifacts; no nonexistent standalone verify CLI is documented; no complete reconciliation product, fuzzy matching, HTTP endpoint, or MCP tool is presented as current behavior.

### 3. Concepts

#### Page: Provenance Model

- `Route/path`: `concepts/provenance-model.md`
- `Audience`: All readers, with emphasis on model-risk reviewers, technical auditors, and platform evaluators.
- `Purpose`: Explain the core Provenant model from source bytes to captured reads, tokens, audit chain, bundles, replay, and `submit_answer`.
- `Content outline`: Why provenance is an enforcement boundary rather than an observability log; source-byte contract; declared SHA-256; selectors; `CapturedRead` and `ReadResult`; token purpose and canonical decimal/unit handling; derived token lineage; audit entry anatomy and hash chaining; audit bundle snapshot; verify and replay limits; why `submit_answer` exists; claim grounding and fail-closed release; examples from current artifacts.
- `Source material`: `README.md`, `docs/thesis.md`, `architecture/README.md`, `architecture/subsystems/kernel.md`, `architecture/subsystems/audit.md`, `architecture/subsystems/guards.md`, `architecture/subsystems/tools.md`, `architecture/tools/extract/README.md`, `architecture/subsystems/storage.md`, `examples/reconciliation/README.md`, `examples/derivation-graph/README.md`, current reader tests.
- `Status label`: `Implemented now` for current example shapes; `Architecture contract` for broader token graph, reader-extension, and release semantics.
- `Acceptance notes`: The page separates `extract.json` in `read-mint-bundle` from `extract.csv_rows` and `extract.spreadsheet` in the reconciliation fixture; numeric examples use canonical decimal strings and avoid `float64` framing; the page says `tamper-detectable by verifying the internal hash chain`; `Provenant-signed` or `Provenant-attested` is used only for output that passed `submit_answer` and can be verified against the chain or pack.

#### Page: Modes

- `Route/path`: `concepts/modes.md`
- `Audience`: Integrators, MCP host authors, platform engineers, and compliance reviewers.
- `Purpose`: Explain Mode A and Mode B before readers choose an integration path.
- `Content outline`: Mode A CLI/embedded harness control; Mode B host-driven loop; non-bypassable output channel in Mode A; Mode B host-channel limitation; host conformance self-declaration; when Mode A is required; adapter equality and no MCP primacy; streaming and split-turn caveats.
- `Source material`: `docs/thesis.md`, `architecture/subsystems/harness.md`, `architecture/adapters/mcp.md`, `architecture/adrs/0010-mcp-equal-not-primary.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page contains a prominent Mode B warning and does not imply MCP can force arbitrary host-channel output through Provenant.

### 4. Guides

#### Page: Review Audit Evidence

- `Route/path`: `guides/review-audit-evidence.md`
- `Audience`: Controllers, audit leads, compliance reviewers, and model-risk reviewers.
- `Purpose`: Provide a practical, non-technical review flow for inspecting evidence artifacts.
- `Content outline`: What artifact to start with; manifest and bundle hash; captured reads; tokens; audit JSONL; `report.md` and `report.json`; replay output; what passes, what fails, and what remains a business judgment; escalation checklist; Mode B marker caveat.
- `Source material`: `README.md`, `examples/reconciliation/README.md`, `architecture/subsystems/audit.md`, `docs/KNOWN_TRADE_OFFS.md`, `docs/thesis.md`.
- `Status label`: `Implemented now` for current reconciliation artifacts; `Architecture contract` for generic reviewer workflow.
- `Acceptance notes`: The guide explains evidence artifacts before internals, avoids legal advice, and links Mode B interpretation to `ops/adoption-caveats.md`.

#### Page: Integrate The Go Library

- `Route/path`: `guides/integrate-go-library.md`
- `Audience`: Go platform engineers embedding Provenant.
- `Purpose`: Show the intended library-mode shape and host responsibilities.
- `Content outline`: Library-as-product framing; kernel public API boundary; `memio` default; injected `Store`, `AuditSink`, `BlobStore`, `ModelClient`, `Clock`, `RandSource`, and `TransactionalScope`; no kernel I/O; emitted bundle bytes; host storage responsibility; HITL pause/resume responsibility; validation commands.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/subsystems/kernel.md`, `architecture/subsystems/storage.md`, `architecture/adrs/0011-storage-is-a-deployment-concern.md`, `.agents/engineer.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The guide preserves the pure-Go kernel boundary and does not introduce database, network, CLI, or MCP types into kernel examples.

#### Page: Extend Source And Tool Surfaces

- `Route/path`: `guides/extend-source-tools.md`
- `Audience`: Contributors, source-reader authors, tool authors, integration engineers, and skill authors.
- `Purpose`: Provide one task-oriented extension guide for source readers, tool backends, and skill packs without multiplying launch pages.
- `Content outline`: Prerequisites and source docs; source-reader contract; selector design; captured-read fields; output hash; reader name/version; surface-exposure checklist; `ToolBackend` contract; input and output schemas; credentials and credential IDs; `mintable_paths`; backend-owned numeric normalization; tools cannot construct tokens directly; skill-pack format; prompts, tools, guard configuration, bounds; built-in validators and normalizers; deferred custom validators, WASM/plugin loading, hosted registry, and signing beyond local digest status; tests and stop conditions.
- `Source material`: `architecture/subsystems/tools.md`, `architecture/tools/README.md`, `architecture/tools/extract/README.md`, `architecture/subsystems/skills.md`, `architecture/adrs/0006-skill-pack-format.md`, `.agents/engineer.md`, existing reader code and tests, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract` with `Deferred` callouts for custom validators, WASM/plugin loading, hosted registry, and signing beyond local digest status.
- `Acceptance notes`: The guide makes clear that adding a reader implementation is not the same as exposing it through every CLI/runtime surface; invalid backend output must fail closed; skill-pack signing or hosted registry must not be claimed as current behavior unless implementation confirms it.

### 5. Reference

#### Page: Status Matrix

- `Route/path`: `reference/status-matrix.md`
- `Audience`: All readers, especially evaluators deciding what exists today.
- `Purpose`: Provide one authoritative matrix of implemented, architecture-contract, planned, and deferred surfaces.
- `Content outline`: Status-label definitions; command surface; readers; bundle verify/replay; adapters; storage; skill packs; validators/normalizers; hosted registry; non-Go bindings; HTTP; MCP; HITL persistence; external notarization; links to source references.
- `Source material`: `README.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/OVERVIEW.md`, `architecture/adapters/*.md`, implementation status from code.
- `Status label`: `Implemented now` for the page itself once maintained; individual rows carry all four labels.
- `Acceptance notes`: This page is created before broad page authoring and acts as the site-wide overclaim guard.

#### Page: Current Surfaces Reference

- `Route/path`: `reference/current-surfaces.md`
- `Audience`: Engineers, reviewers, support operators, tool authors, and contributors.
- `Purpose`: Compactly document current CLI, Go API, bundle format, audit entry kinds, error codes, and tool categories without creating six launch reference pages.
- `Content outline`: Generation or synchronization method; current CLI commands `read-mint-bundle` and `reconcile`; known absent commands; public Go package map; `kernel/public/bundle`; `kernel/public/memio`; bundle format identifier and top-level fields; reconciliation ZIP companions; audit entry kind table or link; error-code table or link; tool category table; update procedure; last verified commands.
- `Source material`: `README.md`, `cmd/provenant`, `access/cli`, `kernel/public/`, `kernel/public/bundle`, `kernel/public/errorcode`, `architecture/subsystems/kernel.md`, `architecture/subsystems/audit.md`, `architecture/subsystems/tools.md`, `architecture/tools/**`, `examples/reconciliation/README.md`, current tool code and tests, command help output if generated.
- `Status label`: `Implemented now` for rows confirmed in code; `Architecture contract` for manually synchronized architecture rows; mixed rows carry row-level labels.
- `Acceptance notes`: The page does not list planned audit-pack, HTTP, MCP, token-issuance CLI, or non-Go binding flows as current behavior; signatures link to canonical Go docs or source; tables do not hand-invent codes or entry kinds; generated rows fail stale checks if generation is introduced.

### 6. Architecture

#### Page: Architecture Overview

- `Route/path`: `architecture/overview.md`
- `Audience`: Technical evaluators, contributors, and architecture reviewers.
- `Purpose`: Present a curated docs-site version of the integrated architecture.
- `Content outline`: Library-as-product diagram; kernel/runtime/access-surface boundaries; technology stack summary; module boundary contract; where to read next; current open questions.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/README.md`, `docs/thesis.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page links back to canonical architecture docs rather than becoming a divergent source of truth.

#### Page: Kernel, Runtime, And Storage

- `Route/path`: `architecture/kernel-runtime-storage.md`
- `Audience`: Contributors, integrators, adapter authors, platform engineers, and operators.
- `Purpose`: Make the pure-Go kernel boundary, runtime responsibilities, adapter stance, and storage-as-deployment-concern model concrete in one page.
- `Content outline`: Allowed public API types; forbidden imports; injected interfaces; `TransactionalScope`; `memio`; no kernel I/O; token/audit atomicity; runtime owns I/O; adapters translate, not enforce; CLI, MCP, HTTP overview; LangChain and OpenAI adapter slots; adapter equality; no MCP primacy; library emits bytes; host decides persistence; runtime SQLite/Postgres projections; S3-compatible blob storage; HITL pause/resume responsibilities; `RunState` snapshot; bundle retention considerations; contributor checklist.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/subsystems/kernel.md`, `architecture/subsystems/storage.md`, `architecture/adapters/cli.md`, `architecture/adapters/mcp.md`, `architecture/adapters/http.md`, `architecture/adapters/langchain.md`, `architecture/adapters/openai.md`, `.agents/engineer.md`, ADR-0009, ADR-0010, ADR-0011, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page preserves all kernel public-surface prohibitions; does not introduce database, network, CLI, or MCP types into kernel examples; does not present HTTP, LangChain, OpenAI, or production MCP packaging as shipped unless implementation confirms it; distinguishes current code from runtime persistence architecture and keeps kernel storage-free.

#### Page: Security, Determinism, And Trade-Offs

- `Route/path`: `architecture/security-determinism-tradeoffs.md`
- `Audience`: Security reviewers, platform engineers, compliance teams, technical evaluators, and model-risk readers.
- `Purpose`: Summarize the threat model, deterministic replay model, and accepted limitations in one architecture page.
- `Content outline`: Tenant isolation; prompt-injection defense; credential identity and handling; schema-first validation; fail-closed defaults; root-compromise limitation; internal tamper-evidence; supply-chain status; deterministic payloads; non-deterministic envelope fields; canonical JSON and hash construction; numeric canonicalization; parallel sibling ordering; captured replay; streaming and split-turn deferral; external-source fresh-run drift; live observability caveat; self-declared host conformance; locale and normalizer limits; case-bank correction caveat; skill-pack extensibility; library-mode HITL persistence; security review checklist.
- `Source material`: `architecture/cross-cutting/security.md`, `architecture/subsystems/guards.md`, `architecture/subsystems/audit.md`, `architecture/subsystems/harness.md`, `docs/KNOWN_TRADE_OFFS.md`, ADR-0005.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: Root-equivalent host compromise is scoped accurately and not hidden; the page clearly separates captured deterministic replay from fresh-run equivalence across changing external sources; accepted trade-offs are summarized without retroactively editing or weakening their source language.

#### Page: ADR Index

- `Route/path`: `architecture/adrs.md`
- `Audience`: Contributors and architecture reviewers.
- `Purpose`: Make architectural decisions discoverable from the docs site.
- `Content outline`: ADR table; short decision summaries; status and supersession notes; especially ADR-0010 and ADR-0011; when a new ADR is required; links to source ADR files.
- `Source material`: `architecture/adrs/*.md`, `.agents/engineer.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page notes ADR-0010 supersedes MCP-primary framing and ADR-0011 establishes storage as deployment concern.

### 7. Adoption And Compliance

#### Page: Adoption Caveats

- `Route/path`: `ops/adoption-caveats.md`
- `Audience`: Buyers, compliance reviewers, operators, platform engineers, MCP host authors, model-risk reviewers, audit leads, and security reviewers.
- `Purpose`: Consolidate deployment shapes, Mode B attestation interpretation, accepted caveats, and regulatory context that affect adoption decisions.
- `Content outline`: Library mode; CLI mode; MCP/runtime mode; stateless versus persistent runtime; storage choices; air-gapped and single-tenant framing; Mode A versus Mode B selection; what `[PROVENANT-ATTESTED v1 ...]` means; what must be verified against the audit chain or audit pack; host-channel text that bypasses `submit_answer`; `host_text_only` interpretation; self-declared conformance; when Mode A is required; internal-only tamper evidence; no external notary; streaming and split-turn deferral; external-source drift; skill-pack extensibility; library-mode HITL persistence; no hosted SaaS in v0; deferred SaaS and non-Go binding notes; non-legal-advice regulatory context; reconstructability and reviewability; model-risk review questions; what remains the deployer's responsibility.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/cross-cutting/deployment.md`, `architecture/subsystems/harness.md`, `architecture/adapters/mcp.md`, `architecture/references.md`, `docs/thesis.md`, `docs/KNOWN_TRADE_OFFS.md`, `README.md`.
- `Status label`: `Architecture contract` with current implementation callouts.
- `Acceptance notes`: The page contains the strongest Mode B caveat in the site and is cross-linked from all Mode B or MCP pages; it does not claim multi-tenant SaaS, hosted registry, or non-Go bindings are current behavior; caveats remain prominent rather than buried below marketing or setup material; it includes `This is not legal advice` near the top and does not claim regulatory approval.

### 8. Contributor Guardrails

Do not create a standalone contributor page in the launch docs set. Fold contributor-facing accuracy rules into the maintained planning prompt and, where public readers benefit, into the `reference/status-matrix.md` maintenance section.

Contributor guardrails to preserve:

- Required status labels.
- Source-reference requirement.
- Last-verified command rule.
- Overclaim checklist.
- Mode B warning rule.
- Tamper-evidence language rule.
- Regulatory-language rule.
- Generated-reference workflow.
- Local docs commands after scaffold exists.

Future expansion: add `contributing/docs.md` only after the launch site exists and contributor workflow needs justify a standalone public page. Until then, these rules remain governed by this plan, `goal-prompt.md`, and the status matrix/update procedure.

## Navigation Design

Recommended Starlight sidebar order:

- Orientation: `index.md`, `guarantees-and-limits.md`
- Current Artifact Quickstart: `quickstart/current-artifacts.md`
- Concepts: `concepts/provenance-model.md`, `concepts/modes.md`
- Guides: `guides/review-audit-evidence.md`, `guides/integrate-go-library.md`, `guides/extend-source-tools.md`
- Reference: `reference/status-matrix.md`, `reference/current-surfaces.md`
- Architecture: `architecture/overview.md`, `architecture/kernel-runtime-storage.md`, `architecture/security-determinism-tradeoffs.md`, `architecture/adrs.md`
- Adoption And Compliance: `ops/adoption-caveats.md`

This launch sidebar is 15 pages and is the complete planned public page set for launch. Future expansion is optional and should be driven by usage data, support questions, newly shipped surfaces, or demonstrated contributor workflow needs, not by a desire to mirror every internal architecture file as a public page.

Each page should include:

- Status label.
- Audience label when useful: `Finance`, `Engineering`, `Compliance`, or `Integrator`.
- Source references section with links to repository docs/code.
- Last verified command when the page includes runnable commands.

## Validation And Tooling

Minimum local tooling:

- Astro + Starlight site under `docs-site/`.
- Local docs scripts in `docs-site/package.json`: `npm run dev`, `npm run build`, `npm run check`, and optionally `npm run preview`.
- Optional root forwarding scripts only if they fit existing repository conventions; do not add root package infrastructure solely for docs convenience.
- Markdown linting with a small project config. Avoid rules that fight generated tables.
- Link checking for internal Markdown links and built static site links.
- A spell checker with a repository word list for Provenant terms: Provenant, `submit_answer`, `CapturedRead`, `ReadResult`, `memio`, `mintable_paths`, `DFAH`, `HITL`, `Mode A`, `Mode B`, `Starlight`.
- A status-matrix check that requires every capability page to declare one of the four content status labels.
- Generated reference checks for error codes, entry kinds, CLI help, or Go API pages if those pages are generated.

Validation commands after implementation should include:

- `go test ./...` for repository behavior.
- Docs install/build/check commands introduced by the docs project.
- `git diff --check`.

## Implementation Phases

### Phase 0: Inventory And Guardrails

- Confirm the implemented command surface by reading `cmd/`, `access/`, and current tests.
- Create the docs status matrix before writing user-facing pages.
- Create the initial Astro + Starlight project under `docs-site/` and choose the package manager from existing repository conventions, defaulting to `npm` if none exists.
- Add the first Provenant theme CSS using `audit/report/html.go` and generated report HTML as the current design-token source.
- Add docs contribution rules: do not claim planned surfaces as implemented; do not overstate Mode B; keep regulatory language non-legal-advice.

Done when:

- A status matrix exists.
- The docs project can build a placeholder Starlight site.
- CI or local scripts can run docs checks.

### Phase 1: Reader Journey

- Write the orientation pages, the single current-artifacts quickstart, and the compact core concept pages.
- Port only current, runnable examples first.
- Add caveats inline on Mode B, tamper evidence, and Go API-only verify/replay.

Done when:

- A new reader can understand Provenant and run at least one current example.
- Every runnable command has been verified.

### Phase 2: Engineering Guides

- Write the Go library integration guide and the consolidated source/tool/skill extension guide.
- Mark guides as architecture contract where current implementation is not complete.
- Add implementation checklists and stop conditions.

Done when:

- A contributor can identify the correct source docs before changing kernel, tools, or skills.
- Each guide links to validation commands and canonical source docs.

### Phase 3: Reference Generation

- Generate or synchronize the compact current-surfaces reference sections for CLI, error codes, entry kinds, bundle format, Go API, and tool category status.
- Add stale-generation checks if generation scripts are created.

Done when:

- Reference pages are reproducible from source or clearly marked manual.
- CI fails on stale generated pages where generation exists.

### Phase 4: Operations And Compliance

- Write the audit evidence review guide and consolidated adoption caveats page, including deployment shapes, Mode B attestation, accepted caveats, and regulatory context.
- Review with a compliance-audience lens for overclaims.

Done when:

- Non-technical users can inspect an audit artifact and understand what it proves.
- The adoption caveats page explicitly covers all accepted v0 limitations that affect buyer or reviewer interpretation.

### Phase 5: Polish And Release

- Add search metadata, redirects if needed, diagrams, page descriptions, and version banner.
- Run full docs and repo validation.
- Prepare a docs release checklist.

Done when:

- The site builds cleanly.
- Links pass.
- Status matrix and caveats are reviewed.
- No page presents planned/deferred surfaces as current shipped behavior.

## Non-Goals

- Do not implement new Provenant runtime, CLI, MCP, HTTP, kernel, or tool features as part of docs work.
- Do not invent APIs, commands, screenshots, configuration files, or hosted workflows that are not present in the repo or clearly marked planned.
- Do not rewrite canonical architecture decisions in the docs site without updating the source architecture docs first.
- Do not create legal advice or claim regulatory approval.
- Do not claim Mode B is non-bypassable for arbitrary host-channel text.
- Do not claim v0 audit packs are externally notarized, tamper-proof, or cryptographically witnessed by Provenant.

## Stop Conditions

Stop and ask for direction if any of these occur:

- The implementation surface conflicts with the architecture docs.
- A page would need to claim that a planned surface is implemented to make the reader journey coherent.
- The docs tooling requires broad repository restructuring.
- Generated references require code changes outside the docs scope.
- Starlight cannot satisfy a hard requirement discovered during implementation.
- Any command examples cannot be verified locally.

## Caveats For Writers

- Say `Provenant-signed` or `Provenant-attested` only when describing output from passing `submit_answer` with a marker verified against the audit chain/audit pack.
- Say `internally tamper-evident` for v0 audit chains; do not say `tamper-proof`.
- Say `captured replay is deterministic` when discussing replay; do not promise fresh runs across deployments are byte-identical when external sources can drift.
- Treat MCP as an equal optional access surface, not the primary architecture.
- Treat library mode as the product framing: the kernel emits bundle bytes and the host decides storage.
- Keep examples grounded in `README.md`, `examples/`, current tests, and architecture docs.
