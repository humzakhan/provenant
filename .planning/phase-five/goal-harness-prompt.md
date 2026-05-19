/goal Execute Phase 5 end-to-end from `.planning/extension-plan.md` and `.planning/phase-five/packets/`.

You are the autonomous orchestrator for Phase 5. The human is stepping away. Work through every plan and packet under `.planning/phase-five/packets/` in numbered order in one continuous run. Implement each packet end-to-end, run the necessary repo skills, perform implementation review, and open one PR per packet until all packets are complete or a stop condition is reached.

Primary objective:

- Build the deterministic, config-driven reconciliation harness so a caller can pass supported structured files, describe each file's business purpose and column roles, and receive an understandable audited reconciliation package.

Required operating model:

- Treat `.planning/extension-plan.md` as the phase contract.
- Treat `.planning/phase-five/packets/README.md` as the build order.
- Treat each packet directory as the PR scope for that packet.
- Start with `.planning/phase-five/packets/00-request-contract/`.
- Do not skip packets, merge scopes, or stop after the first PR.
- Continue from Packet 00 through Packet 06 in the same `/goal` run.
- Start a later packet only after the previous packet is implemented, reviewed, pushed, and ready for review or merged according to the active orchestration rules.
- For each packet, create a fresh feature branch from updated `origin/main`.
- For each packet, load the repo skills named in that packet README and any narrower Go/provenance skills that apply.
- For each packet, use sub-agents where useful for parallel code reading, fixture review, validation, or independent review.
- For each packet, run implementation validation and an independent review before opening or updating the PR.
- For each packet, use `conventional-commit-guard` before commits and PR title creation or update.

Per-packet completion loop:

1. Read the packet README and packet `implementation-plan.md`.
2. Confirm the packet's non-goals, stop conditions, and PR boundary.
3. Inspect the current code before designing changes.
4. Implement only the packet scope.
5. Add focused tests and fixtures called for by the packet.
6. Run packet-specific validation plus `go test ./...` unless the packet explicitly narrows validation.
7. Run an independent review pass against the changed files.
8. Fix review findings that are in scope.
9. Commit with a conventional commit message.
10. Push the branch and open a PR.
11. Confirm PR status and report the PR URL plus validation evidence.
12. Continue to the next packet without waiting for a new human instruction when the active orchestration rules allow it; otherwise stop only at the required PR approval or merge gate and report the exact next packet.

Expected packet PRs:

- Packet 00: reconciliation request contract.
- Packet 01: reader router.
- Packet 02: deterministic reconciliation engine.
- Packet 03: provenance and token minting.
- Packet 04: CLI command.
- Packet 05: audit report package.
- Packet 06: end-to-end fixtures and docs.

Required implementation boundaries:

- No LLM inference.
- No fuzzy matching.
- No automatic column guessing.
- No runtime database, daemon, object-store binding, or persistence service.
- No PDF/OCR input.
- No UI.
- No reconciliation business logic inside the kernel.
- No new audit semantics unless a packet reaches a stop condition and explicitly replans.
- No report value that cannot be traced to captured reads, tokens, bundle evidence, or manifest evidence.

Stop and report instead of widening scope if:

- a file cannot be routed to a supported reader deterministically
- a selector cannot identify the configured table, row window, or range without inference
- configured columns are missing, duplicated, ambiguous, or parse inconsistently
- a reconciliation mode needs probabilistic matching, fuzzy matching, or automatic business-meaning inference
- a report claim cannot be proven from bundle, token, captured-read, reconciliation, or manifest evidence
- implementation requires runtime persistence, a service boundary, or new kernel audit semantics

Final Phase 5 completion evidence:

- request-schema validation fixtures
- reader-routing fixtures for supported formats
- deterministic reconciliation fixtures for each supported mode
- token and captured-read evidence for report rows
- bundle verify and replay evidence
- report ZIP manifest verification
- end-to-end CLI fixture output
- PR links for every packet
