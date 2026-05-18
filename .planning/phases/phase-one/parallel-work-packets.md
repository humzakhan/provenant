# Phase 1 work packets

Keep the split tiny. One integrator owns shared shapes; everyone else stays inside a narrow lane.

## Packet 0 — contract spine
**Owns:** minimal public types and in-memory run/source/token/audit shapes needed by Phase 1.

**Boundary:** no full registry, no CLI, no persistence abstractions, no broad selectors.

**Validation:** compiles cleanly; constructors/types only; no end-to-end logic.

## Packet 1 — kernel mint + audit/memio
**Owns:** kernel-minted token path, captured read recording, and the deterministic in-memory audit chain.

**Boundary:** do not let the reader create tokens; do not add new surfaces or storage.

**Validation:** mint from one known reader output path; audit chain links deterministically; bad input fails closed.

## Packet 2 — JSON fixture reader
**Owns:** the single `extract.json` reader over injected source bytes and one selector form (JSON Pointer or the existing repo-equivalent).

**Boundary:** no filesystem/network reads, no CSV/JSONL/spreadsheet coverage, no selector zoo.

**Validation:** fixture bytes -> stable structured output -> captured read hash; malformed/absent input fails closed.

## Packet 3 — bundle emit, verify, replay
**Owns:** minimal deterministic bundle bytes, pure verify, and replay back into in-memory state.

**Boundary:** no PDF/XLSX/export polish, no runtime bindings, no alternate source of truth.

**Validation:** bundle verifies from bytes only; replay reconstructs the deterministic phase state; tamper/truncation fails closed.

## Integration order
1. Packet 0 first.
2. Packets 1, 2, and 3 can then proceed in parallel on that contract.
3. Packet 3 should land before or with the final integration pass.
4. Final integration wires source bytes -> reader -> capture -> mint -> bundle -> verify/replay -> mutation failure.

## Ownership boundaries
- Packet 0 owns shared shapes.
- Packet 1 owns kernel mint/audit state.
- Packet 2 owns reader logic only.
- Packet 3 owns bundle bytes and verification.
- The final integrator owns no new behavior; only wiring and acceptance tests.
