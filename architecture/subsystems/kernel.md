# Subsystem: Kernel (Provenance Engine)

Status: **v0 / load-bearing**

> The kernel is the provenance engine. It owns tokens — the unit of currency that wraps every numeric value an agent encounters — and it enforces that no number leaves the system without traceable lineage. Nothing else in Provenant works without this.

## Depends on
- [`subsystems/storage.md`](storage.md) — Postgres + object store
- [`subsystems/audit.md`](audit.md) — append-only log for token mint events

## Depended on by
- [`subsystems/tools.md`](tools.md) — every tool mints tokens through the kernel
- [`subsystems/guards.md`](guards.md) — `submit_answer` verifies tokens against the kernel store
- [`subsystems/eval.md`](eval.md) — faithfulness metric reads token lineage
- All adapters

## Responsibilities

1. Mint provenance tokens from tool outputs.
2. Persist tokens with their full lineage (source tool call, derivation if any).
3. Verify that numeric claims in agent output match real tokens.
4. Compute derivation chains across multiple tokens.
5. Enforce non-bypassability and fail-closed behavior at the token layer.

## Token data model

```go
package provenance

import (
    "time"
    "github.com/shopspring/decimal"
)

// Token is the unit of provenance currency. Every numeric value
// an agent can reference in its final output must correspond to
// exactly one token, with verifiable lineage to a tool call.
type Token struct {
    ID         string           `json:"id"`              // ULID, unique across all tenants
    RunID      string           `json:"run_id"`
    TenantID   string           `json:"tenant_id"`
    Value      decimal.Decimal  `json:"value"`           // never float64
    Currency   string           `json:"currency,omitempty"` // ISO 4217
    Unit       string           `json:"unit,omitempty"`     // for non-currency: "shares", "%", "count", "bps"
    AsOf       time.Time        `json:"as_of"`           // point-in-time validity
    Source     Source           `json:"source"`
    Derivation *Derivation      `json:"derivation,omitempty"`
    CreatedAt  time.Time        `json:"created_at"`
}

// Source identifies where the value originated.
type Source struct {
    ToolCallID string `json:"tool_call_id"` // ULID of the kernel-logged tool invocation
    Tool       string `json:"tool"`         // canonical tool name, e.g., "ledger.get_balance"
    Version    string `json:"version"`      // pinned tool version
    Path       string `json:"path"`         // JSONPath into the tool's output payload
    Origin     string `json:"origin"`       // upstream physical source, e.g., "s3://docs/inv-12345.pdf#page=2"
}

// Derivation records the computation that produced this token from
// other tokens. Tools that compute (calc.*, reconcile.*) populate this.
type Derivation struct {
    Operation string   `json:"operation"`         // e.g., "subtract", "sum", "ratio"
    Inputs    []string `json:"inputs"`            // parent token IDs (ordered)
    FormulaID string   `json:"formula_id"`        // registered formula reference
    Note      string   `json:"note,omitempty"`
}
```

### Why every field exists

- `ID`: enables citation by reference, not by reproduction. The agent emits `tk_01HXYZ...` in its claims list; the answer text shows the human-readable number.
- `RunID` and `TenantID`: enforce isolation. Tokens cannot be referenced from a different run or tenant. See [cross-cutting/security.md](../cross-cutting/security.md).
- `Value` as `decimal.Decimal`: floating-point arithmetic on monetary values is a defect, not a tradeoff.
- `Currency` / `Unit`: prevents adding USD to EUR or shares to dollars at the validator layer.
- `AsOf`: financial data is point-in-time. A balance "as of 2026-05-15" is different from "as of 2026-05-16."
- `Source.Path` and `Source.Origin`: the audit story. A regulator can follow a single number from the final answer back to the physical document and page.
- `Derivation`: makes computed numbers as auditable as raw ones. A "variance of $47,322.18" is itself a token, with parents.

## Token lifecycle

```
   ┌────────────┐         ┌────────────┐         ┌────────────┐
   │  Tool Call │ ──────▶ │   Token    │ ──────▶ │  Verified  │
   │   (mint)   │         │ (in store) │         │ at submit  │
   └────────────┘         └────────────┘         └────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
   audit log                token store           audit log
   (tool call)              (Postgres)            (submit event)
```

### 1. Mint

Triggered by a tool returning a structured result. The tool registry calls `provenance.Mint(ctx, runID, tenantID, ToolCallID, output)` which:

1. Walks the output payload according to the tool's declared `mintable_paths` (a JSONPath list in the tool schema).
2. For each path, extracts the numeric value, infers currency/unit from sibling fields, infers `as_of` from the tool's response or call timestamp.
3. Creates a `Token`, persists it to Postgres, writes an `EntryKindTokenMint` audit entry.
4. Returns the token IDs back to the registry, which embeds them in the tool's response to the agent.

Mint is the *only* way a non-derived token comes into existence. Tools cannot construct `Token` values directly; they declare paths and the kernel does the wrapping. This preserves the "every token has kernel-assigned ID, kernel-controlled source" property.

### 2. Propagate

Tokens travel through the agent's tool-call cycle as part of the tool response payload. The agent sees them as JSON, can reference them by ID, can pass them as inputs to subsequent tools (especially `calc.*` and `reconcile.*`).

### 3. Derive

When a tool computes a new value from existing tokens (e.g., `calc.subtract(tk_A, tk_B)`), the tool returns a derived token whose `Derivation` field records the operation and parents. The kernel verifies:

- Every input token ID exists and belongs to the same run.
- The operation is a registered formula (`registry.formulas["arith.subtract"]`).
- The computed value matches the formula's deterministic output on the input values.

Mismatches fail-closed: the tool call is rejected before the response reaches the agent.

### 4. Verify (at `submit_answer`)

When the agent submits its final answer, the kernel walks the claims list and:

1. Parses every numeric value out of the answer text.
2. For each numeric value, finds a claim that lists the token IDs supporting it.
3. Looks up each token in the run's token store.
4. Verifies the token's `Value` matches the textual number within the configured tolerance.
5. Verifies the token's `Currency`/`Unit` matches the textual unit if specified.

Failure is fail-closed. The agent receives a structured error and must retry. See [`subsystems/guards.md`](guards.md) for the full algorithm.

## Token store

**Schema (Postgres):**

```sql
CREATE TABLE tokens (
    id              TEXT PRIMARY KEY,             -- ULID
    run_id          TEXT NOT NULL REFERENCES runs(id),
    tenant_id       TEXT NOT NULL,
    value           NUMERIC(38, 9) NOT NULL,
    currency        TEXT,
    unit            TEXT,
    as_of           TIMESTAMPTZ NOT NULL,
    source_tool     TEXT NOT NULL,
    source_version  TEXT NOT NULL,
    source_path     TEXT NOT NULL,
    source_origin   TEXT NOT NULL,
    source_call_id  TEXT NOT NULL REFERENCES tool_calls(id),
    derivation      JSONB,                        -- null for non-derived
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tokens_run ON tokens(run_id);
CREATE INDEX idx_tokens_tenant ON tokens(tenant_id);
CREATE INDEX idx_tokens_source_call ON tokens(source_call_id);
```

**In-memory cache.** During a run, tokens are also held in an LRU keyed by `(run_id, token_id)`. The cache is invalidated at run completion. This is for verify-time performance — Postgres remains the source of truth.

**Numeric precision.** `NUMERIC(38, 9)` is the canonical storage format. Tools may carry higher precision internally; the kernel rounds to 9 decimal places at mint time using banker's rounding, with the rounding event audit-logged. Tolerance configuration for verify is separate (see Guards).

## Verification algorithm

See [`subsystems/guards.md`](guards.md) §`submit_answer`. The kernel's role in verify is the per-token check:

```go
func (k *Kernel) VerifyToken(ctx context.Context, runID, tokenID string, claimed Numeric) error {
    tok, err := k.store.Load(ctx, runID, tokenID)
    if err != nil { return ErrTokenNotFound }
    if tok.RunID != runID { return ErrCrossRunReference }
    if !valuesMatch(tok, claimed, k.tolerancePolicy(tok)) {
        return ErrValueMismatch
    }
    if claimed.Currency != "" && claimed.Currency != tok.Currency {
        return ErrCurrencyMismatch
    }
    return nil
}
```

Tolerance policy by unit family:

| Unit family | Tolerance |
|---|---|
| Integer counts (`count`, `shares` whole) | 0 — strict equality |
| Monetary | Currency minimum unit (e.g., 0.01 USD) |
| Percentage / ratio | Configurable per skill, default 0.0001 |
| Fractional shares / units | Configurable per tool |

## Derivation engine

Registered formulas live in `internal/provenance/formulas`. Each formula is a Go function with a typed signature.

```go
type Formula struct {
    ID          string
    InputUnits  []UnitKind  // allowed unit kinds for each input
    OutputUnit  UnitKind
    Compute     func(inputs []decimal.Decimal) (decimal.Decimal, error)
}

var Subtract = Formula{
    ID:         "arith.subtract",
    InputUnits: []UnitKind{Compatible, Compatible},   // two operands, same unit
    OutputUnit: SameAsInput,
    Compute:    func(in []decimal.Decimal) (decimal.Decimal, error) {
        return in[0].Sub(in[1]), nil
    },
}
```

The registry blocks unit-incompatible derivations: you cannot subtract a USD token from an EUR token without going through a currency conversion formula that has FX rate tokens as input.

## Kernel public types

The kernel's public Go types are pinned here as the source of truth. Independent storage adapters and Phase 5 bindings project these into their concrete persistence shapes per the projection rules below.

### `AuditEntry` (in-memory shape) and three-column persistence projection

```go
package kernelpublic

import (
    "encoding/json"
    "time"
)

// AuditEntry is the in-memory kernel-public shape for a single chain row.
// The kernel writes AuditEntry values; AuditSink adapters project them onto
// the three-column persistence shape declared in subsystems/storage.md.
type AuditEntry struct {
    Seq      uint64          // monotonic per tenant; assigned by AuditSink at flush
    Kind     EntryKind
    At       time.Time       // wall clock; NOT hashed (non-deterministic)
    PrevHash []byte          // sha256 of the previous entry's Hash; genesis is sha256("provenant-genesis:" || tenant_id)
    Hash     []byte          // sha256 of the hash-input bytes (see HashInputCanonical)
    Payload  PayloadEnvelope
}

// PayloadEnvelope splits every audit payload into a deterministic sub-object
// (hashed into the chain) and a non-deterministic sub-object (stored verbatim
// but excluded from the hash input). HashInputCanonical is the JCS-canonical
// encoding of Deterministic alone, kept on the in-memory value for debug-only
// purposes (recomputable from Deterministic; kernel asserts equality on every
// write as a tamper-resistance canary).
type PayloadEnvelope struct {
    Deterministic     json.RawMessage // JCS-canonical or raw; the kernel canonicalizes before hashing
    NonDeterministic  json.RawMessage // free-form; never hashed
    HashInputCanonical []byte         // JCS(Deterministic), verbatim; debug-only, NOT persisted in pack files
}
```

**Projection rule (AuditEntry → three-column persistence).** `AuditSink` adapters MUST project an `AuditEntry` onto the `audit_entries` row defined in [`storage.md`](storage.md#schema-deltas-required-by-cycles-35-load-bearing-columns) exactly as:

| Persisted column | Source | Rule |
|---|---|---|
| `seq` | `AuditEntry.Seq` | assigned by sink at flush; not provided by kernel |
| `kind` | `AuditEntry.Kind` | string form of the enum |
| `at` | `AuditEntry.At` | non-deterministic; wall clock |
| `prev_hash` | `AuditEntry.PrevHash` | verbatim bytes |
| `hash` | `AuditEntry.Hash` | verbatim bytes |
| `payload_deterministic` | `AuditEntry.Payload.Deterministic` | verbatim canonical JSON |
| `payload_non_deterministic` | `AuditEntry.Payload.NonDeterministic` | verbatim JSON |
| `hash_input_canonical` | `AuditEntry.Payload.HashInputCanonical` | **equals `JCS(payload_deterministic)` verbatim**; the kernel computes it once and the sink persists it as a tamper-resistance canary so chain verification can re-derive the hash without re-canonicalizing |

**Hash construction (re-pin, authoritative cross-reference).** `AuditEntry.Hash` is computed by the kernel — not the sink — as `sha256(prev_hash || seq_bytes || run_id || tenant_id || kind || hash_input_canonical)`, per [`audit.md`](audit.md#hash-construction). `hash_input_canonical = JCS(Deterministic)`; the sink MUST NOT re-canonicalize on persistence (re-canonicalization risks a JCS-library-version divergence that silently breaks chain verification across deployments).

**Round-trip property.** An `AuditEntry` written by the kernel, projected onto the three-column row, read back, and reconstituted into an `AuditEntry` MUST be byte-identical to the original on every field. Phase 0's done-when adds a fixture-based projection round-trip test covering every `EntryKind` listed in [`audit.md`](audit.md#entry-kinds).

### `Normalizer.Lookup` accessor

The kernel's compile-time normalizer registry (see [`skills.md`](skills.md#registration-mechanism-pinned)) exposes a single public accessor whose signature is pinned uniformly with the rest of the kernel public surface, which uses `(value, error)`:

```go
package kernelpublic

// Lookup returns the registered Normalizer for the given builtin_id, or a
// typed error if the ID is not registered. The error class for an unknown
// ID is skill.normalizer_id_unknown.
func (r *NormalizerRegistry) Lookup(builtinID string) (Normalizer, error)
```

A missing builtin ID returns `(nil, &SkillError{Class: "skill.normalizer_id_unknown", BuiltinID: builtinID})`. The `(Normalizer, bool)` form previously discussed in [`skills.md`](skills.md) is **superseded** by this signature; the convention split is resolved in favor of `error` to match every other kernel public accessor (Store, AuditSink, TransactionalScope methods). [`skills.md`](skills.md) is updated in tandem.

## TransactionalScope (joint-transaction primitive)

The parallel-block atomicity invariant in [`audit.md`](audit.md#parallel-sibling-deterministic-order) ("tokens and `audit_entries` are written in a single transaction at flush time") cannot be honored when `Store` and `AuditSink` are arbitrary separately-injected backends. The kernel public surface therefore declares a third I/O interface — `TransactionalScope` — that hosts MUST inject alongside `Store` and `AuditSink`. The interface is enumerated here as the source of truth; [`../../architecture.md`](../../architecture.md) §3.1 references this definition.

```go
package kernelio

import "context"

// Tx is an opaque per-transaction handle. Concrete tx types (e.g. pgx.Tx) live
// in the runtime layer. Store and AuditSink implementations down-cast as needed
// inside their own packages; the kernel never inspects the concrete type.
type Tx interface{}

// TransactionalScope is the joint-transaction primitive. Store ops and AuditSink
// ops invoked inside the same WithTx callback MUST share the underlying database
// transaction. A host that backs Store with one substrate and AuditSink with
// another is rejected at kernel boot via the TransactionalSubstrate() check below.
type TransactionalScope interface {
    // WithTx runs fn inside a single transaction. The kernel calls Store and
    // AuditSink methods with the provided tx; both halves commit or roll back
    // atomically. If fn returns a non-nil error, the tx is rolled back.
    //
    // Implementations MUST NOT internally split the work across multiple
    // database transactions. The kernel enforces this at runtime via a
    // tx-local canary write (see "boot-time and runtime enforcement" below).
    WithTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error

    // TransactionalSubstrate returns an opaque marker identifying the
    // database / connection-pool / engine backing this scope. The marker is
    // a versioned, vendor-qualified string in the form
    //   "provenant.substrate/<vendor>/<engine>/v<n>"
    // for example:
    //   "provenant.substrate/postgres/pgxpool/v1"
    //   "provenant.substrate/memstore/sharedmap/v1"
    // String equality (byte-for-byte) is the comparison semantics: the kernel
    // does not parse the marker. The `/v<n>` suffix is a hard version: any
    // breaking change to the substrate's transactional semantics requires a
    // bump to v<n+1> so existing chains do not silently mix incompatible
    // backends across deployments.
    TransactionalSubstrate() string
}

// Store and AuditSink each carry a matching marker so the kernel can verify
// the three injected I/O interfaces share one substrate.
type Store interface {
    TransactionalSubstrate() string
    // ... domain methods ...
}

type AuditSink interface {
    TransactionalSubstrate() string
    // ... domain methods ...
}
```

**Marker contract.** `TransactionalSubstrate()` MUST return the exact byte-identical string from every method invocation for the lifetime of the process. The kernel reads the marker once at boot from `Store`, `AuditSink`, and `TransactionalScope`; if any two disagree, the kernel refuses to start with a typed `kernel_integrity` error (`error_code = transactional_substrate_mismatch`). This is the static half of the check.

**Boot-time and runtime enforcement.**

1. *Boot.* On kernel construction the kernel asserts `Store.TransactionalSubstrate() == AuditSink.TransactionalSubstrate() == TransactionalScope.TransactionalSubstrate()`. Disagreement → `kernel_integrity`, refuse to boot.
2. *Runtime.* At every `TransactionalScope.WithTx` callback boundary, the kernel writes a tx-local canary row to a kernel-internal table (`kernel_tx_canary`, schema-defined in [`storage.md`](storage.md)) using the Store half of the injected pair, and immediately reads it back using the AuditSink half. If the read returns no row, the two halves are not sharing a transaction; the kernel fails closed with `kernel_integrity` (`error_code = transactional_scope_split`) before any audit-entry or token row is written. This catches a TransactionalScope implementation whose `WithTx` opens two separate transactions per call despite returning matched markers.
3. *Test substrate.* `kernel/internal/memstore/` provides a `NullTransactionalScope` whose marker is `"provenant.substrate/memstore/sharedmap/v1"` and that backs Store and AuditSink with the same in-memory map. This is the only acceptable shared-substrate test scope; two separately-constructed memstores are not.

Phase 0's done-when (see [`../../implementation-plan.md`](../../implementation-plan.md#phase-0--establish-the-kernel-module-boundary)) ratifies the boot-time and tx-local-canary tests against this contract.

## Concurrency model

- Mint operations are independently safe and can run concurrently. Postgres handles ID uniqueness.
- A single run is generally single-agent and not concurrent at the agent layer, but tool calls *within* a run may execute in parallel; the kernel uses a per-run mutex only for the audit log append (see [audit.md](audit.md)).
- Across runs, no shared state exists in the kernel beyond the Postgres connection pool.

## Failure modes

| Failure | Behavior |
|---|---|
| Tool returns malformed output (schema violation) | Tool registry rejects; no mint; error to agent. Audit-logged. |
| Mint targets a value that isn't numeric at the declared path | Mint skips that path with a warning; other paths still mint. |
| Derivation inputs don't exist | Fail-closed at the calc tool; error to agent. |
| Derivation result doesn't match the formula's deterministic output | Fail-closed; treated as a kernel-integrity event, escalated. |
| Postgres write fails on mint | Tool call is rolled back; agent receives `tool_unavailable`. |

## Tests

- **Unit:** every formula has property-based tests (commutativity, associativity where applicable, identity, edge cases at currency precision boundaries).
- **Integration:** end-to-end mint → propagate → verify against a reconciliation fixture.
- **Property:** Token IDs are unique across 10M synthetic mints; cross-run reference is never silently accepted.
- **Fuzz:** Numeric parser at `submit_answer` is fuzzed against adversarial answer strings.

## Open questions

1. **Should derivation chains have a depth limit?** Pathological cases (a chain of 100 sums) inflate audit logs. Proposal: soft limit at 32, configurable.
2. **Token expiry within a long-running run.** A `tk_balance_as_of_2026_05_15` minted at the start of a 4-hour run is still "fresh" by token model. Does that match the customer's intent? Proposal: tokens carry `as_of`, not `expires_at`; staleness is a per-skill validator concern.
3. **Cross-skill token sharing.** Tokens from a recon run referenced in a downstream variance commentary run — allowed? Proposal: yes within tenant, within a configurable window. ADR to follow if we ship it in v0.

## References

- [Faramesh](../references.md#faramesh) — non-bypassable, fail-closed enforcement at the architectural boundary
- [DFAH](../references.md#dfah) — schema-first determinism, replay requirements
- [Moody's modular AI](../references.md#moodys-modular) — pattern of grounded outputs with citable lineage
