# Goal: Answer-Number Grounding Enforcement

> Pass this file to a harness via `/goal`. It defines the objective, the constraints, and the definition of done. The detailed PR-by-PR breakdown lives in `.planning/plan.md` — follow it.

## Objective

Make it **impossible for an agent to release an answer containing an ungrounded number** through Provenant. Adopt the two-tier enforcement pattern proven by `codegraph` (https://github.com/colbymchenry/codegraph):

- **Soft tier** — MCP `serverInstructions` injected at `initialize` that tell the model to source every number from a Provenant tool and never compute numbers in prose.
- **Hard tier** — a harness hook that fires at answer-release time, extracts every numeric claim from the answer, checks each against the run's minted Provenant tokens, and **blocks release** if any number is ungrounded.

This is not new architecture for Provenant — it is the delivery mechanism for the `submit_answer` numeric-grounding guard already specified in `architecture/subsystems/guards.md` and ADR-0002. The enforcement logic must live in the **pure kernel** so it is framework-agnostic; the Claude Code hook is a thin adapter over it.

## Key structural constraint (read this first)

codegraph's hard hook intercepts an **input** action (`Read` is a tool call). Computing a number is **not** a tool call — it is token generation and cannot be intercepted at the moment of computation. Therefore enforcement happens at the **moment of release** (the harness `Stop` event / answer submission), not at the moment of computation. The rule is "block release of an answer with an ungrounded number," not "block computation."

## Constraints (non-negotiable)

- **Kernel purity**: enforcement logic goes in `kernel/...` as pure functions — no `net/*`, `database/sql`, `pgx`, MCP SDK, or `os/exec` imports. The forbidden-import CI check must stay green.
- **Decimals, never floats**: use `shopspring/decimal`, canonical 9-decimal form, consistent with existing `Token.ValueCanonical`.
- **Real-kernel tests**: every unit and e2e test must exercise the real kernel/driver — no mocks of the enforcement core. Extract shared test helpers to one common location; never copy/paste harness code between test files.
- **Small, independently reviewable PRs**: each PR builds and tests green (`go build ./... && go test ./...`) on its own.
- **Reviews default to sonnet-tier** reviewers unless a change is genuinely high-stakes.

## Deliverables

Implement the PRs defined in `.planning/plan.md` in order:
1. Numeric-claim extraction (pure kernel).
2. `CheckAnswer` grounding guard (pure kernel) + `provenant check-answer` CLI verb.
3. MCP `serverInstructions` soft layer.
4. Claude Code `Stop`-hook adapter + `provenant install` command.
5. **End-to-end test run as its own PR**, with results recorded.

## Definition of done

The task concludes with the **PR 5 end-to-end run**, performed against the real kernel through the installed hook, demonstrating both:
- (a) an answer containing a number the model wrote without grounding is **blocked** at release, with a structured reason naming the ungrounded value; and
- (b) an answer whose numbers all trace to minted tokens is **released**.

Record the full run — exact commands, transcript excerpts, the block reason output, the final grounded answer, environment, and pass/fail — in `.planning/e2e-results.md`. Then report a short summary: what was built, what the e2e run proved, and any open design questions that surfaced.
