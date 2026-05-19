---
name: pr-gate
description: Use only in single-feature mode when a push, PR, merge, milestone transition, or done boundary requires explicit human approval. Do not use when Orchestrator Mode is active.
---

# pr-gate

Use this gate only in **single-feature mode**: one human-directed feature, one
branch, one PR, and human review before merge.

Do **not** use `pr-gate` when Orchestrator Mode is active. Orchestrator Mode
uses automated sub-agent review as its approval gate. After the automated
reviewer returns APPROVE and merge criteria pass, the orchestrator merges the
PR, updates from `origin/main`, and starts the next branch.

## Hard Rule

Stop at every single-feature PR boundary and request explicit human approval
before proceeding.

A single-feature PR boundary is any of:

- Creating a PR (`gh pr create`)
- Pushing a branch that will become a PR
- Merging or squash-merging
- Moving from one milestone or planned slice to the next
- Declaring a feature "done"

## Required Behavior

1. Before any boundary action, summarize: branch, diff stat, what was done,
   what tests were run, and what is still open.
2. Ask plainly: **"Approve to proceed? (y/n)"** Wait for an affirmative human
   reply. Silence, ambiguity, or "looks fine" is not approval; re-ask.
3. Never chain planned slices. Finishing one slice does not authorize the next.
4. Never auto-merge in single-feature mode. Never push with `--force` without
   an explicit instruction in the same turn.
5. If the engineer-reviewer loop has not run, surface that and offer to run it
   before asking for approval.

## When In Doubt

If Orchestrator Mode is active, follow the orchestrator workflow. Otherwise,
stop and ask.
