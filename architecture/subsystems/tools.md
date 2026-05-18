# Subsystem: Tool Registry

Status: **v0 / load-bearing**

> The tool registry is where deterministic computation lives. Tools are the only mechanism by which numbers enter the agent's world. This document specifies the registry contract; individual tools live under [`../tools/`](../tools/).

## Depends on
- [`subsystems/kernel.md`](kernel.md) — token minting
- [`subsystems/audit.md`](audit.md) — every tool call is logged

## Depended on by
- [`subsystems/harness.md`](harness.md) — tool dispatch
- [`subsystems/guards.md`](guards.md) — validators can read tool outputs and tokens
- All adapters

## Responsibilities

1. Register tools with typed input/output schemas.
2. Validate every call against schemas before dispatch.
3. Wrap every call in audit middleware.
4. Mint provenance tokens from tool outputs per the tool's `mintable_paths`.
5. Enforce decimal safety, replayability, and versioning.
6. Expose tool descriptions to the harness for prompt assembly.

## Tool interface

The kernel-public type name for this interface is `ToolBackend` (matches [`architecture.md`](../OVERVIEW.md) §3.1 and [`implementation-plan.md`](../../plans/implementation-plan.md) Phase 0 public-surface list); the heading uses the prose term "tool" but the Go type is `ToolBackend`.

```go
package tools

import (
    "context"
    "encoding/json"

    "github.com/provenant/provenant/internal/provenance"
)

type ToolBackend interface {
    // Name is the canonical, dotted identifier (e.g., "ledger.get_balance").
    Name() string

    // Version is a semver string. The registry pins versions per skill pack.
    Version() string

    // InputSchema is the JSON schema validated against input before Call.
    InputSchema() jsonschema.Schema

    // OutputSchema is the JSON schema the output must satisfy.
    OutputSchema() jsonschema.Schema

    // MintablePaths declares which paths in the output yield provenance tokens.
    // Each path is a JSONPath; the value at that path becomes a token.
    MintablePaths() []MintablePath

    // DeterminismClass declares whether the tool's outputs are reproducible
    // given the same inputs (pure-deterministic) or require captured external
    // reads to replay (captured-deterministic).
    DeterminismClass() DeterminismClass

    // Credentials returns the logical CredentialRef(s) this backend will use
    // when dispatched. Pure-Go tools (calc.*, reconcile.*) return an empty
    // slice. The returned credential_id values MUST match the static
    // declaration in the skill's tools.yaml entry; a mismatch is a kernel-
    // integrity error (error_code = credential.id_manifest_mismatch — see
    // the canonical error_code table in subsystems/audit.md) at
    // dispatch time. See subsystems/skills.md "credential_id field" and
    // cross-cutting/security.md "Silent credential rotation detection".
    Credentials() []CredentialRef

    // Call executes the tool.
    Call(ctx context.Context, input json.RawMessage) (Result, error)
}

// CredentialRef is a typed accessor that names a logical credential the
// backend will use. The kernel uses CredentialRef.ID to reconcile a runtime
// dispatch with the skill's static tools.yaml manifest; the kernel does NOT
// read CredentialRef.Provider for chain-determinism decisions (provider is
// non-deterministic and lives only on the audit entry's non-deterministic
// sub-object).
type CredentialRef struct {
    ID       string  // logical identifier from tools.yaml (e.g., "ledger.sap_eu")
    Provider string  // secret-store backend, e.g., "vault" or "pg_pgcrypto"; non-deterministic
}

type MintablePath struct {
    Path     string    // JSONPath, e.g., "$.balance"
    UnitFrom string    // sibling path that supplies the currency/unit, optional
    AsOfFrom string    // sibling path that supplies as_of timestamp, optional (defaults to call time)
}

type DeterminismClass int
const (
    // PureDeterministic: same inputs always produce same outputs (e.g., calc.subtract).
    // No external state. Replay does not need captured reads.
    PureDeterministic DeterminismClass = iota

    // CapturedDeterministic: outputs depend on external state (a database, an API),
    // but the kernel captures the read so replay is deterministic.
    CapturedDeterministic

    // NonDeterministic: outputs may vary across calls (LLM-backed tools, etc.).
    // Replayed runs cannot guarantee identical outputs.
    NonDeterministic
)

type Result struct {
    Output json.RawMessage      // must validate against OutputSchema
    Tokens []provenance.Token   // minted by the registry from MintablePaths
    Reads  []CapturedRead       // for CapturedDeterministic tools
}

type CapturedRead struct {
    Source   string             // e.g., "blob://run_01HXYZ/captured/ledger.transactions?account=1010&period=2026-05"
    AsOf     time.Time
    Sha256   string             // hash of the read payload
    BlobRef  string             // bundle / BlobStore reference for the full payload
}
```

### Why each field exists

- **`Name()`**: stable canonical identifier for skill packs to reference, regardless of version.
- **`Version()`**: skills pin tool versions in their manifest; the registry refuses unpinned production runs.
- **`InputSchema` / `OutputSchema`**: schema-first per [DFAH](../references.md#dfah). The registry rejects bad payloads before the tool sees them and rejects bad outputs before the agent sees them.
- **`MintablePaths`**: the registry — not the tool — mints tokens. Tools cannot construct tokens with arbitrary sources; this preserves source integrity at the kernel layer.
- **`DeterminismClass`**: the kernel needs to know whether to capture reads for replay. Pure-deterministic tools (`calc.*`) need no capture; captured-deterministic tools (`ledger.*`, `extract.*`) record their reads as bundle / BlobStore references for replay.
- **`CapturedRead`**: enables bit-identical replay of external state. Without this, [DFAH](../references.md#dfah)-style trajectory determinism is impossible for any tool reading the world.

## Registry lifecycle

### Startup

```
1. Kernel boot
2. Tool registry initializes
3. Built-in tools register themselves (calc.*, reconcile.*)
4. Backend-specific tools register based on configuration (e.g., ledger.get_balance with SQL backend)
5. Schemas validated for self-consistency (no name collisions, no missing schemas)
6. Registry available
```

### Skill pack loading

When a skill pack loads:

1. The pack's `tools.yaml` declares required tools and versions.
2. Registry verifies every required tool is registered and at a compatible version.
3. If any required tool is missing or version-mismatched, skill load fails — the pack will not run.

### Per-call execution

```
agent emits tool_use
        │
        ▼
1. Harness routes to registry.Dispatch(call)
2. Registry looks up tool by name+version
3. Registry validates input against InputSchema
   └─ fail-closed: ValidationError back to agent without tool invocation
4. Registry wraps in audit middleware:
   - logs tool_call_started entry
   - records input payload (sensitive fields redacted per skill config)
5. Tool.Call(ctx, input) executes
6. Registry validates output against OutputSchema
   └─ fail-closed: tool_call_failed entry, error to agent
7. Registry mints tokens from MintablePaths
8. Registry persists CapturedReads (CapturedDeterministic only)
9. Registry logs tool_call_completed entry with token IDs
10. Tools.Result returned to harness
        │
        ▼
harness applies pre-LLM guards to the output, feeds to model
```

## Decimal safety

Tools that handle monetary values **must not** use `float64`. Enforced by:

- `internal/lint`: a static linter check during CI that flags `float64` usage in tool packages.
- The token data model itself uses `decimal.Decimal`; conversion from `float64` is rejected at mint time.

Any tool that violates decimal safety fails its conformance test and is rejected from the registry at startup.

## Tool naming convention

Tools follow `<category>.<verb_or_noun>`:

- `ledger.get_balance`, `ledger.get_transactions`, `ledger.search`
- `extract.bank_statement`, `extract.invoice`, `extract.receipt`
- `reconcile.match`
- `calc.subtract`, `calc.sum`, `calc.percent_change`, `calc.variance`
- `policy.lookup`
- `external.fx_rate`, `external.sanctions_check`

Backends extend a category but do not rename tools. `ledger.get_balance` is the same tool whether backed by SQL, QuickBooks, or NetSuite — the backend is a configuration concern, not a schema concern.

## Versioning

Semver:

- Patch bumps: bug fixes, no schema change.
- Minor bumps: backward-compatible schema additions (new optional fields).
- Major bumps: breaking schema changes.

Skill packs pin tools at minor-version granularity. Patch updates roll forward automatically; minor and major updates require pack republish.

The registry maintains version history. A captured run at tool version 1.3.2 can be replayed against 1.3.2 even after 1.4.0 has shipped.

## Conformance checklist

Every tool must pass:

1. **Schemas published**: input + output JSON schemas committed to source.
2. **Schemas tested**: at least 5 positive and 5 negative test cases per schema.
3. **Determinism declared**: `DeterminismClass()` correct.
4. **Token paths declared**: every numeric output value covered by a `MintablePath` (or explicitly excluded with rationale).
5. **Decimal safety**: no `float64` anywhere in monetary paths.
6. **Replayability test**: for `CapturedDeterministic` tools, captured reads enable bit-identical replay.
7. **Failure mode coverage**: explicit error types for network, auth, data-not-found, schema-violation; each tested.
8. **Documentation**: a `tools/<category>/<name>.md` or `tools/<category>/backends/<backend>.md` file following the [`tools/README.md`](../tools/README.md) template.
9. **Performance budget**: p95 latency documented; the registry enforces a per-call timeout.

See [`tools/README.md`](../tools/README.md) for the full template every tool spec must follow.

## Storage

```sql
CREATE TABLE tool_calls (
    id            TEXT PRIMARY KEY,                -- ULID
    run_id        TEXT NOT NULL REFERENCES runs(id),
    tool          TEXT NOT NULL,
    version       TEXT NOT NULL,
    input         JSONB NOT NULL,
    output        JSONB,
    error         TEXT,
    minted_tokens TEXT[] NOT NULL DEFAULT '{}',    -- token IDs
    started_at    TIMESTAMPTZ NOT NULL,
    ended_at      TIMESTAMPTZ NOT NULL,
    latency_ms    INTEGER NOT NULL
);

CREATE INDEX idx_tool_calls_run ON tool_calls(run_id);
CREATE INDEX idx_tool_calls_tool ON tool_calls(tool);
```

```sql
CREATE TABLE captured_reads (
    id           TEXT PRIMARY KEY,
    tool_call_id TEXT NOT NULL REFERENCES tool_calls(id),
    source       TEXT NOT NULL,
    as_of        TIMESTAMPTZ NOT NULL,
    sha256       TEXT NOT NULL,
    blob_ref     TEXT NOT NULL,                     -- bundle / BlobStore reference
    captured_at  TIMESTAMPTZ NOT NULL
);
```

## Failure modes

| Failure | Behavior |
|---|---|
| Tool not registered | Registry refuses dispatch; agent receives `tool_unknown` |
| Input schema violation | Fail-closed; `tool_invalid_input` to agent |
| Output schema violation | Fail-closed; `tool_invalid_output` to agent; the tool is also flagged for triage (broken contract) |
| Tool panic | Caught; treated as failure; tool marked unhealthy; subsequent calls in same run rejected |
| Network/auth error in backend | Returned as `tool_unavailable`; agent may retry per skill policy |
| Backend authentication failure caused by mid-run credential rotation | Returned as typed `credential.rotated` (`error_class = backend_error`, `error_code = credential.rotated`); the harness fails the run closed with `failure_subcode = credential_rotated` rather than retrying with the post-rotation credential — see [`cross-cutting/security.md`](../cross-cutting/security.md#credential-rotation-mid-run-failure-path-contract) |
| `MintablePath` references a non-existent path | Mint is skipped for that path; warning logged; not fail-closed (tool may legitimately return partial data) |

## Tests

- Unit: every built-in tool has a conformance test exercising the checklist.
- Property: invalid inputs are always rejected; valid inputs always produce schema-conforming outputs.
- Replay: `CapturedDeterministic` tools replayed against captured reads produce identical outputs and identical token IDs.
- Fuzz: input validators fuzzed against malformed payloads.

## Open questions

1. **Tool sandboxing.** Should tools that touch external systems run in process-isolated sandboxes? See [FinVault](../references.md#finvault). Defer to post-v0; flag for security review.
2. **Schema evolution policy.** When a tool's output schema bumps minor, do existing captured reads remain valid? Proposal: yes — readers must tolerate added optional fields. Codify in CI.
3. **Per-tool rate limits.** Should the registry enforce per-tool RPS? Proposal: yes, configurable per backend; default 10 RPS per call site.

## References

- [DFAH](../references.md#dfah) — schema-first determinism, captured reads for replay
- [Faramesh](../references.md#faramesh) — non-bypassable boundary
- [FinVault](../references.md#finvault) — sandboxed execution for write-tools (future)
