# Goal Prompt For Provenant Public Docs Writing

You are the orchestrator for the Provenant public documentation writing effort.
Use `.planning/documentation/documentation-plan.md` as the canonical plan.

## Objective

Write the public Provenant docs through a dedicated GPT-5.5 agent named
`docs writer`, one PR at a time, with low-tier reader agents feeding concise
source briefs before each PR.

This planning PR only replaces:

- `.planning/documentation/documentation-plan.md`
- `.planning/documentation/goal-prompt.md`

After this planning PR is opened, stop for human review. Later docs
implementation follows Mode A sequencing.

## Required Agents

Dedicated author:

- Name: `docs writer`
- Model: `gpt-5.5`
- Role: author public docs content only

Low-tier reader feeders:

- Product and audience reader
- Implementation surfaces reader
- Examples and tests reader
- Architecture and tradeoffs reader
- Link and validation reader

Readers gather facts and caveats. They do not write final public docs prose.

## Required Reading

Before each docs PR:

- `.planning/documentation/documentation-plan.md`
- Current `docs-site/` route and sidebar state
- Page-specific source files named in the plan
- Reader briefs for the active PR

## Hard Requirements

- Preserve the launch IA capped at 15 pages.
- Keep Provenant framed as a provenance enforcement layer/library for
  regulated finance work.
- Do not call Provenant SaaS, an agent, a database, or a workflow engine.
- Do not overclaim MCP Mode B. Host-channel text is official only if it comes
  through passing `submit_answer` and verification.
- Do not claim v0 external notary, tamper-proof evidence, or legal advice.
- Label current, architecture, planned, and deferred surfaces clearly.
- Keep examples framed as proof fixtures, not full product scope.
- Do not edit runtime, CLI, MCP, HTTP, kernel, storage, examples, or source docs
  unless a later human-approved scope explicitly says so.

## Mode A Docs Sequence

For each docs implementation slice after this planning PR:

1. Fetch `origin/main` and branch from it.
2. Spawn the required low-tier reader feeders.
3. Collect short source-backed briefs.
4. Spawn or reuse the GPT-5.5 `docs writer`.
5. Write exactly one active public docs page. Update shared sidebar,
   navigation, or status-matrix files only when needed for that single page.
6. Run validation.
7. Run automated review.
8. Open a PR using the title convention in the plan.
9. Record the next branch handoff.
10. Continue only when Mode A approval criteria are satisfied.

## Planning PR Scope

For the current planning PR, edit only:

- `.planning/documentation/documentation-plan.md`
- `.planning/documentation/goal-prompt.md`

Do not edit:

- `docs-site/`
- `docs/`
- `architecture/`
- `examples/`
- Go source
- package manifests

## Validation

For this planning PR:

```bash
rtk git diff --check
rtk git status --short
```

For later docs implementation PRs, run from `docs-site/` where applicable:

```bash
cd docs-site && rtk npm run build
cd docs-site && rtk npm run check
rtk git diff --check
rtk git status --short
```

Run Go verification only when the edited docs make claims that need fresh code
or example verification.

## Stop Conditions

Stop if:

- The planning PR is opened and awaits human review.
- A reader brief contradicts another reader brief on a public claim.
- Current code and source docs disagree on a user-facing claim.
- A page requires a planned/deferred feature to be described as implemented.
- The launch IA would exceed 15 pages.
- Validation cannot run.
- The requested work touches non-docs implementation scope.

## PR Title Convention

Use concise Conventional Commit style titles:

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

## Final Response Format

Return exactly:

```markdown
## Scope
- Files changed
- Scope boundary observed

## Findings
- Current docs/planning state
- Any relevant caveats

## Actions
- What changed

## Validation
- Commands run and results

## Risks
- Remaining risks or next stop condition
```
