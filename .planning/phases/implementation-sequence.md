# Implementation sequence

Keep the build/test loop tight: land the smallest runnable slice first, prove it in isolation, then expand.

## Track 1 — Core loop: kernel + source readers + provenance proof

Goal: a minimal buildable/testable slice that proves the kernel/tool/provenance loop end to end.

Start with:
- the kernel boundary and in-memory harness
- one or two structured source-reader tools first (`extract.json`, `extract.csv_rows` or `extract.jsonl`)
- captured reads + audit-chain emission
- proof/audit verification over bundle bytes

Done-when for this track:
- a source-reader call runs end to end
- a token is minted from a declared source path
- provenance is captured and replayable
- proof/audit verification passes on the emitted artifact
- a targeted mutation fails closed

Keep this slice testable before the full ecosystem exists:
- use source bytes/fixtures directly
- verify captured reads and audit bundles in-process
- avoid runtime adapters, persistent storage, and export polish until the core loop is green

Ship this as small commits/PRs, each with one verifiable behavior.

## Track 2 — Expand surfaces: runtime, export, hardening

Goal: once the core loop is stable, widen the system without changing the proven kernel/provenance contract.

Add in this order:
- remaining source-reader coverage and selector/window edge cases
- runtime/access surfaces (CLI, HTTP, MCP) as thin shells over the proven core
- export artifacts and convenience views
- hardening: broader fixtures, negative cases, perf checks, and recovery tests

Done-when for this track:
- the same verified core behavior is reachable through each access surface
- export remains derived from the captured provenance, not a second source of truth
- failure cases stay fail-closed under replay, export, and runtime paths

## Working rule

Do not wait for the whole ecosystem to build before testing. If a slice can prove one invariant, land it, validate it, and move on.