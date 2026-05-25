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

Use chronological IA. A reader should be able to move from problem, to concept, to first artifact, to verification, to integration, to architecture details.

### 1. Start Here

Pages:

- `index.md`: What Provenant is, who it is for, and what problem it solves. Include a one-paragraph non-technical explanation and the current mental model from source bytes to audit bundle.
- `what-provenant-is-not.md`: Non-goals from `docs/thesis.md`, including not a SaaS in v0, not an agent, not a database, not a workflow engine, not a general guardrail product, and not an external notary.
- `guarantees-and-limits.md`: The four invariants, internal tamper-evidence caveat, Mode B attestation caveat, and the difference between replay of captured cases and fresh-run equivalence.

Expected content:

- Plain-language regulated-finance framing.
- A short glossary: captured read, token, audit entry, audit bundle, replay, `submit_answer`, Mode A, Mode B.
- A strong warning that Mode B host text is not Provenant-attested unless it comes from passing `submit_answer` and matches the audit chain/audit pack.

### 2. Quickstarts

Pages:

- `quickstart/install-and-test.md`: Clone, install prerequisites, run `go test ./...`, and explain expected outputs.
- `quickstart/read-mint-bundle.md`: Walk through the current `read-mint-bundle` request and response. Mark `extract.json` as the only supported reader for this surface today.
- `quickstart/reconciliation-fixture.md`: Run the reconciliation fixture, inspect the ZIP, explain each artifact, and rerun with `--overwrite`.
- `quickstart/verify-and-replay.md`: Show current Go API verification and replay, and explicitly state when no standalone CLI verb exists.

Expected content:

- Copy-paste commands that are tested against the repository.
- Minimal prerequisites.
- Screenshots only if generated from the current repo; otherwise avoid screenshots.
- Failure fixture links for non-happy paths.

### 3. Concepts

Pages:

- `concepts/provenance-boundary.md`: Explain why provenance is an enforcement boundary, not logging.
- `concepts/source-readers.md`: Source bytes, declared hashes, selectors, `CapturedRead`, and `ReadResult`.
- `concepts/provenance-tokens.md`: Token minting, canonical decimals, units, tenant/run binding, and derived values.
- `concepts/audit-chain.md`: Audit entries, hash chain, deterministic versus non-deterministic payload fields, and internal tamper evidence.
- `concepts/audit-bundles.md`: Snapshot, bundle bytes, bundle hash, verify, replay, and what offline replay can and cannot prove.
- `concepts/submit-answer.md`: Single exit, claim-to-token grounding, validators, fail-closed behavior, and signature marker semantics.
- `concepts/modes.md`: Mode A and Mode B, including hard non-bypassable control in Mode A and attestation-only host-channel limits in Mode B.

Expected content:

- Diagrams in Mermaid or Starlight-compatible Markdown, kept ASCII in source.
- Each concept page should link back to the corresponding architecture source docs.
- Avoid marketing claims that exceed the docs. For example, say `tamper-detectable by verifying the internal hash chain`, not `tamper-proof`.

### 4. How-To Guides

Pages:

- `guides/add-a-source-reader.md`: Based on source reader architecture and current reader examples. Include mintable path implications.
- `guides/add-a-tool-backend.md`: ToolBackend contract, schemas, credentials, mintable paths, normalizers, and conformance tests.
- `guides/write-a-skill-pack.md`: Skill prompts, tools, guards, examples, bounds, built-in-only validators/normalizers, and signing status.
- `guides/integrate-go-library.md`: Library-mode shape, `memio`, injected interfaces, no kernel I/O, bundle bytes, and host responsibilities.
- `guides/use-mcp-safely.md`: MCP setup only if current code supports it; otherwise make it architecture-level. Always include Mode B stop warnings.
- `guides/review-an-audit-bundle.md`: How a controller or reviewer inspects tokens, captured reads, audit JSONL, manifest, and replay output.

Expected content:

- Task-based steps with prerequisites and stop conditions.
- Explicit status labels for each guide.
- Cross-links to concepts instead of repeating architecture details.

### 5. Reference

Pages:

- `reference/cli.md`: Generated or manually synchronized CLI commands that exist today. Do not list planned commands as current.
- `reference/go-api.md`: Public Go API overview, with links to package docs if generated later.
- `reference/bundle-format.md`: Current bundle shape and artifact fields.
- `reference/audit-entry-kinds.md`: Generated from `architecture/subsystems/audit.md` or current source tables where possible.
- `reference/error-codes.md`: Generated from canonical error-code tables or source generation output.
- `reference/tool-categories.md`: Tool categories and status, separating implemented fixture tools from architecture categories.
- `reference/status-matrix.md`: One table of implemented, architecture contract, planned, and deferred surfaces.

Expected content:

- Prefer generated reference pages where source tables already exist.
- Add CI checks that fail when generated reference pages are stale.
- Link to source docs and code for every status claim.

### 6. Architecture Deep Dive

Pages:

- `architecture/overview.md`: Curated version of `architecture/OVERVIEW.md` for docs-site readers.
- `architecture/kernel-boundary.md`: Pure Go kernel, no I/O imports, injected interfaces, `TransactionalScope`, and memio.
- `architecture/runtime-and-adapters.md`: Runtime layer, CLI, MCP, HTTP, and adapter shims. Emphasize equal first-class adapters and no MCP primacy.
- `architecture/storage.md`: Library emits bytes; runtime persistence is optional and outside the kernel.
- `architecture/security.md`: Threat model, tenant isolation, prompt injection defense, credential handling, and root-compromise scope.
- `architecture/determinism.md`: Deterministic replay, audit hash input, parallel ordering, and streaming limitations.
- `architecture/trade-offs.md`: Human-readable index into `docs/KNOWN_TRADE_OFFS.md`.
- `architecture/adrs.md`: ADR index with short summaries and supersession notes.

Expected content:

- Architecture pages can be longer and more technical.
- Keep them navigable by linking back to quickstarts and concepts.
- Include caveats inline, not hidden at the bottom.

### 7. Operations And Compliance

Pages:

- `ops/audit-review.md`: Reviewer workflow for inspecting audit artifacts.
- `ops/deployment-shapes.md`: Library mode, CLI mode, MCP/runtime mode, and status labels.
- `ops/mode-b-attestation.md`: Clear rules for interpreting `[PROVENANT-ATTESTED v1 ...]` and host text.
- `ops/caveats.md`: Consolidated accepted limitations from known trade-offs.
- `ops/regulatory-context.md`: SR 11-7 and FINRA 2026 framing sourced from existing references, without legal advice.

Expected content:

- Plain language suitable for non-engineers.
- Include `This is not legal advice` where regulatory interpretation appears.
- Explain evidence artifacts before internals.

## Navigation Design

Recommended Starlight sidebar order:

- Start Here
- Quickstarts
- Concepts
- How-To Guides
- Reference
- Architecture
- Operations And Compliance
- Contributing To Docs

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

- Write Start Here, Quickstarts, and core Concepts pages.
- Port only current, runnable examples first.
- Add caveats inline on Mode B, tamper evidence, and Go API-only verify/replay.

Done when:

- A new reader can understand Provenant and run at least one current example.
- Every runnable command has been verified.

### Phase 2: Engineering Guides

- Write source reader, tool backend, skill pack, and Go library integration guides.
- Mark guides as architecture contract where current implementation is not complete.
- Add implementation checklists and stop conditions.

Done when:

- A contributor can identify the correct source docs before changing kernel, tools, or skills.
- Each guide links to validation commands and canonical source docs.

### Phase 3: Reference Generation

- Generate or synchronize CLI reference, error codes, entry kinds, bundle format, and tool category status.
- Add stale-generation checks if generation scripts are created.

Done when:

- Reference pages are reproducible from source or clearly marked manual.
- CI fails on stale generated pages where generation exists.

### Phase 4: Operations And Compliance

- Write audit review, deployment shapes, Mode B attestation, caveats, and regulatory-context pages.
- Review with a compliance-audience lens for overclaims.

Done when:

- Non-technical users can inspect an audit artifact and understand what it proves.
- The caveat pages explicitly cover all accepted v0 limitations that affect buyer or reviewer interpretation.

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
