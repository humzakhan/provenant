## Scope
Files inspected:
1. `/Users/humza/.pi/agent/AGENTS.md` (lines 1-4) - top-level Codex routing references.
2. `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/architecture-review-prep/AGENTS.md` (lines 1-37) - repo-specific rules, personas, skills, and Codex model routing.
3. `/Users/humza/.codex/RTK.md` (lines 1-32) - shell-command prefix rule.
4. `/Users/humza/.config/agent-routing/AGENTS.routing.md` (lines 1-84) - global routing/model policy.
5. `/Users/humza/.agent-skills/oracle-plan-loop/BODY.md` (lines 1-130) - bounded Claude+Codex plan-review loop.
6. `/Users/humza/.agent-skills/engineering-workflow/BODY.md` (lines 1-130) - workflow, commit, validation, and review rules.
7. `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/architecture-review-prep/.codex/skills/orchestrator-mode/SKILL.md` (lines 1-34) - orchestrator-only constraints and boot-agent shape.
8. `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/architecture-review-prep/.codex/skills/conventional-commit-guard/SKILL.md` (lines 1-26) - commit/PR title format rule.
9. `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/architecture-review-prep/.agents/engineer.md` (lines 1-283) - implementation persona.
10. `/Users/humza/.superset/worktrees/80fca87c-0634-4a2d-a395-e48a6db8de28/architecture-review-prep/.agents/reviewer.md` (lines 1-253) - review persona.

## Operating Rules
- Use the smallest model/reasoning that preserves correctness.
- Search before reading broadly; avoid “just in case” files.
- `rtk` must prefix shell commands.
- Current session is orchestrator-only; do not directly read/write/edit/search/inspect/test/validate repo files when acting as orchestrator.
- Delegate repo reading, implementation, validation, and review to sub-agents.
- Boot/context sub-agent should read AGENTS plus only the referenced Codex-relevant files.
- Boot/context output must cover branch, commit, PR, validation, persona, skill-routing, and review-loop rules.
- Use repo-local skills/personas when they exist; keep skill usage additive and narrow.
- For implementation/review work, preserve worktree hygiene, keep changes small, and run an engineer-reviewer loop.
- Validation should be the narrowest useful check during iteration; broader validation only before declaring done.
- Commit messages and PR titles must use Conventional Commit form when created/updated.
- If this becomes a bounded plan review, use the oracle-plan-loop: Claude review, Codex oracle pass (`gpt-5.5` high by default), then reconcile.
- If an open PR exists for the current worktree/branch, cycle comments are mandatory during review loops.

## Relevant Codex Skills Or Personas
- `orchestrator-mode`: primary for this request because the user explicitly asked for orchestrator-mode boot/context handling.
- `engineering-workflow`: needed for branch/commit/validation/review-loop discipline.
- `conventional-commit-guard`: only when writing commit messages or PR titles.
- `oracle-plan-loop`: only if the task turns into a bounded plan-review loop.
- `gpt-5.4-mini` medium: best fit for boot/context reading and broad docs synthesis.
- `gpt-5.5` high: required for architecture or cross-surface decisions, or oracle-level review.
- Repo personas: `.agents/engineer.md` and `.agents/reviewer.md` are the local implementation/review personas if the task expands beyond context gathering.

## Recommended Agent Plan
Smallest useful plan:
1. Boot/context scout — `gpt-5.4-mini`, medium reasoning. Done here; captures only AGENTS + referenced guidance.
2. If a specific architecture question remains, add one narrow read-only reader on that exact subsystem — `gpt-5.4-mini`, medium reasoning.
3. If the output will influence a plan or change set, add one independent reviewer/oracle pass — `gpt-5.5`, high reasoning.

## Risks
- No explicit branch-naming or checkout rule was present in the loaded guidance.
- Open PR status was not checked; if one exists, review-cycle comments become mandatory.
- The repo personas are highly kernel-specific; they matter if the task expands into implementation/review, but they are overkill for pure context gathering.
- `orchestrator-mode` forbids direct repo inspection by the orchestrator; future work should stay delegated.
- Current guidance does not define any repo-specific validation command for this boot pass.
