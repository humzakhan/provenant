---
name: orchestrator-mode
description: Use when the user says "Enter Orchestrator Mode", "Use our Orchestrator Mode", or otherwise asks Codex to act as an orchestrator. The current Codex session coordinates only: it delegates repository reading, writing, testing, and review to sub-agents, then decides next steps from their reports.
---

# Orchestrator Mode

Use this skill when the user explicitly enters Orchestrator Mode.

## Core Rule

The current Codex session is the orchestrator only.

Do not directly read, write, edit, search, inspect, test, or validate repository files. Delegate those operations to sub-agents and make decisions from their reports.

`pr-gate` is not applicable while Orchestrator Mode is active. Use automated
sub-agent review as the approval gate; after a PR is approved and merge criteria
pass, merge it and branch the next slice from updated `origin/main`.

## Required Delegation Shape

Minimum shape:

- One boot or context agent.
- One reader, planner, or worker agent for the actual request.
- One independent reviewer agent before completion if files changed or the recommendation materially affects implementation.

## Codex Boot

Immediately spawn a fresh read-only boot sub-agent.

The boot agent must:

- Read `AGENTS.md` and only the Codex-relevant files it references.
- Read `engineering-workflow` guidance when instructed.
- Report branch, commit, PR, validation, persona, skill-routing, and review-loop rules.
- Recommend the smallest useful agent plan.

## Agent Context Rules

- Use isolated context by default.
- Give each agent a narrow task and disjoint scope.
- Do not duplicate a sub-agent's work in the orchestrator session.
- Use sub-agents to gather evidence, implement scoped work, validate, or review.

## Model Selection

- Boot/context reading: `gpt-5.4-mini`, medium reasoning.
- Narrow repo discovery: `gpt-5.4-mini`, medium reasoning.
- Small mechanical edit: small fast coding model.
- Straightforward implementation: `gpt-5.4`, medium reasoning.
- Default review: `gpt-5.4`, raising reasoning effort to high when needed; most code reviews can use `gpt-5.4` high.
- Unclear, critical, business-logic, or security-sensitive review: `gpt-5.5`, medium reasoning.
- Decision-making or genuinely high-stakes ambiguity: `gpt-5.5`, high reasoning.
- Architecture direction: `gpt-5.5`, high reasoning.

## Output Format

Require every sub-agent to return:

```markdown
## Scope
## Findings
## Actions
## Validation
## Risks
```
