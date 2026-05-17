# ADR-0004: Token-level grounding (not document-level citation)

Status: **Accepted**
Date: 2026-05-17

## Context

The "grounding" pattern for LLM outputs has converged in industry on document-level citation: every claim in an output points to source documents (passages, page numbers, sometimes bounding boxes). [Moody's](../references.md#moodys-modular) ships memos with passage citations; mainstream RAG products work this way; Atlan's [analysis](../references.md#atlan-hallucination) describes citation-required outputs as the high-confidence pattern.

Document-level citation is necessary but insufficient for finance. A claim like "Revenue grew 18% YoY" can cite the 10-K (correct source) while stating a fabricated number (wrong value), and the document-level cite passes its check. Token grounding closes this gap.

## Decision

Provenant grounds at the per-value (token) level, not the document level. Every numeric value in the agent's final answer maps to exactly one provenance token, which carries a verifiable lineage to a tool call (and through the tool call, to a physical source — document, database row, API response).

Document-level citation is layered *on top* of token grounding for tools that retrieve documents (`policy.lookup`, `extract.*`): the token's `source.origin` includes a document URL and page reference. But the *verification* gate is token-level: the kernel checks numbers against tokens, not claims against documents.

## Rationale

**Document citation is too loose for finance.** "Revenue grew 18% YoY (10-K page 12)" satisfies a document-citation check while being false. Token grounding rejects this because there is no token in the run carrying the value 18% with a derivation chain that bottoms out at the 10-K's revenue lines.

**Token grounding is the only way to enforce no-ungrounded-numbers.** Numbers are the only category of claim where a deterministic gate is feasible. Free text *opinions* and *analyses* can't be ground-truth verified at write time; numbers can. So we constrain the answer-text contract to: text is free, numbers must be tokens.

**[DFAH](../references.md#dfah) findings reinforce this.** DFAH establishes that determinism and accuracy are independent: a model can produce consistent (deterministic) but wrong outputs. Token grounding is the mechanism that adds correctness checks distinct from determinism — by forcing every number to trace to verifiable lineage.

**It composes with document citation.** A token's lineage includes the originating document for extracted values, the database row for ledger values, the source ID for external values. So token-grounded outputs carry document-level citation as a side effect, with stronger guarantees.

## Consequences

**Positive:**
- "Ungrounded number rate = 0" is enforceable as a kernel invariant.
- Audit packs can render per-number provenance chains that an auditor can verify on paper.
- The `confidence.extraction_min` validator can be value-specific (each extracted value carries its own confidence; we don't approve the document and accept its values blindly).
- Derived numbers (variances, ratios, sums) carry full derivation chains automatically.

**Negative:**
- Tools must declare `MintablePaths` accurately and exhaustively. A missed path means a value can't be cited, which means it can't be used.
- Skill authors must structure prompts so the agent thinks in token terms, not document terms. The kernel header included in every run formalizes this.
- Performance overhead: every number flowing through the system gets a database row. Not a real constraint in practice but worth noting.

**Out of scope:**
- Text claims (e.g., "the policy classifies this expense as non-deductible") are not directly token-grounded. They are policy-cited via `policy.lookup` and verified by the `policy.citation_exists` validator. Validating *interpretive* claims is out of scope; we validate *factual* (numeric, structural) claims.

## Alternatives considered

**Document citation only.** Industry-standard, simpler. Rejected because it does not catch fabricated numbers within correctly-cited documents.

**Hash-based per-claim signing.** Sign each claim with the hash of the source span. Considered. Rejected because it doesn't catch derivations (computed values have no single source span) and doesn't enforce unit safety.

**Citation at the sentence level.** Granular but ad-hoc. Rejected because the unit of grounding should match the unit of computation (numbers), not the unit of presentation (sentences).

## References

- [DFAH](../references.md#dfah) — determinism vs accuracy distinction; token grounding addresses accuracy
- [Moody's modular AI](../references.md#moodys-modular) — document-level citation is the industry baseline we exceed
- See [`subsystems/kernel.md`](../subsystems/kernel.md) for the token data model
