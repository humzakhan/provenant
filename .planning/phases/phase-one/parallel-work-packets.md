# Phase 1 work packets — sequential by default

Phase 1 is a same-worktree plan first. In one worktree, execute the packets in order, one at a time. Parallel work is allowed only for read-only planning/review, or when each packet is isolated in its own git worktree. Do not treat "parallelize everything" as the default or best path.

Keep the implementation small: less code, no broad abstractions, no helper sprawl, and no speculative generalization. Each packet should stay inside one narrow boundary and land as a small commit.

## Packet 0 — contract spine
**What needs to happen:** define the minimal shared vocabulary Phase 1 needs before any feature work starts.

**What needs to build:** minimal public run/source/token/audit shapes and the smallest interfaces those packets consume.

**Skills/guidance to use:** `engineering-workflow`; `conventional-commit-guard` before any commit message or PR title change; `orchestrator-mode` only if the user explicitly enables it.

**Implementation boundary:** no reader logic, no bundle encoding, no persistence, no CLI/runtime/API surfaces, no broad abstractions.

**Pass criteria / validation:** the shared types compile cleanly; the shape is minimal and deterministic; later packets can import it without adding new surface area.

**Recommended order / commit boundary:** first. Land one small conventional commit that contains only the contract spine.

## Packet 1 — fixture reader
**What needs to happen:** implement one narrow reader over injected fixture/source bytes.

**What needs to build:** a single structured reader path for `extract.json` (or the repo-equivalent narrow JSON reader), plus its selector/origin handling.

**Skills/guidance to use:** `engineering-workflow`; `conventional-commit-guard` before any commit message or PR title change; `orchestrator-mode` only if the user explicitly enables it.

**Implementation boundary:** source bytes only; no filesystem/network reads; no CSV/JSONL/spreadsheet coverage; no reader zoo.

**Pass criteria / validation:** fixture bytes produce stable structured output; malformed or absent input fails closed; the reader stays narrow and isolated.

**Recommended order / commit boundary:** second, after Packet 0. Land one small commit that changes only the reader lane.

## Packet 2 — kernel mint + audit capture
**What needs to happen:** wire the reader output into kernel-minted tokens and deterministic read capture.

**What needs to build:** the mint path, captured read metadata/hash, and the deterministic in-memory audit chain.

**Skills/guidance to use:** `engineering-workflow`; `conventional-commit-guard` before any commit message or PR title change; `orchestrator-mode` only if the user explicitly enables it.

**Implementation boundary:** the reader must not mint tokens; no new storage backends; no new public surfaces.

**Pass criteria / validation:** one known reader output mints a token; the audit chain links deterministically; bad input fails closed.

**Recommended order / commit boundary:** third. Land one small commit that contains only mint/capture behavior.

## Packet 3 — bundle emit, verify, replay
**What needs to happen:** make the smallest deterministic bundle path and verify/replay it in memory.

**What needs to build:** minimal bundle bytes, pure verification, and replay into in-memory state.

**Skills/guidance to use:** `engineering-workflow`; `conventional-commit-guard` before any commit message or PR title change; `orchestrator-mode` only if the user explicitly enables it.

**Implementation boundary:** no PDF/XLSX/export polish, no runtime bindings, no alternate source of truth, no broad proof framework.

**Pass criteria / validation:** bundle verification uses bytes only; replay reconstructs the deterministic Phase 1 state; tamper/truncation fails closed.

**Recommended order / commit boundary:** fourth. Land one small commit dedicated to bundle emit/verify/replay.

## Packet 4 — final integration + mutation gate
**What needs to happen:** wire the narrow end-to-end path and add the deterministic mutation failure test.

**What needs to build:** source bytes -> reader -> capture -> mint -> bundle -> verify/replay, plus the one closed-failure mutation case.

**Skills/guidance to use:** `engineering-workflow`; `conventional-commit-guard` before any commit message or PR title change; `orchestrator-mode` only if the user explicitly enables it.

**Implementation boundary:** no new behavior beyond wiring and acceptance coverage; no extra abstractions; no scope creep into later phases.

**Pass criteria / validation:** the end-to-end path passes in memory, the mutation test fails closed, and the packet does not widen the contract.

**Recommended order / commit boundary:** last. Land one final small commit for wiring plus acceptance tests only.
