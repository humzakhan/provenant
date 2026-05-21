# ADR-0011: Storage is a deployment concern, not a kernel concern

Status: **Accepted**
Date: 2026-05-18
Supersedes: [ADR-0008](0008-postgres-as-canonical-store.md) ("Postgres as the canonical transactional store") — Postgres ceases to be a kernel-level requirement; it becomes one of several optional persistence bindings owned by the runtime layer and by host applications that embed the library.

## Context

Through cycles 1–10 of the Hone loop the spec was written under the implicit assumption that **the Provenant system is a service that owns a Postgres database and an S3 bucket**. [ADR-0008](0008-postgres-as-canonical-store.md) made that assumption explicit ("Postgres 15+ is the canonical transactional store. Every transactional artifact lives in Postgres."). [`architecture/OVERVIEW.md`](../OVERVIEW.md) §1's system diagram terminated in a "Storage" box; [`subsystems/storage.md`](../subsystems/storage.md) described the schema as if every Provenant deployment owned it; [`cross-cutting/deployment.md`](../cross-cutting/deployment.md) described four deployment shapes that all assumed Postgres and an object store were sitting next to the kernel.

That framing is wrong about what the product actually is.

The core product is the enforcement layer **as a library**. The kernel is plugged into whatever already drives the agent loop — a LangChain pipeline, a Go service, a notebook, Claude Code over MCP, a CI step. The library receives input, enforces invariants while computing, and emits a complete provenance bundle as bytes. **What the caller does with those bytes is the caller's concern.** The MCP server and the CLI are *access surfaces* that exist on the side, for hosts that don't want to (or can't) embed the library directly.

[ADR-0009](0009-kernel-public-go-surface.md) ratified the Go-only public surface as a v0 first-class deliverable, but the spec around it still treated storage as an architectural pillar rather than a deployment choice. This ADR closes the gap.

## Decision

**Storage is not part of the Provenant kernel.** The kernel:

- has **no storage requirement** and no opinion on persistence;
- ships an **in-memory binding** of `Store`, `AuditSink`, `BlobStore`, and `TransactionalScope` as a **first-class public API** — not a test artifact — under `kernel/public/memio/`. The in-memory binding ships at `kernel/public/memio/` **only**. The cycle-2/3 `kernel/internal/memstore/` location is **removed**; tests under `kernel/` use `kernel/public/memio/` exclusively. There is no parallel internal/public surface pair (per [ADR-0009](0009-kernel-public-go-surface.md));
- ships **nothing else** as a storage binding. No Postgres `Store`, no S3 `BlobStore`, no SQLite `Store` lives in the kernel module.

The kernel's public Go API exposes the four I/O interfaces (`Store`, `AuditSink`, `BlobStore`, `TransactionalScope`) only because complete runs need an in-memory place to accumulate state and an atomic-flush primitive for the cycle-3 Mint+audit atomicity rule. The interfaces are not a request that the host provide a database; they are the seams at which an embedding host may *optionally* wire one up if it wants persistence beyond the run.

After a run, the in-memory binding **emits a self-contained single JSON audit bundle** as bytes the caller can verify, hand off, persist, or throw away. The bundle is the canonical output. Bundle bytes verify via `provenant audit verify <bundle-path>` with no database access at any point.

Persistent storage bindings live **outside the kernel module**:

- `runtime/sqlite/` — SQLite-backed `Store` / `AuditSink` / `BlobStore` / `TransactionalScope`. SQLite is the default persistent binding for the optional MCP and CLI access surfaces because it is zero-ops and single-file.
- `runtime/postgres/` — Postgres-backed bindings for multi-process / multi-tenant deployments of the optional access surfaces.
- `runtime/blobstore/s3/` — S3-compatible `BlobStore` for the runtime layer's large-payload needs.

A host that embeds the library may either inject `kernel/public/memio/` (library mode default; the run is in-memory, the bundle is the artifact) or wire its own implementations of the interfaces against whatever substrate the host already operates. **The host is free to ignore the runtime/sqlite and runtime/postgres bindings entirely.** They are reference implementations for the MCP/CLI access surfaces, not a required dependency of the library.

HITL pause/resume in library mode is the host's responsibility. The kernel exposes a typed `RunState` snapshot (canonical-JSON encodable, schema enumerated in [`subsystems/kernel.md`](../subsystems/kernel.md)) at the pause boundary; the host serializes it, stores it wherever the host already keeps state, and hands it back on resume. The kernel's pure-API `Resume(ctx, runState)` accepts a `RunState` and continues. **The kernel has no opinion on where the host kept it.**

In runtime/CLI mode that ships HITL pause/resume, the runtime layer owns the persistence — and it picks SQLite by default (zero ops, single-file, fine for a developer machine or a single-tenant on-prem) or Postgres for multi-process / multi-tenant. That choice belongs to `runtime/`, not `kernel/`.

## Rationale

- **The library *is* the product.** A spec that treats storage as a kernel concern leaks deployment-shape assumptions into every consumer who would otherwise just call `kernel.Run(...)` and read the resulting bundle bytes. Removing Postgres from the kernel's required surface is the precondition for "you can use Provenant from a notebook" being literally true.
- **Byte-identity is between access surfaces, not between databases.** The thesis claim is that the deterministic-portion audit chain is byte-identical across **library, MCP, and CLI** because the same kernel Go code is doing the work in all three. Persistence does not enter this claim. A library-mode host that stores the bundle in S3 and a CLI run that stores it in Postgres produce the same chain bytes; the storage choice does not appear in the hash.
- **HITL state is the only thing that has to outlive a single call**, and it's the runtime layer's problem (or the host's), not the kernel's. The kernel can checkpoint a `RunState` and reload from one; whether that `RunState` was sitting in Postgres, SQLite, Redis, the host's existing job queue, or a base64 string in a Lambda event payload does not change a single byte the kernel produces.
- **`TransactionalScope` is preserved unchanged.** The cycle-3 contract — Store + AuditSink + TransactionalScope share one substrate, marker mismatch fails closed at boot, tx-local canary catches split-tx at runtime — was about the **shared transactional substrate**, not about Postgres specifically. The in-memory binding provides a `NullTransactionalScope` with marker `"provenant.substrate/memstore/sharedmap/v1"` that backs all three interfaces from the same map. The `runtime/sqlite/` and `runtime/postgres/` bindings each provide their own matching markers. None of this is kernel-level policy; the kernel only checks the markers agree.
- **The "audit chain in Postgres" claim from cycle 1 was a deployment shape, not an invariant.** The invariant is "the chain is hash-chained and tamper-detectable from its own bytes." Postgres makes append-only chain construction operationally convenient; it is not what makes the chain trustworthy. SHA-256 over canonical JSON makes the chain trustworthy. A chain embedded as `audit_jsonl` in the single JSON audit bundle is exactly as verifiable as a chain stored as Postgres rows.

## Consequences

**Positive:**

- Provenant becomes a thing you `go get` and call, full stop. The dependency footprint of a library-mode run is the kernel module and nothing else.
- The cross-mode byte-identity property in [`architecture/OVERVIEW.md`](../OVERVIEW.md) §5 and [`thesis.md`](../../docs/thesis.md) "What Success Looks Like" is sharper: byte-identity is between **library, MCP, CLI** (same kernel Go code, different bytes-in/bytes-out plumbing), not between **embedded-Postgres and runtime-Postgres** (which was always a degenerate case).
- Non-Go embedding (Phase 5) becomes easier. There is no longer a hidden assumption that a Python host has a Postgres connection lying around; the FFI / sidecar binding only has to marshal the typed inputs and bundle bytes.
- Operational surface for the simplest deployments collapses. A team running Provenant in a CI step or a batch job does not stand up Postgres for it.
- SQLite becomes a real first-class binding for the optional MCP/CLI surfaces, which makes the "zero ops, single binary, single file" demo concrete.

**Negative:**

- A library-mode host that needs HITL **across process boundaries** is now responsible for serializing the kernel's `RunState` and restoring it on a future call. The kernel provides the typed snapshot; the host provides the bytes-to-disk. This is captured as a new trade-off in [`docs/KNOWN_TRADE_OFFS.md`](../../docs/KNOWN_TRADE_OFFS.md).
- The mental model "Provenant runs on Postgres" that was implicit through cycles 1–10 has to be unlearned by anyone reading the older docs. ADR-0008 is left in place with a supersession header so the historical context survives.
- Two persistence bindings (SQLite, Postgres) now have to be maintained for the optional MCP/CLI surfaces. The runtime layer's test surface roughly doubles. Mitigation: the runtime-layer bindings share most of the projection logic (the in-memory binding is the reference; the persistent bindings project onto rows from the same canonical JSON).

**Out of scope:**

- This ADR does **not** retract any cycle-2 through cycle-10 fix that is still load-bearing. The `TransactionalScope` contract, the canonical `error_code` table, the `AuditEntry` / `PayloadEnvelope` split with deterministic / non-deterministic sub-objects, the JCS-canonical hash input, the `kernel_header_hash`, the `signature_marker`, the `host_conformance_attestation`, the `system:` sentinel principals, the normalizer registry, the match/dedupe tool surfaces, the orphan-recovery rules — all stay exactly as they are. **The change is about who owns persistence, not about what the kernel produces.**

## What this supersedes

- [ADR-0008](0008-postgres-as-canonical-store.md) is superseded as a kernel-level decision. Postgres is no longer "the canonical transactional store"; it is one of several optional persistence bindings the runtime layer provides. The detailed schema in ADR-0008 and in [`subsystems/storage.md`](../subsystems/storage.md) remains useful as a reference for anyone writing a Postgres binding under `runtime/postgres/`, but it is not a description of what Provenant *requires*.
- The cycle-1 framing that "the kernel is stateless; this layer is the source of truth" (lead paragraph of the old `storage.md`) is replaced by: **the kernel takes typed inputs and produces typed outputs including a self-contained audit bundle. All persistence is the caller's choice.**
- The implicit "every deployment of Provenant has a Postgres" baseline in [`cross-cutting/deployment.md`](../cross-cutting/deployment.md) is replaced by three explicit deployment modes: **library mode** (no required substrate), **MCP server access surface** (SQLite-by-default; Postgres optional), **CLI access surface** (SQLite-by-default; Postgres optional; bundle-emitting batch mode requires nothing persistent).

## Alternatives considered

**Keep ADR-0008; add ADR-0011 as "and the kernel can also run in-memory."** Rejected. The whole point is that in-memory is the **default**; persistence is the option. Treating Postgres as default and in-memory as "also supported" preserves the wrong center of gravity and leaks deployment-shape assumptions into every consumer.

**Ship Postgres binding inside the kernel module as a "default storage" subpackage that consumers can ignore.** Rejected. Any storage binding inside `kernel/` drags a database driver into the kernel's transitive dependency surface and breaks the [ADR-0009](0009-kernel-public-go-surface.md) import-guard contract. The import-guard CI check is precisely the mechanism that prevents this regression.

**Ship only SQLite in the kernel module (the "lightest possible" persistence).** Rejected for the same reason. Even SQLite is `database/sql` + a CGo dependency in pure-Go SQLite implementations' transitive set. The kernel does not need persistence to exist; making it accept one as a "lightweight default" weakens the no-I/O claim.

## References

- [`docs/thesis.md`](../../docs/thesis.md) — the library-as-core, surfaces-on-the-side framing
- [`architecture/OVERVIEW.md`](../OVERVIEW.md) — system diagram with library kernel and access surfaces as siblings
- [`architecture/subsystems/storage.md`](../subsystems/storage.md) — rewritten as "Storage (runtime concern, not kernel concern)" with reference Postgres/SQLite bindings retained for the runtime layer
- [`architecture/subsystems/kernel.md`](../subsystems/kernel.md) — `Store`, `AuditSink`, `BlobStore`, `TransactionalScope` interfaces and `RunState` snapshot
- [`docs/KNOWN_TRADE_OFFS.md`](../../docs/KNOWN_TRADE_OFFS.md) — new entry: library-mode HITL across process boundaries requires host-side `RunState` persistence
- [ADR-0008](0008-postgres-as-canonical-store.md) — superseded
- [ADR-0009](0009-kernel-public-go-surface.md) — kernel public Go surface, import-guard contract
- [ADR-0010](0010-mcp-equal-not-primary.md) — access surfaces are equals; byte-identity is between surfaces, not between databases
