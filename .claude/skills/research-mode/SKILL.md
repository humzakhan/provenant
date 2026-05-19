---
name: research-mode
description: Use when the user says "Enter Research Mode", "Use our Research Mode", or otherwise asks Claude to coordinate a research workflow. The current Claude session coordinates only: it enables plan mode, delegates read-only research scouting to subagents with customized prompts, then synthesizes cited findings from their reports.
---

# Research Mode

Use this skill when the user explicitly enters Research Mode.

## Core Rule

The current Claude session is the research orchestrator only.

Enable plan mode for the harness the same way Orchestrator Mode does before coordinating the research workflow.

Do not directly write, edit, mutate state, run implementation, or perform execution work. Delegate read-only research operations to subagents and make decisions from their reports.

If the user asks for implementation work, hand off to the appropriate implementation workflow or skill. Research Mode may produce findings, options, citations, risks, and recommendations, but it does not implement.

## Required Delegation Shape

Minimum shape:

- One boot or context subagent.
- One or more research scout subagents for the actual research questions.
- One synthesis or review subagent before completion if the recommendation materially affects architecture, implementation, security, policy, or external commitments.

## Claude Boot

Immediately spawn a fresh read-only boot subagent.

The boot subagent must:

- Read `CLAUDE.md` and only the Claude-relevant files it references.
- Read `engineering-workflow` guidance only when the research may affect workflow, commits, PRs, or validation.
- Report branch, commit, PR, validation, persona, skill-routing, and research boundary rules.
- Recommend the smallest useful scout plan.

## Subagent Context Rules

- Use isolated context by default.
- Give each subagent a narrow research question and disjoint evidence scope.
- Do not duplicate a subagent's work in the orchestrator session.
- Use subagents only to gather evidence, summarize, cite sources, compare options, identify risks, or independently review research conclusions.
- Research subagents are scouts: they gather, summarize, cite, and report back. They do not modify code, files, tickets, branches, PRs, external services, or any other state.

## Deterministic Lookup

Use `claude-haiku-4-5` for bounded read-only lookup or state checks when the
query, files, commands, and expected output are already narrow.

Haiku scouts may gather exact facts, citations, command output, or status
checks. Do not use Haiku scouts for synthesis, recommendations, risk
assessment, architecture judgment, or deciding what evidence is sufficient.

## Scout Prompt Rules

Generate a customized prompt on the fly for each spawned research scout.

Each scout prompt must be tailored to the specific research question, evidence scope, source constraints, expected citations, and output needed for that scout. Do not use one fixed static scout template for all subagents.

Each scout prompt must explicitly say:

- The scout is read-only.
- The scout must cite exact files, lines, URLs, docs, commands, or artifacts used as evidence.
- The scout must separate facts from inferences.
- The scout must report uncertainty and gaps instead of filling them in.
- The scout must not implement, edit, commit, push, open PRs, or mutate external state.

## Model Selection

- Deterministic lookup/state check: `claude-haiku-4-5`, thinking off.
- Boot/context reading: `claude-haiku-4-5`, thinking off.
- Narrow research scouting: `claude-haiku-4-5`, thinking off.
- Broad research synthesis or high-stakes review: `claude-opus-4-7`, thinking on.

## Output Format

Require every subagent to return:

```markdown
## Scope
## Findings
## Evidence
## Inferences
## Gaps
## Risks
```
