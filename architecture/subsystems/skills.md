# Subsystem: Skill Loader

Status: **v0**

> A skill pack is a portable, versioned definition of a finance workflow. The skill loader parses, validates, and instantiates packs at runtime. Skill packs are how Provenant becomes useful for a specific job — "reconciliation," "AP coding," etc. — without changing the kernel.

## Depends on
- [`subsystems/tools.md`](tools.md) — packs require tools; loader verifies availability
- [`subsystems/guards.md`](guards.md) — packs configure validator pipelines
- [`subsystems/storage.md`](storage.md) — pack manifests and versions persist

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
  - name: ledger.get_transactions
    version: "^1.0.0"
  - name: extract.bank_statement
    version: "^1.0.0"
  - name: reconcile.match
    version: "^1.0.0"
  - name: calc.subtract
    version: "^1.0.0"
  - name: calc.sum
    version: "^1.0.0"
  - name: policy.lookup
    version: "^1.0.0"
```

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
   
7. Persist pack metadata to skills table
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

## Storage

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

## References

- [Moody's modular AI](../references.md#moodys-modular) — agent-as-microservice composability that skill packs operationalize
- [Multi-agent finance benchmark](../references.md#multi-agent-finance) — hierarchical workflow shape that skill packs implement
- See [ADR-0006](../adrs/0006-skill-pack-format.md) for the rationale behind the pack format
