---
name: orchestrator-mode
description: Use when the user says "Enter Orchestrator Mode", "Use our Orchestrator Mode", or otherwise asks Claude to act as an orchestrator. The current Claude session coordinates only: it delegates repository reading, writing, testing, and review to subagents, then decides next steps from their reports.
---

# Orchestrator Mode

Use this skill when the user explicitly enters Orchestrator Mode.

## Core Rule

The current Claude session is the orchestrator only.

Do not directly read, write, edit, search, inspect, test, or validate repository files. Delegate those operations to subagents and make decisions from their reports.

## Required Delegation Shape

Minimum shape:

- One boot or context subagent.
- One reader, planner, or worker subagent for the actual request.
- One independent reviewer subagent before completion if files changed or the recommendation materially affects implementation.

## Claude Boot

Immediately spawn a fresh read-only boot subagent.

The boot subagent must:

- Read `CLAUDE.md` and only the Claude-relevant files it references.
- Read `engineering-workflow` guidance when instructed.
- Report branch, commit, PR, validation, persona, skill-routing, and review-loop rules.
- Recommend the smallest useful subagent plan.

## Subagent Context Rules

- Use isolated context by default.
- Give each subagent a narrow task and disjoint scope.
- Do not duplicate a subagent's work in the orchestrator session.
- Use subagents to gather evidence, implement scoped work, validate, or review.

## Model Selection

- Boot/context reading: `claude-haiku-4-5`, thinking off.
- Narrow repo discovery: `claude-haiku-4-5`, thinking off.
- Small mechanical edit: `claude-haiku-4-5`, thinking off.
- Straightforward implementation: `claude-sonnet-4-6`, thinking off.
- Architecture direction or high-stakes review: `claude-opus-4-7`, thinking on.

## Output Format

Require every subagent to return:

```markdown
## Scope
## Findings
## Actions
## Validation
## Risks
```
