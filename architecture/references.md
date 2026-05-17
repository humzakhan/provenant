# Provenant — References

Canonical list of papers, industry sources, regulatory documents, and tooling that Provenant draws from. Other docs link to anchors in this file rather than restating URLs.

## Academic papers

### <a id="dfah"></a> IBM DFAH — Replayable Financial Agents (2026)
Khatchadourian, R., et al. *Replayable Financial Agents: A Determinism-Faithfulness Assurance Harness for Tool-Using LLM Agents*.
https://arxiv.org/abs/2601.15322

Closest published analogue to Provenant's thesis. Establishes that determinism and accuracy are not correlated in tool-using financial agents (r = -0.11) and must be measured independently. Schema-first architectures with smaller models achieve near-perfect determinism under stress. Ships an open-source benchmark suite (compliance triage, portfolio constraints, DataOps exceptions).

**Used in:** [ADR-0005](adrs/0005-determinism-faithfulness-independent.md), [subsystems/eval.md](subsystems/eval.md), core invariant #3 and #4.

### <a id="faramesh"></a> Faramesh — Execution Control Plane (2026)
*Faramesh: A Protocol-Agnostic Execution Control Plane for Autonomous Agent Systems*.
https://arxiv.org/pdf/2601.17744

Proposes six architectural guarantees for autonomous agent execution: non-bypassable enforcement, deterministic authorization, replayable provenance-complete decision records, policy-agnostic enforcement, fail-closed behavior, multi-agent / multi-tenant support. Explicit that intent correctness is out of scope — the contribution is the architectural boundary.

**Used in:** [ADR-0002](adrs/0002-submit-answer-as-single-exit.md), [ADR-0003](adrs/0003-fail-closed-default.md), core invariants #1 and #2.

### <a id="dccd"></a> DCCD — Draft-Conditioned Constrained Decoding (2026)
Reddy, A., Walker, T. T., Ide, J. S., Bedi, A. S. *Draft-Conditioned Constrained Decoding for Structured Generation in LLMs*.
https://arxiv.org/pdf/2603.03305

Decouples semantic planning from structural enforcement: generate an unconstrained draft, then apply constrained decoding conditioned on the draft. Consistent accuracy improvements over standard constrained decoding across model scales (1B–14B).

**Used in:** [ADR-0002](adrs/0002-submit-answer-as-single-exit.md), [subsystems/guards.md](subsystems/guards.md). The `submit_answer` contract is DCCD applied to numeric grounding.

### <a id="faith"></a> FAITH — Tabular Hallucinations in Finance (2026)
*FAITH: A Framework for Assessing Intrinsic Tabular Hallucinations in Finance*.
https://arxiv.org/pdf/2508.05201

Benchmark methodology for evaluating hallucinations in financial table reasoning. Provides datasets and metrics for in-house LLM evaluation.

**Used in:** [subsystems/eval.md](subsystems/eval.md) as a third-party reference benchmark.

### <a id="multi-agent-finance"></a> Multi-Agent LLM Architectures for Financial Documents (2026)
Kulkarni, S., Kulkarni, Y. *Benchmarking Multi-Agent LLM Architectures for Financial Document Processing*.
https://arxiv.org/abs/2603.22651v1

Systematic benchmark of four orchestration architectures (sequential pipeline, parallel fan-out, hierarchical supervisor-worker, reflexive self-correcting) on 10,000 SEC filings. Reflexive architectures achieve highest field-level F1 (0.943) but at 2.3× the cost of sequential baselines.

**Used in:** [ADR-0006](adrs/0006-skill-pack-format.md), [subsystems/guards.md](subsystems/guards.md), [subsystems/eval.md](subsystems/eval.md).

### <a id="bon-mav"></a> BoN-MAV — Multi-Agent Verification (2025)
Lifshitz, S., Du, Y. *Multi-Agent Verification: Scaling Test-Time Compute with Multiple Verifiers*.
https://arxiv.org/pdf/2502.20379

Best-of-N sampling combined with binary approvals from multiple aspect verifiers. Establishes verifier count as a new test-time compute scaling dimension.

**Used in:** [subsystems/guards.md](subsystems/guards.md), [subsystems/eval.md](subsystems/eval.md). Optional opt-in mode for high-stakes skills.

### <a id="finvault"></a> FinVault — Financial Agent Safety Benchmark (2026)
*FinVault: Benchmarking Financial Agent Safety in Execution-Grounded Environments*.
https://arxiv.org/pdf/2601.07853

Security benchmark for financial agents with state-writable business databases, permission and quota constraints, and auditable operation trails.

**Used in:** [cross-cutting/security.md](cross-cutting/security.md) as a reference for sandboxed-execution patterns.

## Industry sources

### <a id="moodys-modular"></a> Moody's modular AI architecture (VentureBeat, 2025)
https://venturebeat.com/ai/moodys-cut-credit-memo-prep-from-40-hours-to-2-minutes-with-modular-ai

Coverage of Moody's AI Studio: decomposition of credit memo generation into specialized agents (entity identification, financial extraction, peer analysis, risk assessment). Compared explicitly to microservices.

### <a id="moodys-agentic"></a> Moody's Agentic Solutions overview
https://www.moodys.com/web/en/us/capabilities/gen-ai/agentic-solutions.html

Vendor positioning: "context layer" of 600M entities, "decision-grade" outputs, transparency and oversight.

### <a id="brex-ai-native"></a> Brex AI-native operations
https://www.brex.com/journal/brex-ai-native-operations

Shared-platform agent architecture across credit, fraud, KYC, compliance, disputes.

### <a id="brex-first-round"></a> Brex agent ops (First Round)
https://www.firstround.com/ai/brex

Incremental automation pattern: confidence-routed HITL, auto-handle high-confidence cases, route the rest to analysts with AI assistance.

### <a id="arthur-guardrails"></a> Arthur AI guardrails (Part 5)
https://www.arthur.ai/blog/best-practices-for-building-agents-guardrails

Pre-LLM and post-LLM guardrail patterns. Real-time interception, PII redaction, hallucination detection, self-correction loops.

### <a id="anthropic-finance-agents"></a> Anthropic finance agents launch
https://www.anthropic.com/news/finance-agents

Anthropic's ten finance agent templates including GL reconciler, month-end closer, statement auditor, valuation reviewer, KYC screener.

### <a id="atlan-hallucination"></a> AI agent hallucination analysis (Atlan)
https://atlan.com/know/ai-agent-hallucination/

Production-grade analysis of acceptable hallucination rates per use case. High-risk applications (financial calculations, compliance reporting) need <5% hallucination rates with mandatory human validation.

### <a id="constrained-decoding"></a> Constrained decoding overview (Better ML)
https://medium.com/better-ml/herding-llms-structured-output-with-constraints-ae157ecf5d81

Survey of constrained-decoding techniques in vLLM, Outlines, OpenAI function calling.

## Regulatory sources

### <a id="finra-2026"></a> FINRA 2026 Annual Regulatory Oversight Report — GenAI section
https://www.finra.org/rules-guidance/guidance/reports/2026-finra-annual-regulatory-oversight-report/gen-ai

First dedicated FINRA guidance on GenAI. Auditability and explainability are central. Firms must demonstrate how agents reach decisions.

### <a id="sr-11-7"></a> SR 11-7 model risk management (Federal Reserve / OCC)
https://www.kiteworks.com/regulatory-compliance/ai-compliance-financial-services-firms/ (summary)

Applies to any model influencing financial decisions: documented purpose, independent validation, continuous monitoring, human override, defined retirement.

### <a id="nydfs-raise"></a> NYDFS RAISE Act amendments (2026)
https://www.dwt.com/blogs/artificial-intelligence-law-advisor/2026/04/ny-overhauls-frontier-ai-transparency-law

Broad NYDFS rulemaking authority for frontier AI; civil penalties up to $3M for repeat violations.

## Tooling and libraries

### Go ecosystem
- `chi` HTTP router: https://github.com/go-chi/chi
- `pgx` Postgres driver: https://github.com/jackc/pgx
- `sqlc` typed SQL queries: https://github.com/sqlc-dev/sqlc
- `atlas` migration tool: https://atlasgo.io
- `shopspring/decimal`: https://github.com/shopspring/decimal
- `ulid`: https://github.com/oklog/ulid

### MCP
- Model Context Protocol spec: https://modelcontextprotocol.io
- Go SDK: https://github.com/modelcontextprotocol/go-sdk

### Document extraction
- AWS Textract: https://docs.aws.amazon.com/textract/
- Azure Document Intelligence: https://learn.microsoft.com/en-us/azure/ai-services/document-intelligence/

### External data
- OpenFX-style FX rates: https://www.openexchangerates.org (or equivalent)
- OFAC SDN list: https://www.treasury.gov/ofac/downloads/

### Eval and benchmarks
- DFAH harness (IBM open source): https://github.com/ibm-client-engineering/output-drift-financial-llms
