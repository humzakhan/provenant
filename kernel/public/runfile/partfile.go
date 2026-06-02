package runfile

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gowebpki/jcs"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/bundle"
)

// partRecord is one line in the .part JSONL streaming file.
// Exactly one of the payload fields is non-nil per record.
type partRecord struct {
	Kind         string               `json:"kind"`
	Header       *partHeader          `json:"header,omitempty"`
	CapturedRead *kernel.CapturedRead `json:"captured_read,omitempty"`
	Token        *kernel.Token        `json:"token,omitempty"`
	Entry        *kernel.AuditEntry   `json:"audit,omitempty"`
}

// partHeader is the first record in a .part file.
type partHeader struct {
	Format string     `json:"format"`
	Run    kernel.Run `json:"run"`
}

// appendFile wraps an open *os.File for append-only writes.
type appendFile struct {
	f        *os.File
	syncMode PartSyncMode
	dirty    bool
}

// writeRecord marshals rec to JCS-canonicalized JSON, appends a newline, writes
// in a single Write call, and syncs according to a.syncMode.
func (a *appendFile) writeRecord(rec partRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal partRecord: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return fmt.Errorf("canonicalize partRecord: %w", err)
	}
	line := append(canonical, '\n')
	if _, err := a.f.Write(line); err != nil {
		return fmt.Errorf("write partRecord: %w", err)
	}
	a.dirty = true
	if a.syncMode == PartSyncOnClose {
		return nil
	}
	return a.sync()
}

func (a *appendFile) sync() error {
	if a.f == nil || !a.dirty {
		return nil
	}
	if err := a.f.Sync(); err != nil {
		return err
	}
	a.dirty = false
	return nil
}

// partPath returns the .part file path for a given final bundle path.
func partFilePath(finalPath string) string {
	dir := filepath.Dir(finalPath)
	base := filepath.Base(finalPath)
	return filepath.Join(dir, "."+base+".part")
}

// Start opens or creates a run file. If Path already exists and is a finalized
// bundle, returns ErrAlreadyFinalized. If the .part file already exists and
// holds the advisory lock, returns ErrConcurrentAppender. Otherwise it
// acquires the lock, writes the header record (run_started audit entry),
// and returns a *Run ready for AppendEntry.
func Start(opts Options) (*Run, error) {
	if opts.Path == "" {
		return nil, errors.New("runfile: Options.Path must be non-empty")
	}
	if opts.Run.ID == "" || opts.Run.TenantID == "" {
		return nil, errors.New("runfile: Options.Run.ID and TenantID must be non-empty")
	}
	if opts.Tenant != opts.Run.TenantID {
		return nil, errors.New("runfile: Options.Tenant must equal Run.TenantID")
	}
	if opts.Clock == nil {
		return nil, errors.New("runfile: Options.Clock must be non-nil")
	}
	if opts.PartSync != PartSyncEveryRecord && opts.PartSync != PartSyncOnClose {
		return nil, errors.New("runfile: unsupported PartSync mode")
	}

	// If final path exists, it must be a valid finalized bundle.
	if _, err := os.Stat(opts.Path); err == nil {
		// Final path already exists — check if it's a valid bundle.
		data, readErr := os.ReadFile(opts.Path)
		if readErr == nil {
			if verifyErr := bundle.Verify(data); verifyErr == nil {
				return nil, ErrAlreadyFinalized
			}
		}
		// Not a valid bundle — fall through and attempt to use the .part file.
	}

	ppath := partFilePath(opts.Path)

	// Clean up any stale .tmp file from a previous crashed finalize.
	tmpPath := opts.Path + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		_ = os.Remove(tmpPath)
	}

	// Open or create the .part file.
	f, err := os.OpenFile(ppath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("runfile: open part file: %w", err)
	}

	// Acquire the advisory exclusive lock (non-blocking).
	lh, err := acquireLock(f)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, ErrConcurrentAppender) {
			return nil, ErrConcurrentAppender
		}
		return nil, fmt.Errorf("runfile: acquire lock: %w", err)
	}

	r := &Run{
		opts:     opts,
		partPath: ppath,
		lockFile: lh,
		partFile: appendFile{f: f, syncMode: opts.PartSync},
		state:    runStateOpen,
	}

	// Check if the .part file is non-empty (existing partial run).
	fi, err := f.Stat()
	if err != nil {
		_ = r.release()
		return nil, fmt.Errorf("runfile: stat part file: %w", err)
	}

	if fi.Size() > 0 {
		// Existing partial run: validate chain and position at EOF.
		if err := r.reloadFromPart(f); err != nil {
			_ = r.release()
			return nil, err
		}
	} else {
		// New .part file: write header + run_started.
		if err := r.writeHeader(); err != nil {
			_ = r.release()
			return nil, err
		}
	}

	return r, nil
}

// Recover opens an existing .part for inspection or resumption.
// If no lock is held, it acquires the lock, validates the chain up to the
// last fully written record, truncates anything beyond, and returns a *Run.
func Recover(path string) (*Run, error) {
	ppath := partFilePath(path)

	f, err := os.OpenFile(ppath, os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("runfile: open part file for recovery: %w", err)
	}

	lh, err := acquireLock(f)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, ErrConcurrentAppender) {
			return nil, ErrConcurrentAppender
		}
		return nil, fmt.Errorf("runfile: acquire lock for recovery: %w", err)
	}

	r := &Run{
		opts:     Options{Path: path},
		partPath: ppath,
		lockFile: lh,
		partFile: appendFile{f: f},
		state:    runStateOpen,
	}

	if err := r.reloadFromPart(f); err != nil {
		_ = r.release()
		return nil, err
	}

	return r, nil
}

// reloadFromPart reads and validates the existing .part content, collecting
// state into r. On any trailing partial line it truncates the file.
func (r *Run) reloadFromPart(f *os.File) error {
	// Seek to beginning to read full content.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("runfile: seek part file: %w", err)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	var validOffset int64
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		validOffset += int64(len(line)) + 1 // +1 for newline
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: scanner error: %v", ErrCorruptedPart, err)
	}

	// Parse records, rebuilding in-memory state.
	tokens, reads, entries, run, err := parsePartRecords(lines)
	if err != nil {
		return err
	}

	if run.ID == "" {
		return fmt.Errorf("%w: no header record found", ErrCorruptedPart)
	}

	// If opts.Run is empty (Recover path), populate from part header.
	if r.opts.Run.ID == "" {
		r.opts.Run = run
	}

	r.tokens = tokens
	r.reads = reads
	r.entries = entries

	if len(entries) > 0 {
		last := entries[len(entries)-1]
		r.lastSeq = last.entry.Seq
		r.lastHash = append([]byte(nil), last.entry.Hash...)
	}

	// Truncate any partial trailing content beyond the last valid newline.
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("runfile: stat for truncation: %w", err)
	}
	if fi.Size() > validOffset {
		if err := f.Truncate(validOffset); err != nil {
			return fmt.Errorf("runfile: truncate partial record: %w", err)
		}
	}

	// Position at EOF for further appends.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("runfile: seek to EOF: %w", err)
	}

	return nil
}

// parsePartRecords parses JSONL lines from a .part file, validating the chain.
// Returns the collected tokens, reads, entries, and the run identity from the header.
func parsePartRecords(lines []string) ([]tokenRecord, []readRecord, []entryRecord, kernel.Run, error) {
	var (
		tokens  []tokenRecord
		reads   []readRecord
		entries []entryRecord
		run     kernel.Run

		prevHash []byte
		lastSeq  uint64
	)

	for lineIdx, line := range lines {
		var rec partRecord
		dec := json.NewDecoder(strings.NewReader(line))
		if err := dec.Decode(&rec); err != nil {
			if lineIdx == len(lines)-1 {
				// Trailing partial line — treat as truncated, not corrupted.
				break
			}
			return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: parse line %d: %v", ErrCorruptedPart, lineIdx, err)
		}

		switch rec.Kind {
		case "header":
			if rec.Header == nil {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: header record missing header field", ErrCorruptedPart)
			}
			if rec.Header.Format != bundle.FormatVersion {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: unexpected format %q", ErrCorruptedPart, rec.Header.Format)
			}
			run = rec.Header.Run
			prevHash = genesisHash(run.TenantID)

		case "captured_read":
			if rec.CapturedRead == nil {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: captured_read record missing payload at line %d", ErrCorruptedPart, lineIdx)
			}
			reads = append(reads, readRecord{read: *rec.CapturedRead})

		case "token":
			if rec.Token == nil {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: token record missing payload at line %d", ErrCorruptedPart, lineIdx)
			}
			tokens = append(tokens, tokenRecord{token: *rec.Token})

		case "audit":
			if rec.Entry == nil {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: audit record missing payload at line %d", ErrCorruptedPart, lineIdx)
			}
			entry := rec.Entry

			// Validate seq monotonicity.
			wantSeq := lastSeq + 1
			if entry.Seq != wantSeq {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: seq %d at line %d, want %d", ErrCorruptedPart, entry.Seq, lineIdx, wantSeq)
			}

			// Validate prev_hash.
			if !bytes.Equal(entry.PrevHash, prevHash) {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: prev_hash mismatch at seq %d", ErrCorruptedPart, entry.Seq)
			}

			// Recompute and validate the hash.
			det, err := jcs.Transform(entry.Payload.Deterministic)
			if err != nil {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: canonical deterministic at seq %d: %v", ErrCorruptedPart, entry.Seq, err)
			}
			want := auditHash(entry.PrevHash, entry.Seq, entry.RunID, entry.TenantID, entry.Kind, det)
			if !bytes.Equal(entry.Hash, want) {
				return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: hash mismatch at seq %d", ErrCorruptedPart, entry.Seq)
			}

			prevHash = append([]byte(nil), entry.Hash...)
			lastSeq = entry.Seq
			entries = append(entries, entryRecord{entry: *entry})

		case "footer":
			// Footer is informational; stop processing after it.
			break

		default:
			return nil, nil, nil, kernel.Run{}, fmt.Errorf("%w: unknown record kind %q at line %d", ErrCorruptedPart, rec.Kind, lineIdx)
		}
	}

	return tokens, reads, entries, run, nil
}

// writeHeader writes the header record and the initial run_started audit entry.
func (r *Run) writeHeader() error {
	header := partRecord{
		Kind:   "header",
		Header: &partHeader{Format: bundle.FormatVersion, Run: r.opts.Run},
	}
	if err := r.partFile.writeRecord(header); err != nil {
		return fmt.Errorf("runfile: write header: %w", err)
	}

	prevHash := genesisHash(r.opts.Run.TenantID)
	startedEntry := buildRunLifecycleEntry(
		r.opts.Run,
		kernel.EntryKindRunStarted,
		r.opts.Clock.Now().UTC(),
		1,
		prevHash,
	)

	if err := r.partFile.writeRecord(partRecord{Kind: "audit", Entry: &startedEntry}); err != nil {
		return fmt.Errorf("runfile: write run_started: %w", err)
	}

	r.entries = append(r.entries, entryRecord{entry: startedEntry})
	r.lastSeq = 1
	r.lastHash = append([]byte(nil), startedEntry.Hash...)
	return nil
}

// finalize builds the canonical bundle from in-memory state, writes it to
// Path+".tmp", fsyncs, renames to Path, removes the .part file, and releases
// the lock.
//
// The finalized bundle includes ALL audit entries (including run lifecycle
// entries). bundle.Emit / normalizeState now handles lifecycle entries
// transparently, so the finalized file verifies with bundle.Verify.
func (r *Run) finalize() error {
	if err := r.partFile.sync(); err != nil {
		return fmt.Errorf("runfile: sync part file: %w", err)
	}

	// Collect tokens from records.
	tokens := make([]kernel.Token, len(r.tokens))
	for i, tr := range r.tokens {
		tokens[i] = tr.token
	}
	reads := make([]kernel.CapturedRead, len(r.reads))
	for i, rr := range r.reads {
		reads[i] = rr.read
	}

	allEntries := make([]kernel.AuditEntry, len(r.entries))
	for i, er := range r.entries {
		allEntries[i] = er.entry
	}

	state := bundle.State{
		Run:           r.opts.Run,
		CapturedReads: reads,
		Tokens:        tokens,
		AuditEntries:  allEntries,
	}

	canonical, err := bundle.Emit(state)
	if err != nil {
		return fmt.Errorf("runfile: emit bundle: %w", err)
	}

	// Write to tmp, fsync, rename.
	tmpPath := r.opts.Path + ".tmp"
	tmpF, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("runfile: create tmp: %w", err)
	}
	if _, err := tmpF.Write(canonical); err != nil {
		_ = tmpF.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runfile: write tmp: %w", err)
	}
	if err := tmpF.Sync(); err != nil {
		_ = tmpF.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runfile: sync tmp: %w", err)
	}
	if err := tmpF.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runfile: close tmp: %w", err)
	}

	if err := os.Rename(tmpPath, r.opts.Path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runfile: rename tmp -> final: %w", err)
	}

	// Remove .part file.
	_ = os.Remove(r.partPath)

	r.state = runStateFinalized
	return r.release()
}

// discard removes the .part file and releases the lock.
func (r *Run) discard() error {
	_ = os.Remove(r.partPath)
	return r.release()
}

// release closes file handles and releases the advisory lock.
func (r *Run) release() error {
	var firstErr error
	if err := releaseLock(r.lockFile); err != nil && firstErr == nil {
		firstErr = err
	}
	if r.partFile.f != nil {
		if err := r.partFile.f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		r.partFile.f = nil
	}
	return firstErr
}

// genesisHash mirrors the kernel package's unexported genesisHash function.
func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

// auditHash mirrors the kernel package's unexported auditHash function.
func auditHash(prevHash []byte, seq uint64, runID, tenantID string, kind kernel.EntryKind, hashInput []byte) []byte {
	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, seq)

	h := sha256.New()
	h.Write(prevHash)
	h.Write(seqBytes)
	h.Write([]byte(runID))
	h.Write([]byte(tenantID))
	h.Write([]byte(kind))
	h.Write(hashInput)
	return h.Sum(nil)
}
