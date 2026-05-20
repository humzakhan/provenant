# Phase 04 — Run Lifecycle: Long-Lived Bundle File

## Objective

Today the CLI produces a bundle as a single end-of-run snapshot. The Derivation Graph needs many incremental writes within one run: each `read-mint`, each `calc <op>`, and the eventual `submit-answer` all append one audit entry to the same chain. Phase 04 introduces a `runfile` package that owns a long-lived bundle file, provides atomic append, refuses concurrent appenders, and crash-safely finalizes the file when the run completes.

## Scope (in)

- New package `kernel/public/runfile`.
- File format: a streaming representation of the existing bundle document, written as `kernel.RunDescriptor` header + per-entry JCS-canonicalized JSONL records, followed by a finalize record that re-emits the consolidated `bundle.json` shape on `run finish`.
- Extend `kernel/public/bundle/bundle.go` `normalizeState` dispatch to also accept `EntryKindRunStarted`, `EntryKindRunCompleted`, and `EntryKindRunFailed` audit entries (they produce no token and carry no payload beyond seq/prev_hash bookkeeping). Phase 01's dispatch only knew `token_mint` and `token_derive`; without this extension, any finalized run-lifecycle bundle would fail `bundle.Verify` with `ErrTamperDetected`.
- POSIX advisory file lock (`flock`) to refuse concurrent appenders.
- Atomic append via append-only writes to `.<bundle>.part`, with periodic fsync. Finalize renames `.part` to the final path.
- Crash-recovery: opening an existing `.part` with no matching lockfile re-validates the chain and truncates any half-written trailing record.

## Scope (out)

- No CLI changes (Phase 05).
- No new ops.
- No bundle format change visible to `bundle.Replay` after finalize — the finalized file is byte-identical to what Phase 01 / 02 bundles produce today.
- No network locking. Single host, single process.

## Technical Design

### Package shape

```go
// kernel/public/runfile/runfile.go
package runfile

type Options struct {
    Path     string         // final bundle path; partfile is "." + base + ".part"
    Run      kernel.Run
    Clock    kernel.Clock
    Tenant   string         // must equal Run.TenantID
}

type Run struct {
    // unexported fields: lockFile *os.File, partFile *os.File, state runState
}

// Start opens or creates a run file. If Path already exists and is a finalized
// bundle, returns ErrAlreadyFinalized. If the .part file already exists and
// holds the advisory lock, returns ErrConcurrentAppender. Otherwise it
// acquires the lock, writes the header record (run_started audit entry),
// and returns a *Run ready for AppendEntry.
func Start(opts Options) (*Run, error)

// AppendEntry validates the entry against the existing chain (prev_hash, seq)
// and appends it to the .part file. The write is single-record-atomic: the
// record is built fully in memory, then written with a single Write call,
// then fsynced.
func (r *Run) AppendEntry(entry kernel.AuditEntry, token *kernel.Token, read *kernel.CapturedRead) error

// Finish writes the run_completed audit entry, renames .part -> Path
// atomically, releases the lock, and closes file handles.
func (r *Run) Finish() error

// Abort discards the .part file and releases the lock without finalizing.
// The original Path remains untouched.
func (r *Run) Abort() error

// Recover opens an existing .part for inspection or resumption. If no lock
// is held, it acquires the lock, validates the chain up to the last fully
// written record, truncates anything beyond, and returns a *Run.
func Recover(path string) (*Run, error)
```

### Partfile format

The `.part` file is a streaming superset of the finalized bundle. Records are JCS-canonicalized JSON objects, one per line:

```
{"kind":"header","format":"phase1.bundle/v1","run":{...}}
{"kind":"captured_read","captured_read":{...}}
{"kind":"token","token":{...}}
{"kind":"audit","entry":{...}}
{"kind":"audit","entry":{...}}
...
{"kind":"footer","sha256":"<sha256 of canonical bundle bytes>"}
```

Finalize transforms the partfile to a `phase1.bundle/v1` document by:

1. Reading all records.
2. Sorting captured reads by ID and tokens by ID (matching `bundle.Emit`).
3. Marshaling the same `document` struct from `kernel/public/bundle/bundle.go`.
4. Running `jcs.Transform` on the result.
5. Writing the canonical bytes to a sibling temp file, fsyncing, then renaming over `Path`.

### Locking

Use `golang.org/x/sys/unix.Flock` (already transitively available via `go.sum`; if not, vendor `github.com/gofrs/flock`). Lock the partfile itself with `LOCK_EX | LOCK_NB`. If the lock is already held, return `ErrConcurrentAppender` immediately — no blocking wait. On Finish/Abort, release the lock and close the file.

Windows portability is explicitly out of scope. Document this.

### Atomicity guarantees

- **Per-record**: each `AppendEntry` builds the line in memory, calls `f.Write(line)` once, then `f.Sync()`. If the process dies mid-write, the next `Recover` truncates the partial trailing line.
- **Finalize**: the canonical bundle is built in memory, written to `Path + ".tmp"`, fsynced, then `os.Rename(tmp, Path)`. The partfile is removed only after the rename succeeds.
- **Crash before finalize**: `Path` is absent. `Path + ".part"` contains the canonical chain up to the last fsynced record. Re-opening yields the partial run; the operator can resume or abort.
- **Crash during finalize**: either `Path.tmp` exists (resume by deleting it and replaying finalize) or `Path` exists (run is done).

### Audit chain semantics

The header record's `run_started` audit entry is `seq=1`. Every subsequent `AppendEntry` is `seq=N+1`. `run_completed` is the last entry; once written, no more entries are allowed.

`AppendEntry` enforces:

- `entry.Seq == lastSeq + 1`
- `entry.RunID == opts.Run.ID`
- `entry.TenantID == opts.Run.TenantID`
- `entry.PrevHash == lastHash`
- `entry.Kind ∈ {token_mint, token_derive, run_completed, run_failed}` (header `run_started` is written by `Start` only)

## Implementation Steps

1. Create `kernel/public/runfile/` with `runfile.go`, `partfile.go`, `lock_unix.go`, and `recover.go`.
2. Implement `Start`:
   - Create or open `.part` with `O_RDWR|O_CREATE`.
   - Acquire `flock(LOCK_EX|LOCK_NB)`; return `ErrConcurrentAppender` if EAGAIN.
   - If the file is non-empty, read existing records and validate the chain. If valid, position at EOF for append. If invalid, return `ErrCorruptedPart`.
   - If empty, write header + `run_started` audit entry.
3. Implement `AppendEntry`. Reject double-writes (same seq) by verifying against in-memory state, not by re-reading the file.
4. Implement `Finish`:
   - Append `run_completed` entry.
   - Build canonical bundle via `bundle.Emit` from collected state.
   - Write to `Path.tmp`, fsync, rename.
   - Remove partfile, release lock.
5. Implement `Abort`:
   - Remove partfile, release lock.
6. Implement `Recover` as described above.
7. Extend `kernel/public/bundle/bundle.go` `normalizeState` to dispatch on `EntryKindRunStarted`, `EntryKindRunCompleted`, and `EntryKindRunFailed`: validate seq/prev_hash linkage, do not require a paired token, do not contribute to `referencedReadIDs`. Update the `len(tokens)` invariant to `len(tokens) == count(entries where kind in {token_mint, token_derive})`. Keep the regression: pre-Phase-01 fixtures (which contain only `token_mint` entries) still verify.
8. Add `kernel/public/runfile/runfile_test.go` with the cases below.

## Tests Required

| Test | Asserts |
|---|---|
| Start → append × 3 → Finish | Bundle byte-identical to direct `bundle.Emit` from same in-memory state |
| Concurrent Start | Second `Start` against the same path returns `ErrConcurrentAppender` immediately |
| Truncated partfile recovery | Manually truncate the last line by N bytes; `Recover` accepts and truncates the partial line |
| Tampered partfile recovery | Flip a byte mid-record; `Recover` returns `ErrCorruptedPart` |
| Append after Finish | Returns `ErrRunFinalized` |
| Abort | Partfile removed; `Path` does not exist |
| Crash during finalize | Simulated by writing `Path.tmp` then exiting; `Start` on the same path cleans up `.tmp` and reattaches the partfile |
| Seq monotonicity | Appending a non-monotonic seq returns `ErrSeqOutOfOrder` |
| Bundle backward compat | Existing reconciliation fixture still produces byte-identical bundles when migrated to use `runfile` internally |

## Definition of Done

- [ ] Code merged to `main`.
- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] Public API is `runfile.Start`, `runfile.Recover`, and the `*Run` methods listed above. No other exports.
- [ ] Finalized bundles are byte-identical to bundles produced by direct `bundle.Emit` on the same logical state — verified by a comparison test that runs both paths on the reconciliation fixture.
- [ ] Concurrent-append refusal is exercised by an actual two-process test (`exec.Command` of the test binary with a flag), not a mock.
- [ ] Pre-existing bundles still verify and replay.

## Risks / Open Questions

- `golang.org/x/sys/unix.Flock` is the right primitive on Linux and macOS but not Windows. Phase 04 declares the package POSIX-only and gates the file with a `//go:build !windows` constraint plus a stub returning `ErrUnsupportedPlatform` on Windows. Acceptable for now; CI runs on Linux and macOS.
- fsync per append has perf cost. Acceptable: runs are dozens to low hundreds of entries, not millions.
- Should `run_failed` be a normal terminal kind callable from `Abort`? Yes — `Abort(reason string)` writes `run_failed` then finalizes, vs. `Abort()` removes the partfile. We expose both as `Abort` (discard) and `Fail(reason)` (preserve and finalize as failed).

## Dependencies

- [Phase 01](./phase-01-token-dag-schema.md) (bundle accepts derive entries).
- Phase 02 and 03 are not strictly required for Phase 04 to land, but Phase 04 is only useful when ops exist. Schedule Phase 04 after Phase 03 to minimize churn.
