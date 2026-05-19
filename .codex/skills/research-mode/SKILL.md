---
name: research-mode
description: 'Use when the user says "Enter Research Mode", "Use our Research Mode", or otherwise asks Codex to coordinate a research workflow. The current Codex session coordinates only: it enables plan mode, delegates read-only research scouting to sub-agents with customized prompts, then synthesizes cited findings from their reports.'
---

# Research Mode

Use this skill when the user explicitly enters Research Mode.

## Core Rule

The current Codex session is the research orchestrator only.

Enable plan mode for the harness the same way Orchestrator Mode does before coordinating the research workflow.

Do not directly write, edit, mutate state, run implementation, or perform execution work. Delegate read-only research operations to sub-agents and make decisions from their reports.

If the user asks for implementation work, hand off to the appropriate implementation workflow or skill. Research Mode may produce findings, options, citations, risks, and recommendations, but it does not implement.

## Required Delegation Shape

Minimum shape:

- One boot or context agent.
- One or more research scout agents for the actual research questions.
- One synthesis or review agent before completion if the recommendation materially affects architecture, implementation, security, policy, or external commitments.

## Codex Boot

Immediately spawn a fresh read-only boot sub-agent.

The boot agent must:

- Read `AGENTS.md` and only the Codex-relevant files it references.
- Read `engineering-workflow` guidance only when the research may affect workflow, commits, PRs, or validation.
- Report branch, commit, PR, validation, persona, skill-routing, and research boundary rules.
- Recommend the smallest useful scout plan.

## Agent Context Rules

- Use isolated context by default.
- Give each agent a narrow research question and disjoint evidence scope.
- Do not duplicate a sub-agent's work in the orchestrator session.
- Use sub-agents only to gather evidence, summarize, cite sources, compare options, identify risks, or independently review research conclusions.
- Research sub-agents are scouts: they gather, summarize, cite, and report back. They do not modify code, files, tickets, branches, PRs, external services, or any other state.

## Scout Prompt Rules

Generate a customized prompt on the fly for each spawned research scout.

Each scout prompt must be tailored to the specific research question, evidence scope, source constraints, expected citations, and output needed for that scout. Do not use one fixed static scout template for all sub-agents.

Each scout prompt must explicitly say:

- The scout is read-only.
- The scout must cite exact files, lines, URLs, docs, commands, or artifacts used as evidence.
- The scout must separate facts from inferences.
- The scout must report uncertainty and gaps instead of filling them in.
- The scout must not implement, edit, commit, push, open PRs, or mutate external state.

## Model Selection

- Boot/context reading: `gpt-5.4-mini`, medium reasoning.
- Narrow research scouting: `gpt-5.4-mini`, medium reasoning.
- Broad research synthesis or high-stakes review: `gpt-5.5`, high reasoning.

## Output Format

Require every sub-agent to return:

```markdown
## Scope
## Findings
## Evidence
## Inferences
## Gaps
## Risks
```
