# Thesis Context Brief

## Mission / thesis
Provenant is an **open-source enforcement substrate for regulated-finance LLM agents**: a Go library that makes agent output provenance-verified, replayable, and tamper-evident rather than merely logged. The thesis is that **provenance is an enforcement boundary, not a logging concern**; an agent should not be able to release a Provenant-signed answer without verifiable lineage to tool calls.

Key sources:
- `docs/thesis.md` lines **13-31**: core thesis, dual-mode model, `submit_answer` boundary, library vs runtime surfaces.
- `README.md` lines **1-5**: top-level product statement (“Go library you embed”).
- `architecture/OVERVIEW.md` lines **1-4**, **7-18**: synthesis view and “library is the product”.

## What the system is trying to be
- A **Go library/kernel** that enforces invariants at the architecture level.
- A **substate for finance agents**, not an agent, model, workflow engine, database, or hosted SaaS.
- A **dual-mode product**:
  - **Mode A / embedded library**: host owns the loop; kernel is called directly.
  - **Mode B / runtime access surfaces** (MCP/HTTP/CLI): thin shells over the same kernel.
- A substrate for **regulated finance work product** (reconciliations, AP coding, variance commentary, sanctions dispositions, SAR drafts).

Key sources:
- `docs/thesis.md` lines **21-31**, **35-45**, **64-72**.
- `architecture/OVERVIEW.md` lines **29-65**, **7-18**, **7.1-7.4**.
- `architecture/README.md` lines **155-166** (v0 scope summary).

## Non-negotiable invariants
Four kernel invariants are load-bearing and cannot be disabled by configuration:
1. **Non-bypassable** — the only exit is `submit_answer`.
2. **Fail-closed** — errors default to denial.
3. **Schema-first** — every tool/input/output is typed against a published schema.
4. **Replay-deterministic** — deterministic portions of a run are bit-identical on replay.

Also explicit in the thesis/overview:
- **Token-level grounding**: numeric claims must map to kernel-minted provenance tokens.
- **Hash-chained audit log**: every run emits a tamper-evident audit bundle.
- **No kernel storage requirement**: storage is a deployment concern, not a kernel concern.
- **No enforcement bypass in runtime adapters**: adapters translate, but do not decide policy.

Key sources:
- `architecture/README.md` lines **67-76**.
- `docs/thesis.md` lines **21-31**, **37-45**, **66-72**.
- `architecture/OVERVIEW.md` lines **65-76**, **87-90**, **154-166**.

## Explicit criteria for judging architecture alignment
An architecture is aligned if it preserves the following:

- **Enforcement boundary stays in architecture**: `submit_answer` remains the single release path; no alternate output path can bypass grounding/validation.
- **Library remains the product**: MCP/HTTP/CLI are sidecar shells, not primary product surfaces.
- **Kernel stays pure**: no direct I/O, no storage dependency, no transport types in the kernel public surface.
- **Storage remains externalized**: persistence belongs to host/runtime choice, not the kernel.
- **Deterministic portions stay byte-identical across modes**: same inputs + same kernel => same deterministic audit chain whether embedded or runtime.
- **Schemas are first-class**: tools/validators/inputs/outputs are all schema-backed.
- **Finance/regulatory calibration stays intact**: the system targets regulated finance evidence/replay needs, not generic chat-assistant guardrails.
- **Not framework-coupled**: LangChain/OpenAI/MCP clients are adapters only.
- **Cleaning is provenanced too**: normalizers, match/dedupe, and other preprocessing steps must remain on-chain and auditable.

Evidence / source anchors:
- `docs/thesis.md` lines **15-31**, **37-45**, **56-62**, **66-72**.
- `architecture/README.md` lines **69-76**, **155-166**.
- `architecture/OVERVIEW.md` lines **31-65**, **73-76**, **78-90**, **155-166**.

## Practical alignment checklist for reviewers
- Does the proposal keep `submit_answer` as the only release path?
- Does it preserve fail-closed behavior on tool/validator/schema/token failures?
- Does it keep kernel public API free of storage/network/transport dependencies?
- Does it avoid turning MCP/HTTP/CLI into primary product semantics?
- Does it preserve byte-identical deterministic audit output across library vs runtime modes?
- Does it keep token-level grounding and audit-chain replayability intact?
- Does it stay within the regulated-finance thesis rather than generic agent observability?

## Scope notes / boundaries
- v0 is **single-tenant / air-gapped capable**, with adapters MCP, HTTP, CLI.
- Go is the v0 kernel language; non-Go embedding is a later concern.
- The docs explicitly reject interpretations of Provenant as a database, SaaS, workflow engine, model, or guardrail-only product.

Key sources:
- `docs/thesis.md` lines **25-31**, **37-45**, **42-45**, **49-52**.
- `architecture/README.md` lines **155-168**.
- `architecture/OVERVIEW.md` lines **7.1-7.4**, **8-9**.
