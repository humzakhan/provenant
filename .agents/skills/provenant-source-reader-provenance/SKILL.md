---
name: provenant-source-reader-provenance
description: Use when implementing or reviewing source readers, captured reads, mintable paths, or tool outputs that feed token minting.
---

# Provenant Source Reader Provenance

Use this skill for source readers, extractors, captured reads, or any path that leads into token minting.

## Triggers
`source reader`, `extract`, `structured file`, `captured read`, `source hash`, `mintable_paths`, `selector`, `locator`, `raw_value`, `value_canonical`, `token mint`, `parser`, `options`

## Rules
- Readers return structured evidence, not `Token` values.
- The kernel/registry mints from declared mintable paths.
- Preserve source hash, parser/version/options, selector, locator, raw value, and canonical value where applicable.
- Fail closed on ambiguity, malformed input, missing selector, unsupported numeric forms, or source-hash mismatch.
- Keep reader coverage within the requested scope; do not widen into additional formats unless the user or an explicitly loaded spec asks for them.
