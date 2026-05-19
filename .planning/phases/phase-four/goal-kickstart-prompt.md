/goal
Kickstart Phase 4 only. Treat `.planning/implementation-plan.md` and `.planning/phases/phase-four/README.md` as the Phase 4 contract. Harden the shipped Phase 3 behavior for release handoff through negative and recovery coverage, lightweight performance smoke, and final test-matrix and documentation alignment only.

Follow `.planning/phases/phase-four/sequential-work-packets.md` in order. Execute in the same worktree, one packet at a time; parallelism is only for read-only planning/review or isolated worktrees.

Objective:
- add fail-closed negative regression coverage for shipped paths
- add recovery behavior coverage for already-supported flows
- add lightweight performance smoke for shipped paths
- finish with final test-matrix, docs, and handoff alignment

Non-goals:
- no new features or new user-facing surfaces
- no expansion of Phase 3 wrappers or derived export behavior
- no new storage, runtime, model, or adapter work
- no future Phase 5+ roadmap work folded into hardening

Allowed scope:
- focused negative-path coverage on already-shipped behavior
- focused recovery-path coverage on already-supported behavior
- lightweight performance smoke and baseline evidence
- docs and handoff updates that reflect implemented behavior and deferred work

Disallowed actions:
- widening hardening into feature work, surface work, or contract expansion
- adding new storage, runtime, model, or adapter paths to satisfy a hardening gap
- changing proof, audit, minting, replay, or reader semantics
- pulling future-phase cleanup or replanning into the Phase 4 delivery slice

Escalation / stop conditions:
- stop if a hardening gap requires a new feature, new surface, or Phase 3 scope expansion
- stop if a recovery scenario needs new storage/runtime/model/adapter behavior to be testable
- stop if fail-closed coverage exposes proof, audit, minting, replay, or reader semantic work
- stop if final handoff alignment depends on future-phase roadmap decisions rather than current shipped behavior

Observable evidence to report:
- exact packet completed and hardening lane touched
- files changed
- tests run
- negative fail-closed result per shipped path covered
- recovery result per already-supported flow covered
- lightweight performance smoke result and any handoff caveat
- final doc and test-matrix alignment result
