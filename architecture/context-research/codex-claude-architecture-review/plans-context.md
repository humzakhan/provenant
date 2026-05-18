# Plans context for thesis alignment

## Planned
- **Phase 0–1** (`plans/implementation-plan.md` §§ Phase 0–1, L7–L68): carve out a pure-Go kernel boundary; pin `header_v1.go`/`EMPTY_CJSON_SHA256`; ship `kernel/public/memio/`; build in-memory audit chain + bundle verify.
- **Phase 2** (`implementation-plan.md` § Phase 2, L70–L104): tool registry, pre/post guards, `SubmitAnswer`, pure-Go tools, `match`/`dedupe`, built-in normalizers, credential-id reconciliation.
- **Phase 3** (`implementation-plan.md` § Phase 3, L106–L129): Mode A CLI harness with Claude, external tool backends, replay, audit pack/verify, overhead baseline.
- **Phase 4** (`implementation-plan.md` § Phase 4, L131–L163): runtime layer (SQLite/Postgres/S3), MCP + HTTP parity, HITL routing, PDF rendering, persistent-storage tests.
- **Phase 5–6** (`implementation-plan.md` §§ Phase 5–6, L165–L214): choose FFI vs sidecar for non-Go binding; then eval harness + case bank + release-rate alarm.

## Deferred / out of scope
- **v0.5+ adapter/workflow growth** (`implementation-plan.md` § Deferred, L216–L232): LangChain, OpenAI Agents, BoN-MAV, web HITL UI, hosted skill registry, additional skill packs/backends, second non-Go binding, extra model classes.
- **Known limitations** (`docs/KNOWN_TRADE_OFFS.md`, L8–L82): no external notary; no streaming/split-turn parallel tool_use; live progress is trace-based; host conformance attestation is self-declared; locale-specific numerics are only partially normalized; cross-deployment external-source drift is not validated; human corrections are not token-provenance checked; no custom validators/sandboxed loading; library mode does not persist cross-process HITL state.

## Assumptions / sequencing constraints
- The thesis’s “dual-mode pluggability” is still an open architectural question in `architecture/OVERVIEW.md` §2.1.1–2.1.2 (L68–L90): Go is assumed for the kernel, but non-Go binding shape is unresolved until Phase 5.
- Phase 5 depends on Phase 4 runtime measurements and either a viable sidecar latency budget or an explicit C-ABI projection strategy (`implementation-plan.md` L171–L188; `architecture/OVERVIEW.md` §3.4, L134–L141).
- `docs/SESSION.md` says the critique loop reached **STABLE** at cycle 10 (L236–L240): all VALID issues were resolved or accepted as trade-offs; remaining notes are ergonomic / Phase-0+ only.

## Thesis / architecture alignment notes
- Thesis core claim (`docs/thesis.md` § The Solution, L19–L31) and architecture overview (`architecture/OVERVIEW.md` §§1, 7, L9–L45; L196–L228) are aligned on: library-first substrate, optional MCP/CLI/HTTP shells, same kernel, replay-deterministic deterministic portions.
- The main caveat is that byte-identical chains are **conditioned on deployment-time facts** being identical too: `docs/KNOWN_TRADE_OFFS.md` L32–L38 (self-declared host conformance attestation), L48–L53 (external-source drift), and `architecture/OVERVIEW.md` L192–L193 (skill-pack digest pinning). The thesis states the stronger end-state, but the docs make clear that “same input + same skill pack” is not sufficient if those deployment facts differ.
- No hard contradiction found between the thesis and the architecture docs; the remaining differences are either explicit trade-offs or sequencing assumptions rather than spec conflicts.
