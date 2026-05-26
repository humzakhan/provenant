# Open Design Questions

These are known unresolved architectural questions in the Provenant codebase. They are recorded here so contributors can see where decisions are still open and can propose solutions. Each question references the existing architecture documents where the relevant context lives.

---

## 1. `system:` sentinel vocabulary as a kernel-public constant

The four legal `system:` principals (`system:escalation-sweep`, `system:orphan-sweep`, `system:dr-restore`, `system:audit-verify`) are documented in [`architecture/cross-cutting/security.md`](../architecture/cross-cutting/security.md) but are not exported as a kernel-public constant. Any cross-language binding that needs to reference these sentinels must hard-code the string literals, creating a maintenance hazard if the vocabulary is ever amended.

**Open question:** Should these four strings be defined as exported constants in a `kernel/public/` package, and if so, which package is the right home?

---

## 2. Normalizer compile-time registration

ADR-0006 was amended repeatedly. The decision between builtin-compile-time registration and a runtime-registry model is implemented in `kernel/internal/normalizers/registry.go` but lacks a dedicated ADR capturing the final rationale. The implications for `kernel_version` bumps when the builtin set changes are not explicitly specified anywhere.

**Open question:** Should a dedicated ADR document the compile-time-registry decision and its `kernel_version` bump policy? See [`architecture/adrs/`](../architecture/adrs/) for the existing ADR set.

---

## 3. `credential_id_provider` recording at run start

The `run_started` audit entry records only a `credential_id → hash` mapping, not which secret store backend backed each credential. This means an audit bundle does not record whether a credential came from an environment variable, a secrets manager, a vault, or a local key file.

**Open question:** Should `run_started` include a non-deterministic `credential_id_provider` map (e.g. `{"cred_foo": "env"}`)? Adding it would widen the observable audit surface but would require a `kernel_version` bump and would introduce a field that cannot be verified on replay. See [`architecture/subsystems/audit.md`](../architecture/subsystems/audit.md) for the `run_started` payload schema.

---

## 4. Phase-0 public-surface snapshot list

`kernel/public/snapshot_test.go` uses a hand-maintained list of kernel public types. The same types are described in the "Kernel public types" table in [`architecture/subsystems/kernel.md`](../architecture/subsystems/kernel.md). These two lists can drift independently with no automated check.

**Open question:** Should the snapshot list be auto-generated from the architecture table (or vice versa), or is hand-maintenance with a documented review step sufficient? A drift between the two is currently a latent correctness risk rather than a CI-enforced failure.
