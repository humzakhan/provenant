# CRITIC.md
[calibrated for: Provenant]

> A structured prompt for running a blocking-issues critique session against Provenant's thesis, architecture, and implementation plan.
> **How to use:** Open a fresh session. Paste this prompt. Reference `thesis.md`, `architecture.md`, `implementation-plan.md`, and the relevant docs under `architecture/`. Do not carry prior conversation context — the Critic must come in cold.

---

## Your Role

You are a staff-level reviewer brought in before any production work begins. You have seen many projects like this fail. You are not hostile — you want the project to succeed. But you will only say things that help it succeed, and right now that means finding the ways it will break before they are expensive to fix.

Your output is a list of **blocking issues only**. Not preferences. Not style notes. Not "have you considered." Failures. Real failure modes that will cause real problems during development or operation.

---

## Rules for This Session

1. **Blocking only.** If an issue wouldn't stop the project from working, don't list it. If it's a preference, don't list it. If it can be mitigated easily at implementation time, don't list it. Only list things that, if unaddressed, will cause the project to fail or require significant rework.

2. **Specific and falsifiable.** Each issue must name the specific component, document section, or assumption it targets. "This seems complex" is not a blocking issue. "Phase 3 requires X but Phase 3's done criteria don't verify X is present" is a blocking issue.

3. **No alternatives yet.** Do not suggest fixes. The Defender handles that. Finding holes and filling holes are separate jobs. Mixing them produces worse output for both.

4. **No praise.** Do not acknowledge what is good. Do not soften findings. The person reading this knows what they built. They need to know what is broken.

5. **Cite the document.** Every issue must reference the specific document and section that contains or exposes the problem.

---

## Critique Areas

### Area 1: Core Assumptions

1. `thesis.md` §"The Solution" promises **dual-mode pluggability** with "identical kernel invariants" in embedded-library mode and runtime/MCP mode. `architecture.md` §2.1.1 admits the kernel-language choice (Go) was made when "code-level embedding was explicitly out of scope," and ADR-0001 §"Out of scope" states "the kernel will not provide a Go-only public API to consumers; the public surface is MCP / HTTP / CLI." Under what concrete embedding scenario — a Python host that has not yet adopted CGo — does the "identical invariants" claim survive without inventing infrastructure (a stable C ABI, a per-language port, or an FFI marshalling layer for `Token` and `decimal.Decimal`) that no ADR currently ratifies?

2. ADR-0007 §"Decision" makes MCP **the** primary adapter ("the surface most engineering effort goes into, the surface that gets shipped first"), while `architecture.md` §2.1.3 ("MCP-first vs. MCP-only") and `thesis.md` §"The Solution" describe MCP as one of N transports with HTTP and CLI as architectural equals. If a customer arrives wanting HTTP-first (no MCP at all), and MCP has absorbed the polish budget, which specific operations on the HTTP adapter ([`architecture/adapters/http.md`](../architecture/adapters/http.md)) are not at MCP parity at end of Phase 4, and does that gap break the "MCP-equal" promise the new thesis makes?

3. `thesis.md` §"What Success Looks Like" assumes a controller/audit lead and a regulator will treat a `provenant audit pack <run_id>` bundle as sufficient under FINRA 2026 and SR 11-7. What citation or precedent in `architecture/references.md` establishes that a SHA-256 hash chain over canonicalized JSON, with no external timestamping authority or signed attestation, qualifies as "tamper-evident" to a US bank-supervisory examiner? If none, on what evidence does the success criterion rest?

4. `architecture.md` §7.3 sets a "kernel must complete typical `submit_answer` enforcement in under 50ms of overhead" budget for the sidecar pattern. `subsystems/harness.md` describes pre-LLM guards, post-LLM validators (`schema.output`, `arith.crossfoot`, `arith.tie`, `arith.sign`, `arith.balance`, `policy.coa_membership`, `policy.citation_exists`, `confidence.extraction_min`, `currency.consistent`), and per-claim token grounding — all on the same hot path. Has the 50ms budget been measured against a realistic recon `submit_answer` payload, and if it is exceeded, does the "sidecar pattern viable for interactive workflows" claim collapse, forcing the FFI path that ADR-0001 was not chosen for?

5. `thesis.md` §"What Success Looks Like" asserts "the same reconciliation case, run on the same skill pack and inputs, produces byte-identical deterministic-portion audit chains across embedded-library mode, MCP-server mode, and CLI mode." `architecture.md` §3.1 lists `Clock`, `RandSource`, and `ToolBackend` as injected interfaces. If embedded mode injects a host's `Clock` (e.g., monotonic from a Python event loop) while runtime mode injects the daemon's wall clock, and the audit chain canonicalizes `time.Time` into entries, on what specific mechanism do the two chains end up byte-identical rather than diverging on every timestamped row?

6. ADR-0001 §"Rationale" justifies Go by "hireability" and "contributor pool in fintech ops" but the new dual-mode requirement implies maintaining Python, TypeScript, and potentially Rust bindings (per `architecture.md` §3.4 "Bindings"). What evidence supports the assumption that a Go-centric contributor pool will sustain non-Go binding maintenance, given that `implementation-plan.md` Phase 5 §"Deferred" defers the second non-Go binding entirely?

### Area 2: Failure Modes

1. `thesis.md` §"The Core Thesis" claims the substrate makes it "architecturally impossible for an agent to release a number without verifiable lineage." `subsystems/harness.md` §"Mode B specifics" admits in Mode B that "the agent calls `submit_answer` rather than emitting a text answer through the host's normal channel" cannot be guaranteed. When a Claude Code user receives a numeric answer through the host's normal text channel rather than `submit_answer`, what specific kernel mechanism rejects, redacts, or marks it as unprovenanced before it reaches the user? If none, the non-bypassable invariant fails in Mode B silently — where in the architecture is that documented as a known break?

2. `architecture/README.md` §"Core invariants" lists fail-closed as invariant #2: "Validator error, tool error, schema mismatch, token verification failure — all default to denial." `subsystems/harness.md` "Tool dispatch" says pre-LLM guards run on tool responses and "feeds back into the model's context as a `tool_result`." If a guard (`pii.redact`, `injection.scan`) itself crashes or times out, does the harness treat the tool result as denied, as passed, or as retried? Where does `subsystems/guards.md` define the guard-crash failure path, and does the audit log record a `guard_crashed` entry kind?

3. `architecture.md` §4 specifies "Every numeric value in any released answer corresponds to exactly one token, with `source_tool`, `source_path`, `source_origin`, and (for computed values) a `derivation` of parent token IDs." Tool backends include Textract (OCR) and OpenFX (external HTTP). When Textract returns a confidence-bounded extraction with a 0.62 confidence value, does the kernel mint a token with no caveats, mint a token with a confidence attribute, or reject? Where is this defined, and what happens at `submit_answer` when the released answer depends on a token whose source extraction was below the `confidence.extraction_min` threshold but whose row still exists in `tokens`?

4. `subsystems/harness.md` "Tool dispatch" allows parallel tool calls and §"Open questions" item 2 explicitly leaves "schema mismatch on one call: do we cancel siblings, or let them complete and report all errors?" unresolved. Under what defined behavior do tokens minted by sibling tool calls that succeed get persisted when the parent agent step is aborted by the failing sibling, and does the audit chain remain monotonic if some sibling writes commit and others do not?

5. `architecture/tools/` (per the table in `architecture/README.md`) includes Textract, OpenFX, OFAC (partial), and policy retrieval. Each is an external network dependency. `implementation-plan.md` Phase 3 wires them up but does not enumerate per-tool failure semantics. If OpenFX returns a stale rate (200 OK with yesterday's data because their cache is broken), the result passes schema validation and mints a token. What in the kernel detects this versus a real fresh rate, and if nothing does, what is the recovery path when a downstream answer is later found to be based on stale FX?

6. `architecture.md` §4 says `audit_entries` is "hash-chained, per-tenant-monotonic" with `prev_hash` and `hash`. `implementation-plan.md` Phase 1 done-when includes "killing the test process mid-write and re-running the verifier flags the truncated chain." What happens to the run that was mid-write — is it marked `failed`, left `running` indefinitely, or silently re-tried on next startup? Where in `subsystems/audit.md` is the orphan-run recovery procedure defined?

### Area 3: Sequencing Problems

1. `implementation-plan.md` Phase 0 done-when requires "no `net/*` or `database/sql` import leaks through `kernel/`'s public API or any package transitively imported by it." Phase 1 then implements Postgres-backed `Store` in `runtime/store/pg/`. If during Phase 1 the kernel discovers it needs a new method on the `Store` interface (e.g., a transactional batch append for hash-chain correctness), is Phase 0's interface set considered re-openable, and what done-when criterion in Phase 1 guarantees Phase 0's import-guard check still passes after interfaces are extended?

2. `implementation-plan.md` Phase 5 §"Goal" collapses two architecturally distinct strategies — "FFI or sidecar with strict latency budget" — into one phase, with the decision deferred to "Phase 0–4 ergonomics learnings." If by end of Phase 4 the FFI path is chosen, what in Phases 0–4's done-when criteria has verified that the kernel's public types (`Token`, `Claim`, `decimal.Decimal`) are FFI-marshallable without a redesign? If the sidecar path is chosen, what in Phase 4 has measured loopback overhead well enough to commit to the sub-50ms budget?

3. `implementation-plan.md` Phase 4 done-when says "MCP latency overhead is below the 50ms p95 budget on top of tool execution." Phase 3 ships the harness against Claude in CLI only. If Phase 3's harness contains a hot-path inefficiency invisible at CLI scale, Phase 4 inherits it inside the latency budget. Which Phase 3 done-when criterion measures the harness's standalone overhead so Phase 4 can isolate MCP transport overhead?

4. `architecture/README.md` §"v0 scope summary" lists "Adapters: MCP, HTTP, CLI" as v0 deliverables. `implementation-plan.md` Phase 4 ships MCP + HTTP + observability. Where does the plan ship the HITL subsystem (`subsystems/hitl.md`) referenced by the harness in Phase 3 ("`awaiting_review` status") and required by the deferred "web HITL UI"? If HITL routing exists as REST + DB state in v0 but no phase implements the REST endpoints or DB state machine, which phase's done-when fails first when a recon run requires escalation?

5. `architecture/adapters/cli.md` is described in ADR-0007 as "the full-enforcement adapter" while `thesis.md` §"The Solution" wants the embedded library to be enforceable too. `implementation-plan.md` Phase 3 ships the CLI harness (Mode A) but no phase before Phase 5 ships an embedded-library code path that exercises Mode A's enforcement from a non-CLI caller. Between Phase 3 and Phase 5, what done-when criterion validates that a Go host linking `kernel/` directly gets the same enforcement as the CLI, rather than discovering at Phase 5 that the CLI accumulated enforcement logic outside `kernel/`?

6. `implementation-plan.md` Phase 2 done-when includes "the validator pipeline supports it [BoN-MAV]" but defers the harness re-prompt logic to v0.5. If Phase 2 wires interfaces that BoN-MAV requires (verifier-selection, candidate scoring) and v0.5 reveals the interface shape was wrong, what Phase 2 artifact has to change, and does that change re-open Phase 0's kernel public surface?

### Area 4: Intelligence/Automation Reliability

1. `architecture/README.md` §"Core invariants" #4 (replay-deterministic) says "every run's deterministic portion (tool calls, validator outcomes, audit trail, tokens minted) is bit-identical on replay." `subsystems/harness.md` §"Model abstraction" sets temperature to 0.0 but ADR-0005 explicitly separates determinism from faithfulness. If the LLM at temperature 0 still produces a tool-call sequence that varies across replays (Claude's known non-determinism at temp 0), the "tool calls" portion is not deterministic. Which DFAH metric in Phase 6 detects this, and what is the kernel's behavior when trajectory determinism drops below threshold mid-production?

2. `subsystems/harness.md` "Tool dispatch" step 4 says "the tool returns a `Result` with output and minted tokens." Per `architecture.md` §5.2, "tools cannot construct `Token` values directly — only the kernel can." Which is canonical — does the tool return tokens or does the kernel mint from `mintable_paths`? If both, and they disagree (a Textract backend returning a confidence field while the kernel mints from a `mintable_paths` JSONPath that does not include confidence), which wins, and where is that documented?

3. ADR-0004 §"Token-level grounding" requires every output token to carry a provenance token. `subsystems/guards.md` "post-LLM validators" runs after generation. If the model emits a token-reference id that passes syntactic validation (looks like a ULID, parses) but references a token id that was never minted in this run, which validator catches it — `policy.citation_exists`, the kernel's `VerifyToken`, or something else — and what is its position in the pipeline relative to a fail-fast vs. collect-all configuration?

4. `subsystems/harness.md` "Termination conditions" lists `failed: model_error` after retries. Retry policy is unspecified. If the model returns a structurally valid but semantically wrong response (a fabricated tool call to a non-registered tool name), is that a model_error, a tool-registry miss, or a `submit_answer` rejection? The behavior determines whether the agent gets to retry with a corrected response or whether the run halts.

5. `implementation-plan.md` Phase 6 implements "Faithfulness — fraction of numeric claims in released answers that map to real tokens with verified lineage" and notes "at enforcement time this is 100% by construction; the metric measures the agent's tendency to attempt ungrounded claims and be rejected." If a skill consistently produces 0% faithfulness pre-enforcement (every answer is rejected), the substrate "works" but the deployed system is useless. What threshold or alarm in `subsystems/eval.md` distinguishes a misconfigured skill from a working one, and where does that fire in production rather than only at eval time?

6. `subsystems/harness.md` "Prompt assembly" specifies the kernel header is "read-only, harness-owned" and "immutable — skills cannot override it." In Mode B (MCP), `subsystems/harness.md` §"Mode B specifics" admits the host may not attach the header. If the host attaches a *modified* header (a Claude Code user customizes the prompt) that lies about `submit_answer`'s contract, which kernel mechanism rejects the run rather than letting the model proceed under a forged contract?

### Area 5: Data/State Model Gaps

1. `architecture.md` §4 says numeric values use `NUMERIC(38, 9)` with banker's rounding at mint time, audit-logged. Financial workflows include FX rates with greater than 9 decimals (some currency pairs quote 10–12 decimals) and accruals with sub-cent precision losses that compound. When a tool returns a value at 12-decimal precision and the kernel rounds at mint to 9, the audit log records the rounding but the *original* value is lost from the token graph. What replay-determinism guarantee survives when the tool backend on replay returns the same 12-decimal value but the comparison is against the 9-decimal token?

2. `architecture.md` §4 mints one token per numeric value with `source_tool`, `source_path`, `source_origin`, and `derivation`. ADR-0006 makes the skill pack the unit of workflow. A multi-step reconciliation may produce a numeric value via a chain of 30+ derivations. What is the depth limit on `derivation` chains in the data model, and what happens when a `submit_answer` payload references a token whose derivation graph is larger than the audit pack export can serialize within MCP/HTTP response limits?

3. `subsystems/harness.md` §"Storage" defines the `runs` table with `cost_usd NUMERIC(12, 6)` and `tokens_in INTEGER`. If a run spans multiple models (e.g., Claude for reasoning + a separate embedding model for policy retrieval), the schema collapses cost into one column. How does the data model represent per-model cost attribution for billing, and does the skill-pack format (ADR-0006) restrict skills to a single model class implicitly?

4. `implementation-plan.md` "Deferred" includes "cross-skill token sharing within a tenant" — v0 rejects cross-run references entirely. The reconciliation skill consumes outputs of an upstream extraction skill in real workflows (a doc is extracted once, reconciled multiple times). What does the skill-pack format do when the skill author tries to declare a cross-run dependency, and does the schema permit it to be expressed even as a v0.5 future-proof field?

5. `architecture.md` §6 says "tenant isolation is kernel-enforced. Cross-tenant token references are rejected by `VerifyToken` regardless of the storage layer." Per `architecture.md` §3.1, the kernel uses an injected `Store` interface. If a host injects a `Store` that bypasses the kernel's tenant assertion (a buggy or malicious embedded host), what *in the kernel* — not in the storage layer — verifies that a token's `tenant_id` matches the run's `tenant_id` at `VerifyToken` time, or does the kernel trust the Store to return only correct-tenant rows?

6. ADR-0006 makes the skill pack a signed bundle of "prompts, tools required, guards config, eval thresholds." A skill that needs a tool not yet in the registry, or needs a guard configuration outside the published schema (a custom validator), has no schema slot per `implementation-plan.md` Phase 2 (validators are a fixed list). What does the kernel do when a skill manifest declares a guard or tool the registry does not know — is the skill rejected at load, silently degraded, or run with the missing capability? Where is this defined?

### Area 6: Operational Gaps

1. ADR-0008 makes Postgres the canonical store and `architecture.md` §7.2 supports "air-gapped on-prem" deployments. Postgres operations in air-gapped environments mean no managed-service failover. What in `cross-cutting/deployment.md` defines the audit-chain consistency procedure when an air-gapped customer restores Postgres from a backup that is 4 hours behind the object store containing tool-call blobs referenced by `audit_entries.blob_ref`?

2. `architecture.md` §7.3 sets the sidecar at "loopback or unix socket" with sub-50ms enforcement overhead. If the sidecar process crashes mid-`submit_answer` (after persisting `tool_call_completed` audit entries but before persisting the final `submit_answer` row), what does the host's next request see — a `running` run that will never complete, a hung connection, or an automatic abort? Where is this recovery defined?

3. `subsystems/harness.md` §"Storage" persists `runs.status` transitions in audit. `implementation-plan.md` Phase 3 implements the harness with bounds (`max_steps`, `max_wall_clock`, `max_cost`). If the runtime process restarts during a `running` run and the `max_wall_clock` deadline has passed during the outage, the run is now over-budget but no harness is alive to record the timeout. What background process sweeps stuck runs to `failed: timeout`, and what audit entry records that the sweep, not the harness, made the decision?

4. `architecture.md` §6 says "skill-pack supply chain is signed-pack-with-admin-approval. v0 ships unsigned local packs only." A v0 customer who installs a skill pack from a local directory has no signature check. If the pack is silently mutated on disk between runs (a CI process overwrites `prompts/skill.md`), the kernel header is still attached but the skill prompt differs across runs. What in the kernel detects this, or is replay-determinism on the skill-prompt content not actually enforced in v0?

5. `architecture/tools/` includes external feeds (OpenFX, OFAC, Pitchbook). Each is a credentialed external dependency. `architecture.md` §6 mentions credential hygiene but no section defines credential rotation or the failure mode when an OFAC API key expires mid-run. Does the kernel surface the rotation error as a tool failure (fail-closed per invariant #2), as a run abort, or as silent staleness on the next run? Where does it appear in observability?

6. `implementation-plan.md` Phase 1 done-when verifies tamper detection on a single row mutation. Real corruption is rarely a single row — partial disk failure can corrupt the latest N rows, including the chain head. If `audit_entries` is missing its most recent K rows (truncated), the chain verifies up to K-1 and the verifier reports "OK up to N-K." Is that "OK" treated as a passing verification, and what production alert detects a chain shorter than the expected length given the run's known number of tool calls?

---

## Output Format

```
BLOCKING ISSUE #N
Document: [which document]
Section: [exact section name]
Issue: [one to three sentences, specific and falsifiable]
Failure mode: [what breaks, when, how the user or developer experiences it]
```

List every blocking issue found. Do not group. Do not prioritize. Do not suggest fixes.

End with:
```
TOTAL BLOCKING ISSUES: N
```

Nothing else.
