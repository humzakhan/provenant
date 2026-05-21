# Provenant Architecture

This document is the synthesis view of Provenant's architecture. It does not duplicate the per-subsystem, per-adapter, or per-tool documents under [`architecture/`](./); it links to them and adds the integrated picture, the technology choices, the module-boundary contract, and the open questions.

Start with [`docs/thesis.md`](../docs/thesis.md) for the product claim. Read [`architecture/README.md`](README.md) for the navigational map and the canonical invariant list. This document is what to read after both, before touching code.

## 1. System overview

**The core product is the enforcement layer as a library.** It is plugged into anything that already drives an agent loop — a LangChain pipeline, a Go service, a notebook, Claude Code over MCP, a CI step. The library receives input, enforces invariants while computing, and emits a complete provenance bundle as bytes. **What the caller does with those bytes is the caller's concern.** The MCP server and the CLI are *access surfaces* that exist on the side, for hosts that don't want to (or can't) embed the library directly.

```
                      ┌─────────────────────────────┐
                      │     Provenant kernel        │
                      │   (Go library, no I/O,      │
                      │   no storage assumption)    │
                      │                             │
                      │   - Guard pipeline          │
                      │   - Tool registry           │
                      │   - Token graph             │
                      │   - Hash-chained audit      │
                      │   - submit_answer           │
                      │   - HITL pause/resume API   │
                      └──────────────┬──────────────┘
                                     │ pure-Go public API
       ┌─────────────────────────────┼─────────────────────────────┐
       │                             │                             │
   ┌───▼────────┐               ┌────▼─────────┐             ┌─────▼──────┐
   │ Embedded   │               │  MCP server  │             │    CLI     │
   │  (Mode A)  │               │  (Mode B)    │             │   binary   │
   │            │               │              │             │            │
   │ Your host  │               │ Optional     │             │ Optional   │
   │ owns the   │               │ access       │             │ access     │
   │ lifecycle  │               │ surface for  │             │ surface    │
   │ and the    │               │ hosts that   │             │ for batch  │
   │ bytes      │               │ talk MCP     │             │ + scripts  │
   └────────────┘               └──────────────┘             └────────────┘
```

The library is the product. MCP and CLI are **thin shells that wrap the library** and trade convenience for transparency (e.g. you can't see internal events the library exposes naturally as Go return values). **Storage is a concern of (a) the host that calls the library and (b) the optional MCP/CLI surfaces *when they choose to persist*, not of provenant itself.** See [ADR-0011](adrs/0011-storage-is-a-deployment-concern.md).

The novelty is not the box diagram — it is the boundary lines:

- **Kernel** is pure: no I/O at the public surface, no `net/*` or `database/sql` import anywhere transitively reachable from `kernel/`. Every I/O dependency is an injected interface. The kernel ships a **first-class in-memory binding** of those interfaces under `kernel/public/memio/` as the library-mode default; it ships nothing else as a storage binding.
- **Access surfaces** (MCP server, CLI, HTTP) are thin runtime-layer shells over the kernel public Go surface. None has architectural primacy ([ADR-0010](adrs/0010-mcp-equal-not-primary.md)). All three must reach the same verified core path and avoid a second source of truth when the captured inputs, pinned facts, kernel version, and backend versions match ([implementation-plan.md](../.planning/implementation-plan.md) Phase 3/4 boundaries).
- **Persistent storage bindings** (`runtime/sqlite/` default, `runtime/postgres/` for multi-tenant) live **outside the kernel module** and are owned by the runtime layer. A host that embeds the library is free to ignore them entirely.

For the canonical invariants (non-bypassable, fail-closed, schema-first, replay-deterministic), see [`architecture/README.md`](README.md#core-invariants). Every component below preserves them.

## 2. Technology stack and rationale

| Layer | Choice | Rationale | Reference |
|---|---|---|---|
| Kernel language | Go | Single static binary, mature MCP ecosystem, contributor pool in fintech ops, strong-enough type system for tool contracts | [ADR-0001](adrs/0001-go-as-kernel-language.md), [ADR-0009](adrs/0009-kernel-public-go-surface.md) |
| Library-mode persistence | **None required.** In-memory binding under `kernel/public/memio/` ships as the first-class default; emits a self-contained audit bundle as bytes | The kernel has no storage requirement; the bundle is the canonical output | [ADR-0011](adrs/0011-storage-is-a-deployment-concern.md), [`subsystems/storage.md`](subsystems/storage.md) |
| Runtime-mode persistence (optional access surfaces) | SQLite by default (`runtime/sqlite/`); Postgres 15+ for multi-tenant (`runtime/postgres/`) | Zero-ops single-file (SQLite) covers the dev / single-tenant deployments; Postgres covers multi-process and multi-tenant | [ADR-0011](adrs/0011-storage-is-a-deployment-concern.md), [`subsystems/storage.md`](subsystems/storage.md) |
| Blob store (optional, runtime-mode) | S3-compatible (S3, MinIO, GCS, R2) via `runtime/blobstore/s3/` | Source PDFs, OCR outputs, full prompts/responses are kilobyte-to-megabyte payloads that the runtime layer may park out of the database; library-mode bundles include them in-line | [`subsystems/storage.md`](subsystems/storage.md) |
| Wire protocols (adapters) | MCP, HTTP, CLI (equal first-class) | Universal agent tool protocol (MCP), REST surface for non-MCP hosts (HTTP), full Mode A in-process driver (CLI); no adapter has architectural primacy | [ADR-0010](adrs/0010-mcp-equal-not-primary.md) (supersedes [ADR-0007](adrs/0007-mcp-as-primary-adapter.md)), [`adapters/mcp.md`](adapters/mcp.md) |
| Numeric type | `shopspring/decimal` | Float arithmetic on monetary values is a defect, not a tradeoff; banker's rounding at canonical 9-decimal precision | [`subsystems/kernel.md`](subsystems/kernel.md) |
| Audit chain hash | SHA-256 over canonicalized JSON | Industry-standard; tamper-evident; cheap to verify | [`subsystems/audit.md`](subsystems/audit.md) |
| Output enforcement pattern | Pre-LLM input guards + post-LLM validator pipeline + token grounding at single `submit_answer` exit | [Arthur AI](references.md#arthur-guardrails) pre/post split; [DCCD](references.md#dccd) draft-then-enforce; [Faramesh](references.md#faramesh) non-bypassable enforcement | [ADR-0002](adrs/0002-submit-answer-as-single-exit.md), [ADR-0003](adrs/0003-fail-closed-default.md), [ADR-0004](adrs/0004-token-level-grounding.md), [`subsystems/guards.md`](subsystems/guards.md) |
| Determinism stance | Decision determinism and faithfulness as independent metrics; temperature 0 on compliance paths | [DFAH](references.md#dfah) | [ADR-0005](adrs/0005-determinism-faithfulness-independent.md) |
| Workflow unit | Signed skill pack (prompts, tools required, guards config, eval thresholds) | [Moody's modular AI](references.md#moodys-modular) pattern; supply-chain story via signing | [ADR-0006](adrs/0006-skill-pack-format.md), [`subsystems/skills.md`](subsystems/skills.md) |
| Migrations (Postgres binding only) | atlas; forward-only | Audit chain consistency over time requires no backwards mutations; lives under `runtime/postgres/migrations/` | [`subsystems/storage.md`](subsystems/storage.md) |
| Postgres binding (runtime layer) | `pgx` + `sqlc` | Type-safe, mature, no ORM overhead; lives in `runtime/postgres/`, not `kernel/` | [ADR-0001](adrs/0001-go-as-kernel-language.md), [ADR-0011](adrs/0011-storage-is-a-deployment-concern.md) |
| HTTP | `chi` | Idiomatic Go, stdlib-aligned | [ADR-0001](adrs/0001-go-as-kernel-language.md) |
| Observability | Prometheus metrics + OpenTelemetry traces + JSON logs | Standard cloud-native stack | [`cross-cutting/observability.md`](cross-cutting/observability.md) |

### 2.1 Open questions on the stack

These are deliberately unresolved here. The hone critic/defender loop will adjudicate.

1. **Go is accepted for v0; the remaining question is binding shape.** [ADR-0001](adrs/0001-go-as-kernel-language.md) remains the v0 kernel-language decision. The open question now is whether non-Go hosts reach the library through FFI or through a strict-latency sidecar. Surfaces to test against Go:
   - CGo/FFI ergonomics for Python and TypeScript hosts: viable, but the cost of marshaling provenance tokens and decimal values across the FFI boundary is non-trivial and may force a sidecar-only strategy for non-Go hosts.
   - Distribution as a library to non-Go hosts: requires either a stable C ABI (substantial engineering) or a sidecar with strict latency budget (gives up on true in-process embedding).
   - GC pause behavior under sustained audit-hash workloads at high throughput: not validated; could matter for hot paths.
   - Alternatives to revisit: Rust (better FFI story; harder contributor cohort) only as a future non-Go host binding, not as a Phase-0 kernel rewrite. No decision is taken in this document.

2. **Is the current top-level architecture/ structure the right shape for dual-mode pluggability?** Today: `architecture/{subsystems,adapters,tools,adrs,cross-cutting}`. The implicit assumption is that the kernel and the runtime co-locate. If code-level embedding is first-class, a better shape may be:
   ```
   architecture/
     kernel/         pure-library design (no I/O assumptions)
     runtime/        process/daemon/MCP/HTTP/CLI surfacing
     adapters/       host-side framework shims (LangChain, OpenAI Agents, etc.)
     bindings/       non-Go FFI / SDK design (Python, TS, Rust)
     tools/          tool category specs + per-backend specs
     adrs/           decision records
     cross-cutting/  security, observability, deployment
   ```
   The current ADRs do not address this split. No decision is taken here; this is flagged for the critic loop.

3. **MCP-first vs. MCP-only - resolved cycle 4.** ~~[ADR-0007](adrs/0007-mcp-as-primary-adapter.md) makes MCP primary.~~ Resolved by [ADR-0010](adrs/0010-mcp-equal-not-primary.md): MCP, HTTP, and CLI are equal first-class adapters; none has architectural primacy. The runtime layer treats MCP as one of N transports. Engineering sequencing is tracked in [`implementation-plan.md`](../.planning/implementation-plan.md): Phase 3 adds thin access surfaces and Phase 4 hardens them. Listed here for cross-reference rather than as an open question.

## 3. Module boundary contract

The library-as-core framing in [`docs/thesis.md`](../docs/thesis.md) requires a strict separation between what is "pure kernel" and what is "runtime / access-surface". This section is the contract.

**The kernel ships an in-memory binding of every I/O interface as a first-class public API**, not a test artifact, under `kernel/public/memio/`. This is the library-mode default — a host that does `kernel.Run(ctx, input)` against the in-memory binding gets a complete run and a self-contained audit bundle as bytes, with no I/O dependency beyond the kernel module itself. **The kernel module imports nothing from `runtime/` or any database driver.** The kernel's public-API snapshot test (`kernel/public/snapshot_test.go`) rejects any new public type that would require a network or DB dependency at any transitive import.

### 3.1 Kernel public surface (pure library)

**Allowed in the kernel's public API:**

- Function and method signatures whose parameters and return types are: Go primitives, kernel-defined value types (`Token`, `Claim`, `ValidationError`, `AuditEntry`, etc.), `context.Context`, `decimal.Decimal`, `time.Time`, and interfaces defined in the kernel.
- Interfaces for every I/O dependency: `Store` (token + run + audit row persistence), `BlobStore` (large payload persistence), `ModelClient` (LLM invocation), `Clock`, `RandSource`, `AuditSink` (sequence + hash-chain append target), `ToolBackend` (per-tool I/O — SQL ledger, Textract, etc.), and `TransactionalScope` (joint-transaction primitive across `Store` and `AuditSink` — see below).
- **`TransactionalScope` (joint-transaction primitive).** The Go interface block — `WithTx(ctx, fn)` signature, `TransactionalSubstrate()` marker contract, and the versioned `provenant.substrate/<vendor>/<engine>/v<n>` marker grammar — is enumerated in [`subsystems/kernel.md`](subsystems/kernel.md#transactionalscope-joint-transaction-primitive) as the source of truth. The parallel-block atomicity invariant in [`subsystems/audit.md`](subsystems/audit.md#parallel-sibling-deterministic-order) ("tokens and `audit_entries` are written in a single transaction at flush time") cannot be honored when `Store` and `AuditSink` are arbitrary separately-injected backends. The kernel public surface therefore declares `TransactionalScope` as a third interface that a host MUST inject alongside `Store` and `AuditSink` whenever it injects either, and that the host MUST back with a single transactional substrate (typically: the same Postgres connection pool backs all three). The kernel calls `TransactionalScope.WithTx(ctx, func(tx) { storeOp(tx); auditOp(tx) })` at parallel-block flush, at `submit_answer` persistence, and at orphan-recovery; if `WithTx` returns separate transactions per call or commits separately, the kernel detects the violation at runtime via a per-tx canary write (`audit_entries` and `tokens` written in the same tx must share a tx-local sequence the kernel checks before commit) and fails closed with `kernel_integrity`. A `NullTransactionalScope` provided in `kernel/public/memio/` is acceptable only inside the in-memory binding that backs both `Store` and `AuditSink` from the same map. Two-backend hosts that inject a Postgres `Store` and an S3 `AuditSink` are rejected at kernel boot: the kernel refuses to start if the host injects `Store` and `AuditSink` whose `TransactionalSubstrate()` markers disagree. This closes the cycle-3 outstanding item: "embedding hosts MUST use a shared transactional substrate" is now a typed contract, not documentation.
- Pure functions: hash construction, schema validation, token derivation, numeric extraction, validator pipeline composition.

**Forbidden in the kernel's public API surface:**

- Direct imports of `net/*`, `net/http`, `database/sql`, `github.com/jackc/pgx`, MCP SDK types, `os` (beyond minimal use in test helpers), `os/exec`.
- Process lifecycle: no `main`, no signal handling, no daemon supervision.
- Transport types: no HTTP handlers, no MCP request/response shapes, no CLI flag definitions.
- Goroutine-spawning helpers that escape function scope without an explicit `context.Context` and an injected cancel mechanism.

**Phase 0 of the implementation plan** is the explicit carve-out of this surface. A linter or `go vet` check enforces the forbidden-import list against `kernel/...`.

### 3.2 Runtime layer (the optional access surfaces)

The runtime layer is the only place where I/O is allowed to be implementation-specific. It is **optional**: a host that embeds the library directly never imports anything from `runtime/`.

- Owns: persistent storage bindings (`runtime/sqlite/` default, `runtime/postgres/` for multi-tenant) implementing the kernel's `Store`, `AuditSink`, and `TransactionalScope` interfaces; the S3 client (`runtime/blobstore/s3/`) implementing `BlobStore`; Anthropic/OpenAI/local-LLM HTTP clients (concrete `ModelClient`); the MCP server (`runtime/mcp/`, using the official Go MCP SDK); the CLI entry points (`runtime/cli/`, `cobra` or equivalent); process lifecycle, configuration loading, telemetry export.
- Forbidden: any enforcement logic. The runtime layer must not, for example, decide whether a `submit_answer` payload passes — it must call the kernel's pipeline. The runtime layer is allowed to translate kernel errors into transport shapes; it is not allowed to suppress, invent, or reinterpret enforcement decisions.
- Persistence policy (per [ADR-0011](adrs/0011-storage-is-a-deployment-concern.md)): SQLite is the default persistent binding when HITL pause/resume is enabled; Postgres is the multi-tenant / multi-process option. The bundle-emitting batch mode of the CLI requires no persistent binding — it runs the kernel against `kernel/public/memio/` and writes the bundle to disk.

### 3.3 Adapter shims

Adapter shims are translation layers between a host framework's idioms and runtime APIs. They are thin by design.

- MCP adapter ([`adapters/mcp.md`](adapters/mcp.md)): registers Provenant tools as MCP tools, skill prompts as MCP prompts, audit/case data as MCP resources.
- HTTP adapter ([`adapters/http.md`](adapters/http.md)): REST surfacing of the same operations.
- CLI adapter ([`adapters/cli.md`](adapters/cli.md)): full Mode A harness; the only adapter that drives the agent loop in-process.
- LangChain adapter ([`adapters/langchain.md`](adapters/langchain.md)) and OpenAI Agents adapter ([`adapters/openai.md`](adapters/openai.md)): deferred to v0.5; documented to establish the integration slot.

### 3.4 Bindings (non-Go embedding)

Not present in the current `architecture/` tree. Flagged as part of the top-level-structure open question (§2.1.2). The dual-mode promise requires that at least one non-Go binding exist by Phase 5 of the implementation plan. Two viable shapes:

- **Sidecar protocol with native client.** A Python or TypeScript client speaks a minimal, latency-tight wire protocol (a subset of HTTP or MCP) to a local Provenant runtime process. Pros: zero FFI complexity; cons: process-boundary overhead, slight loss of in-process embedding ergonomics.
- **CGo / FFI binding.** The kernel exposes a stable C ABI subset; Python and TypeScript bindings wrap it. Pros: true in-process embedding; cons: substantial engineering, GC interactions to validate.

Phase 5 will pick one (the critic loop and prototype work decide).

## 4. In-memory data model

The kernel holds the following typed structures in memory during a run. These are the **load-bearing shapes**; any persistent binding (`runtime/sqlite/`, `runtime/postgres/`) projects them onto a row schema — see [`subsystems/storage.md`](subsystems/storage.md) for the reference projection. The in-memory binding emits them directly as canonical JSON into the audit bundle.

- **`Run`** — one value per agent execution; status transitions audited.
- **`Token`** — provenance tokens. Every numeric value in any released answer corresponds to exactly one token, with `source_tool`, `source_path`, `source_origin`, and (for computed values) a `derivation` of parent token IDs. See [`subsystems/kernel.md`](subsystems/kernel.md). The token graph is the per-number lineage substrate.
- **`ToolCall`** — every tool invocation, with input and output payloads (pointers to `BlobStore` for large blobs; in-memory bytes in library mode). Foreign-keyed from `Token.SourceCallID`. Without this, replay is impossible.
- **`AuditEntry`** — hash-chained, per-tenant-monotonic. Every event in [`subsystems/audit.md`](subsystems/audit.md)'s `EntryKind` list produces an entry; `prev_hash` and `hash` form the chain. This is the regulator artifact. In library mode the chain is embedded as `audit_jsonl` inside the single JSON audit bundle; in runtime mode the runtime binding projects it onto the three-column `audit_entries` row shape.
- **`GuardRun`** — pre-LLM and post-LLM guard executions, with actions and errors. Referenced by the audit chain.
- **`Case`** and **`CaseMetric`** — case bank for replay and the four DFAH-style metrics ([`subsystems/eval.md`](subsystems/eval.md)). Library-mode hosts that want a case bank persist these themselves; the runtime layer ships a reference persistence for them.

Cross-cutting invariants in the in-memory data model:

- Every consequential value carries `tenant_id`. Cross-tenant references are kernel-rejected at every join point.
- IDs are ULIDs (lexicographically sortable) for fresh-minted entities; deterministic IDs (`tool_call_id`, `parent_step_id`, `sibling_group_id`) are hash-derived per [`subsystems/audit.md`](subsystems/audit.md).
- Numeric values use canonical 9-decimal precision with banker's rounding applied at mint time and audit-logged. The original full-precision value as returned by the tool backend (e.g., 10–12-decimal FX rates from OpenFX, 38-decimal raw extraction confidences) is also retained on the token as `source_value_raw` alongside the canonicalized `value`. The audit chain hashes the canonical value; the `source_value_raw` is retained for provenance answers ("what did the upstream actually return?") and is included in audit packs. Derivation arithmetic (sum, subtract, percent-change) operates on the canonical value; raw-vs-canonical drift is reportable per token. The exact parsing grammar, rounding mode (banker's / `roundTiesToEven` at 1e-9), tie-break (always to even, sign-independent), serialization format (decimal-only, no exponent, 9 zero-padded fractional digits, `-0` normalized to `0`), and the cross-language reference fixture vector are enumerated in [`subsystems/audit.md`](subsystems/audit.md#numeric-canonicalization-cross-language-replay) so Phase 5 non-Go bindings produce byte-identical chains.
- (Runtime-mode persistent bindings only.) The Postgres reference binding's schema is forward-only; rollbacks are reversing migrations, never reversions. This is what allows the audit chain to remain consistent across years for runtime-mode deployments. Library-mode hosts inherit the host's own retention posture.

## 5. Component interactions

### 5.1 Kernel ↔ adapters

Adapters call kernel functions through the public API. They do not know about Postgres, the model client, or the audit chain hash — those are kernel internals or injected interfaces.

- An MCP `tools/call` request arrives at the runtime → runtime authenticates and resolves tenant → runtime calls `kernel.Tools.Invoke(ctx, runID, toolName, payload)` → kernel runs schema validation, dispatches to the registered `ToolBackend`, mints tokens, writes audit entries via `AuditSink`, returns the result → runtime translates to MCP response shape.
- The same flow embedded in a Go host: the host calls `kernel.Tools.Invoke(...)` directly, having injected its own `Store`, `BlobStore`, `ModelClient`, `AuditSink`. No runtime layer involved.

### 5.2 Kernel ↔ tools

Tools register typed input/output schemas plus `mintable_paths` (JSONPaths) with the kernel's tool registry. The registry wraps every tool with audit middleware: every invocation gets a `tool_call_started` and `tool_call_completed` entry, and the kernel mints tokens from the declared paths in the response. See [`subsystems/tools.md`](subsystems/tools.md) and [`architecture/tools/README.md`](tools/README.md). Tools cannot construct `Token` values directly — only the kernel can — which preserves the "every token has a kernel-assigned ID and kernel-controlled source" property.

The v0 category set is `ledger`, `extract`, `reconcile`, `calc`, `policy`, `external`, `match`, and `dedupe`. The two new categories (`match`, `dedupe`) address messy financial inputs: `match` is the typed fuzzy-comparison primitive (pgvector reference backend; operator-configured embedding model, same pattern as `policy`'s pgvector backend), and `dedupe` is the clustering tool built on top of `match` with auto/HITL routing per cluster confidence. Together with the v0 built-in normalizer set (`numeric.locale.eu_comma_decimal`, `numeric.thousands_separator.nbsp`, `currency.iso4217`, `date.iso8601_lenient`, `entity_name.basic`) the cleaning step is provenanced like any other tool output. See [`architecture/tools/match/README.md`](tools/match/README.md), [`architecture/tools/dedupe/README.md`](tools/dedupe/README.md), and [`subsystems/skills.md`](subsystems/skills.md#pack-level-normalizers-normalizers).

### 5.3 Kernel ↔ storage

The kernel's `Store`, `AuditSink`, `BlobStore`, and `TransactionalScope` interfaces have a **first-class in-memory implementation at `kernel/public/memio/`** — this is the library-mode default, not a test artifact. A host that calls `kernel.Run(ctx, input)` against `kernel/public/memio/` gets a complete run and a self-contained audit bundle as bytes with no I/O dependency beyond the kernel module itself. Persistent bindings (SQLite for single-process runtime, Postgres for multi-process runtime) live in `runtime/` and are loaded by the optional MCP and CLI access surfaces only when persistence beyond a single call is required (typically HITL pause/resume). **The kernel module itself never imports a database driver.** Phase 0's import-guard CI check enforces this and is what allows the kernel to build and test with no I/O dependency.

### 5.4 Submit pipeline

The submit pipeline is the single output bottleneck. Every adapter, every mode, every host hits the same kernel function: `kernel.SubmitAnswer(ctx, runID, payload)`. The function runs numeric extraction, per-claim token grounding (calling `kernel.VerifyToken`), the configured validator pipeline, optional BoN-MAV, and finally persists or rejects. The algorithm is in [`subsystems/guards.md`](subsystems/guards.md). The kernel returns either `OK` with the persisted answer or a structured error list. Adapters translate. No adapter is permitted to short-circuit this call.

## 6. Security model

The threat model, multi-tenant isolation guarantees, prompt-injection defense strategy, and credential-hygiene rules live in [`cross-cutting/security.md`](cross-cutting/security.md). The relevant integrated points for this synthesis view:

- **Tenant isolation is kernel-enforced**, not just storage-enforced. `VerifyToken` independently asserts that the returned token's `tenant_id` equals the verifying run's `tenant_id` and that the token's `run_id` is the verifying run (or, for the cross-run case explicitly enabled by skill config, a run within the same tenant). A `Store` implementation that returns rows from any tenant is caught at the kernel boundary: the kernel does not trust `Store` to filter by tenant. This is enforced via a kernel unit test that uses a deliberately permissive in-memory `Store` (returns all tokens regardless of `tenant_id` argument) and verifies that `VerifyToken` still rejects cross-tenant references. Without this check, the claim that tenant isolation survives storage-layer compromise would be false.
- **Prompt injection defense is architectural**, not promptual. The pre-LLM guard pipeline is a code path the agent cannot route around; see [`subsystems/guards.md`](subsystems/guards.md#pre-llm-input-guards).
- **Audit tamper-evidence** is the per-tenant hash chain over canonicalized JSON entries. The audit chain is constructed **in memory by the kernel** and emitted as bytes in the single JSON audit bundle (`audit_jsonl` carries one JSONL row per `AuditEntry`). Tamper-evidence is a property of **those bytes**, regardless of where the host stores them — in library mode the bundle bytes go wherever the host writes them; in runtime mode the runtime binding projects the chain onto rows in addition to (not instead of) the bundle. Tampering is detectable, not preventable (we do not defend against root on the host).
- **Skill-pack supply chain** is signed-pack-with-admin-approval ([ADR-0006](adrs/0006-skill-pack-format.md)); v0 ships unsigned local packs only, v0.5 adds the hosted registry with signing.
- **Skill-pack content pinning (v0).** Even without signing, every run records the SHA-256 digest of the loaded skill pack's canonical content in the `run_started` audit entry. The digest covers prompts, guards config, manifest, examples, and templates. `provenant replay` recomputes the digest from the on-disk skill pack and refuses to replay if the digest does not match the recorded digest. This catches silent mutation of `prompts/skill.md` between runs — without it, the deterministic-portion hash chain could be identical across two runs whose actual agent input differed. Skill-pack signing in v0.5 is a stronger version of this same pin (signature instead of digest comparison). The full digest procedure — file enumeration rules, the default exclusion list (`.DS_Store`, `Thumbs.db`, `.git/`, `.gitignore`, `.gitattributes`, `*.swp`/`*.swo`/`*~`/`.#*`/`#*#`), the symlink-rejection rule, the POSIX-path normalization, the length-prefixed concatenation, and the verbatim (no trailing-newline normalization, no line-ending conversion) per-file byte rule — is enumerated in [`subsystems/audit.md`](subsystems/audit.md#skill-pack-content-digest-procedure). The procedure is locked; changes require a kernel version bump.

For the full threat list and mitigation table, read the cross-cutting doc.

## 7. Deployment model

Provenant ships in three modes. The library is the product; the MCP server and the CLI are access surfaces on the side. Detail lives in [`cross-cutting/deployment.md`](cross-cutting/deployment.md); the integrated picture:

### 7.1 Library mode (Mode A)

The host application links the Provenant kernel directly (Go module import; or, post-Phase 5, a non-Go binding) and calls `kernel.Run(...)` against the in-memory binding under `kernel/public/memio/`. The kernel emits a self-contained single JSON audit bundle as bytes; the host decides what to do with them.

- **Deployer is responsible for:** invoking the kernel, doing something with the returned bundle bytes (storing, signing, forwarding, throwing away — its choice), and serializing the kernel's `RunState` snapshot if it wants HITL pause/resume across process boundaries.
- **Kernel handles:** every enforcement decision, the audit chain construction, the bundle emission, the typed `RunState` snapshot at any HITL pause boundary. **No I/O.** No network socket, no file handle, no database connection.
- Lifecycle: host owns it. Kernel exposes a `Shutdown(ctx)` for graceful teardown only as a convenience for hosts that want to flush in-flight goroutines; the kernel does not own a process.
- Multi-tenancy: the host passes `tenant_id` per call; the kernel enforces.
- Audit chain: byte-identical (on the deterministic portion) to runtime-mode chains when the captured inputs, pinned facts, kernel version, and backend versions match, because the same kernel Go code is doing the work.

### 7.2 MCP server access surface (optional)

The runtime layer ships `provenant-server`, an MCP server binary that wraps the kernel public Go surface and exposes Provenant tools, prompts, and audit/case resources over MCP. The host (Claude Code, Claude.ai, a custom MCP client) speaks MCP over stdio or loopback.

- **Deployer is responsible for:** running the binary, providing model API credentials, picking a persistent storage binding **only if HITL pause/resume is enabled** (SQLite by default, Postgres for multi-tenant / multi-process — both live in `runtime/`, not `kernel/`).
- **Kernel handles:** every enforcement decision, identical to library mode.
- **Stateless option:** an MCP server that does not need HITL pause/resume can run entirely against `kernel/public/memio/` and emit bundles per request. No database needed.

### 7.3 CLI access surface (optional)

The runtime layer ships `provenant`, a CLI binary that wraps the kernel public Go surface and the Mode A harness for batch and scripting workflows.

- **Deployer is responsible for:** invoking the binary, providing model API credentials, picking a persistent storage binding **only if HITL pause/resume across CLI invocations is needed**. A single-shot `provenant run` + `provenant audit pack` flow against in-memory state requires no database — the bundle goes to disk.
- **Kernel handles:** every enforcement decision, identical to library and MCP modes.
- **Batch / CI mode:** `provenant run --bundle-out ./out.zip` runs the kernel against `kernel/public/memio/` and writes the bundle as a single zip. This is the recommended deployment shape for CI gates, regulatory replay drills, and air-gapped batch jobs.

### 7.4 Latency budget for the access-surface shells

The MCP and CLI shells over the kernel public Go surface must add under 50ms p95 of overhead on top of validator costs (matches the MCP latency budget in [`adapters/mcp.md`](adapters/mcp.md)). Higher overhead makes the access surface unviable for interactive workflows and pushes hosts toward the library-mode embed.

## 8. What this system explicitly does not do

Adapted and tightened from [`architecture/README.md`](architecture/README.md):

- **The kernel does not persist anything.** Persistence is the host's choice (library mode) or the runtime layer's choice (optional MCP/CLI access surfaces). See [ADR-0011](adrs/0011-storage-is-a-deployment-concern.md).
- **The kernel does not require Postgres, SQLite, or any database.** The kernel ships an in-memory binding under `kernel/public/memio/` as the library-mode default. Persistent bindings live under `runtime/` and are optional.
- **The kernel does not open a network socket, a file handle, or a database connection.** The import-guard CI check enforces it. The kernel takes typed inputs and produces typed outputs including a self-contained audit bundle.
- The optional MCP server ships SQLite-by-default *if HITL pause/resume is enabled*, but you can run the MCP server stateless (in-memory only) if you don't need HITL pause/resume across process boundaries.
- Does not write the agent's prompts. Skill packs do that, and only within the kernel-immutable header that announces `submit_answer`.
- Does not pick the model. The harness is model-pluggable; v0 verifies one model class (Claude).
- Does not run the user's GL, ERP, sanctions list, FX feed, or extraction service. It wraps them as tools with declared input/output schemas. See [`architecture/tools/`](tools/).
- Does not provide a hosted SaaS in v0. Single-tenant and air-gapped only; SaaS is v0.5.
- Does not provide a web HITL UI in v0. HITL routing is REST + database-state; the UI is v0.5.
- Does not provide a hosted skill registry in v0. Skill packs ship as local files; registry with signing is v0.5+.
- Does not provide BoN-MAV mode in v0. The validator pipeline supports it ([`subsystems/guards.md`](subsystems/guards.md#optional-bon-mav-mode)); the runtime is wired but the verifier-selection mode ships v0.5+.
- Does not provide a LangChain or OpenAI Agents adapter in v0. The docs ([`adapters/langchain.md`](adapters/langchain.md), [`adapters/openai.md`](adapters/openai.md)) establish the slot; implementation is v0.5.
- Does not solve framework-internal correctness issues. If the host framework has its own bugs, Provenant cannot fix them; it can only enforce its own contract at the kernel boundary.
- Does not promise complete defense against a maliciously-compromised host (root access on the server). Tamper *detection* is provided; tamper *prevention* against root-equivalent actors is out of scope.

## 9. Open architectural questions

Aggregated for the critic/defender loop. Three are first-class and named above; this section lists them once.

1. **Kernel language choice under dual-mode pluggability.** §2.1.1. Revisits [ADR-0001](adrs/0001-go-as-kernel-language.md) given the new requirement that the kernel be embeddable into non-Go hosts.
2. **Top-level architecture/ structure under dual-mode pluggability.** §2.1.2. Proposes a possible `kernel/` + `runtime/` + `adapters/` + `bindings/` reshape; deliberately unresolved.
3. ~~**MCP-first vs. MCP-equal.** §2.1.3.~~ **Resolved cycle 4** by [ADR-0010](adrs/0010-mcp-equal-not-primary.md): MCP, HTTP, and CLI are equal first-class adapters.

Per-subsystem open questions live in the "Open questions" sections of each [`subsystems/<name>.md`](subsystems/). They are not duplicated here.

## 10. Where to read next

- For the rationale behind every load-bearing decision: [`architecture/adrs/`](adrs/).
- For per-subsystem detail: [`architecture/subsystems/`](subsystems/).
- For the wire protocol and adapter contracts: [`architecture/adapters/`](adapters/).
- For the tool conformance contract: [`architecture/tools/README.md`](tools/README.md).
- For the security threat model: [`architecture/cross-cutting/security.md`](cross-cutting/security.md).
- For the observability story: [`architecture/cross-cutting/observability.md`](cross-cutting/observability.md).
- For deployment shapes and runbooks: [`architecture/cross-cutting/deployment.md`](cross-cutting/deployment.md).
- For the phased build plan: [`implementation-plan.md`](../.planning/implementation-plan.md).
