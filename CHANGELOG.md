# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

This project is **pre-1.0**. Breaking changes may occur between any two commits until a stable release is tagged.

## [Unreleased]

### Added

- **Kernel**: core provenance engine — deterministic token minting from structured input records, with cryptographic chaining across derived values.
- **Token minting**: mint provenance tokens capturing source, selector, hash, and derivation ancestry; tokens are content-addressed and replay-stable.
- **Bundle verify and replay**: bundle format with integrity verification and full replay support, allowing any derivation chain to be audited offline.
- **CLI**: command-line surface for minting tokens, verifying bundles, and inspecting provenance output.
- **Derivation-graph demo**: scaled end-to-end demonstration of multi-hop derivation graphs, confirming chain integrity across realistic record volumes.
