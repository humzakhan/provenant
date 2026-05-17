# ADR-0006: Skill pack as the unit of workflow

Status: **Accepted**
Date: 2026-05-17

## Context

A finance workflow ("recon," "AP coding," "expense review") is a bundle of: prompts, required tools, guards, validators, evaluation cases, policy retrievals, and per-workflow thresholds. We need a portable, versioned unit that encapsulates these so that:

- A customer can adopt "recon" by installing one thing.
- A workflow's behavior is reproducible across deployments.
- New workflows can be authored without changing the kernel.
- Workflows can be distributed (open-source or commercial) independently.

## Decision

The unit is a **skill pack** — a directory tree with a manifest, declared tool dependencies, guard configuration, prompts, evaluation cases, and policy snippets. Skill packs are versioned (semver). The kernel loads packs from local filesystem (v0) and eventually a registry (v0.5+). Pack hashes (sha256 over canonicalized content) are part of every run's audit trail.

See [`subsystems/skills.md`](../subsystems/skills.md) for the format details.

## Rationale

**[Moody's modular AI](../references.md#moodys-modular) is the validating reference.** Moody's decomposes credit memo generation into specialized agents and ships them as composable units. The pattern works because workflows decompose cleanly into "specialist agents with clear contracts." Skill packs operationalize this open-source.

**[Multi-agent finance benchmark](../references.md#multi-agent-finance) findings inform the per-pack architecture choice.** The benchmark shows that hierarchical-with-reflexive orchestration achieves highest F1 on financial document tasks but at higher cost. Skill packs let workflow authors choose the orchestration shape and pay the cost only where warranted — recon may need only sequential; M&A diligence may need hierarchical with reflexive verifiers.

**Versioning and audit hashing are non-negotiable for finance.** Auditors ask "what version of the recon workflow ran this case?" The pack hash answers that with cryptographic certainty.

**Pack format is a public contract.** Customers and the community will author packs. The format must be specified, versioned, and stable.

## Consequences

**Positive:**
- Workflows can be authored, distributed, and adopted without kernel changes.
- Audit story includes the exact pack hash for every run.
- The community can contribute packs (recon for QuickBooks, AP coding for NetSuite) without modifying core code.
- Commercial differentiation can layer over the OSS kernel: customers can publish private packs in their own registry.

**Negative:**
- Pack authoring has a learning curve. We mitigate with templates, the `provenant skill validate` command, and reference packs.
- Pack format changes are breaking for the whole ecosystem; we manage with careful versioning and migration tooling.
- The kernel must validate packs strictly on load; permissive loading would allow non-deterministic packs to ship.

**Out of scope:**
- We do not yet ship a hosted skill registry. Packs install from local filesystem in v0. The registry is the v0.5+ scope and the natural starting point for a commercial offering.

## Alternatives considered

**Hard-coded workflows in the kernel.** Each workflow ("recon," "AP coding") implemented as a Go package. Rejected because it makes contribution and customer customization heavy.

**Workflow-as-code in Python (à la Airflow).** Considered. Rejected because we want declarative workflows (manifest + assets), not imperative DAGs. Imperative workflows create more surfaces for non-determinism.

**LangChain-style chains.** Considered. Rejected because they couple workflow to framework — the same workflow can run on different frameworks (CLI, MCP, etc.) only if the workflow is declarative.

## Amendment — cycle 6: normalizer registration mechanism

Cycle 5's `normalizers/` slot left the kernel-registered built-in set unspecified as to registration mechanism (compile-time table, runtime registry, boot-time JSON?). Cycle 6 pins it: registration is **compile-time** in `kernel/internal/normalizers/registry.go` with per-`builtin_id` `init()` registration. Each `builtin_id` is part of the kernel's public API contract; adding one is a kernel-version bump. The v0 registry is empty; v0 chains carry `normalizer_applied = null` on every token. `null` is a canonical value in the deterministic payload, so a v0 chain replays byte-identically on a v0.5 kernel that has registered new IDs. Full mechanism details and Phase-2 plumbing in [`subsystems/skills.md`](../subsystems/skills.md#pack-level-normalizers-normalizers). This amendment does not change the cycle-5 pack-format contract; it pins the registration shape so two implementations cannot disagree on it.

## Amendment — cycle 5: `normalizers/` slot

The original pack format enumerated `manifest.yaml`, `tools.yaml`, `guards.yaml`, `prompts/`, `eval/`, `policy/`, `ui/`, and `README.md`. Cycle 5 added an optional `normalizers/` slot so per-backend pre-canonicalization needs (locale-specific numeric forms, currency-symbol stripping) have a declared, hashed, audit-visible location in the pack rather than living implicitly in `tools/.../<backend>.go`. v0 restricts `normalizers/manifest.yaml` to `kind: builtin` entries that reference kernel-registered built-in normalizer IDs; arbitrary code loading is explicitly out of scope for v0 (the open question of pack-shipped custom validators / WASM-sandboxed extensions is tracked in [`subsystems/skills.md`](../subsystems/skills.md) Open questions and in KNOWN_TRADE_OFFS.md cycle 5). Full spec in [`subsystems/skills.md`](../subsystems/skills.md#pack-level-normalizers-normalizers). This amendment does not break existing packs (the slot is optional).

## References

- [Moody's modular AI](../references.md#moodys-modular) — agent-as-microservice composability
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — orchestration patterns to support per-pack
- See [`subsystems/skills.md`](../subsystems/skills.md) for the pack format spec
