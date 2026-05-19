/goal
Kickstart Phase 3 only. Treat `.planning/implementation-plan.md` and `.planning/phases/phase-three/README.md` as the Phase 3 contract. Expose the proven core through thin CLI, HTTP, and MCP wrappers, and add a derived export path that reads only from captured provenance.

Follow `.planning/phases/phase-three/sequential-work-packets.md` in order. Execute in the same worktree, one packet at a time; parallelism is only for read-only planning/review or isolated worktrees.

Objective:
- add thin CLI access to the verified core path
- add thin HTTP access to the verified core path
- add thin MCP access to the verified core path
- add a derived export path validated against canonical bundle bytes/proof
- finish with smoke coverage and doc alignment

Non-goals:
- no alternate provenance store or divergent read path
- no proof, audit, minting, replay, or reader semantic changes
- no broad runtime persistence, daemon lifecycle, model harness, or storage-backend work
- no export artifact that reads outside captured provenance

Allowed scope:
- thin wrappers around the existing core behavior
- minimal request/response shaping needed for smoke-tested access
- derived export generation from captured provenance only
- focused smoke tests and validation evidence for each surface
- documentation updates that describe shipped behavior and deferred work

Disallowed actions:
- bypassing the verified core path from any access surface
- introducing a second source of truth for proof, replay, or export data
- widening Phase 3 into new storage, runtime, model, or feature work
- changing Phase 1 or Phase 2 proof/audit/reader semantics

Escalation / stop conditions:
- stop if a surface requires a new provenance store or divergent read path
- stop if export requires data that is not present in captured provenance
- stop if wrapper behavior requires proof/audit semantic changes
- stop if a proposed smoke test cannot prove use of the same verified core path

Observable evidence to report:
- exact packet completed and surface or export path touched
- files changed
- tests run
- smoke result per access surface
- export verification result against canonical bundle bytes/proof
- anything deferred explicitly to Phase 4+
