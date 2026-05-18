/goal
Kickstart Phase 1 only. Treat `.planning/phases/implementation-sequence.md` as the Phase 1 contract and keep the scope to the in-process proof loop: fixture/source bytes -> one structured reader -> captured read -> kernel-minted token from a declared path -> minimal bundle -> verify/replay in memory -> one deterministic mutation that fails closed.

Use the smallest JSON fixture reader, `extract.json`, unless the codebase already names the same narrow backend `extract.json_value`; do not widen the reader surface.

Hard boundaries:
- No CLI, HTTP, MCP, daemon, or model integration.
- No Postgres, SQLite, S3, network, or filesystem-backed persistence.
- No PDF/XLSX/export polish.
- No broad reader coverage.
- No speculative public API expansion.

Implementation discipline:
- Keep code small and boring; less code is more.
- Reader/tool code must not mint tokens directly.
- Capture enough read metadata for replay: source bytes hash, selector, reader version/options, and locator/origin.
- Verification and replay must operate on bundle bytes/in-memory state only.
- If a wider scope is needed, stop and ask before expanding Phase 1.

Validation evidence to report:
- the exact reader chosen,
- files changed,
- tests run,
- bundle verification result,
- replay result,
- mutation-failure result,
- and any deferred scope called out explicitly as Phase 2+.
