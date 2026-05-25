# Goal Prompt For Provenant Docs Project

/cool You are working in the Provenant repository. Implement or begin the documentation project described in `.planning/documentation/documentation-plan.md` safely.

Objective:
Create a source-controlled Astro + Starlight documentation site for Provenant, or complete the next smallest safe slice toward that goal if the full project is too large for one session.

Required reading before edits:
- `.planning/documentation/documentation-plan.md`
- `README.md`
- `docs/thesis.md`
- `docs/KNOWN_TRADE_OFFS.md`
- `architecture/OVERVIEW.md`
- `architecture/README.md`
- `architecture/subsystems/harness.md`
- `architecture/adapters/mcp.md`
- Any source docs listed in the plan for pages you edit

Hard requirements:
- Recommend and use Astro + Starlight unless you find and document a concrete blocker.
- Build the public docs site under top-level `docs-site/` unless a concrete repository constraint requires stopping and asking first.
- Do not build the public documentation site under the existing `docs/` directory. Preserve `docs/` as repo-facing narrative, governance, thesis, and trade-off material.
- Preserve `architecture/` as the canonical architecture reference. Public docs may summarize, cite, and link to architecture files, but must not replace them or become a competing source of truth.
- Preserve `examples/` as runnable example evidence. Public quickstarts and guides may summarize, cite, and link to examples, but must not relocate or duplicate committed example artifacts as the authoritative source.
- Include source references in public docs so readers can trace public-facing summaries back to `docs/`, `architecture/`, `examples/`, tests, or code as appropriate.
- Use local docs scripts in `docs-site/package.json`, preferably `npm run dev`, `npm run build`, `npm run check`, and optionally `npm run preview` unless the repo already has a different package-manager convention.
- Make the docs site follow the Provenant landing-page design language and keep it highly customizable with HTML/CSS.
- Because the actual landing-page file is not present in the repo, use `audit/report/html.go` and generated audit report HTML as the discoverable design-token source. Mention future alignment with the actual landing page asset if it is later provided.
- Keep docs specific to Provenant: finance provenance, captured reads, tokens, audit chain, audit bundles, `submit_answer`, Mode A, Mode B, kernel/runtime boundaries, and current reconciliation examples.
- Clearly distinguish `Implemented now`, `Architecture contract`, `Planned`, and `Deferred` surfaces on every capability page.
- Do not overclaim MCP Mode B. State that arbitrary host-channel text is not Provenant-attested unless it came from passing `submit_answer` and the marker is verified against the audit chain/audit pack.
- Do not claim v0 audit evidence is tamper-proof or externally notarized. Use internally tamper-evident language unless source docs have changed.
- Do not present planned or deferred surfaces as shipped behavior.
- Keep regulatory content as context, not legal advice.
- Keep changes scoped to documentation and docs tooling unless you stop and ask first.

Preferred first slice:
1. Confirm current implemented commands and examples from `README.md`, `cmd/`, `examples/`, and tests.
2. Create the Astro + Starlight scaffold in `docs-site/`.
3. Add the status-label convention and a status matrix page.
4. Add Provenant theme overrides and any minimal custom Astro components needed for status labels, caveat callouts, source references, and audit-artifact cards.
5. Add the initial Start Here and Quickstart pages from the plan, using only currently implemented commands.
6. Add package scripts for docs build/check if a package is introduced.
7. Add internal link and content-status validation where practical for the first slice.
8. Run validation.

Validation to run before reporting done:
- `go test ./...`
- Docs install/build/check commands you introduce from `docs-site/`, such as `npm run build` and `npm run check`
- `git diff --check`
- `git status --short`

Non-goals:
- Do not implement new Provenant runtime, CLI, MCP, HTTP, kernel, source reader, tool backend, or skill-pack functionality.
- Do not invent APIs, commands, screenshots, configuration files, hosted workflows, or marketplace flows.
- Do not place the public docs site, public page hierarchy, or Starlight content tree under existing `docs/`.
- Do not move, rename, or collapse existing `docs/`, `architecture/`, or `examples/` source material into `docs-site/`.
- Do not rewrite canonical architecture decisions in the docs site without updating the source architecture docs first and getting explicit approval.
- Do not add broad repo restructuring just to host docs.

Stop conditions:
- Stop if current code and architecture docs disagree in a way that affects user-facing claims.
- Stop if Starlight requires a dependency or repo layout change that seems broader than docs tooling.
- Stop if a planned/deferred feature would need to be described as implemented to complete a page.
- Stop if generated reference pages require code changes outside docs scope.
- Stop if validation cannot run because dependencies or network access are unavailable; report exactly what was skipped and why.

Final response format:
- Summarize files changed.
- State which plan phase or slice was completed.
- List validation commands and results.
- List any caveats, skipped items, and next recommended slice.
