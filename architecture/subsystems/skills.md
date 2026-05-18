# Subsystem: Skill Loader

Status: **v0**

> A skill pack is a portable, versioned definition of a finance workflow. The skill loader parses, validates, and instantiates packs at runtime. Skill packs are how Provenant becomes useful for a specific job — "reconciliation," "AP coding," etc. — without changing the kernel.

## Depends on
- [`subsystems/tools.md`](tools.md) — packs require tools; loader verifies availability
- [`subsystems/guards.md`](guards.md) — packs configure validator pipelines
- [`subsystems/storage.md`](storage.md) — runtime-layer reference bindings project pack metadata

## Depended on by
- [`subsystems/harness.md`](harness.md) — harness loads a pack for each run
- All adapters

## Pack format

```
recon/
├── manifest.yaml                # name, version, dependencies, model recs
├── tools.yaml                   # required tools and pinned versions
├── guards.yaml                  # pre-LLM + post-LLM pipeline
├── prompts/
│   ├── system.md                # base system prompt
│   ├── skill.md                 # skill-specific instructions
│   ├── examples/                # few-shot examples
│   │   ├── 001-simple-recon.json
│   │   └── 002-unmatched-residual.json
│   └── templates/               # parameterized prompts (optional)
├── eval/
│   ├── cases/                   # eval cases shipped with the pack
│   │   ├── 001-clean-recon.json
│   │   └── 002-bank-fee-mismatch.json
│   └── thresholds.yaml          # per-metric minimum thresholds
├── policy/                      # policy snippets retrievable via policy.lookup
│   └── tolerance.md
├── normalizers/                 # OPTIONAL per-backend numeric / string normalization shims
│   └── (see "Pack-level normalizers" below)
├── ui/                          # optional HITL review templates
│   └── review-template.html
└── README.md                    # human-readable pack documentation
```

### `manifest.yaml`

```yaml
name: recon
version: 0.3.0
display_name: "Bank ↔ GL Reconciliation"
description: >
  Reconcile a bank statement to the GL operating account, producing
  a structured recon report with matched pairs, unmatched residuals,
  and a difference analysis.

authors:
  - "Provenant Project"

license: MIT

model_recommendations:
  default: claude-opus-4-7
  acceptable:
    - claude-opus-4-7
    - gpt-5
  not_recommended:
    - any model with non-zero temperature on compliance path

bounds:
  max_steps: 30
  max_wall_clock_seconds: 300
  max_cost_usd: 1.00
  orphan_multiplier: 2            # used by the orphan sweep: a run is swept if its last entry
                                  # is older than max_wall_clock_seconds * orphan_multiplier.
                                  # Pinned in the manifest — NOT deployment-overridable — so that
                                  # the deterministic `threshold_seconds_used` recorded on
                                  # EntryKindRunOrphanRecovered is identical across deployments
                                  # of the same (skill_id, skill_version). See
                                  # architecture/subsystems/harness.md and architecture/subsystems/audit.md.

hitl:
  routing:
    auto_below: 0.85           # confidence < 0.85 always routes to human
    single_review_below: 0.95
    dual_review_above: 0.95

bon_mav:
  enabled: false               # opt-in; off for recon

provenant_kernel:
  min_version: "0.1.0"
```

### `tools.yaml`

```yaml
required:
  - name: ledger.get_balance
    version: "^1.0.0"
    credential_id: ledger.sap_eu        # optional; logical credential identifier; omit / null for pure-Go tools
  - name: ledger.get_transactions
    version: "^1.0.0"
    credential_id: ledger.sap_eu
  - name: extract.bank_statement
    version: "^1.0.0"
    credential_id: textract.aws_us_east_1
  - name: reconcile.match
    version: "^1.0.0"                   # pure-Go; no credential_id
  - name: calc.subtract
    version: "^1.0.0"
  - name: calc.sum
    version: "^1.0.0"
  - name: policy.lookup
    version: "^1.0.0"
```

**`credential_id` field (load-bearing for credential-identity baseline).** Each entry in `tools.yaml` declares `credential_id` **if and only if** the bound `ToolBackend.Credentials()` returns non-empty; the load-time check in §"Loading lifecycle" step 3 rejects packs that violate this rule with `skill.credential_id_missing` or `skill.credential_id_unexpected`. The field names the logical credential the tool dispatches under (e.g., `ledger.sap_eu` is the entry in the tenant secret store the tool reads). The field is the **static manifest** input to the kernel's `credential_identity_baseline` computation at `EntryKindRunStarted` (see [`cross-cutting/security.md`](../cross-cutting/security.md#silent-credential-rotation-detection-per-run-identity-baseline) and [`audit.md`](audit.md)): the baseline map is built by iterating `tools.yaml`'s declared `credential_id` values and hashing the secret-store's stable identifier for each. A tool entry **without** a `credential_id` (pure-Go calculators, kernel-internal extractors) contributes nothing to the baseline; at every `tool_call_started` for such a tool, `credential_identity_witness = "none"` (the literal four-character string per [`audit.md`](audit.md#credential_identity_witness-positive-evidence-that-identity-check-ran)).

Two siblings declaring the same `credential_id` share a baseline entry; the pre-check phase in [`audit.md`](audit.md#parallel-batch-credential-rotation-rule) reads each `credential_id` exactly once per batch.

The static manifest is the **only** source for the baseline. A tool's runtime `ToolBackend.Credentials()` accessor (see [`tools.md`](tools.md#tool-interface)) is consulted at runtime witness emission, but it MUST agree with the manifest's `credential_id` declaration on every dispatched call; a mismatch is a typed `kernel_integrity` error (`error_code = credential.id_manifest_mismatch`, per the canonical `error_code` table in [`audit.md`](audit.md#canonical-error_code-table)) and the run fails closed. This pins the deterministic decision rule for "witness vs none". By the load-time iff-rule above, this is equivalent to: **backend `Credentials()` non-empty (and therefore manifest declares `credential_id`)** ⇒ witness present and equal to `base32(sha256(credential_id || ":" || observed_hash))[:hex_len]`; **backend `Credentials()` empty (and therefore manifest does not declare `credential_id`)** ⇒ witness is the literal `"none"`. A pack that disagrees with this rule never reaches runtime — it is rejected at load time, so the runtime decision is total over the legal pack space rather than agreeing with absence.

### `guards.yaml`

```yaml
pre_llm:
  - name: injection.scan
    severity: L2                # quarantine
  - name: pii.redact
    severity: L1                # advisory; recon doesn't surface PII

post_llm:
  - name: schema.output
  - name: arith.crossfoot
  - name: arith.tie
  - name: arith.sign
  - name: confidence.extraction_min
    config:
      threshold: 0.85
  - name: currency.consistent

mode: collect_all              # alternative: fail_fast
```

### `eval/thresholds.yaml`

```yaml
trajectory_determinism: 0.85   # required minimum across N=20 replays
decision_determinism: 0.98
faithfulness: 1.00             # every claim must be grounded
accuracy: 0.95                 # against case-bank ground truth

replay_n: 20
release_blocks_on_regression: true
```

### Pack-level normalizers (`normalizers/`)

The kernel's numeric canonicalization grammar in [`audit.md`](audit.md#numeric-canonicalization-cross-language-replay) is locale-independent and rejects non-US/POSIX numeric forms with a typed `numeric.parse_error`. Backends that genuinely return locale-specific numeric strings (European `1,5`, NBSP-thousands `1 234 567,89`, etc.) — and other per-backend pre-canonicalization needs (currency-symbol stripping, whitespace trimming for legacy ERP feeds) — are normalized **before** the value reaches `mintable_paths`. The kernel does not ship a built-in locale layer; instead, the **skill pack** declares per-backend normalizers in the `normalizers/` slot.

Pack layout:

```
normalizers/
├── manifest.yaml           # declares which normalizer applies to which backend + tool path
├── eu-locale-decimal.go    # OR a registered built-in name; v0 ships pure-Go normalizers only
└── (...)
```

`normalizers/manifest.yaml`:

```yaml
normalizers:
  - id: eu-locale-decimal
    applies_to:
      - tool: ledger.get_transactions
        backend: sap_eu
        mintable_path: "$.transactions[*].amount"
    kind: builtin       # v0: only built-in normalizer IDs from the kernel's registered set
    builtin_id: numeric.locale.eu_comma_decimal
```

**v0 scope.** v0 ships only built-in normalizer IDs (`kind: builtin`) from a kernel-registered set; v0 does **not** load arbitrary Go or WASM code from skill packs. The cycle-5 amendment shipped an empty registry; the cycle-9 amendment (this section) pins the v0 built-in normalizer set so that the messy-financial-inputs cleaning step has provenanced primitives from day one. The first non-empty backend that needs a normalizer not in this set still requires a kernel-version bump in tandem with the addition.

### v0 built-in normalizer set

The kernel-registered set (`kind: builtin`) is the following five normalizers. Each is a deterministic, pure-Go transformation registered at compile time via the mechanism pinned below; the set is part of the kernel's public API contract and adding to it is a `kernel_version` bump per the cycle-6 rule. Each normalizer specifies its input grammar, output canonical form, error cases, and the `error_code` it can emit (canonical `error_code` table in [`audit.md`](audit.md#canonical-error_code-table)).

| `builtin_id` | Transformation | Error code on rejection |
|---|---|---|
| `numeric.locale.eu_comma_decimal` | Input grammar: `^-?[0-9]{1,3}(\.[0-9]{3})*(,[0-9]+)?$` or `^-?[0-9]+(,[0-9]+)?$` (European comma-decimal with optional dot thousands). Transformation: drop dot thousands separators, replace the comma decimal with a `.`, emit a canonical decimal string conforming to the kernel's locale-independent grammar. Examples: `1.234,56` → `1234.56`; `1,5` → `1.5`; `12.345` (no decimal portion) → `12345`. Output canonical form: matches `^-?(0|[1-9][0-9]*)(\.[0-9]+)?$` so the kernel's downstream `source_value_raw` canonicalization accepts it. Error cases: ambiguous inputs that match both EU and US grammar (e.g., `1,234`) raise `normalizer.locale_ambiguous`. | `normalizer.locale_ambiguous` |
| `numeric.thousands_separator.nbsp` | Input grammar: any decimal-shaped string that may contain U+00A0 (NBSP), U+202F (narrow NBSP), U+2009 (thin space), or U+0020 (regular space) as thousands separators. Transformation: strip every NBSP / thin-space / narrow-NBSP / regular-space character anywhere in the string; remaining content must match the kernel's locale-independent decimal grammar. Examples: `1 234 567.89` → `1234567.89`; `1 234,56` → `1234,56` (which then chains to `numeric.locale.eu_comma_decimal` if that normalizer is also configured for the path; **chain order = the order in which normalizers appear in `applies_to[*].normalizers` for that path** — the manifest list is the deterministic chain order, not the table order in this document). Output canonical form: the input minus the listed separator characters. Error cases: if the post-strip result does not match a decimal grammar (e.g., embedded letters), the value is passed through unchanged for downstream parsing to reject as `numeric.parse_error`; this normalizer does not raise its own error. | (none — downstream `numeric.parse_error` if invalid) |
| `currency.iso4217` | Input grammar: a currency designator that is either an ISO 4217 three-letter code (`USD`, `EUR`, `GBP`, ...), a recognized symbol (`$`, `€`, `£`, `¥`, `¢`), or a country-code-prefixed variant (`US$`, `CA$`, `A$`, `HK$`, `NZ$`, `S$`, `SFr`). The full mapping table is pinned in `kernel/internal/normalizers/currency_iso4217_table.go` and is part of the kernel's public API contract. Transformation: case-folded lookup against the table; emit the canonical three-letter ISO 4217 code. Examples: `$` → `USD`; `US$` → `USD`; `A$` → `AUD`; `€` → `EUR`; `eur` → `EUR`. Output canonical form: a three-letter uppercase ASCII code. Error cases: an input that does not resolve to a known code raises `normalizer.currency_unknown`. | `normalizer.currency_unknown` |
| `date.iso8601_lenient` | Input grammar: common date formats — `YYYY-MM-DD`, `YYYY/MM/DD`, `DD/MM/YYYY`, `MM/DD/YYYY` (US), `DD-MM-YYYY`, `DD Mon YYYY` (`02 May 2026`), `Mon DD, YYYY` (`May 2, 2026`), with optional time component `HH:MM[:SS]` and optional timezone (`Z`, `+HH:MM`, `-HH:MM`, named zone identifier from the IANA zoneinfo subset shipped with the kernel). Transformation: parse to a canonical ISO 8601 form `YYYY-MM-DDTHH:MM:SS[.fff]Z` with an explicit timezone. If no timezone is supplied, default to UTC **and** emit the `normalizer_applied` audit record with a `tz_defaulted: true` flag in the **deterministic** sub-object so a reviewer can tell that the timezone was assumed. (The absence of a timezone in the input is a deterministic property of the input bytes; a flag derived deterministically from input belongs in the deterministic payload regardless of whether the output canonical form is byte-identical to a tz-supplied input.) The ambiguous-locale case `DD/MM/YYYY` vs `MM/DD/YYYY` is resolved per the skill's `normalizers/manifest.yaml` `applies_to[*].config.date_order` (`dmy` or `mdy`); a path with no declared order and a value that could be either is rejected as `normalizer.date_unparseable`. Examples: `2026-05-17` → `2026-05-17T00:00:00Z`; `17/05/2026` (with `date_order: dmy`) → `2026-05-17T00:00:00Z`. Output canonical form: ISO 8601 UTC string. Error cases: unparseable input raises `normalizer.date_unparseable`. | `normalizer.date_unparseable` |
| `entity_name.basic` | Input grammar: any UTF-8 string representing an entity (vendor, counterparty, customer) name. Transformation: (a) apply Unicode NFC normalization; (b) Unicode case-fold (lowercase); (c) collapse runs of whitespace (including NBSP, tabs, newlines) to a single ASCII space; (d) strip legal-form suffixes — the `legal_forms` list is **manifest-pinned per path via `applies_to[*].config.legal_forms`** and is hashed into `skill_pack_hash`; it is **NOT deployment-overridable** (same precedent as `bounds.orphan_multiplier` per `audit.md` §`run_orphan_recovered`). The **default list**, used when a path declares no override, is part of the kernel's public contract and changes to it require a `kernel_version` bump: `Inc.`, `Inc`, `Ltd.`, `Ltd`, `LLC`, `L.L.C.`, `Corp.`, `Corp`, `Corporation`, `Company`, `Co.`, `Co`, `GmbH`, `S.A.`, `S.A.S.`, `B.V.`, `N.V.`, `AG`, `PLC`, `PTY LTD`, `LP`, `LLP`. Strip semantics: **longest-match-first in a single pass** — when two suffixes nest (e.g., `Co., Ltd.` contains both `Co.` and `Ltd.`), the longest matching suffix at the end of the string is stripped first, then the strip is re-attempted against the remainder until no list member matches as a trailing token; this rule is the same on every path regardless of `legal_forms` membership ordering, so two implementations that share the (default-or-overridden) set produce byte-identical output. (e) trim leading/trailing whitespace. The transformation is intended as a **pre-step** for entity-resolution `match` calls, not as a replacement for fuzzy matching. Examples: `ACME Corp.` → `acme`; `Globex GmbH` → `globex`; `Smith & Co., Ltd.` → `smith &`. Output canonical form: lowercase, NFC-normalized, trimmed, collapsed-whitespace string. Error cases: the transformation is total over the input space; an empty post-transform result is permitted (it surfaces as an empty string, which downstream `match` validators reject as `match.input_token_unknown` when applicable). | (none) |

**Audit visibility on firing.** When a normalizer in this set fires, the kernel records `EntryKindTokenMint.deterministic.normalizer_applied = { id: "<pack-local id>", builtin_id: "<one of the five above>", input_raw: "<verbatim>", output_canonical: "<canonical>" }` as already specified in this subsystem's existing cycle-5 amendment. The `date.iso8601_lenient` `tz_defaulted` flag lives in the **deterministic** sub-object: the absence of a timezone in the input is a deterministic property of the input bytes, and a flag derived deterministically from input belongs in the deterministic payload regardless of whether the output canonical form is byte-identical to a tz-supplied input. A reviewer can still see "this date was assumed UTC" via the same field.

**Addition of a new built-in normalizer is a `kernel_version` bump.** Per the cycle-6 rule (ADR-0006 amendment), adding, renaming, or removing a `builtin_id` requires a kernel-version bump because each ID is part of the kernel's public API contract and is hashed into chain payloads when the normalizer fires.

**Registration mechanism (pinned).** Built-in normalizers are registered at **compile time** in the kernel package `kernel/internal/normalizers/registry.go`, which exposes a private `map[string]Normalizer` populated by per-normalizer `init()` functions in sibling files (one normalizer per file, e.g., `kernel/internal/normalizers/numeric_locale_eu_comma_decimal.go`). The registry is **not** a runtime registry, not a boot-time JSON manifest, and not a plugin-loaded set; the kernel public surface exposes only a read-only `Lookup(builtin_id string) (Normalizer, error)` accessor (see [`kernel.md`](kernel.md#normalizerlookup-accessor); a missing ID returns the typed `skill.normalizer_id_unknown` error).

Implications, pinned here so two implementations cannot diverge:

- **Each `builtin_id` is part of the kernel's public API contract.** Adding, renaming, or removing a `builtin_id` is a kernel-version bump (and an audit-chain-format event if any deployed chain references the old ID).
- **Old chains with `normalizer_applied = null` remain valid across version bumps.** `normalizer_applied` is in the deterministic payload and `null` is a legal canonical value; a v0 chain (empty registry, every token has `normalizer_applied = null`) replays byte-identically on a v0.5 kernel that has registered new IDs.
- **A pack referencing an unregistered `builtin_id` is rejected at pack-load time** with a typed `skill.normalizer_id_unknown` error. Loading is strict; there is no "soft missing normalizer" mode.
- **The v0 registry ships the five built-ins (cycle 9).** Cycle 5's "v0 registry is empty" rule is historical; cycle 9 supersedes that content in this section: the registry ships with the five built-in normalizers enumerated above. The Phase 0 unit test that previously asserted `len(normalizers.Registry()) == 0` is replaced by a test asserting the registry contains exactly the five `builtin_id` values listed in the cycle-9 table, in stable order, and that each registered `Normalizer` implementation passes its fixture-input → canonical-output unit test. The `tools/enumgen/` mechanism (see [`audit.md`](audit.md#canonical-error_code-table)) is extended to also parse the cycle-9 table here and emit `kernel/public/normalizers/registry.go` so the markdown stays the single source of truth; CI's `git diff --exit-code` post-`go generate` catches any drift between this table and the kernel-registered set.

Phase 2 task list registers exactly the five v0 built-ins and rejects unregistered IDs; the registry plumbing — `kernel/internal/normalizers/registry.go`, the `Normalizer` interface, the `Lookup` accessor, and the pack-load-time `skill.normalizer_id_unknown` rejection path — ships in Phase 2 as part of the pack-loader code path so the format is enforceable from day one. See [`implementation-plan.md`](../../plans/implementation-plan.md#phase-2--tool-registry-and-guard-pipeline-pre--post).

**Hashing.** The `normalizers/` directory is included in the skill-pack content digest under the same rules as `prompts/` and `guards.yaml` (see [`audit.md`](audit.md#skill-pack-content-digest-procedure)): two packs with different normalizer configurations produce different `skill_pack_hash` values, so a skill that silently changes its normalization is caught on replay.

**Audit visibility.** When a normalizer fires, the kernel records the normalizer ID and the (raw, normalized) string pair in `EntryKindTokenMint.deterministic` under a new optional field `normalizer_applied = { id: "eu-locale-decimal", builtin_id: "numeric.locale.eu_comma_decimal", input_raw: "<verbatim>", output_canonical: "<canonical>" }`. Absence of the field means the value reached `mintable_paths` already in canonical form.

This closes the cycle-5 critic #5 gap: the locale-numeric trade-off's mitigation ("per-backend normalization shim in skill-pack") now has a concrete slot in the pack format and an audit-visible record when it fires. ADR-0006 is amended accordingly.

## Loading lifecycle

```
1. provenant skill load <path>
   or implicit: harness loads pack at run start
   
2. Parse manifest.yaml
   - validate against the manifest schema
   - check provenant_kernel.min_version compatibility
   
3. Parse tools.yaml
   - check every required tool is registered
   - check version constraints (^major.minor.patch)
   - reject load if any tool is missing or incompatible
   - **Bind each entry to its `ToolBackend` and reconcile `credential_id` against `ToolBackend.Credentials()`:**
     - For each entry, look up the bound `ToolBackend` and call `Credentials()`.
     - If `Credentials()` returns non-empty **and** the entry omits `credential_id` (or sets it to null) → reject the pack with `error_code = skill.credential_id_missing` (per the canonical `error_code` table in [`audit.md`](audit.md#canonical-error_code-table)).
     - If `Credentials()` returns empty **and** the entry declares `credential_id` → reject with `error_code = skill.credential_id_unexpected`.
     - If `Credentials()` returns non-empty and the entry's `credential_id` does not match any `CredentialRef.ID` returned → reject with `error_code = credential.id_manifest_mismatch` (the same rule as the dispatch-time check in [`tools.md`](tools.md), applied at load time so the pack cannot ship with a stale `credential_id`).
   - This step is **strict** and runs at every pack load (including the implicit harness-time load); there is no override. The reconciliation rule pins the iff-relationship documented in §"`credential_id` field": manifest declares `credential_id` if and only if the bound backend's `Credentials()` returns non-empty.
   
4. Parse guards.yaml
   - check every named guard is registered
   - validate config against each guard's config schema
   
5. Parse prompts/
   - render templates if any
   - validate the kernel header injection point (the harness wraps the system prompt)
   
6. Parse eval/
   - load case files
   - validate against the eval case schema
   - read thresholds
   
7. Persist pack metadata to the runtime-layer skills table projection
   - assigns a deterministic skill_pack_hash (sha256 of canonicalized content)
   
8. Mark pack ready
```

Loading is **strict** — any error during loading rejects the pack. There is no "partial load" mode. This is intentional: a partially-loaded pack would behave non-deterministically across deployments.

## Versioning and pinning

Skill packs follow semver. Major bumps indicate behavioral changes that may regress historical cases. Skill version is recorded on every run; the audit trail includes the exact pack hash.

A run pinned to skill 0.3.0 will not silently upgrade. Adapters explicitly resolve `skill@0.3.0` at run start.

## Distribution

**v0**: packs loaded from local filesystem. The CLI's `provenant skill install <path>` copies into `~/.provenant/skills/`.

**v0.5+ (deferred)**: skill registry — a public registry analogous to npm, where signed packs are published. The kernel verifies signatures on install.

**Private packs**: customers publish private packs to their own registry. The commercial offering hosts private registries with signing infrastructure.

## Storage projection (runtime layer)

The `skills` table below is the runtime-layer projection for pack metadata; the kernel sees canonical pack bytes and hashes, not a storage requirement.

```sql
CREATE TABLE skills (
    id              TEXT NOT NULL,                  -- e.g., "recon"
    version         TEXT NOT NULL,
    pack_hash       TEXT NOT NULL,                  -- sha256 of canonicalized content
    manifest        JSONB NOT NULL,
    tools           JSONB NOT NULL,
    guards          JSONB NOT NULL,
    eval_thresholds JSONB NOT NULL,
    installed_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, version)
);
```

## Conformance for skill authors

A pack must:

1. Declare a manifest passing the manifest schema.
2. Reference only registered tools and guards.
3. Ship at least 10 eval cases covering the workflow's variability.
4. Declare eval thresholds; CI will enforce regression below them.
5. Document each prompt's intent (what the few-shot demonstrates, what the system prompt forbids).
6. Pass a dry-run load against the current Provenant kernel.

## Open questions

1. **Pack composition.** Can a skill pack inherit from another (e.g., `recon-strict` extends `recon` with tighter thresholds)? Proposal: yes, single inheritance, but the inheritance chain is part of the pack hash.
2. **Multi-language prompts.** Packs may need to support multiple natural languages. Proposal: defer; v0 is English-only.
3. **Prompt A/B testing.** Should packs ship multiple prompt variants for online experimentation? Defer to commercial offering.
4. **Skill-specific custom validators / normalizers (code loading from packs).** v0 packs reference only kernel-registered built-in guards, validators, and normalizers (`normalizers/manifest.yaml` `kind: builtin` only). Loading arbitrary Go / WASM-sandboxed code from a skill pack — so a pack author can ship a skill-specific custom validator without a kernel-version bump — is **out of scope for v0** and tracked in KNOWN_TRADE_OFFS.md ("skill-pack extensibility deferred"). The v0 disclaimer is "the unit of workflow is a pack of built-in-only components"; v0.5+ will revisit a sandboxed extension surface (likely `pack/validators/<lang.wasm>` declared in `tools.yaml` under a new `custom_validators:` key, signed alongside the rest of the pack).

## References

- [Moody's modular AI](../references.md#moodys-modular) — agent-as-microservice composability that skill packs operationalize
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — hierarchical workflow shape that skill packs implement
- See [ADR-0006](../adrs/0006-skill-pack-format.md) for the rationale behind the pack format
