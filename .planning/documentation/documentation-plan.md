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

Use chronological IA. A reader should be able to move from problem, to concept, to first artifact, to verification, to integration, to architecture details. The site should support two visible reader paths: a non-technical evidence path for controllers, audit leads, compliance officers, and model-risk reviewers; and a technical implementation path for platform engineers, integration authors, and contributors. The same sidebar serves both paths by starting with plain-language framing, moving through runnable artifacts, then branching into task guides, reference, architecture, and operations.

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
3. `quickstart/reconciliation-fixture.md`
4. `guides/review-an-audit-bundle.md`
5. `ops/audit-review.md`
6. `ops/mode-b-attestation.md`
7. `ops/caveats.md`
8. `ops/regulatory-context.md`

Technical path:

1. `index.md`
2. `quickstart/install-and-test.md`
3. `quickstart/read-mint-bundle.md`
4. `quickstart/verify-and-replay.md`
5. `concepts/source-readers.md`
6. `concepts/provenance-tokens.md`
7. `guides/integrate-go-library.md`
8. `guides/add-a-source-reader.md`
9. `guides/add-a-tool-backend.md`
10. `reference/status-matrix.md`
11. Architecture deep dives as needed.

### 1. Start Here

#### Page: Home

- `Route/path`: `index.md`
- `Audience`: All readers; especially first-time finance, compliance, and platform evaluators.
- `Purpose`: Explain Provenant in one sitting without assuming Go, MCP, or agent-framework knowledge.
- `Content outline`: Plain-language one-paragraph explanation; regulated-finance problem statement; what Provenant is; current mental model from source bytes to audit bundle; who should read which path next; short glossary for captured read, token, audit entry, audit bundle, replay, `submit_answer`, Mode A, and Mode B; status callout for current repository examples.
- `Source material`: `README.md`, `docs/thesis.md`, `architecture/OVERVIEW.md`.
- `Status label`: `Implemented now` for the repository mental model and examples; `Architecture contract` for broader deployment framing.
- `Acceptance notes`: A non-engineer can explain what evidence Provenant produces after reading this page; the page does not call Provenant a SaaS, database, agent, logger, or generic guardrail product; every current command claim links to a quickstart.

#### Page: What Provenant Is Not

- `Route/path`: `what-provenant-is-not.md`
- `Audience`: Buyers, compliance reviewers, integration evaluators, and contributors who need scope boundaries.
- `Purpose`: Prevent overclaiming and make non-goals explicit before readers reach deeper technical material.
- `Content outline`: Not a SaaS in v0; not an agent or model; not a database or workflow engine; not a replacement for ERP/GL/extraction systems; not a general chat guardrail; not an external notary; not framework-coupled; what each non-goal means for adoption decisions.
- `Source material`: `docs/thesis.md`, `README.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/OVERVIEW.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page distinguishes optional access surfaces from the library-as-product framing; it explicitly says v0 is not externally notarized or tamper-proof.

#### Page: Guarantees And Limits

- `Route/path`: `guarantees-and-limits.md`
- `Audience`: Compliance, audit, model-risk, and engineering evaluators.
- `Purpose`: State what the four invariants guarantee and where the guarantees stop.
- `Content outline`: Four kernel invariants; internally tamper-evident audit chains; captured replay versus fresh-run equivalence; Mode A non-bypassability; Mode B host-channel caveat; self-declared host conformance caveat; root-compromise scope; legal-advice disclaimer for regulatory interpretation; links to deeper operations pages.
- `Source material`: `README.md`, `docs/thesis.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/README.md`, `architecture/subsystems/harness.md`, `architecture/adapters/mcp.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page says `internally tamper-evident`, not `tamper-proof`; it says arbitrary Mode B host text is not Provenant-attested unless it came from passing `submit_answer` and the marker verifies against the audit chain or audit pack.

### 2. Quickstarts

#### Page: Install And Test

- `Route/path`: `quickstart/install-and-test.md`
- `Audience`: Engineers, contributors, and technical evaluators.
- `Purpose`: Get the repository cloned and validated with the current repo-wide test command.
- `Content outline`: Prerequisites; clone command; repository layout snapshot; `go test ./...`; expected high-level result; what failures usually mean; next quickstart choices.
- `Source material`: `README.md`, `go.mod`, current repository tests.
- `Status label`: `Implemented now`.
- `Acceptance notes`: Commands are copy-pasteable from repository root; last verified command is recorded; no docs-site installation assumptions appear on this page until a docs project exists.

#### Page: Read, Mint, Bundle

- `Route/path`: `quickstart/read-mint-bundle.md`
- `Audience`: Engineers evaluating the minimum Provenant loop.
- `Purpose`: Show the smallest implemented flow from JSON source bytes to a token and bundle.
- `Content outline`: What the command demonstrates; request JSON shape; `go run ./cmd/provenant read-mint-bundle --request request.json`; response keys; decoded `bundle_bytes` shape; why `extract.json` is the only reader supported by this CLI surface today; link to smoke tests; common failure modes.
- `Source material`: `README.md`, `cmd/provenant`, `access/smoke/smoke_test.go`, source reader implementation and tests.
- `Status label`: `Implemented now`.
- `Acceptance notes`: The page does not imply CSV/XLSX readers are exposed through `read-mint-bundle`; it labels `extract.json` as the only current reader for this surface.

#### Page: Reconciliation Fixture

- `Route/path`: `quickstart/reconciliation-fixture.md`
- `Audience`: Finance reviewers, compliance reviewers, and engineers who want a tangible artifact quickly.
- `Purpose`: Run the deterministic reconciliation example and inspect the ZIP evidence package.
- `Content outline`: What the fixture does; input files and exact-match behavior; command from `examples/reconciliation`; `--overwrite`; the eight ZIP entries in reader-friendly order; sample `audit.jsonl` and `manifest.json` interpretation; absolute path caveat; failure and edge-case fixture links; what the fixture is not.
- `Source material`: `README.md`, `examples/reconciliation/README.md`, `examples/reconciliation/`, `access/cli` tests.
- `Status label`: `Implemented now`.
- `Acceptance notes`: The page lists exactly the current ZIP artifacts; it avoids describing a complete reconciliation product, fuzzy matching, HTTP endpoint, or MCP tool as current behavior.

#### Page: Verify And Replay

- `Route/path`: `quickstart/verify-and-replay.md`
- `Audience`: Engineers and audit reviewers who need to validate bundle integrity.
- `Purpose`: Explain current offline verification and replay using the Go API.
- `Content outline`: What verify checks; what replay re-derives; Go snippet using `kernel/public/bundle`; `go test ./access/cli`; no standalone CLI verb caveat; captured replay versus fresh-run equivalence; where bundle format reference lives.
- `Source material`: `README.md`, `kernel/public/bundle`, `access/cli` tests, `architecture/subsystems/audit.md`.
- `Status label`: `Implemented now`.
- `Acceptance notes`: The page explicitly states that `bundle.Verify` and `bundle.Replay` are Go API-only today unless implementation changes; no nonexistent `provenant verify` command is documented.

#### Page: Derivation Graph Demo

- `Route/path`: `quickstart/derivation-graph-demo.md`
- `Audience`: Technical evaluators and finance readers who need to see multi-step lineage.
- `Purpose`: Show a richer end-to-end demo where a final operating-income value traces back to source rows and derived tokens.
- `Content outline`: Demo goal; committed CSV datasets; operating-income calculation tree; seed and determinism note; `go test ./examples/derivation-graph/...`; artifact and trace overview; golden regeneration pointer; relationship to provenance-token and audit-bundle concepts.
- `Source material`: `README.md`, `examples/derivation-graph/README.md`, `examples/derivation-graph/` tests and fixtures.
- `Status label`: `Implemented now`.
- `Acceptance notes`: The page keeps the walkthrough grounded in committed demo files and does not generalize the demo into arbitrary financial planning or reconciliation behavior.

### 3. Concepts

#### Page: Provenance Boundary

- `Route/path`: `concepts/provenance-boundary.md`
- `Audience`: All readers, with emphasis on model-risk and platform evaluators.
- `Purpose`: Explain why Provenant treats provenance as an enforcement boundary rather than an observability log.
- `Content outline`: Logging-after-the-fact problem; single exit as the boundary; why fail-closed matters; what a Provenant-signed answer means; examples of claims that should be rejected; links to `submit_answer`, audit chain, and modes.
- `Source material`: `docs/thesis.md`, `architecture/subsystems/guards.md`, `architecture/README.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page avoids implying Provenant controls arbitrary host text in Mode B; it routes that caveat to `concepts/modes.md` and `ops/mode-b-attestation.md`.

#### Page: Source Readers

- `Route/path`: `concepts/source-readers.md`
- `Audience`: Engineers, source-reader authors, and technical auditors.
- `Purpose`: Define source bytes, declared hashes, selectors, `CapturedRead`, and `ReadResult`.
- `Content outline`: Source-byte contract; declared SHA-256; selector fields; captured-read identity and origin; reader name/version; output hash; implemented reader status by surface; how source readers feed token minting; diagrams for source bytes to captured read.
- `Source material`: `README.md`, `architecture/subsystems/tools.md`, `architecture/tools/extract/README.md`, current reader tests.
- `Status label`: `Implemented now` for existing extract readers and fixture paths; `Architecture contract` for general reader-extension rules.
- `Acceptance notes`: The page separates `extract.json` in `read-mint-bundle` from `extract.csv_rows` and `extract.spreadsheet` in the reconciliation fixture.

#### Page: Provenance Tokens

- `Route/path`: `concepts/provenance-tokens.md`
- `Audience`: Engineers, audit reviewers, and finance readers who need per-number lineage.
- `Purpose`: Explain how consequential values become kernel-minted tokens.
- `Content outline`: Token purpose; binding to captured reads; canonical decimal and unit; run and tenant binding; derived values and parent token IDs; normalizer visibility; fail-closed minting; examples from read-mint-bundle, reconciliation, and derivation graph.
- `Source material`: `README.md`, `architecture/subsystems/kernel.md`, `architecture/subsystems/audit.md`, `examples/derivation-graph/README.md`.
- `Status label`: `Implemented now` for current token shapes in examples; `Architecture contract` for full token graph semantics.
- `Acceptance notes`: Numeric examples use canonical decimal strings and avoid `float64` framing; tenant/run boundary language matches architecture docs.

#### Page: Audit Chain

- `Route/path`: `concepts/audit-chain.md`
- `Audience`: Audit reviewers, compliance readers, and engineers.
- `Purpose`: Explain the hash-chained audit log and deterministic payload rules.
- `Content outline`: Audit entry anatomy; sequence, kind, previous hash, current hash; deterministic versus non-deterministic payload fields; canonical hash input; internal tamper-evidence; sample `token_mint` entry; what breaks verification; why this is not external notarization.
- `Source material`: `README.md`, `architecture/subsystems/audit.md`, `docs/KNOWN_TRADE_OFFS.md`, reconciliation output examples.
- `Status label`: `Implemented now` for current audit JSONL examples; `Architecture contract` for full entry-kind model.
- `Acceptance notes`: The page uses `tamper-detectable by verifying the internal hash chain`; it includes the no-external-notary caveat inline.

#### Page: Audit Bundles

- `Route/path`: `concepts/audit-bundles.md`
- `Audience`: Finance reviewers, compliance reviewers, and engineers.
- `Purpose`: Describe the self-contained evidence package emitted by the current examples and the architecture.
- `Content outline`: Bundle snapshot; bundle bytes and SHA-256; run descriptor; captured reads; tokens; audit JSONL; companion artifacts in reconciliation ZIP; verify and replay; limitations of offline replay; storage is caller-owned in library mode.
- `Source material`: `README.md`, `architecture/subsystems/audit.md`, `architecture/subsystems/storage.md`, `examples/reconciliation/README.md`.
- `Status label`: `Implemented now` for current bundle shape and reconciliation artifacts; `Architecture contract` for broader storage stance.
- `Acceptance notes`: The page does not claim externally witnessed bundles; it distinguishes bundle bytes from where a host stores them.

#### Page: Submit Answer

- `Route/path`: `concepts/submit-answer.md`
- `Audience`: Engineers, integrators, and compliance readers reviewing release semantics.
- `Purpose`: Explain the single release exit and how claims are grounded before output is considered Provenant-attested.
- `Content outline`: Why `submit_answer` exists; claim extraction; claim-to-token grounding; validators and guard severity; fail-closed outcome; signature marker semantics; what a rejected answer means; relationship to Mode A and Mode B.
- `Source material`: `docs/thesis.md`, `architecture/subsystems/guards.md`, `architecture/subsystems/harness.md`, `architecture/subsystems/audit.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page only uses `Provenant-signed` or `Provenant-attested` for output that passed `submit_answer` and can be verified against the chain or pack.

#### Page: Modes

- `Route/path`: `concepts/modes.md`
- `Audience`: Integrators, MCP host authors, platform engineers, and compliance reviewers.
- `Purpose`: Explain Mode A and Mode B before readers choose an integration path.
- `Content outline`: Mode A CLI/embedded harness control; Mode B host-driven loop; non-bypassable output channel in Mode A; Mode B host-channel limitation; host conformance self-declaration; when Mode A is required; adapter equality and no MCP primacy; streaming and split-turn caveats.
- `Source material`: `docs/thesis.md`, `architecture/subsystems/harness.md`, `architecture/adapters/mcp.md`, `architecture/adrs/0010-mcp-equal-not-primary.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page contains a prominent Mode B warning and does not imply MCP can force arbitrary host-channel output through Provenant.

### 4. How-To Guides

#### Page: Add A Source Reader

- `Route/path`: `guides/add-a-source-reader.md`
- `Audience`: Contributors and integration engineers adding new extraction paths.
- `Purpose`: Provide a task-oriented checklist for implementing a source reader without weakening provenance.
- `Content outline`: Prerequisites and source docs; reader contract; selector design; captured-read fields; output hash; reader name/version; mintable path implications; tests; status and surface-exposure checklist; stop conditions.
- `Source material`: `architecture/subsystems/tools.md`, `architecture/tools/extract/README.md`, `.agents/engineer.md`, existing reader code and tests.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The guide makes clear that adding a reader implementation is not the same as exposing it through every CLI/runtime surface.

#### Page: Add A Tool Backend

- `Route/path`: `guides/add-a-tool-backend.md`
- `Audience`: Tool authors, platform engineers, and contributors.
- `Purpose`: Explain how to add or evaluate a tool backend while preserving schema-first validation and token minting.
- `Content outline`: `ToolBackend` contract; input and output schemas; credentials and credential IDs; `mintable_paths`; backend-owned numeric normalization; built-in normalizers; conformance tests; audit entries; failure behavior; security review checklist.
- `Source material`: `architecture/subsystems/tools.md`, `architecture/tools/README.md`, `.agents/engineer.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The guide states tools cannot construct tokens directly and that invalid backend output must fail closed.

#### Page: Write A Skill Pack

- `Route/path`: `guides/write-a-skill-pack.md`
- `Audience`: Skill authors, integrators, and contributors.
- `Purpose`: Describe how skill packs should be structured and where v0 extensibility stops.
- `Content outline`: Pack purpose; prompts; tools; guard configuration; examples; bounds; built-in validators and normalizers; pack digest and future signing; no custom validators or sandboxed code in v0; supply-chain status; testing and review checklist.
- `Source material`: `architecture/subsystems/skills.md`, `architecture/adrs/0006-skill-pack-format.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/subsystems/audit.md`.
- `Status label`: `Architecture contract` with `Deferred` callouts for custom validators, WASM/plugin loading, hosted registry, and signing beyond local digest status.
- `Acceptance notes`: The guide does not claim skill-pack signing or hosted registry exists today unless implementation confirms it.

#### Page: Integrate The Go Library

- `Route/path`: `guides/integrate-go-library.md`
- `Audience`: Go platform engineers embedding Provenant.
- `Purpose`: Show the intended library-mode shape and host responsibilities.
- `Content outline`: Library-as-product framing; kernel public API boundary; `memio` default; injected `Store`, `AuditSink`, `BlobStore`, `ModelClient`, `Clock`, `RandSource`, and `TransactionalScope`; no kernel I/O; emitted bundle bytes; host storage responsibility; HITL pause/resume responsibility; validation commands.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/subsystems/kernel.md`, `architecture/subsystems/storage.md`, `architecture/adrs/0011-storage-is-a-deployment-concern.md`, `.agents/engineer.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The guide preserves the pure-Go kernel boundary and does not introduce database, network, CLI, or MCP types into kernel examples.

#### Page: Use MCP Safely

- `Route/path`: `guides/use-mcp-safely.md`
- `Audience`: MCP host authors, integration engineers, and compliance evaluators.
- `Purpose`: Explain how to reason about MCP integration without overstating Mode B.
- `Content outline`: Current implementation status check; MCP as optional equal adapter; Mode B host-driven loop; host-channel text warning; conformance self-declaration; when to choose Mode A instead; audit-pack interpretation; stop conditions for unsigned host text.
- `Source material`: `architecture/adapters/mcp.md`, `architecture/subsystems/harness.md`, `architecture/adrs/0010-mcp-equal-not-primary.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract` unless current code exposes a specific MCP setup flow; specific setup steps must be `Implemented now` only if verified.
- `Acceptance notes`: The guide always includes the Mode B stop warning and never presents MCP as architecturally primary.

#### Page: Review An Audit Bundle

- `Route/path`: `guides/review-an-audit-bundle.md`
- `Audience`: Controllers, audit leads, compliance reviewers, and model-risk reviewers.
- `Purpose`: Provide a practical, non-technical review flow for inspecting evidence artifacts.
- `Content outline`: What artifact to start with; manifest and bundle hash; captured reads; tokens; audit JSONL; `report.md` and `report.json`; replay output; what passes, what fails, and what remains a business judgment; escalation checklist; Mode B marker caveat.
- `Source material`: `README.md`, `examples/reconciliation/README.md`, `architecture/subsystems/audit.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Implemented now` for current reconciliation artifacts; `Architecture contract` for generic reviewer workflow.
- `Acceptance notes`: The guide explains evidence artifacts before internals and avoids legal-advice language.

### 5. Reference

#### Page: CLI Reference

- `Route/path`: `reference/cli.md`
- `Audience`: Engineers and contributors.
- `Purpose`: List current CLI commands accurately and leave planned commands out of the current surface.
- `Content outline`: Generation or synchronization method; `read-mint-bundle`; `reconcile`; flags and examples; known absent commands; tests that cover CLI behavior; last verified command.
- `Source material`: `README.md`, `cmd/provenant`, `access/cli`, command help output if generated.
- `Status label`: `Implemented now` for commands confirmed in code.
- `Acceptance notes`: The page does not list planned audit-pack, HTTP, MCP, or token-issuance CLI flows as current behavior.

#### Page: Go API Reference

- `Route/path`: `reference/go-api.md`
- `Audience`: Go engineers and contributors.
- `Purpose`: Orient readers to stable public Go packages without duplicating generated package docs.
- `Content outline`: Public package map; `kernel/public/bundle`; `kernel/public/memio`; source reader and token surfaces; import-boundary warning; links to generated docs if added later; examples that compile.
- `Source material`: `README.md`, `kernel/public/`, `architecture/subsystems/kernel.md`, `architecture/OVERVIEW.md`.
- `Status label`: `Implemented now` for packages present in code; `Architecture contract` for public-surface rules.
- `Acceptance notes`: The page links to canonical Go docs or source for signatures and does not invent APIs.

#### Page: Bundle Format

- `Route/path`: `reference/bundle-format.md`
- `Audience`: Engineers, audit reviewers, and tool authors.
- `Purpose`: Document the current bundle schema and companion reconciliation artifacts.
- `Content outline`: Bundle format identifier; top-level fields; run descriptor; captured reads; tokens; audit JSONL; bundle hash; reconciliation ZIP companions; manifest artifact hashes; compatibility and generation notes.
- `Source material`: `README.md`, `kernel/public/bundle`, `architecture/subsystems/audit.md`, `examples/reconciliation/README.md`.
- `Status label`: `Implemented now`.
- `Acceptance notes`: The page uses current fixture output or generated reference data and fails stale checks if generation is introduced.

#### Page: Audit Entry Kinds

- `Route/path`: `reference/audit-entry-kinds.md`
- `Audience`: Engineers, reviewers, and contributors.
- `Purpose`: Provide a table of audit entry kinds with deterministic and non-deterministic payload expectations.
- `Content outline`: Generation status; entry kind table; per-kind deterministic payload summary; non-deterministic fields; hash-input inclusion rules; links to schemas; update procedure.
- `Source material`: `architecture/subsystems/audit.md`, generated enum/source tables if available.
- `Status label`: `Architecture contract` unless generated from current code, then generated rows can be `Implemented now`.
- `Acceptance notes`: Prefer generation from canonical tables where possible; manual pages must be labeled manually synchronized.

#### Page: Error Codes

- `Route/path`: `reference/error-codes.md`
- `Audience`: Engineers, support operators, and contributors.
- `Purpose`: Provide canonical error-code meanings and update workflow.
- `Content outline`: Code namespace rules; generated or synchronized table; common errors from quickstarts; failure fixture mapping; how to add a code; drift check procedure.
- `Source material`: `architecture/subsystems/audit.md`, `kernel/public/errorcode`, `examples/reconciliation/failures/`, `.agents/engineer.md`.
- `Status label`: `Implemented now` if generated from source; otherwise `Architecture contract` with manual-sync caveat.
- `Acceptance notes`: The page does not hand-invent codes; every code maps back to the canonical architecture table or generated source.

#### Page: Tool Categories

- `Route/path`: `reference/tool-categories.md`
- `Audience`: Tool authors, integrators, and technical evaluators.
- `Purpose`: Summarize tool categories and distinguish implemented fixture tools from architecture-level categories.
- `Content outline`: Category table for `ledger`, `extract`, `reconcile`, `calc`, `policy`, `external`, `match`, and `dedupe`; implemented readers and fixture tools; planned/deferred backend surfaces; mintable path expectations; links to per-tool architecture docs.
- `Source material`: `architecture/subsystems/tools.md`, `architecture/tools/**`, `README.md`, current tool code and tests.
- `Status label`: Mixed; each row must carry its own status label.
- `Acceptance notes`: The table clearly separates current `extract.json`, `extract.csv_rows`, `extract.spreadsheet`, reconciliation fixture, and derivation graph tooling from broader planned tool categories.

#### Page: Status Matrix

- `Route/path`: `reference/status-matrix.md`
- `Audience`: All readers, especially evaluators deciding what exists today.
- `Purpose`: Provide one authoritative matrix of implemented, architecture-contract, planned, and deferred surfaces.
- `Content outline`: Status-label definitions; command surface; readers; bundle verify/replay; adapters; storage; skill packs; validators/normalizers; hosted registry; non-Go bindings; HTTP; MCP; HITL persistence; external notarization; links to source references.
- `Source material`: `README.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/OVERVIEW.md`, `architecture/adapters/*.md`, implementation status from code.
- `Status label`: `Implemented now` for the page itself once maintained; individual rows carry all four labels.
- `Acceptance notes`: This page is created before broad page authoring and acts as the site-wide overclaim guard.

### 6. Architecture Deep Dive

#### Page: Architecture Overview

- `Route/path`: `architecture/overview.md`
- `Audience`: Technical evaluators, contributors, and architecture reviewers.
- `Purpose`: Present a curated docs-site version of the integrated architecture.
- `Content outline`: Library-as-product diagram; kernel/runtime/access-surface boundaries; technology stack summary; module boundary contract; where to read next; current open questions.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/README.md`, `docs/thesis.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page links back to canonical architecture docs rather than becoming a divergent source of truth.

#### Page: Kernel Boundary

- `Route/path`: `architecture/kernel-boundary.md`
- `Audience`: Contributors and platform engineers.
- `Purpose`: Make the pure-Go kernel boundary and allowed public surface concrete.
- `Content outline`: Allowed public API types; forbidden imports; injected interfaces; `TransactionalScope`; `memio`; no I/O; token/audit atomicity; contributor checklist.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/subsystems/kernel.md`, `.agents/engineer.md`, ADR-0009, ADR-0011.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page preserves all kernel public-surface prohibitions and routes implementation details to canonical docs.

#### Page: Runtime And Adapters

- `Route/path`: `architecture/runtime-and-adapters.md`
- `Audience`: Integrators, adapter authors, and platform engineers.
- `Purpose`: Explain runtime responsibilities and equal adapter stance.
- `Content outline`: Runtime owns I/O; adapters translate, not enforce; CLI, MCP, HTTP overview; LangChain and OpenAI adapter slots; adapter equality; no MCP primacy; deterministic parity expectations; latency budget.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/adapters/cli.md`, `architecture/adapters/mcp.md`, `architecture/adapters/http.md`, `architecture/adapters/langchain.md`, `architecture/adapters/openai.md`, ADR-0010.
- `Status label`: `Architecture contract` with current implementation callouts where code confirms surfaces.
- `Acceptance notes`: The page does not present HTTP, LangChain, OpenAI, or production MCP packaging as shipped unless implementation confirms it.

#### Page: Storage

- `Route/path`: `architecture/storage.md`
- `Audience`: Platform engineers, operators, and contributors.
- `Purpose`: Explain storage as a deployment concern and not a kernel concern.
- `Content outline`: Library emits bytes; host decides persistence; runtime SQLite/Postgres projections; S3-compatible blob storage; HITL pause/resume responsibilities; `RunState` snapshot; no database requirement; bundle retention considerations.
- `Source material`: `architecture/subsystems/storage.md`, `architecture/OVERVIEW.md`, ADR-0011, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page distinguishes current code from runtime persistence architecture and keeps kernel storage-free.

#### Page: Security

- `Route/path`: `architecture/security.md`
- `Audience`: Security reviewers, platform engineers, and compliance teams.
- `Purpose`: Summarize the threat model and security controls relevant to adoption.
- `Content outline`: Tenant isolation; prompt-injection defense; credential identity and handling; schema-first validation; fail-closed defaults; root-compromise limitation; internal tamper-evidence; supply-chain status; security review checklist.
- `Source material`: `architecture/cross-cutting/security.md`, `architecture/subsystems/guards.md`, `architecture/subsystems/audit.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: Root-equivalent host compromise is scoped accurately and not hidden.

#### Page: Determinism

- `Route/path`: `architecture/determinism.md`
- `Audience`: Engineers, reviewers, and model-risk readers.
- `Purpose`: Explain replay determinism, audit hash input, and known determinism limits.
- `Content outline`: Deterministic payloads; non-deterministic envelope fields; canonical JSON and hash construction; numeric canonicalization; parallel sibling ordering; captured replay; streaming and split-turn deferral; external-source fresh-run drift; verification commands.
- `Source material`: `architecture/subsystems/audit.md`, `architecture/subsystems/harness.md`, `docs/KNOWN_TRADE_OFFS.md`, ADR-0005.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page clearly separates captured deterministic replay from fresh-run equivalence across changing external sources.

#### Page: Trade-Offs

- `Route/path`: `architecture/trade-offs.md`
- `Audience`: Technical evaluators, compliance reviewers, and contributors.
- `Purpose`: Provide a human-readable index into accepted limitations.
- `Content outline`: How to read accepted trade-offs; internal-only tamper evidence; streaming and split-turn deferral; live observability caveat; self-declared host conformance; locale and normalizer limits; external-source version drift; case-bank correction caveat; skill-pack extensibility; library-mode HITL persistence; links to original entries.
- `Source material`: `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page summarizes without retroactively editing or weakening accepted trade-off language.

#### Page: ADR Index

- `Route/path`: `architecture/adrs.md`
- `Audience`: Contributors and architecture reviewers.
- `Purpose`: Make architectural decisions discoverable from the docs site.
- `Content outline`: ADR table; short decision summaries; status and supersession notes; especially ADR-0010 and ADR-0011; when a new ADR is required; links to source ADR files.
- `Source material`: `architecture/adrs/*.md`, `.agents/engineer.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page notes ADR-0010 supersedes MCP-primary framing and ADR-0011 establishes storage as deployment concern.

### 7. Operations And Compliance

#### Page: Audit Review

- `Route/path`: `ops/audit-review.md`
- `Audience`: Controllers, audit leads, compliance officers, and model-risk reviewers.
- `Purpose`: Provide a plain-language operating procedure for reviewing audit artifacts.
- `Content outline`: Review goals; artifact checklist; manifest and hash checks; captured-read inspection; token lineage inspection; audit chain verification; replay interpretation; business-judgment review; escalation outcomes; recordkeeping suggestions.
- `Source material`: `README.md`, `examples/reconciliation/README.md`, `architecture/subsystems/audit.md`, `docs/thesis.md`.
- `Status label`: `Implemented now` for current fixture artifacts; `Architecture contract` for general operating procedure.
- `Acceptance notes`: The page explains evidence artifacts before internals and avoids legal advice.

#### Page: Deployment Shapes

- `Route/path`: `ops/deployment-shapes.md`
- `Audience`: Operators, platform engineers, security reviewers, and compliance evaluators.
- `Purpose`: Compare library mode, CLI mode, MCP/runtime mode, and future/deferred deployment surfaces.
- `Content outline`: Library mode; CLI mode; MCP/runtime mode; stateless versus persistent runtime; storage choices; air-gapped and single-tenant framing; Mode A versus Mode B selection; deferred SaaS and non-Go binding notes; status table.
- `Source material`: `architecture/OVERVIEW.md`, `architecture/cross-cutting/deployment.md`, `docs/thesis.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract` with current implementation callouts.
- `Acceptance notes`: The page does not claim multi-tenant SaaS, hosted registry, or non-Go bindings are current behavior.

#### Page: Mode B Attestation

- `Route/path`: `ops/mode-b-attestation.md`
- `Audience`: Compliance reviewers, MCP host authors, and platform operators.
- `Purpose`: Give precise rules for interpreting `[PROVENANT-ATTESTED v1 ...]` and host text.
- `Content outline`: What the marker means; what must be verified against the audit chain or audit pack; host-channel text that bypasses `submit_answer`; `host_text_only` interpretation; self-declared conformance; when Mode A is required; reviewer checklist.
- `Source material`: `architecture/subsystems/harness.md`, `architecture/adapters/mcp.md`, `docs/thesis.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page contains the strongest Mode B caveat in the site and is cross-linked from all Mode B or MCP pages.

#### Page: Caveats

- `Route/path`: `ops/caveats.md`
- `Audience`: Buyers, compliance reviewers, operators, and technical evaluators.
- `Purpose`: Consolidate limitations that affect buyer, reviewer, or operator interpretation.
- `Content outline`: Internal-only tamper evidence; no external notary; Mode B self-declared conformance; streaming and split-turn deferral; external-source drift; skill-pack extensibility; library-mode HITL persistence; no hosted SaaS in v0; no legal advice; how to decide whether each caveat matters.
- `Source material`: `docs/KNOWN_TRADE_OFFS.md`, `docs/thesis.md`, `architecture/OVERVIEW.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page keeps caveats prominent rather than burying them below marketing or setup material.

#### Page: Regulatory Context

- `Route/path`: `ops/regulatory-context.md`
- `Audience`: Compliance officers, model-risk reviewers, audit leads, and buyers.
- `Purpose`: Frame why Provenant exists in SR 11-7 and FINRA 2026 contexts without making legal claims.
- `Content outline`: Non-legal-advice banner; reconstructability; reviewability; internal tamper detection; model-risk review questions; what Provenant evidence can help answer; what remains the deployer's responsibility; source references.
- `Source material`: `docs/thesis.md`, `architecture/references.md`, `README.md`, `docs/KNOWN_TRADE_OFFS.md`.
- `Status label`: `Architecture contract`.
- `Acceptance notes`: The page includes `This is not legal advice` near the top and does not claim regulatory approval.

### 8. Contributing To Docs

#### Page: Docs Contribution Guide

- `Route/path`: `contributing/docs.md`
- `Audience`: Contributors and maintainers writing or reviewing docs pages.
- `Purpose`: Keep future docs changes accurate, source-grounded, and status-labeled.
- `Content outline`: Required status labels; source-reference requirement; last-verified command rule; overclaim checklist; Mode B warning rule; tamper-evidence language rule; regulatory-language rule; generated-reference workflow; local docs commands after scaffold exists.
- `Source material`: This plan, `goal-prompt.md`, `README.md`, `docs/KNOWN_TRADE_OFFS.md`, `architecture/OVERVIEW.md`.
- `Status label`: `Planned` until docs site tooling exists; then `Implemented now` for the contribution process.
- `Acceptance notes`: The guide makes every capability page include a status label and source-reference section.

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
