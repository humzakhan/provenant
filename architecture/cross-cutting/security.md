# Cross-cutting: Security

Status: **v0 / load-bearing**

> Provenant runs in regulated environments. Security is a kernel concern, not a deployment add-on. This document defines the threat model, the mitigation strategy for each threat, and the multi-tenant isolation guarantees. Specific implementation details live in the subsystem docs; this is the integrated view.

## Threat model

Threats Provenant defends against:

| Threat | Source | Mitigation |
|---|---|---|
| Prompt injection via retrieved documents | Untrusted documents (extract outputs, policy retrievals, external API responses) | Pre-LLM input guards quarantine injection patterns; documents flow through guards before any model call |
| Prompt injection via tool responses | Compromised upstream systems (ledger, sanctions feed) | Same pre-LLM guards apply to every tool response; tools are not exempt |
| Hallucinated numeric claims | Model error, model drift | Token grounding at `submit_answer`; ungrounded numbers fail-closed |
| Hallucinated policy citations | Model error | `policy.citation_exists` validator; unresolvable anchors fail-closed |
| Cross-tenant token references | Bugs or attempted exploitation | Token verification checks `tenant_id` and `run_id` match |
| Audit log tampering | Insider access, compromised host | Hash-chained log; periodic verifier task; tamper detection alerts |
| Replay attacks (re-submitting old payloads) | Adversarial actors | Idempotency on the API layer; request signing |
| Token exfiltration via output | Adversarial agent prompt | The agent only sees token IDs in its context; the kernel rejects cross-run references |
| Backend credential leak | Compromised host, accidental log | Credentials in tenant-scoped secret store; no credential in audit log or model context |
| Data egress to model APIs | Tool responses fed into model context contain PII | PII redaction guard; per-skill configurable severity |
| Skill pack supply-chain attack | Malicious skill author | Signed packs (v0.5+ with registry); pack hash in audit; admin approval workflow |
| Denial of service | Adversarial input causing expensive computation | Per-tool timeouts; per-tenant rate limits; cost cap per run |

Threats explicitly **out of scope**:

- A maliciously-compromised host environment (root access on the server). We trust the OS and the operator; we provide tamper *detection*, not prevention against root-equivalent actors.
- Side-channel attacks on the model (e.g., timing attacks against the LLM provider). We trust the LLM API surface.
- Cryptographic attacks on SHA-256 (the audit chain hash). If SHA-256 falls, we upgrade.

## Multi-tenant isolation

Three isolation modes, configurable per deployment. See [`subsystems/storage.md`](../subsystems/storage.md).

| Mode | Boundary | Use case |
|---|---|---|
| Shared schema with RLS | Postgres row-level security | SaaS multi-tenant with similar trust between tenants |
| Schema-per-tenant | Postgres schemas | Enterprise multi-tenant with stricter separation |
| DB-per-tenant | Separate Postgres databases | Regulated tenants with data residency requirements |

Kernel-level enforcement (regardless of mode):

- Every token, tool call, audit entry, run, case carries `tenant_id`.
- Every API request resolves to a tenant via bearer token; the tenant is set in the Postgres session.
- Cross-tenant references in tokens are kernel-rejected at verification time.
- The audit chain is per-tenant; no chain entries span tenants.
- Object store buckets are per-tenant prefixed; bucket policies enforce isolation at the storage layer.

## Prompt injection defense

The pre-LLM guard pipeline (see [`subsystems/guards.md`](../subsystems/guards.md)) is the first line. It applies to:

- The initial user prompt.
- Every retrieved document or chunk added to context (RAG hits, uploads, OCR outputs).
- Every tool response before it returns to the model.

Severity tiers (L1 advisory, L2 quarantine, L3 block) are configured per skill but default to safe choices: injection patterns at L2 (quarantine the passage and continue) or L3 (block the run) for high-stakes skills.

Why pre-LLM is necessary: output validation alone catches *what the agent said* but not *what the agent learned*. A successful injection might cause the agent to issue a perfectly-formatted, validator-passing answer that happens to follow attacker instructions (e.g., approve a fraudulent transaction within thresholds). Pre-LLM guards prevent the steering from happening at all.

Why pre-LLM is insufficient alone: novel injections will pass naive pattern matchers. We compose pre-LLM heuristic guards with post-LLM validators that enforce structural and semantic constraints on the output. The combination raises the cost of successful injection beyond what any single layer can.

## Secret handling

- All upstream credentials (ERP API keys, sanctions feed tokens, model API keys) live in a tenant-scoped secret store. Default backend: Postgres `tenant_secrets` table with `pgcrypto`-encrypted columns; production: HashiCorp Vault or cloud KMS-backed.
- Credentials are never logged. The audit log records *which* credential was used (by identifier) but never the credential value.
- Credentials are never inserted into model context. Tool calls that need credentials use them internally; the credential is not part of the tool's input visible to the agent.

## API security

- Bearer tokens scoped to tenant, skills, and tools.
- Token rotation: tokens expire on a configurable schedule (default 90 days); rotation is a `POST /v1/admin/tokens/rotate` operation that issues a new token and revokes the old after a grace period.
- TLS required for all transports in production deployments.
- Rate limits per token: configurable; default 10 RPS.
- Idempotency keys on mutating endpoints (`POST /v1/runs`) to prevent replay-induced duplicate work.

## Audit log integrity

The audit chain (see [`subsystems/audit.md`](../subsystems/audit.md)) provides tamper detection:

- Each entry's hash includes the previous hash; mutation of any entry invalidates the rest of the chain.
- A scheduled verifier task walks every tenant's chain weekly.
- Manual verification is exposed via `provenant audit verify`.

What the chain does **not** prevent: an attacker with root and database write access can construct a new valid chain from scratch. The mitigation is operational: separation of duties, audit log replication to a separate write-once store, customer's internal audit team's periodic chain verification.

## Backend isolation

Tools that touch external systems (ledger, extract, external) run in-process in v0. The kernel does not sandbox them at the process level.

For v0.5+, [FinVault](../references.md#finvault)-style sandboxing is on the roadmap for write-tools (when we add them — journal entries, write-back to ERP). Read-only tools in v0 are deemed acceptable risk in-process given:

- All credentials are kernel-managed; tools cannot exfiltrate credentials they don't have access to.
- All network egress is logged.
- Tools are written in Go and reviewed; no untrusted plugin loading in v0.

When write-tools enter the picture, the threat surface shifts and sandboxing becomes necessary.

## Compliance posture

Provenant is designed to satisfy:

- **[FINRA 2026 GenAI guidance](../references.md#finra-2026)**: auditability, explainability, supervision. The audit log and case bank are the evidence trail.
- **[SR 11-7](../references.md#sr-11-7)** model risk: documented purpose (skill manifest), validation (eval harness), monitoring (case bank metrics), human override (HITL), retirement (skill versioning).
- **[NYDFS RAISE Act](../references.md#nydfs-raise)**: applicable to frontier AI usage; Provenant's audit story supports compliance.

These are *enabled* by Provenant, not certified. Customers achieve compliance by combining Provenant's evidence with their own governance practices.

## Open questions

1. **Skill pack signing.** v0 has no signature requirement; packs install from local filesystem with implicit trust. v0.5+ registry should require signed packs and verify on install. Codify in an ADR when we ship it.
2. **Secret store backends.** Default Postgres `pgcrypto` is sufficient for many; Vault/KMS-backed is the enterprise path. Should we ship both as first-class? Probably yes; defer the second to v0.5 unless customer demand pulls it forward.
3. **Network egress allowlist.** Should the kernel enforce that tools only reach approved external endpoints? Proposal: yes — a per-tool/per-backend allowlist in config; auditable. Defer to v0.5.

## References

- [Faramesh](../references.md#faramesh) — non-bypassable + fail-closed as security primitives
- [FinVault](../references.md#finvault) — sandboxed execution for future write-tools
- [Arthur AI guardrails](../references.md#arthur-guardrails) — pre/post-LLM split for injection defense
- See [`subsystems/guards.md`](../subsystems/guards.md), [`subsystems/audit.md`](../subsystems/audit.md), [`subsystems/storage.md`](../subsystems/storage.md) for implementation
