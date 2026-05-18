# Phase 1 work packets — sequential by default

Phase 1 is a same-worktree plan first. Execute the packets in order, one at a time, in the same worktree. Parallel work is allowed only for read-only planning/review, or when each packet is isolated in its own git worktree. Keep each packet small and boring; do not widen scope to future-phase behavior.

## PR 1 / Packet 1 — contract spine
**What the PR should do:** define the minimal shared vocabulary Phase 1 needs before any feature work starts.

**What it should build:** minimal public run/source/token/audit shapes and the smallest interfaces those packets consume.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no reader logic, no bundle encoding, no persistence, no CLI/runtime/API surfaces, no broad abstractions.

**Pass criteria / validation:** the shared types compile cleanly; the shape is minimal and deterministic; later packets can import it without adding new surface area.

**Commit/PR boundary:** first. Land one small conventional commit in one PR; do not start Packet 2 until this PR is merged or otherwise accepted.

## PR 2 / Packet 2 — fixture reader
**What the PR should do:** implement one narrow reader over injected fixture/source bytes.

**What it should build:** a single structured reader path for `extract.json` (or the repo-equivalent narrow JSON reader), plus its selector/origin handling.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** source bytes only; no filesystem/network reads; no CSV/JSONL/spreadsheet coverage; no reader zoo.

**Pass criteria / validation:** fixture bytes produce stable structured output; malformed or absent input fails closed; the reader stays narrow and isolated.

**Commit/PR boundary:** second, after Packet 1. Land one small conventional-commit PR that changes only the reader lane.

## PR 3 / Packet 3 — kernel mint + audit capture
**What the PR should do:** wire the reader output into kernel-minted tokens and deterministic read capture.

**What it should build:** the mint path, captured read metadata/hash, and the deterministic in-memory audit chain.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** the reader must not mint tokens; no new storage backends; no new public surfaces.

**Pass criteria / validation:** one known reader output mints a token; the audit chain links deterministically; bad input fails closed.

**Commit/PR boundary:** third. Land one small conventional-commit PR that contains only mint/capture behavior.

## PR 4 / Packet 4 — bundle emit, verify, replay
**What the PR should do:** make the smallest deterministic bundle path and verify/replay it in memory.

**What it should build:** minimal bundle bytes, pure verification, and replay into in-memory state.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-testing-fuzzing`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no PDF/XLSX/export polish, no runtime bindings, no alternate source of truth, no broad proof framework.

**Pass criteria / validation:** bundle verification uses bytes only; replay reconstructs the deterministic Phase 1 state; tamper/truncation fails closed.

**Commit/PR boundary:** fourth. Land one small conventional-commit PR dedicated to bundle emit/verify/replay.

## PR 5 / Packet 5 — final integration + mutation gate
**What the PR should do:** wire the narrow end-to-end path and add the deterministic mutation failure test.

**What it should build:** source bytes -> reader -> capture -> mint -> bundle -> verify/replay, plus the one closed-failure mutation case.

**Skills/guidance to load:** `engineering-workflow`; `go-implementation-quality`; `go-testing-fuzzing`; `go-input-validation-security`; `conventional-commit-guard` before any commit message or PR title change.

**Implementation boundary:** no new behavior beyond wiring and acceptance coverage; no extra abstractions; no scope creep into later phases.

**Pass criteria / validation:** the end-to-end path passes in memory, the mutation test fails closed, and the packet does not widen the contract.

**Commit/PR boundary:** last. Land one final small conventional-commit PR for wiring plus acceptance tests only.