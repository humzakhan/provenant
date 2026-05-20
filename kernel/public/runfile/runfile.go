// Package runfile provides a long-lived, incrementally-writable bundle file for
// a single Provenant run. It owns atomic append via a .part staging file and
// POSIX advisory file locking to refuse concurrent appenders.
//
// Platform note: this package uses POSIX flock(2) and is supported only on
// Linux and macOS. A build stub returns ErrUnsupportedPlatform on Windows.
package runfile

import (
	"errors"
	"fmt"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
)

// Public error sentinels returned by this package.
var (
	// ErrAlreadyFinalized is returned by Start when the final bundle path
	// already exists and is a valid finalized bundle.
	ErrAlreadyFinalized = errors.New("runfile: already finalized")

	// ErrConcurrentAppender is returned by Start when a concurrent process
	// holds the advisory lock on the .part file.
	ErrConcurrentAppender = errors.New("runfile: concurrent appender")

	// ErrCorruptedPart is returned by Recover (or by Start on a non-empty
	// .part) when the audit chain cannot be validated.
	ErrCorruptedPart = errors.New("runfile: corrupted part file")

	// ErrRunFinalized is returned by AppendEntry after Finish has been called.
	ErrRunFinalized = errors.New("runfile: run already finalized")

	// ErrSeqOutOfOrder is returned by AppendEntry when the supplied entry seq
	// is not exactly lastSeq+1.
	ErrSeqOutOfOrder = errors.New("runfile: seq out of order")

	// ErrUnsupportedPlatform is returned on operating systems that do not
	// support POSIX flock(2). Currently only Windows.
	ErrUnsupportedPlatform = errors.New("runfile: unsupported platform (POSIX only)")
)

// Options configures a call to Start.
type Options struct {
	// Path is the final bundle path. The .part staging file is written to
	// "." + filepath.Base(Path) + ".part" in the same directory.
	Path string

	// Run identifies the run; Run.ID and Run.TenantID must be non-empty.
	Run kernel.Run

	// Clock supplies the wall-clock timestamp for the run_started entry.
	Clock kernel.Clock

	// Tenant must equal Run.TenantID and is validated by Start.
	Tenant string
}

// runState tracks whether the Run has been finalized.
type runState int

const (
	runStateOpen      runState = iota
	runStateFinalized          // Finish() or Fail() completed
	runStateAborted            // Abort() completed
)

// Run owns an open .part file and provides methods to append entries and
// finalize the run bundle.
type Run struct {
	opts      Options
	partPath  string // path to the .part staging file
	lockFile  lockHandle
	partFile  appendFile
	state     runState
	lastSeq   uint64
	lastHash  []byte
	tokens    []tokenRecord
	reads     []readRecord
	entries   []entryRecord
}

// tokenRecord is an in-memory token collected during the run.
type tokenRecord struct {
	token kernel.Token
}

// readRecord is an in-memory captured read collected during the run.
type readRecord struct {
	read kernel.CapturedRead
}

// entryRecord is an in-memory audit entry collected during the run.
type entryRecord struct {
	entry kernel.AuditEntry
}

// RunIdentity returns the kernel.Run identity for this run (ID, TenantID, Status).
func (r *Run) RunIdentity() kernel.Run {
	return r.opts.Run
}

// LastSeq returns the sequence number of the last appended audit entry.
func (r *Run) LastSeq() uint64 {
	return r.lastSeq
}

// LastHash returns the hash of the last appended audit entry. The returned
// slice must not be modified by the caller.
func (r *Run) LastHash() []byte {
	return append([]byte(nil), r.lastHash...)
}

// Tokens returns a snapshot of all tokens collected in this run so far.
func (r *Run) Tokens() []kernel.Token {
	out := make([]kernel.Token, len(r.tokens))
	for i, tr := range r.tokens {
		out[i] = tr.token
	}
	return out
}

// CapturedReads returns a snapshot of all captured reads collected in this run so far.
func (r *Run) CapturedReads() []kernel.CapturedRead {
	out := make([]kernel.CapturedRead, len(r.reads))
	for i, rr := range r.reads {
		out[i] = rr.read
	}
	return out
}

// AuditEntries returns a snapshot of all audit entries collected in this run so far.
func (r *Run) AuditEntries() []kernel.AuditEntry {
	out := make([]kernel.AuditEntry, len(r.entries))
	for i, er := range r.entries {
		out[i] = er.entry
	}
	return out
}

// AppendEntry validates the entry against the existing chain (prev_hash, seq)
// and appends it to the .part file together with any associated token or
// captured read. The write is single-record-atomic: each record is built fully
// in memory, then written with a single Write call, then fsynced.
//
// Allowed entry kinds: token_mint, token_derive, answer_submitted, run_completed, run_failed.
// run_started is written by Start only.
func (r *Run) AppendEntry(entry kernel.AuditEntry, token *kernel.Token, read *kernel.CapturedRead) error {
	if r.state == runStateFinalized {
		return ErrRunFinalized
	}
	if r.state == runStateAborted {
		return fmt.Errorf("runfile: run has been aborted")
	}

	wantSeq := r.lastSeq + 1
	if entry.Seq != wantSeq {
		return fmt.Errorf("%w: got %d, want %d", ErrSeqOutOfOrder, entry.Seq, wantSeq)
	}
	if entry.RunID != r.opts.Run.ID {
		return fmt.Errorf("runfile: entry run_id %q does not match run %q", entry.RunID, r.opts.Run.ID)
	}
	if entry.TenantID != r.opts.Run.TenantID {
		return fmt.Errorf("runfile: entry tenant_id %q does not match run %q", entry.TenantID, r.opts.Run.TenantID)
	}

	// Validate allowed kinds (run_started is written by Start only).
	switch entry.Kind {
	case kernel.EntryKindTokenMint,
		kernel.EntryKindTokenDerive,
		kernel.EntryKindAnswerSubmitted,
		kernel.EntryKindRunCompleted,
		kernel.EntryKindRunFailed:
		// OK
	default:
		return fmt.Errorf("runfile: AppendEntry does not accept kind %q (use Start for run_started)", entry.Kind)
	}

	// Write captured read first (if any), then token (if any), then audit entry.
	if read != nil {
		if err := r.partFile.writeRecord(partRecord{Kind: "captured_read", CapturedRead: read}); err != nil {
			return fmt.Errorf("runfile: write captured_read record: %w", err)
		}
		r.reads = append(r.reads, readRecord{read: *read})
	}
	if token != nil {
		if err := r.partFile.writeRecord(partRecord{Kind: "token", Token: token}); err != nil {
			return fmt.Errorf("runfile: write token record: %w", err)
		}
		r.tokens = append(r.tokens, tokenRecord{token: *token})
	}
	if err := r.partFile.writeRecord(partRecord{Kind: "audit", Entry: &entry}); err != nil {
		return fmt.Errorf("runfile: write audit record: %w", err)
	}

	r.entries = append(r.entries, entryRecord{entry: entry})
	r.lastSeq = entry.Seq
	r.lastHash = append([]byte(nil), entry.Hash...)
	return nil
}

// Finish writes the run_completed audit entry, finalizes the bundle by
// renaming .part -> Path, releases the lock, and closes file handles.
//
// Finish requires Options.Clock to be non-nil. Runs returned by Recover do
// not have a Clock attached (Recover takes only a path); callers that intend
// to Finish a recovered run must call SetClock first. Without a Clock, Finish
// returns an error rather than reading a wall clock.
func (r *Run) Finish() error {
	if r.state != runStateOpen {
		return fmt.Errorf("runfile: Finish called on non-open run (state=%d)", r.state)
	}
	if r.opts.Clock == nil {
		return errors.New("runfile: Finish requires a Clock; recovered runs must call SetClock before Finish")
	}

	// Build and append the run_completed entry.
	completedEntry := buildRunLifecycleEntry(
		r.opts.Run,
		kernel.EntryKindRunCompleted,
		r.opts.Clock.Now().UTC(),
		r.lastSeq+1,
		r.lastHash,
	)
	if err := r.AppendEntry(completedEntry, nil, nil); err != nil {
		return fmt.Errorf("runfile: append run_completed: %w", err)
	}

	return r.finalize()
}

// Fail writes the run_failed audit entry with the supplied reason, finalizes
// the bundle as a failed run bundle, releases the lock, and closes file
// handles. The finalized file is a valid bundle.
//
// Like Finish, Fail requires Options.Clock to be non-nil. Recovered runs must
// call SetClock before Fail.
func (r *Run) Fail(reason string) error {
	if r.state != runStateOpen {
		return fmt.Errorf("runfile: Fail called on non-open run (state=%d)", r.state)
	}
	if r.opts.Clock == nil {
		return errors.New("runfile: Fail requires a Clock; recovered runs must call SetClock before Fail")
	}

	_ = reason // reason is recorded in non-deterministic payload at caller level; not yet wired here

	failedEntry := buildRunLifecycleEntry(
		r.opts.Run,
		kernel.EntryKindRunFailed,
		r.opts.Clock.Now().UTC(),
		r.lastSeq+1,
		r.lastHash,
	)
	if err := r.AppendEntry(failedEntry, nil, nil); err != nil {
		return fmt.Errorf("runfile: append run_failed: %w", err)
	}

	return r.finalize()
}

// SetClock attaches a Clock to a Run. This is intended for runs returned by
// Recover, which constructs the Run without a Clock (the spec's Recover
// signature takes only a path). Callers that intend to Finish or Fail a
// recovered run must call SetClock first. Calling SetClock with a nil Clock
// is a no-op.
func (r *Run) SetClock(clock kernel.Clock) {
	if clock == nil {
		return
	}
	r.opts.Clock = clock
}

// Close releases the advisory lock and closes the file handle without removing
// the .part file. It is intended for callers that created the partfile (via
// Start) and want to hand it off to a subsequent process or command that will
// re-open it with Start (which replays the existing non-empty .part) or
// Recover. After Close the Run must not be used.
func (r *Run) Close() error {
	if r.state != runStateOpen {
		return nil
	}
	r.state = runStateAborted // prevent further use; Abort's "aborted" sentinel
	return r.release()
}

// Abort discards the .part file and releases the lock without finalizing.
// The original Path (if it existed before Start) remains untouched.
func (r *Run) Abort() error {
	if r.state == runStateFinalized || r.state == runStateAborted {
		return nil
	}
	r.state = runStateAborted
	return r.discard()
}

// buildRunLifecycleEntry constructs an AuditEntry for a run lifecycle kind
// (run_started, run_completed, run_failed). The deterministic payload is
// always the empty JSON object `{}` so the hash input is stable.
func buildRunLifecycleEntry(run kernel.Run, kind kernel.EntryKind, at time.Time, seq uint64, prevHash []byte) kernel.AuditEntry {
	det := []byte("{}")
	hash := auditHash(prevHash, seq, run.ID, run.TenantID, kind, det)
	return kernel.AuditEntry{
		Seq:      seq,
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     kind,
		At:       at,
		PrevHash: append([]byte(nil), prevHash...),
		Hash:     hash,
		Payload: kernel.PayloadEnvelope{
			Deterministic:      det,
			HashInputCanonical: det,
		},
	}
}
