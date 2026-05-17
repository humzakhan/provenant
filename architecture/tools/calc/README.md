# Tool Category: Calc

Status: **v0 / pure Go**

> Decimal-safe arithmetic primitives. Every numeric computation the agent performs goes through `calc.*` — never through LLM-generated arithmetic. Each call produces a **derived token** whose parents are the input tokens, recording the formula and providing full derivation chains.

## Depends on
- [`subsystems/tools.md`](../../subsystems/tools.md)
- [`subsystems/kernel.md`](../../subsystems/kernel.md) — derivation engine

## Depended on by
- Every skill pack that does arithmetic (essentially all of them)

## Purpose

LLMs cannot be trusted to do arithmetic, especially on monetary values. The error rate is too high (low single digits on simple sums, higher with units involved), and even when correct, the operation is unverifiable. Separating the *reasoning* (what to compute) from the *arithmetic* (compute it) — the [DCCD pattern](../../references.md#dccd) applied at the field level — gives us both correctness and provenance.

A skill pack instructs the agent: "you may state numeric conclusions only by passing source tokens through `calc.*` tools." The kernel's `submit_answer` enforces this — any numeric in the final answer not traceable to a token (which is either minted directly by a ledger/extract tool, or derived by calc) fails verification.

## Tools provided

| Tool name | Purpose | Determinism class |
|---|---|---|
| `calc.subtract` | a − b | PureDeterministic |
| `calc.add` | a + b | PureDeterministic |
| `calc.sum` | sum over a list | PureDeterministic |
| `calc.multiply` | a × b | PureDeterministic |
| `calc.divide` | a / b (with rounding mode) | PureDeterministic |
| `calc.ratio` | a / b → percentage | PureDeterministic |
| `calc.percent_change` | (after − before) / before | PureDeterministic |
| `calc.variance` | actual − budget, plus variance ratio | PureDeterministic |
| `calc.weighted_average` | Σ(weight × value) / Σ(weight) | PureDeterministic |
| `calc.convert_currency` | amount × FX rate, both as tokens | PureDeterministic |

All operate on `decimal.Decimal` values. The set is intentionally small; complex computations decompose into compositions.

## Schemas

### `calc.subtract`

Input:
```json
{
  "minuend":     {"token_id": "tk_01HXYZ_balance_bank"},
  "subtrahend":  {"token_id": "tk_01HXYZ_balance_gl"},
  "result_label": "bank_minus_gl_difference"
}
```

Output:
```json
{
  "result": {
    "token_id": "tk_01HXYZ_diff",
    "value": "237.42",
    "currency": "USD",
    "as_of": "2026-05-31T23:59:59Z",
    "derivation": {
      "operation": "arith.subtract",
      "inputs": ["tk_01HXYZ_balance_bank", "tk_01HXYZ_balance_gl"],
      "formula_id": "arith.subtract"
    }
  }
}
```

Mintable paths: none in the conventional sense — the kernel's *derivation engine* mints the result token. Calc tools work in concert with the kernel rather than through `MintablePaths`.

### `calc.sum`

Input:
```json
{
  "addends": [
    {"token_id": "tk_a"},
    {"token_id": "tk_b"},
    {"token_id": "tk_c"}
  ],
  "result_label": "total_deposits"
}
```

Output: same shape as subtract, with `inputs` listing all addend token IDs.

### `calc.convert_currency`

Input:
```json
{
  "amount":  {"token_id": "tk_amount_eur"},
  "fx_rate": {"token_id": "tk_fx_eur_usd_2026_05_31"},
  "direction": "multiply"   // amount × rate, with the rate's unit denominator validated
}
```

Output: token with currency = USD, derivation referencing both parents and the `arith.convert_currency` formula.

The formula is registered in the kernel's derivation engine with strict unit semantics: the FX rate's unit must match the source currency, and the output unit must match the target currency. Mismatch fails-closed.

## Unit safety

Calc tools enforce unit compatibility at the formula level:

- `subtract`, `add`, `sum`: all inputs must share the same currency (or be unit-less for non-monetary tokens). Mismatch fails-closed.
- `multiply`, `divide`: unit semantics depend on the formula. `amount × rate` → currency conversion; `count × rate` → derived rate; `amount / amount` → ratio (unit-less).
- `ratio`, `percent_change`: inputs must share unit; output is unit-less or percentage.
- `weighted_average`: weights are unit-less; values share unit; output shares value unit.

Trying to subtract USD from EUR (without an explicit currency conversion through `calc.convert_currency`) fails at the formula registry with `derivation.unit_mismatch`. The agent must explicitly convert first.

## Rounding

Calc operations use **banker's rounding** (round-half-to-even) at the currency's minimum unit:

- USD, EUR, GBP, etc.: 2 decimal places
- JPY: 0 decimal places
- BHD (Bahraini dinar): 3 decimal places
- BTC: 8 decimal places

The rounding step records the pre-rounding value in the derivation metadata for audit. Division uses `divide_to_decimal_places` with explicit rounding mode (default banker's).

## Derivation chains

A complex computation like "bank balance minus GL balance, divided by the prior month's total deposits, as a percentage" produces a derivation tree:

```
tk_pct  derivation:
        formula: "arith.percent"
        inputs:
          - tk_diff  derivation:
                     formula: "arith.subtract"
                     inputs: [tk_balance_bank, tk_balance_gl]
          - tk_prior_deposits  derivation:
                                formula: "arith.sum"
                                inputs: [tk_deposit_1, tk_deposit_2, ...]
```

The kernel's `audit pack` exporter renders these as readable derivation chains for auditors. Every leaf is a token minted by a non-calc tool (ledger, extract, external) — the chain bottoms out at real data.

## Determinism and replay

`PureDeterministic` everywhere: same inputs always produce the same output. Replay produces identical token IDs (because derivation token IDs are computed from `(operation, ordered_input_ids, input_values)` with a deterministic hash).

## Failure modes

| Code | When | Recommended handling |
|---|---|---|
| `calc.token_not_found` | Referenced token ID doesn't exist in this run | Skill must investigate; usually a bug in the agent's call construction |
| `calc.unit_mismatch` | Operands have incompatible units | Skill must add an explicit conversion (FX, unit cast) |
| `calc.division_by_zero` | Denominator is zero | Skill decides: surface as N/A or as a specific value |
| `calc.overflow` | Result exceeds `decimal.Decimal` representable range | Should not happen in practice; kernel-integrity event |
| `calc.invalid_formula` | Formula ID not registered | Code bug; surfaces as `tool_invalid_input` |

## Performance budget

All calc operations are sub-millisecond on the underlying decimal library. Per-call timeout 1 second. Pure CPU; no I/O.

## Backends

None. Pure-Go under `internal/tools/calc/`.

## Tests

- Unit: each formula on canonical inputs with known outputs.
- Property: commutativity (where applicable), associativity (where applicable), identity, zero, currency-precision edge cases.
- Property: same inputs across 1000 invocations produce identical token IDs.
- Stress: 10,000-element `calc.sum` completes in <100ms.

## Open questions

1. **Statistical aggregates**. Should `calc.median`, `calc.percentile`, `calc.stddev` be in the v0 set? Defer; current skill packs (recon, AP coding) don't need them.
2. **Cross-period escalation.** Tokens have an `as_of`; what's the behavior when summing tokens with different `as_of`? Proposal: allow it but warn; the result token takes the latest `as_of`. Validators may flag inconsistent `as_of` in downstream contexts.

## References

- [`subsystems/kernel.md`](../../subsystems/kernel.md) — derivation engine and unit safety enforcement
- [DCCD](../../references.md#dccd) — draft-then-enforce pattern that calc operationalizes at the field level
