# Provenant Architecture

This document is the synthesis view of Provenant's architecture. It does not duplicate the per-subsystem, per-adapter, or per-tool documents under [`architecture/`](architecture/); it links to them and adds the integrated picture, the technology choices, the module-boundary contract, and the open questions.

Start with [`thesis.md`](thesis.md) for the product claim. Read [`architecture/README.md`](architecture/README.md) for the navigational map and the canonical invariant list. This document is what to read after both, before touching code.

## 1. System overview

Provenant is a kernel plus adapters plus tools plus skills, with storage as a separate substrate. The novelty is not the box diagram — it is the boundary lines.

```
                       ┌────────────────────────────────────────────────┐
                       │              HOST APPLICATIONS                  │
                       │  (Claude Code, Claude.ai, Cursor, custom        │
                       │   Python/TS/Go agents, batch jobs, CI)          │
                       └────────────────────────────────────────────────┘
                                          │
   ─── wire-protocol edge ────────────────┼───── library edge ──────────────
                                          │
   ┌──────────────────────────────────────┴──────────────────────────────┐
   │                      ADAPTER SHIMS (thin)                            │
   │  ┌────────┐  ┌────────┐  ┌────────┐  ┌──────────┐  ┌──────────┐    │
   │  │  MCP   │  │  HTTP  │  │  CLI   │  │ LangChain│  │  OpenAI  │    │
   │  │ server │  │  API   │  │harness │  │ adapter  │  │ adapter  │    │
   │  └────┬───┘  └────┬───┘  └────┬───┘  └────┬─────┘  └────┬─────┘    │
   │       │           │           │           │ (v0.5)      │ (v0.5)   │
   │       └───────────┴───────────┴───────────┴─────────────┘          │
   │                                │                                    │
   └────────────────────────────────┼────────────────────────────────────┘
                                    │
                                    ▼
   ┌──────────────────────────────────────────────────────────────────────┐
   │                       RUNTIME LAYER (Go)                              │
   │  Process lifecycle, transport plumbing, auth, rate limits, daemon-   │
   │  ization, MCP/HTTP/CLI surfacing of kernel APIs. Does NOT contain    │
   │  enforcement logic — it delegates every decision to the kernel.      │
   └──────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
   ┌──────────────────────────────────────────────────────────────────────┐
   │                  PROVENANT KERNEL (pure library)                      │
   │   ┌──────────────────────────────────────────────────────────────┐   │
   │   │  Provenance engine (tokens, mint, derive, verify)            │   │
   │   │  Guard pipeline (pre-LLM input guards, post-LLM validators)  │   │
   │   │  Tool registry (schema-validated dispatch + audit middleware)│   │
   │   │  Harness (Mode A embedded agent loop; pure orchestration)    │   │
   │   │  Skill loader (pack format, lifecycle, signature verify)     │   │
   │   │  Audit logger (hash-chain construction; pure)                │   │
   │   │  Case bank / eval harness (replay, four DFAH-style metrics)  │   │
   │   │  HITL router (decision-only; transport injected)             │   │
   │   └──────────────────────────────────────────────────────────────┘   │
   │   Public API: function calls only. No net/*, database/sql, os, or    │
   │   process-lifecycle imports leak through the kernel's public surface. │
   │   I/O is injected via interfaces (Store, BlobStore, ModelClient,     │
   │   Clock, RandSource, AuditSink).                                     │
   └──────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
   ┌──────────────────────────────────────────────────────────────────────┐
   │                          STORAGE                                      │
   │   Postgres 15+ (transactional canonical store)                        │
   │   S3-compatible object store (blobs: docs, OCR, full prompts/resp.)   │
   └──────────────────────────────────────────────────────────────────────┘
```

Compared to [`architecture/README.md`](architecture/README.md)'s diagram, this view makes explicit a three-way split that the current docs blur:

- **Adapter shims** are thin translation layers between a host's idioms and runtime APIs.
- **Runtime layer** owns process and transport concerns. Enforcement does not live here.
- **Kernel** is pure: no I/O at the public surface, no `net/*` or `database/sql` import in the public API. Every I/O dependency is an injected interface.

This separation is the mechanical foundation of the dual-mode pluggability promise in [`thesis.md`](thesis.md). A host that wants to embed Provenant links the kernel directly and injects its own I/O implementations; a host that wants a sidecar talks to the runtime layer, which in turn calls the kernel.

For the canonical invariants (non-bypassable, fail-closed, schema-first, replay-deterministic), see [`architecture/README.md`](architecture/README.md#core-invariants). Every component below preserves them.

## 2. Technology stack and rationale

| Layer | Choice | Rationale | Reference |
|---|---|---|---|
| Kernel language | Go | Single static binary, mature Postgres/MCP ecosystem, contributor pool in fintech ops, strong-enough type system for tool contracts | [ADR-0001](architecture/adrs/0001-go-as-kernel-language.md) |
| Canonical transactional store | Postgres 15+ | JSONB for variable payloads, row-level security for multi-tenant, hash-chain-friendly append semantics, single-binary-friendly | [ADR-0008](architecture/adrs/0008-postgres-as-canonical-store.md), [`subsystems/storage.md`](architecture/subsystems/storage.md) |
| Blob store | S3-compatible (S3, MinIO, GCS, R2) | Source PDFs, OCR outputs, full prompts/responses are kilobyte-to-megabyte payloads ill-suited to Postgres; S3 API is universal | [`subsystems/storage.md`](architecture/subsystems/storage.md) |
| Primary wire protocol | MCP | Universal agent tool protocol; both Anthropic and Moody's ship finance agents through it; zero-code integration for MCP hosts | [ADR-0007](architecture/adrs/0007-mcp-as-primary-adapter.md), [`adapters/mcp.md`](architecture/adapters/mcp.md) |
| Numeric type | `shopspring/decimal` | Float arithmetic on monetary values is a defect, not a tradeoff; banker's rounding at canonical 9-decimal precision | [`subsystems/kernel.md`](architecture/subsystems/kernel.md) |
| Audit chain hash | SHA-256 over canonicalized JSON | Industry-standard; tamper-evident; cheap to verify | [`subsystems/audit.md`](architecture/subsystems/audit.md) |
| Output enforcement pattern | Pre-LLM input guards + post-LLM validator pipeline + token grounding at single `submit_answer` exit | [Arthur AI](architecture/references.md#arthur-guardrails) pre/post split; [DCCD](architecture/references.md#dccd) draft-then-enforce; [Faramesh](architecture/references.md#faramesh) non-bypassable enforcement | [ADR-0002](architecture/adrs/0002-submit-answer-as-single-exit.md), [ADR-0003](architecture/adrs/0003-fail-closed-default.md), [ADR-0004](architecture/adrs/0004-token-level-grounding.md), [`subsystems/guards.md`](architecture/subsystems/guards.md) |
| Determinism stance | Decision determinism and faithfulness as independent metrics; temperature 0 on compliance paths | [DFAH](architecture/references.md#dfah) | [ADR-0005](architecture/adrs/0005-determinism-faithfulness-independent.md) |
| Workflow unit | Signed skill pack (prompts, tools required, guards config, eval thresholds) | [Moody's modular AI](architecture/references.md#moodys-modular) pattern; supply-chain story via signing | [ADR-0006](architecture/adrs/0006-skill-pack-format.md), [`subsystems/skills.md`](architecture/subsystems/skills.md) |
| Migrations | atlas; forward-only | Audit chain consistency over time requires no backwards mutations | [`subsystems/storage.md`](architecture/subsystems/storage.md) |
| Postgres binding | `pgx` + `sqlc` | Type-safe, mature, no ORM overhead | [ADR-0001](architecture/adrs/0001-go-as-kernel-language.md) |
| HTTP | `chi` | Idiomatic Go, stdlib-aligned | [ADR-0001](architecture/adrs/0001-go-as-kernel-language.md) |
| Observability | Prometheus metrics + OpenTelemetry traces + JSON logs | Standard cloud-native stack | [`cross-cutting/observability.md`](architecture/cross-cutting/observability.md) |

### 2.1 Open questions on the stack

These are deliberately unresolved here. The hone critic/defender loop will adjudicate.

1. **Is Go the right kernel language given the dual-mode pluggability requirement?** [ADR-0001](architecture/adrs/0001-go-as-kernel-language.md) was decided when "code-level embedding" was explicitly out of scope ("the kernel will not provide a Go-only public API to consumers; the public surface is MCP / HTTP / CLI"). The thesis now elevates code-level embeddability to a first-class requirement. Surfaces to test against Go:
   - CGo/FFI ergonomics for Python and TypeScript hosts: viable, but the cost of marshaling provenance tokens and decimal values across the FFI boundary is non-trivial and may force a sidecar-only strategy for non-Go hosts.
   - Distribution as a library to non-Go hosts: requires either a stable C ABI (substantial engineering), a sidecar with strict latency budget (gives up on true in-process embedding), or a per-language port of the kernel (massive ongoing burden).
   - GC pause behavior under sustained audit-hash workloads at high throughput: not validated; could matter for hot paths.
   - Alternatives to revisit: Rust (better FFI story; harder contributor cohort), or keep Go for the runtime and carve a minimal pure-Go-and-portable kernel core that can also be ported per-language. No decision is taken in this document.

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

3. **MCP-first vs. MCP-only — resolved cycle 4.** ~~[ADR-0007](architecture/adrs/0007-mcp-as-primary-adapter.md) makes MCP primary.~~ Resolved by [ADR-0010](architecture/adrs/0010-mcp-equal-not-primary.md): MCP, HTTP, and CLI are equal first-class adapters; none has architectural primacy. The runtime layer treats MCP as one of N transports. Engineering-sequencing remains in [`implementation-plan.md`](implementation-plan.md) (CLI in Phase 3, MCP and HTTP in Phase 4). Listed here for cross-reference rather than as an open question.

## 3. Module boundary contract

The dual-mode promise from [`thesis.md`](thesis.md) requires a strict separation between what is "pure kernel" and what is "runtime / adapter". This section is the contract.

### 3.1 Kernel public surface (pure library)

**Allowed in the kernel's public API:**

- Function and method signatures whose parameters and return types are: Go primitives, kernel-defined value types (`Token`, `Claim`, `ValidationError`, `AuditEntry`, etc.), `context.Context`, `decimal.Decimal`, `time.Time`, and interfaces defined in the kernel.
- Interfaces for every I/O dependency: `Store` (token + run + audit row persistence), `BlobStore` (large payload persistence), `ModelClient` (LLM invocation), `Clock`, `RandSource`, `AuditSink` (sequence + hash-chain append target), `ToolBackend` (per-tool I/O — SQL ledger, Textract, etc.), and `TransactionalScope` (joint-transaction primitive across `Store` and `AuditSink` — see below).
- **`TransactionalScope` (joint-transaction primitive).** The Go interface block — `WithTx(ctx, fn)` signature, `TransactionalSubstrate()` marker contract, and the versioned `provenant.substrate/<vendor>/<engine>/v<n>` marker grammar — is enumerated in [`subsystems/kernel.md`](architecture/subsystems/kernel.md#transactionalscope-joint-transaction-primitive) as the source of truth. The parallel-block atomicity invariant in [`subsystems/audit.md`](architecture/subsystems/audit.md#parallel-sibling-deterministic-order) ("tokens and `audit_entries` are written in a single transaction at flush time") cannot be honored when `Store` and `AuditSink` are arbitrary separately-injected backends. The kernel public surface therefore declares `TransactionalScope` as a third interface that a host MUST inject alongside `Store` and `AuditSink` whenever it injects either, and that the host MUST back with a single transactional substrate (typically: the same Postgres connection pool backs all three). The kernel calls `TransactionalScope.WithTx(ctx, func(tx) { storeOp(tx); auditOp(tx) })` at parallel-block flush, at `submit_answer` persistence, and at orphan-recovery; if `WithTx` returns separate transactions per call or commits separately, the kernel detects the violation at runtime via a per-tx canary write (`audit_entries` and `tokens` written in the same tx must share a tx-local sequence the kernel checks before commit) and fails closed with `kernel_integrity`. A `NullTransactionalScope` provided in `kernel/internal/memstore/` is acceptable only for the in-memory test sink that backs both `Store` and `AuditSink` from the same map. Two-backend hosts that inject a Postgres `Store` and an S3 `AuditSink` are rejected at kernel boot: the kernel refuses to start if the host injects `Store` and `AuditSink` whose `TransactionalSubstrate()` markers disagree. This closes the cycle-3 outstanding item: "embedding hosts MUST use a shared transactional substrate" is now a typed contract, not documentation.
- Pure functions: hash construction, schema validation, token derivation, numeric extraction, validator pipeline composition.

**Forbidden in the kernel's public API surface:**

- Direct imports of `net/*`, `net/http`, `database/sql`, `github.com/jackc/pgx`, MCP SDK types, `os` (beyond minimal use in test helpers), `os/exec`.
- Process lifecycle: no `main`, no signal handling, no daemon supervision.
- Transport types: no HTTP handlers, no MCP request/response shapes, no CLI flag definitions.
- Goroutine-spawning helpers that escape function scope without an explicit `context.Context` and an injected cancel mechanism.

**Phase 0 of the implementation plan** is the explicit carve-out of this surface. A linter or `go vet` check enforces the forbidden-import list against `kernel/...`.

### 3.2 Runtime layer

The runtime layer is the only place where I/O is allowed to be implementation-specific.

- Owns: Postgres connection pool (concrete `pgx` implementation of the kernel's `Store` interface), S3 client (concrete `BlobStore`), Anthropic/OpenAI/local-LLM HTTP clients (concrete `ModelClient`), the MCP server (using the official Go MCP SDK), the HTTP API server (`chi`), the CLI entry points (`cobra` or equivalent), process lifecycle, configuration loading, telemetry export.
- Forbidden: any enforcement logic. The runtime layer must not, for example, decide whether a `submit_answer` payload passes — it must call the kernel's pipeline. The runtime layer is allowed to translate kernel errors into HTTP status codes or MCP error shapes; it is not allowed to suppress, invent, or reinterpret enforcement decisions.

### 3.3 Adapter shims

Adapter shims are translation layers between a host framework's idioms and runtime APIs. They are thin by design.

- MCP adapter ([`adapters/mcp.md`](architecture/adapters/mcp.md)): registers Provenant tools as MCP tools, skill prompts as MCP prompts, audit/case data as MCP resources.
- HTTP adapter ([`adapters/http.md`](architecture/adapters/http.md)): REST surfacing of the same operations.
- CLI adapter ([`adapters/cli.md`](architecture/adapters/cli.md)): full Mode A harness; the only adapter that drives the agent loop in-process.
- LangChain adapter ([`adapters/langchain.md`](architecture/adapters/langchain.md)) and OpenAI Agents adapter ([`adapters/openai.md`](architecture/adapters/openai.md)): deferred to v0.5; documented to establish the integration slot.

### 3.4 Bindings (non-Go embedding)

Not present in the current `architecture/` tree. Flagged as part of the top-level-structure open question (§2.1.2). The dual-mode promise requires that at least one non-Go binding exist by Phase 5 of the implementation plan. Two viable shapes:

- **Sidecar protocol with native client.** A Python or TypeScript client speaks a minimal, latency-tight wire protocol (a subset of HTTP or MCP) to a local Provenant runtime process. Pros: zero FFI complexity; cons: process-boundary overhead, slight loss of in-process embedding ergonomics.
- **CGo / FFI binding.** The kernel exposes a stable C ABI subset; Python and TypeScript bindings wrap it. Pros: true in-process embedding; cons: substantial engineering, GC interactions to validate.

Phase 5 will pick one (the critic loop and prototype work decide).

## 4. Data model summary

The full schema lives in [`subsystems/storage.md`](architecture/subsystems/storage.md). The invariant-bearing tables are:

- **`runs`** — one row per agent execution; status transitions audited.
- **`tokens`** — provenance tokens. Every numeric value in any released answer corresponds to exactly one token, with `source_tool`, `source_path`, `source_origin`, and (for computed values) a `derivation` of parent token IDs. See [`subsystems/kernel.md`](architecture/subsystems/kernel.md). This table is the per-number lineage substrate.
- **`tool_calls`** — every tool invocation, with input and output payloads (pointers to object store for large blobs). Foreign-keyed from `tokens.source_call_id`. Without this, replay is impossible.
- **`audit_entries`** — hash-chained, per-tenant-monotonic. Every event in [`subsystems/audit.md`](architecture/subsystems/audit.md)'s `EntryKind` list writes a row; `prev_hash` and `hash` form the chain. This is the regulator artifact.
- **`guard_runs`** — pre-LLM and post-LLM guard executions, with actions and errors. Referenced by the audit chain.
- **`cases`** and **`case_metrics`** — case bank for replay and the four DFAH-style metrics ([`subsystems/eval.md`](architecture/subsystems/eval.md)).

Cross-cutting invariants in the data model:

- Every consequential row carries `tenant_id`. Cross-tenant references are kernel-rejected at every join point.
- IDs are ULIDs (lexicographically sortable).
- Numeric values use `NUMERIC(38, 9)` as the **canonical comparison/derivation precision** with banker's rounding applied at mint time and audit-logged. The original full-precision value as returned by the tool backend (e.g., 10–12-decimal FX rates from OpenFX, 38-decimal raw extraction confidences) is also persisted on the token in a `source_value_raw TEXT` column alongside the canonicalized `value NUMERIC(38,9)`. The audit chain hashes the canonical value; the `source_value_raw` is retained for provenance answers ("what did the upstream actually return?") and is included in audit packs. Derivation arithmetic (sum, subtract, percent-change) operates on the canonical value; raw-vs-canonical drift is reportable per token. The exact parsing grammar, rounding mode (banker's / `roundTiesToEven` at 1e-9), tie-break (always to even, sign-independent), serialization format (decimal-only, no exponent, 9 zero-padded fractional digits, `-0` normalized to `0`), and the cross-language reference fixture vector are enumerated in [`subsystems/audit.md`](architecture/subsystems/audit.md#numeric-canonicalization-cross-language-replay) so Phase 5 Python bindings produce byte-identical chains.
- The schema is forward-only; rollbacks are reversing migrations, never reversions. This is what allows the audit chain to remain consistent across years.

## 5. Component interactions

### 5.1 Kernel ↔ adapters

Adapters call kernel functions through the public API. They do not know about Postgres, the model client, or the audit chain hash — those are kernel internals or injected interfaces.

- An MCP `tools/call` request arrives at the runtime → runtime authenticates and resolves tenant → runtime calls `kernel.Tools.Invoke(ctx, runID, toolName, payload)` → kernel runs schema validation, dispatches to the registered `ToolBackend`, mints tokens, writes audit entries via `AuditSink`, returns the result → runtime translates to MCP response shape.
- The same flow embedded in a Go host: the host calls `kernel.Tools.Invoke(...)` directly, having injected its own `Store`, `BlobStore`, `ModelClient`, `AuditSink`. No runtime layer involved.

### 5.2 Kernel ↔ tools

Tools register typed input/output schemas plus `mintable_paths` (JSONPaths) with the kernel's tool registry. The registry wraps every tool with audit middleware: every invocation gets a `tool_call_started` and `tool_call_completed` entry, and the kernel mints tokens from the declared paths in the response. See [`subsystems/tools.md`](architecture/subsystems/tools.md) and [`architecture/tools/README.md`](architecture/tools/README.md). Tools cannot construct `Token` values directly — only the kernel can — which preserves the "every token has a kernel-assigned ID and kernel-controlled source" property.

### 5.3 Kernel ↔ storage

The kernel never imports `pgx` or `database/sql`. It depends on a `Store` interface. The runtime layer provides a `pgx`-backed implementation; tests provide an in-memory implementation. This is what allows Phase 0 (kernel builds and tests pass with all I/O mocked) to be the first phase.

### 5.4 Submit pipeline

The submit pipeline is the single output bottleneck. Every adapter, every mode, every host hits the same kernel function: `kernel.SubmitAnswer(ctx, runID, payload)`. The function runs numeric extraction, per-claim token grounding (calling `kernel.VerifyToken`), the configured validator pipeline, optional BoN-MAV, and finally persists or rejects. The algorithm is in [`subsystems/guards.md`](architecture/subsystems/guards.md). The kernel returns either `OK` with the persisted answer or a structured error list. Adapters translate. No adapter is permitted to short-circuit this call.

## 6. Security model

The threat model, multi-tenant isolation guarantees, prompt-injection defense strategy, and credential-hygiene rules live in [`cross-cutting/security.md`](architecture/cross-cutting/security.md). The relevant integrated points for this synthesis view:

- **Tenant isolation is kernel-enforced**, not just storage-enforced. `VerifyToken` independently asserts that the returned token's `tenant_id` equals the verifying run's `tenant_id` and that the token's `run_id` is the verifying run (or, for the cross-run case explicitly enabled by skill config, a run within the same tenant). A `Store` implementation that returns rows from any tenant is caught at the kernel boundary: the kernel does not trust `Store` to filter by tenant. This is enforced via a kernel unit test that uses a deliberately permissive in-memory `Store` (returns all tokens regardless of `tenant_id` argument) and verifies that `VerifyToken` still rejects cross-tenant references. Without this check, the claim that tenant isolation survives storage-layer compromise would be false.
- **Prompt injection defense is architectural**, not promptual. The pre-LLM guard pipeline is a code path the agent cannot route around; see [`subsystems/guards.md`](architecture/subsystems/guards.md#pre-llm-input-guards).
- **Audit tamper-evidence** is the per-tenant hash chain over canonicalized JSON entries; tampering is detectable, not preventable (we do not defend against root on the host).
- **Skill-pack supply chain** is signed-pack-with-admin-approval ([ADR-0006](architecture/adrs/0006-skill-pack-format.md)); v0 ships unsigned local packs only, v0.5 adds the hosted registry with signing.
- **Skill-pack content pinning (v0).** Even without signing, every run records the SHA-256 digest of the loaded skill pack's canonical content in the `run_started` audit entry. The digest covers prompts, guards config, manifest, examples, and templates. `provenant replay` recomputes the digest from the on-disk skill pack and refuses to replay if the digest does not match the recorded digest. This catches silent mutation of `prompts/skill.md` between runs — without it, the deterministic-portion hash chain could be identical across two runs whose actual agent input differed. Skill-pack signing in v0.5 is a stronger version of this same pin (signature instead of digest comparison). The full digest procedure — file enumeration rules, the default exclusion list (`.DS_Store`, `Thumbs.db`, `.git/`, `.gitignore`, `.gitattributes`, `*.swp`/`*.swo`/`*~`/`.#*`/`#*#`), the symlink-rejection rule, the POSIX-path normalization, the length-prefixed concatenation, and the verbatim (no trailing-newline normalization, no line-ending conversion) per-file byte rule — is enumerated in [`subsystems/audit.md`](architecture/subsystems/audit.md#skill-pack-content-digest-procedure). The procedure is locked; changes require a kernel version bump.

For the full threat list and mitigation table, read the cross-cutting doc.

## 7. Deployment model

Both pluggability modes ship as deployment shapes from day one. Detail lives in [`cross-cutting/deployment.md`](architecture/cross-cutting/deployment.md); the integrated picture:

### 7.1 Embedded library deploys (code-level pluggability)

The host application is responsible for the process. It links the Provenant kernel (Go module import; or, post-Phase 5, a non-Go binding) and provides its own implementations of the kernel's injected interfaces — or uses the kernel's default-provided `pgx` Store and S3 BlobStore via convenience constructors that live in the runtime layer.

- Lifecycle: host owns it. Kernel exposes a `Shutdown(ctx)` for graceful teardown.
- State: still in Postgres + object store. The kernel does not bundle them — embedding does not eliminate the storage layer; it eliminates the wire protocol.
- Multi-tenancy: the host passes `tenant_id` per call; the kernel enforces.
- Audit chain: identical to runtime-mode chains for the same input.

### 7.2 Runtime-process deploys (runtime-level pluggability)

The Provenant binary runs as a separate process. Four shapes, all supported from v0 except multi-tenant SaaS:

| Shape | Tenants | Process owner | When to use |
|---|---|---|---|
| Local dev | 1 | developer (docker compose) | development |
| Single-tenant production | 1 | customer ops | most v0 customers |
| Air-gapped on-prem | 1+ | customer ops | regulated finance with no egress |
| Multi-tenant SaaS | many | Provenant (us) | v0.5 deferred |

The runtime binary is identical across shapes; what differs is topology, isolation, and operations. See [`cross-cutting/deployment.md`](architecture/cross-cutting/deployment.md) for sizing, ops procedures, DR, and the air-gapped configuration patterns.

### 7.3 Sidecar / MCP-server deploys (host-adjacent runtime)

A special case of runtime-process deployment: the Provenant runtime sits next to the host application (same machine or same Kubernetes pod). The host speaks MCP (or HTTP) to it over loopback or unix socket. This is the recommended shape for non-Go hosts until Phase 5 ships a native binding (and may remain the recommendation indefinitely if the binding work is deferred).

Latency budget for the sidecar pattern: kernel must complete typical `submit_answer` enforcement in under 50ms of overhead on top of validator costs (matches the MCP latency budget in [`adapters/mcp.md`](architecture/adapters/mcp.md)). Higher overhead makes the sidecar pattern unviable for interactive workflows and pushes hosts toward FFI.

## 8. What this system explicitly does not do

Adapted and tightened from [`architecture/README.md`](architecture/README.md):

- Does not write the agent's prompts. Skill packs do that, and only within the kernel-immutable header that announces `submit_answer`.
- Does not pick the model. The harness is model-pluggable; v0 verifies one model class (Claude).
- Does not run the user's GL, ERP, sanctions list, FX feed, or extraction service. It wraps them as tools with declared input/output schemas. See [`architecture/tools/`](architecture/tools/).
- Does not provide a hosted SaaS in v0. Single-tenant and air-gapped only; SaaS is v0.5.
- Does not provide a web HITL UI in v0. HITL routing is REST + database-state; the UI is v0.5.
- Does not provide a hosted skill registry in v0. Skill packs ship as local files; registry with signing is v0.5+.
- Does not provide BoN-MAV mode in v0. The validator pipeline supports it ([`subsystems/guards.md`](architecture/subsystems/guards.md#optional-bon-mav-mode)); the runtime is wired but the verifier-selection mode ships v0.5+.
- Does not provide a LangChain or OpenAI Agents adapter in v0. The docs ([`adapters/langchain.md`](architecture/adapters/langchain.md), [`adapters/openai.md`](architecture/adapters/openai.md)) establish the slot; implementation is v0.5.
- Does not solve framework-internal correctness issues. If the host framework has its own bugs, Provenant cannot fix them; it can only enforce its own contract at the kernel boundary.
- Does not promise complete defense against a maliciously-compromised host (root access on the server). Tamper *detection* is provided; tamper *prevention* against root-equivalent actors is out of scope.

## 9. Open architectural questions

Aggregated for the critic/defender loop. Three are first-class and named above; this section lists them once.

1. **Kernel language choice under dual-mode pluggability.** §2.1.1. Revisits [ADR-0001](architecture/adrs/0001-go-as-kernel-language.md) given the new requirement that the kernel be embeddable into non-Go hosts.
2. **Top-level architecture/ structure under dual-mode pluggability.** §2.1.2. Proposes a possible `kernel/` + `runtime/` + `adapters/` + `bindings/` reshape; deliberately unresolved.
3. ~~**MCP-first vs. MCP-equal.** §2.1.3.~~ **Resolved cycle 4** by [ADR-0010](architecture/adrs/0010-mcp-equal-not-primary.md): MCP, HTTP, and CLI are equal first-class adapters.

Per-subsystem open questions live in the "Open questions" sections of each [`subsystems/<name>.md`](architecture/subsystems/). They are not duplicated here.

## 10. Where to read next

- For the rationale behind every load-bearing decision: [`architecture/adrs/`](architecture/adrs/).
- For per-subsystem detail: [`architecture/subsystems/`](architecture/subsystems/).
- For the wire protocol and adapter contracts: [`architecture/adapters/`](architecture/adapters/).
- For the tool conformance contract: [`architecture/tools/README.md`](architecture/tools/README.md).
- For the security threat model: [`architecture/cross-cutting/security.md`](architecture/cross-cutting/security.md).
- For the observability story: [`architecture/cross-cutting/observability.md`](architecture/cross-cutting/observability.md).
- For deployment shapes and runbooks: [`architecture/cross-cutting/deployment.md`](architecture/cross-cutting/deployment.md).
- For the phased build plan: [`implementation-plan.md`](implementation-plan.md).
