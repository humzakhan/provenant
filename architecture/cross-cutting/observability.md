# Cross-cutting: Observability

Status: **v0**

> Provenant's observability surface has three layers: structured logs, distributed traces, and time-series metrics. The audit log is not observability — it's the *product*. Observability is the kernel's view into its own health and performance. Both are needed.

## What goes where

| Signal | Goes into | Purpose |
|---|---|---|
| Every consequential agent action | Audit log (`subsystems/audit.md`) | Product / compliance |
| Tool latency, model latency, error rates | Metrics (Prometheus) | Operator monitoring |
| Per-run trace of all kernel + tool spans | Traces (OpenTelemetry) | Debugging individual runs |
| Structured logs (warn/error/info) | Logs (JSON to stdout) | Debugging + alerting |

The four overlap in places. The rule of thumb: if a finance team needs to read it, it's audit. If an SRE needs to read it, it's observability. Same event may write to both.

## Metrics

Exposed via `/metrics` (Prometheus format). Default labels: `tenant_id`, `skill_id`, `skill_version`, `model`.

**Cardinality rule.** `run_id`, `sibling_group_id`, `tool_call_id`, `token_id`, `review_id`, and any other per-execution ULID-shaped identifier are **forbidden as Prometheus labels**. Prometheus is a low-cardinality time-series store; per-run labels cause unbounded series growth and OOM the scraper at any production volume. Per-run state belongs in OpenTelemetry traces, structured logs, or the SQL-queryable `run inspect` surface — never in metric labels. A `/metrics` endpoint that emits a per-run label is rejected by the Prometheus-format conformance test in Phase 4. This rule is load-bearing and cycle-5-enforced.

### Run metrics

```
provenant_runs_started_total{...}
provenant_runs_completed_total{status="completed|failed|awaiting_review|aborted",...}
provenant_run_duration_seconds{...}          histogram, p50/p95/p99 visible
provenant_run_cost_usd{...}                   histogram
provenant_run_steps{...}                      histogram
```

### Tool metrics

```
provenant_tool_calls_total{tool,version,backend,...}
provenant_tool_call_duration_seconds{tool,...}    histogram
provenant_tool_errors_total{tool,error_code,...}
provenant_tokens_minted_total{tool,...}
```

### Guard metrics

```
provenant_guard_runs_total{guard,stage,passed="true|false",...}
provenant_guard_run_duration_seconds{guard,stage,...}    histogram
provenant_submit_answer_attempts_total{outcome="released|rejected|hitl",...}
provenant_submit_answer_rejection_reasons_total{reason,...}
```

### Audit and storage metrics

```
provenant_audit_entries_appended_total{kind,...}
provenant_audit_chain_verifications_total{result="ok|mismatch",...}
provenant_postgres_pool_in_use
provenant_object_store_operations_total{op,result,...}
```

### Model API metrics

```
provenant_model_calls_total{model,result,...}
provenant_model_call_duration_seconds{model,...}    histogram
provenant_model_tokens_total{model,direction="in|out",...}
```

### HITL metrics

```
provenant_hitl_dispatched_total{skill,tier,...}
provenant_hitl_time_to_decision_seconds{tier,...}    histogram
provenant_hitl_decisions_total{kind="approved|edited|rejected|escalated",...}
```

### Alert thresholds (recommended)

| Metric | Threshold | Action |
|---|---|---|
| `submit_answer_rejection_rate` for any skill | >20% over 1h | Page on-call; possible skill regression |
| `model_call_duration p95` | >30s sustained 5m | Investigate model provider |
| `audit_chain_verifications{result=mismatch}` | any | Immediate page; halt tenant |
| `tool_errors_total` rate increase | 5× baseline | Page on-call |
| `hitl_time_to_decision p95` | >2h | Notify operator; review staffing |

## Distributed tracing

OpenTelemetry traces emitted for every run. Span structure:

```
run (root span)
├── load_skill
├── assemble_prompt
├── model_call (step 1)
│   └── (model API call span — vendored from SDK if available)
├── tool_dispatch (step 1)
│   ├── input_schema_validate
│   ├── audit_log_start
│   ├── tool_call (e.g., extract.bank_statement)
│   │   └── backend_call (textract)
│   ├── pre_llm_guards
│   ├── token_mint
│   └── audit_log_complete
├── model_call (step 2)
├── ...
└── submit_answer
    ├── numeric_extract
    ├── token_grounding
    ├── validator_pipeline
    │   ├── schema.output
    │   ├── arith.crossfoot
    │   ├── arith.tie
    │   └── (...)
    ├── audit_release
    └── case_capture
```

Every span has:
- `tenant_id`, `run_id`, `skill_id`, `skill_version`
- For tool spans: `tool_name`, `tool_version`, `backend`
- For guard spans: `guard_name`, `stage`, `severity`
- For model spans: `model`, `tokens_in`, `tokens_out`

Trace export to any OTLP-compatible backend (Jaeger, Honeycomb, Datadog, Grafana Tempo).

### Live progress observability for buffered parallel blocks (OTLP span events)

The audit chain's parallel-sibling buffer-and-flush rule (see [`subsystems/audit.md`](../subsystems/audit.md#parallel-sibling-deterministic-order)) means per-sibling progress inside an in-flight parallel batch does not appear in `audit_entries` until the batch flushes. For operator-facing live visibility into a stuck sibling, the runtime emits a `parallel_block` OTLP span per batch with **span events** for each sibling state transition:

- One `parallel_block` span per `sibling_group_id`, opened when the batch is dispatched and closed at flush.
- A span event per transition: `sibling.started{sibling_index}`, `sibling.completed{sibling_index}`, `sibling.failed{sibling_index, error_class}`.
- Span attributes carry `tenant_id`, `run_id`, `sibling_group_id`, and `sibling_count`. These are OTLP attributes (the trace backend can handle the cardinality), **not** Prometheus labels.

Operators alert on long-in-flight siblings via the trace backend (Tempo / Jaeger query) or via the `provenant run inspect <run_id>` SQL view over an in-memory ephemeral table the runtime maintains for live sibling state. The trace surface is explicitly labeled "not part of the replay-deterministic chain; emitted live; not buffered" and is independent of `audit_entries`. The Phase 4 done-when requires that the span+events surface at least the sibling count and per-sibling started/completed/failed transitions during a parallel block.

## Structured logs

Logs are JSON to stdout (one event per line). The default logger is `slog` with a JSON handler.

Each log line includes:

```json
{
  "ts": "2026-05-17T14:32:11.483Z",
  "level": "info",
  "msg": "run completed",
  "tenant_id": "acme",
  "run_id": "run_01HXYZ...",
  "skill_id": "recon",
  "skill_version": "0.3.0",
  "duration_ms": 47200,
  "cost_usd": 0.31,
  "status": "completed"
}
```

Log levels:

- `error`: kernel-integrity issues, unexpected failures.
- `warn`: validator rejections, retried tool failures, deferred work.
- `info`: run lifecycle events (start, complete), notable state changes.
- `debug`: per-step internals (off by default in production).

PII redaction applies to logs as it does to model context. The `pii.redact` guard is also a log filter.

## Debugging guide

When a customer reports "this run produced an unexpected output":

1. **Open the audit pack.** `provenant audit pack <run_id>` reproduces the full state.
2. **Check the trace.** Find the OpenTelemetry trace by `run_id`; identify the slow or failing span.
3. **Examine the trajectory.** `provenant audit show <run_id>` lists every tool call and guard run.
4. **Check the token tree.** `provenant tokens tree <run_id>` displays derivation chains; visualize which numbers came from where.
5. **Replay.** `provenant replay <run_id> --n 10` reproduces the run; compare across replays to identify variance.
6. **Compare to case bank.** If a similar case is in the bank with a different correct answer, the diff guides investigation.

When a metric crosses an alert threshold:

1. **Find the run.** Most metrics carry `run_id` as a label or example.
2. **Pull the trace.** Slow/failing spans surface the root cause.
3. **Check skill version.** A regression often correlates with a recent skill version bump; `provenant skill diff <ver_a> <ver_b>` shows changes.

## Health endpoints

```
GET /healthz                  liveness (no auth)
GET /readyz                   readiness (Postgres + object store reachable; no auth)
GET /v1/admin/health          detailed health (admin token; tenant-specific subsystem statuses)
```

## Local dev observability stack

`docker-compose.yml` includes:

- Postgres (with `pg_stat_statements`)
- MinIO (S3-compatible)
- Prometheus + Grafana (with default dashboards in `ops/grafana/`)
- Jaeger (for trace inspection)

Default dashboards: "Run overview," "Per-tool latency," "Validator outcomes," "Audit chain health."

## Open questions

1. **Sampling traces.** For high-volume tenants, trace sampling may be needed. Proposal: per-tenant configurable rate; never sample audit-relevant spans (always-on for `submit_answer` and validators).
2. **Log retention.** Logs vs. audit entries have different retention requirements; logs may rotate weekly while audit retains 7 years. Codify in operator docs.
3. **Per-tenant Grafana dashboards.** Multi-tenant deployments may want per-tenant dashboards. Proposal: ship a templated dashboard with tenant as a variable; SaaS deployment can fork.

## References

- OpenTelemetry: https://opentelemetry.io
- Prometheus: https://prometheus.io
- See [`subsystems/audit.md`](../subsystems/audit.md) for the audit-vs-observability split
