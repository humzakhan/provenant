# Match Backend: pgvector

Status: **v0 / reference**

> Vector-similarity match backend built on Postgres + the pgvector extension. Encodes each token-bearing field with an operator-configured embedding model, persists vectors per tenant, and serves cosine-similarity proposals back to the `match` category. Provenant does **not** bundle an embedding model; the operator picks one — the same pattern as [`tools/policy/backends/pgvector.md`](../../policy/backends/pgvector.md).

## Connection

- **Database**: the tenant's existing Postgres instance (the same canonical store used for `audit_entries`, `tokens`, etc.). The `pgvector` extension must be installed and enabled.
- **Auth**: re-uses the runtime layer's existing Postgres connection pool; no new credential is introduced. The backend's `Credentials()` accessor returns the same `CredentialRef.ID` as the canonical Postgres store (typical: `pg.canonical`), so the skill pack's `tools.yaml` declares the same `credential_id` it already declares for any other Postgres-backed tool.
- **Embedding model client**: operator-configured. The runtime layer's `ModelClient` abstraction has an optional `Embeddings(text []string) [][]float32` method; backends pick from the supported providers (Anthropic, OpenAI, a local BGE/GTE serving endpoint, etc.). The choice is recorded as part of `backend_version` (see "Backend version" below).

## Schema mapping

The backend persists two tables per tenant:

```sql
CREATE TABLE match_vectors (
    tenant_id        TEXT NOT NULL,
    token_id         TEXT NOT NULL,
    field_name       TEXT NOT NULL,
    embedding        vector(N),       -- N = embedding model dimension; pinned per backend_version
    backend_version  TEXT NOT NULL,   -- pgvector@<ver>+<embedding_model>@<index_version>
    created_at       TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, token_id, field_name, backend_version)
);

CREATE INDEX idx_match_vectors_ann
    ON match_vectors USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
```

On a `match.pair`, `match.candidates`, or `match.cross_set` call the backend:

1. Looks up vectors for every input `token_id` × `field_name` pair under the current `backend_version`. If a vector is missing, it computes one on the fly via the operator-configured embedding model and persists it.
2. For each candidate pair (left × right), computes weighted cosine similarity: `sum_f(weight_f × cos(left_f, right_f)) / sum_f(weight_f)`. `field_weights` come from `method_config.field_weights`; missing weights default to equal weighting across declared fields.
3. Applies the calibration map (similarity → confidence; see below) to produce `confidence`.
4. Sorts the resulting proposals per the category's deterministic-order rule before returning.

## Method support

| `method` | Supported | Notes |
|---|---|---|
| `cosine_embedding` | yes | The reference method for this backend |
| `jaccard` | no | Use an `in-process-onnx` or future lexical backend |
| `levenshtein_normalized` | no | Same |

A skill that configures `match.method_allowed: ["cosine_embedding"]` and binds the pgvector backend is internally consistent. A skill that names a method this backend does not support produces `match.method_not_allowed` at validator time, **before** the backend is dispatched (the validator is part of the pre-dispatch guard pipeline).

## Captured reads

`CapturedDeterministic`. The backend's raw output for each `match.*` call is captured:

```
match:pgvector:<tool>:<sha256_of_input_plus_backend_version>
```

Captured payload:

```json
{
  "backend_version": "pgvector@0.7.4+oai-text-embedding-3-small@2024-01",
  "proposals_raw": [
    {
      "left_token_id": "tk_01H...",
      "right_token_id": "tk_01H...",
      "similarity_score_raw": "0.9432100",
      "calibrated_confidence": "0.9120000"
    }
  ]
}
```

The captured payload is the input to the category's deterministic-sort step; replay reproduces the same sorted output even if the backend in a fresh run would return rows in a different permutation.

## Backend version

`backend_version` is the string the category records on every proposal:

```
pgvector@<pgvector_ext_version>+<embedding_provider>-<embedding_model>@<index_version>
```

- `pgvector_ext_version` — the Postgres extension version (`SELECT extversion FROM pg_extension WHERE extname = 'vector'`).
- `embedding_provider` — `oai`, `anthropic`, `local-bge`, etc.
- `embedding_model` — the provider's model identifier.
- `index_version` — bumped when the operator regenerates vectors (e.g., model swap, index rebuild). Vectors persist under the version that produced them; queries scoped to a `backend_version` see only the matching vectors.

Two captured runs against different `backend_version` strings cannot be cross-compared; each captures its own replay lineage.

## Calibration map

The backend ships a default calibration table mapping `similarity_score` (cosine) to `confidence`, recomputed quarterly against the case bank. The table is a piecewise-linear map persisted at:

```
match_pgvector_calibration:
  - similarity: 0.95, confidence: 0.99
  - similarity: 0.90, confidence: 0.95
  - similarity: 0.85, confidence: 0.88
  - similarity: 0.75, confidence: 0.70
  - similarity: 0.65, confidence: 0.50
```

The calibration map is part of `backend_version` (a recalibration bumps `index_version`). Operators who need a custom calibration override the default map in the backend's config and the override's content hash contributes to `backend_version`.

## Rate limits and retries

- Embedding model calls: per the provider's rate limits. Default retry policy: 3 attempts with exponential backoff (250ms, 1s, 4s). Beyond that, the call fails with `match.backend_unavailable`.
- pgvector queries: bounded by the canonical Postgres pool's existing limits; no separate budget.

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `match.backend_unavailable` | Embedding provider unreachable, or pgvector extension not enabled | Surface to skill; do not retry indefinitely (use the captured-replay path for prior runs) |
| `match.input_token_unknown` | A `token_id` on `left` or `right` is not minted on the current run | Hard rejection; the category-layer check fires before the backend dispatches |
| `tool.timeout` | The combined embedding + similarity step exceeded the 30-second per-call budget | Skill should retry with smaller `top_k` or batch the cross-set call |

## Tests

- Schema-conformance tests against fixture vectors (deterministic embeddings supplied by a test stub) verify cosine arithmetic.
- Captured-read replay test: a recorded run replays identically without re-calling the embedding model.
- Calibration test: the default calibration map produces confidence scores within ±0.02 of the documented values against the case bank's labeled vendor-dedupe cases.
- Determinism test: 1000 runs of the same `cross_set` input produce byte-identical proposals.

## Configuration

```yaml
match_backends:
  - id: pgvector
    method: cosine_embedding
    embedding:
      provider: openai
      model: text-embedding-3-small
    index:
      lists: 100
      version: "2026-05"
    calibration:
      # optional override; omit for default
      map:
        - {similarity: 0.95, confidence: 0.99}
        - {similarity: 0.90, confidence: 0.95}
        - {similarity: 0.85, confidence: 0.88}
        - {similarity: 0.75, confidence: 0.70}
        - {similarity: 0.65, confidence: 0.50}
```

## References

- [`tools/match/README.md`](../README.md)
- [`tools/policy/backends/pgvector.md`](../../policy/backends/pgvector.md) — the precedent for operator-configured embedding under pgvector
- pgvector: https://github.com/pgvector/pgvector
