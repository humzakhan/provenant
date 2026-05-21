# Goal Prompt

Use the repo's `engineering-workflow` skill to implement the simplification audit plan in `.planning/simplification-audit-2026-05-21/implementation-plan.md`.

Stay aligned with the PR sequence in that plan. Prefer the smallest PRs and complete one coherent PR-sized slice at a time:

1. Lowest-risk docs terminology cleanup.
2. Documentation drift around no-JavaScript/no-external-assets and broken-demo invariant wording.
3. Architecture/current-code consistency around audit event names and bundle layout.
4. User-facing report/presentation wording only where it can avoid golden-output changes; otherwise create or update a design/spec note before implementation.
5. Output-contract or golden changes only as an explicitly reviewed migration/versioning effort.

Use narrow skills where relevant:

- Use `provenant-audit-determinism` before changing audit entries, deterministic payloads, canonical hash inputs, bundle verify/replay behavior, generated fixtures, or enum/source-table generation.
- Use `provenant-kernel-boundary` before changing kernel public APIs, `memio`, `TransactionalScope`, injected clock/rand, or anything that could widen the pure-Go kernel surface.
- Use `provenant-source-reader-provenance` before changing source readers, captured reads, mintable paths, or tool outputs that feed token minting.
- Use Go skills only for actual Go implementation or tests, and keep them narrow to the files being changed.

Do not break golden outputs unless the current PR explicitly targets a versioned/golden-reviewed change. Avoid changing generated report HTML, scaled demo golden reports, `golden.bundle.json`, `golden.trace.json`, reconciliation JSON/CSV artifacts, report ZIP entries, manifest schemas, bundle formats, serialized JSON field names, audit event kinds, public APIs, or CLI verbs as part of docs-only simplification.

Generated golden output changes require explicit migration/versioning and golden review. Treat any byte-compared HTML wording, deterministic bundle/trace output, reconciliation artifact, report pack manifest, ZIP layout, audit kind, or serialized field change as an output-contract change, not a casual simplification.

For each PR, run validation appropriate to that PR before marking it complete. At minimum, inspect diffs to confirm the PR did not touch unintended contracts. Run targeted tests when code, examples, report sources, or generated artifacts are touched. Keep verify/replay integrity distinct from reconciliation/business correctness in wording and implementation.
