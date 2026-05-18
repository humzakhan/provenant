---
name: phase-planning
description: Use when the user asks Claude to plan the next phase, create or update `.planning/phases/<phase-slug>/` docs, or mirror the repo's phase-planning format. Produces the phase README, goal kickstart prompt, and sequential work packets with explicit scope, non-goals, stop conditions, pass criteria, and validation evidence.
---

# Phase Planning

Use this skill when the request is to plan a new implementation phase or revise an existing future-phase plan.

## Read first

1. `AGENTS.md`
2. `.planning/phases/implementation-sequence.md`
3. The nearest prior phase docs:
   - `.planning/phases/<previous-phase>/README.md`
   - `.planning/phases/<previous-phase>/goal-kickstart-prompt.md`
   - `.planning/phases/<previous-phase>/sequential-work-packets.md`
4. Any phase-specific architecture or contract docs named by the user.
5. `plans/implementation-plan.md` only as background after the phase sequence and current architecture inputs are clear.

If a target phase directory already exists, read its three phase files before rewriting them.

Use the nearest existing phase docs as the base template for file shape and packet style. Use Phase 2 as the current model for optional scope sections, richer non-goals, and larger reader or fixture matrices without changing the overall format.

## Outputs

Create or update exactly:

- `.planning/phases/<phase-slug>/README.md`
- `.planning/phases/<phase-slug>/goal-kickstart-prompt.md`
- `.planning/phases/<phase-slug>/sequential-work-packets.md`

Use lowercase hyphenated phase slugs that match repo convention, such as `phase-two`, `phase-three`, and so on.

Keep all files ASCII, concise, and operational.

## Planning rules

- Mirror the prior phase structure before inventing a new format.
- Existing phase docs define the default structure; later phases may add optional middle sections but should keep the same three-file contract.
- Keep the phase independently buildable, testable, and reviewable.
- Treat `implementation-sequence.md` as the boundary contract; use phase-specific architecture docs to sharpen the slice, not to widen it.
- Pull in the smallest additive contract delta needed for the phase.
- Use OpenAI or Google goal-boundary ideas only as heuristics for tight goals, explicit non-goals, stop conditions, and observable evidence. Do not cite them as authority or let them change the repo format.
- If an input implies later-phase work, split it out or mark it deferred.

## Derive the phase boundary

Write the phase so a future implementation agent can answer these questions without guessing:

- What exact capability is proved in this phase?
- What must be built now?
- What is explicitly out of scope?
- What conditions force a stop instead of scope expansion?
- What evidence proves the phase is complete?

Prefer narrow boundaries around one stable proof loop or one additive capability family. Avoid mixed phases that combine core semantic changes with new access surfaces or broad polish.

## README format

Mirror the established repo shape:

- `# Phase N`
- `## Goal`
- `## Packet sequence`
- optional middle sections only when the phase needs them, such as `## Coverage target` or `## Recommended single reader`
- `## Build`
- `## Do not build`
- `## Pass criteria`

README guidance:

- `Goal`: one short paragraph with the exact phase outcome.
- `Packet sequence`: point to `sequential-work-packets.md` and restate same-worktree sequencing.
- `Build`: only the deliverables this phase is allowed to land.
- `Do not build`: excluded surfaces, semantic changes, runtime/storage additions, and speculative abstractions.
- `Pass criteria`: observable acceptance evidence, not implementation chores.

## Goal prompt format

`goal-kickstart-prompt.md` must be copy-pasteable and start with `/goal`.

Use this shape:

- opening paragraph that names the contract docs and the narrow phase objective
- one line on packet order and same-worktree sequencing
- `Objective:`
- `Non-goals:`
- `Allowed scope:`
- `Disallowed actions:`
- `Escalation / stop conditions:`
- `Observable evidence to report:`

Write in imperative language. Make stop conditions concrete enough that the implementing agent pauses instead of widening scope.

## Packet plan format

`sequential-work-packets.md` defines the PR-sized execution order.

Rules:

- State that packets are sequential by default.
- Allow parallelism only for read-only planning/review or isolated git worktrees.
- Keep packet count low; split only when a packet would otherwise mix contracts, capability families, or final acceptance closure.
- Each packet must unlock the next with minimal backtracking.
- Every packet maps to one small PR.

For each packet, use this exact sub-structure:

- `**What the PR should do:**`
- `**What it should build:**`
- `**Skills/guidance to load:**`
- `**Implementation boundary:**`
- `**Pass criteria / validation:**`
- `**Commit/PR boundary:**`

Always include `engineering-workflow`. Add narrower skills only when the packet needs them. Mention `conventional-commit-guard` before any commit message or PR title change.

## Consistency checks

Before finishing:

1. Check that README, goal prompt, and packet plan describe the same phase boundary.
2. Check that every `Build` item appears in at least one packet.
3. Check that every packet traces to a pass criterion or explicit evidence line.
4. Check that non-goals, disallowed actions, and stop conditions all block the same kinds of scope creep.
5. Check that implementation-sequence ordering still makes sense after the new phase is added or revised.

## Review

Run a doc-only review before handing off:

- self-review for structure, scope, and contradictions at minimum
- independent review if the environment supports it

Call out any unresolved ambiguity instead of smoothing it over.
