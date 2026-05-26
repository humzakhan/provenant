# Contributing to Provenant

Provenant is an open-source enforcement substrate for LLM agents in regulated finance. It is a Go library; read `docs/thesis.md` and `architecture/README.md` before contributing.

---

## Table of contents

1. [Build, test, and run](#build-test-and-run)
2. [Commit conventions](#commit-conventions)
3. [PR and review expectations](#pr-and-review-expectations)
4. [Code conventions and invariants](#code-conventions-and-invariants)

---

## Build, test, and run

**Requirements:** Go 1.22 or later.

```sh
# build all packages
go build ./...

# run all tests
go test ./...

# vet all packages
go vet ./...

# regenerate kernel enum constants after editing the source tables in
# architecture/subsystems/audit.md (see "Generated files" below)
go generate ./kernel/public/...

# confirm no drift between the markdown tables and generated Go
git diff --exit-code
```

No build tags, no external services, and no database or network access are required to build or test the core kernel packages. Packages under `runtime/` may have optional runtime dependencies; their README files note what is needed.

### Generated files

Three files are generated from the canonical tables in `architecture/subsystems/audit.md` by `tools/enumgen/`:

- `kernel/public/errorcode/codes.go` — `error_code` constants
- `kernel/public/audit/entrykind.go` — `EntryKind` constants
- `kernel/public/audit/failuresubcode.go` — `failure_subcode` constants

Do not hand-edit these files. Run `go generate ./kernel/public/...` after modifying the source tables, then verify `git diff --exit-code` is clean before opening a PR.

---

## Commit conventions

All commit subjects and PR titles must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(optional-scope): <summary>
```

Allowed types:

| Type | Use when |
|---|---|
| `feat` | a new feature or capability |
| `fix` | a bug fix |
| `docs` | documentation only |
| `test` | test additions or corrections |
| `refactor` | code restructuring without behavior change |
| `chore` | build, tooling, dependency, or maintenance work |
| `perf` | performance improvement |
| `ci` | CI configuration changes |
| `build` | build system changes |

**Rules:**

- Use a lowercase type.
- Keep the summary concise and specific; it must match the staged diff.
- Do not end the subject with a period.
- Do not use vague subjects like `WIP`, `temp`, `misc`, or `fix stuff`.
- PR titles must follow the same format; prefer a title that would also work as the squash-commit subject.

**Examples:**

```
feat(kernel): add EntryKindAnswerSubmitted audit entry kind
fix(bundle): reject token_derive entries missing Derivation field
docs: add ADR-0012 for normalizer compile-time registration
test(trace): add golden replay for fauxbooks trace root
```

---

## PR and review expectations

### Branch and slice discipline

- Create one branch per task from `main`. Use a prefix that matches the commit type: `feat/*`, `fix/*`, `refactor/*`, `test/*`, `docs/*`, `chore/*`.
- Never commit directly to `main`.
- Keep each branch independently buildable and reviewable. Do not bundle unrelated changes.
- One PR per branch. Rebase rather than merge. Squash-merge to `main` unless an explicit exception is granted.

### What a PR must include

Every PR description must contain the following sections:

```markdown
## Changes
## Validation
## Risks & Follow-ups
## Security Considerations
```

Fill each section honestly. "N/A" is acceptable only when genuinely not applicable.

### Tests

- All PRs that change kernel, runtime, or adapter behavior must include tests.
- Prefer table-driven tests for behavior matrices.
- Use golden tests when output shape or byte-identity matters (audit bundles, canonical payloads, generated HTML).
- Add fuzz targets for new parsers, serialization paths, and external-input handlers.
- Test failure modes and invariant violations, not only happy paths.
- The `go test ./...` command must pass before marking a PR ready for review.

### What reviewers look at first

Reviewers triage in this order. A finding at a higher tier takes precedence:

1. **Deterministic-payload integrity.** New fields in `PayloadEnvelope.Deterministic` require a `kernel_version` bump and updated header fixtures. Wall-clock times, retry counters, and latency values belong in `NonDeterministic`.
2. **TransactionalScope honored.** `Store` and `AuditSink` writes that must be atomic must go through `TransactionalScope.WithTx`. See `architecture/subsystems/kernel.md`.
3. **Fail-closed paths.** No validator panic, timeout, or error may silently return `nil` (allowing the call through). Pass-with-warning is not a valid outcome.
4. **Error codes in the canonical table.** Any new `error_code` must appear in `architecture/subsystems/audit.md` before the code is written.
5. **Kernel public surface cleanliness.** No `net/*`, `database/sql`, `pgx`, or process-lifecycle import under `kernel/`. See `architecture/adrs/0009-kernel-public-go-surface.md`.
6. **Adapter equality.** Enforcement logic belongs in the kernel, not in an adapter. All three access surfaces (MCP, HTTP, CLI) must expose every feature equally. See `architecture/adrs/0010-mcp-equal-not-primary.md`.

Reviewers will not block on: naming bikeshedding, comment density, import ordering, minor performance tweaks that do not affect enforcement semantics, or issues already accepted in `docs/KNOWN_TRADE_OFFS.md`.

### When a new ADR is required

Open a draft ADR (under `architecture/adrs/`) before writing code for any change that:

- Alters the module boundary contract (which packages may import what)
- Adds a new adapter or access surface
- Changes the determinism model or hash construction formula
- Modifies the fail-closed default behavior
- Introduces a new kernel-public Go type that changes the public API surface

Implementation changes that stay inside an existing boundary (a new tool backend, a new guard in the built-in set, a new op in the op catalog) do not require an ADR.

---

## Code conventions and invariants

This section distills the load-bearing technical rules of the codebase. Read it before opening a file. The definitive sources are the subsystem docs and ADRs cited throughout.

### The four non-configurable kernel invariants

These hold everywhere. No configuration or flag disables them.

1. **Non-bypassable.** The only path to a released answer is `kernel.SubmitAnswer`. No adapter, runtime wrapper, or test helper bypasses it. See `docs/thesis.md` and `architecture/adrs/0002-submit-answer-as-single-exit.md`.

2. **Fail-closed.** Every join point defaults to denial on error. A panicking validator is a block. A missing token is a rejection. A `TransactionalScope` that splits transactions is a `kernel_integrity` abort. There is no pass-with-warning mode. See `architecture/adrs/0003-fail-closed-default.md`.

3. **Schema-first.** Every tool input and output is validated against a published schema before dispatch. Unvalidated payloads never reach the kernel's mint path.

4. **Replay-deterministic.** The deterministic sub-object of every `AuditEntry` must be byte-identical across embedded-mode and runtime-mode runs on the same input. Wall-clock timestamps belong in `PayloadEnvelope.NonDeterministic` only. See `architecture/subsystems/audit.md`.

### Kernel boundary

The `kernel/` module is pure Go and must remain I/O-free:

- No imports of `net/*`, `net/http`, `database/sql`, `github.com/jackc/pgx`, any MCP SDK types, `os/exec`, or process-lifecycle packages are permitted anywhere under `kernel/`. A CI import-guard check enforces this. See `architecture/adrs/0009-kernel-public-go-surface.md`.
- The kernel's public in-memory binding is `kernel/public/memio/`. It is the library-mode default and is part of the kernel module.
- Any code that opens a network socket, file handle, or database connection belongs under `runtime/`, not `kernel/`. See `architecture/adrs/0011-storage-is-a-deployment-concern.md`.
- Inject `Clock` and `RandSource` interfaces; do not call `time.Now()`, `rand.Int()`, or `uuid.New()` inside `kernel/` directly.

### TransactionalScope

`Store` and `AuditSink` writes that must be atomic — parallel-block flush, `SubmitAnswer` persistence, orphan recovery — must go through `TransactionalScope.WithTx`. A host that injects a `Store` and `AuditSink` backed by different transactional substrates is rejected at kernel boot. See `architecture/subsystems/kernel.md` §TransactionalScope.

`NullTransactionalScope` is acceptable only inside `kernel/public/memio/`, where `Store` and `AuditSink` are backed by the same in-memory map.

### Determinism rules

- Fields that are byte-identical on replay belong in `PayloadEnvelope.Deterministic`. Fields that are wall-clock-dependent, retry-counter-dependent, or otherwise vary between runs belong in `PayloadEnvelope.NonDeterministic`.
- Any change to the set of fields in `PayloadEnvelope.Deterministic` for any `EntryKind`, any change to the hash construction formula, or any change to `header_v1.go` requires a `kernel_version` bump and updated fixture files under `kernel/testdata/header-fixtures/`.
- Deterministic IDs (`tool_call_id`, `parent_step_id`, `sibling_group_id`) are hash-derived, not fresh ULIDs. See `architecture/subsystems/audit.md` §Tool-call ID and seq determinism.
- Hash input must use JCS (RFC 8785) canonicalization; standard `encoding/json` marshalling does not guarantee key ordering and must not be used on the hash-input path.
- Derived token IDs are a deterministic function of `(operation, ordered input IDs, rounding policy, evaluated value)`. Identical computations always produce identical IDs across runs.

### Input validation

- Decode strictly and fail closed. Reject ambiguous, malformed, or unsupported input rather than coercing silently.
- Bound reads, collection sizes, recursion depth, and payload sizes at the boundary before data reaches deeper logic.
- Prefer allowlists over denylists for accepted values.
- Numeric values use `shopspring/decimal`. Never use `float64` for monetary or quota values.
- Numeric canonical form: plain decimal string, no exponent, 9 zero-padded fractional digits, `-0` normalized to `0`. Normalization of locale-specific forms (`1,5`, `1 234 567,89`) is the responsibility of the tool backend, not the kernel. See `docs/KNOWN_TRADE_OFFS.md` §Locale-specific numeric forms.

### Error codes and enum tables

- Error codes use dotted-namespace ASCII: `credential.*`, `numeric.*`, `skill.*`, `tool.*`, `kernel.*`, `extract.*`, `policy.*`. Bare `snake_case` is reserved for `failure_subcode`.
- Before adding a new `error_code`, add it to the markdown table in `architecture/subsystems/audit.md` §Canonical error_code table. Then run `go generate ./kernel/public/...` and verify `git diff --exit-code` is clean.
- The same rule applies to `EntryKind` and `failure_subcode`: markdown table first, then generator, then code.
- `AuditSink.Append` rejects at write time any entry whose `kind`, `error_code`, or `failure_subcode` is not in the canonical table.

### Adapter equality

MCP, HTTP, and CLI are equal access surfaces. None has architectural primacy. Enforcement logic must not live in an adapter layer; it belongs in the kernel. See `architecture/adrs/0010-mcp-equal-not-primary.md`.

### Adding a tool backend

1. Read `architecture/subsystems/tools.md` and `architecture/tools/README.md` fully.
2. Implement the `ToolBackend` interface in `tools/<category>/<backend>.go`.
3. Declare `mintable_paths` in the output schema; only the kernel mints `Token` values.
4. If the backend uses a credential, declare `CredentialRef.ID` in `Credentials()` and add a matching `credential_id` entry in the skill's `tools.yaml`. Pack-load-time reconciliation rejects mismatches.
5. Normalize numeric outputs to the kernel grammar before they reach `mintable_paths`.
6. Register the backend in the runtime layer, not in `kernel/`.
7. Add conformance tests covering: schema-validation rejection on bad input, successful mint from representative output, and a `tool_call_failed` entry on backend error.

### Adding an EntryKind

1. Add a row to `architecture/subsystems/audit.md` §Per-EntryKind payload schema. Declare deterministic and non-deterministic fields explicitly.
2. Run `go generate ./kernel/public/...`; verify `git diff --exit-code`.
3. Bump `kernel_version`.
4. Update `kernel/testdata/header-fixtures/` with a fixture for the new version.
5. Add a projection round-trip test to `kernel/public/snapshot_test.go`.

### Code style

- No comments unless the WHY is non-obvious.
- No premature abstractions. Generalize only when a second concrete use case exists.
- Keep commits small and coherent. Do not bundle unrelated spec or code changes.
- `tools/enumgen/` is the only writer of the three generated enum files. Do not hand-edit them.
- The `credential_identity_witness` field is required on every `EntryKindToolCallStarted`, including pure-Go tool calls (which record the literal `"none"`).

### Common mistakes

- **Mixing deterministic and non-deterministic fields.** Wall-clock times, retry counters, and latency measurements go in `NonDeterministic`. Putting them in `Deterministic` silently breaks replay byte-identity.
- **Bypassing TransactionalScope.** Writing to `Store` and `AuditSink` in separate transactions violates the parallel-block atomicity invariant.
- **Using a fresh ULID for `tool_call_id`.** This breaks replay byte-identity from the second tool call onward.
- **Adding a new `error_code` without updating `audit.md` first.** The write-time gate rejects the entry in production; the CI drift-gate rejects the build.
- **Adding a new `system:` principal sentinel without updating the canonical table** in `architecture/cross-cutting/security.md`. The kernel rejects any unlisted sentinel at write time.
- **Treating MCP as the primary adapter.** ADR-0010 supersedes ADR-0007. See `architecture/adrs/0010-mcp-equal-not-primary.md`.
- **Implementing a streaming model response path.** Streaming is deferred. See `docs/KNOWN_TRADE_OFFS.md` §Streaming and split-turn.

---

## Where to find the specs

| Question | Document |
|---|---|
| Kernel types, interfaces, TransactionalScope | `architecture/subsystems/kernel.md` |
| Audit chain, EntryKind, hash construction, error_code table | `architecture/subsystems/audit.md` |
| Harness, Mode A/B, orphan-run threshold | `architecture/subsystems/harness.md` |
| Guards, validators, SubmitAnswer pipeline | `architecture/subsystems/guards.md` |
| Tools, ToolBackend interface, conformance template | `architecture/subsystems/tools.md`, `architecture/tools/README.md` |
| Skill pack format, normalizer slot | `architecture/subsystems/skills.md` |
| Storage schema | `architecture/subsystems/storage.md` |
| Security threat model, system: sentinels | `architecture/cross-cutting/security.md` |
| Module boundary contract, forbidden imports | `architecture/OVERVIEW.md` §3 |
| Accepted trade-offs | `docs/KNOWN_TRADE_OFFS.md` |
