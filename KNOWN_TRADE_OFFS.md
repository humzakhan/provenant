# Known Trade-Offs

> Accepted limitations documented during the Hone critique/defender loop for **provenant**.
> Each entry records what the limitation is, why it was accepted, what mitigates it, and what would trigger revisiting.

<!-- Entries are added by the Defender during the loop. Do not edit retroactively. -->

## Trade-off: Audit-pack tamper-evidence relies on internal hash chain only (no external notary)

**Limitation:** The v0 audit pack's tamper-evidence is a SHA-256 hash chain over canonicalized JSON, persisted in the customer's own Postgres and object store. There is no RFC 3161 trusted timestamping, no third-party notarization, no external witness, and no cryptographic signature from a Provenant-controlled or regulator-controlled key. An attacker with root and database write access on the customer's host can reconstruct a fully self-consistent chain from scratch; tampering by a privileged host operator leaves no external witness. This means "tamper-evident" in v0 is internally tamper-evident, not externally attested.
**Why accepted:** v0 targets single-tenant and air-gapped deployments operated by the customer themselves. In that threat model, the customer is the operator and the auditor of last resort; the customer's separation-of-duties controls (separate audit team, replicated audit log, periodic chain verification) are what give the chain its evidentiary weight. Adding an external notary (RFC 3161 TSA, a hosted Provenant attestation service, blockchain anchoring) before validating customer demand and regulator expectations would expand the surface area without confirmed need. The thesis claim is "tamper-detectable by the customer's audit team," which the hash chain satisfies; the stronger claim "tamper-evident to an external regulator without internal cooperation" is not made in v0.
**Mitigation:** (a) The threat-model section ([`architecture/cross-cutting/security.md`](architecture/cross-cutting/security.md)) explicitly scopes out root-equivalent compromise and documents the chain as detection-not-prevention. (b) Operational guidance includes audit-log replication to a separate write-once store and a separate-team audit-verification cadence. (c) The audit pack format is designed to accept additional attestation artifacts (signed PDFs, TSA tokens) as drop-in additions without schema change.
**Would revisit if:** A regulator or customer compliance team formally rejects an audit pack because it lacks an external notary; or competitive products ship RFC 3161 / TSA integration as table stakes; or Provenant moves into a multi-tenant SaaS model where Provenant itself is the operator and customers need an external witness against Provenant.
**First raised:** cycle 1
