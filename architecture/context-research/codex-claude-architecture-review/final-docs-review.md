## Review

- Correct: Local tracked content is documentation-only and concise. Against `origin/main...HEAD`, the diff is 14 Markdown files with 92 insertions / 70 deletions, and `git diff --check origin/main...HEAD` passed.
- Correct: `Codex_report.md` is removed locally. `find . -name Codex_report.md` returned no matches, and the local commit range includes `7026ead docs: remove Codex report artifact`.
- Correct: Most oracle issues 1-13 are patched in substance:
  - MCP is now an equal adapter with memio/SQLite defaults and Postgres/S3 optional runtime bindings (`architecture/adapters/mcp.md:16`, `architecture/adapters/mcp.md:163-165`).
  - Normalizer determinism and the five built-ins are aligned (`architecture/subsystems/skills.md:196-209`, `plans/implementation-plan.md:86-99`).
  - Kernel storage language now uses injected Store/AuditSink/BlobStore/TransactionalScope and points persistent projections to storage docs (`architecture/subsystems/kernel.md:8`, `architecture/subsystems/kernel.md:20`, `architecture/subsystems/kernel.md:132`).
  - Orphan recovery is scoped by mode (`architecture/subsystems/harness.md:222-224`) and cross-process `provenant-server` recovery is Phase 4 (`plans/implementation-plan.md:127`, `plans/implementation-plan.md:153`).
  - pgvector match backend is Phase 4 optional runtime wiring (`plans/implementation-plan.md:84`, `plans/implementation-plan.md:140`, `architecture/tools/match/backends/pgvector.md:1-10`).
  - CLI is split into Phase 3 embedded memio/bundle mode and Phase 4 runtime/server-attached persistence (`plans/implementation-plan.md:117`, `architecture/adapters/cli.md:203-210`).
  - Go remains the v0 kernel language; Phase 5 is only FFI-vs-sidecar binding shape (`plans/implementation-plan.md:175-177`).
  - Live-progress observability and Phase 6a/6b split are now in the plan (`plans/implementation-plan.md:161`, `plans/implementation-plan.md:202-212`, `plans/implementation-plan.md:247`).
  - Byte-identity/tamper-evidence claims are qualified in top-level docs (reviewed via changed README/overview/thesis content; no obvious contradiction found against the library-first/storage-as-deployment/equal-adapter contracts).
- Correct: PR comment lineage is plausible in wording: `gh pr view 6` shows separate comments for issues 1-14 plus `Codex_report.md` removal, each referencing the expected local commits.

- Blocker: PR #6 on GitHub does not appear to contain the local docs patch commits. `gh pr view 6 --json commits` returned only `686f66f docs: add architecture review report`, while local `origin/main..HEAD` contains `6615e6c`, `09cf467`, `fd9128e`, and `7026ead` in addition to `686f66f`. This makes the posted PR comments claim commits that are not currently visible in the remote PR, and likely means the remote PR still contains the old report-only state until the branch is pushed.
- Blocker: Oracle issue 14 is only partially patched. `run_orphan_recovered.recovery_reason` is pinned inline (`architecture/subsystems/audit.md:159`) and a negative test is listed (`plans/implementation-plan.md:34`), but it is not included in enumgen/drift gates as requested. The generator source list still only includes `error_code`, `EntryKind`, `failure_subcode`, and normalizers (`architecture/subsystems/audit.md:249-256`; `plans/implementation-plan.md:36-41`), and `plans/implementation-plan.md:35` says validators come from `errorcode`, `EntryKind`, and `FailureSubcode` slices only.

- Note: Validation is sufficient for a docs-only patch once the above blockers are resolved: tracked tree is clean, only `orchestrator/` is untracked by design, and `git diff --check origin/main...HEAD` passed.
- Note: Ready/not-ready: **not ready for human review as PR #6** until the docs patch commits are pushed/visible on the PR and issue 14's recovery-reason enum is added to the documented enumgen/drift-gate contract.