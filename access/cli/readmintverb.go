package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/runfile"
)

// readMintRequest extends the existing ReadMintBundleRequest shape with
// a required run_file field for the long-lived partfile verb.
type readMintRequest struct {
	RunFile string                    `json:"run_file"`
	Reader  string                    `json:"reader"`
	Source  kernel.SourceBytes        `json:"source"`
	Run     kernel.Run                `json:"run"`
	Read    kernel.ReadAndMintRequest `json:"read"`
}

// readMintResponse is written to stdout on `read-mint` success.
type readMintResponse struct {
	Token      kernel.Token      `json:"token"`
	AuditEntry kernel.AuditEntry `json:"audit_entry"`
	Seq        uint64            `json:"seq"`
}

func (r Runner) runReadMint(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[readMintRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required for read-mint"))
	}
	if err := validateReadMintVerbRequest(req); err != nil {
		return err
	}

	clock := fixedClock{t: r.now()}

	// Open the run partfile (creates new or replays existing).
	run, err := runfile.Start(runfile.Options{
		Path:   req.RunFile,
		Run:    req.Run,
		Clock:  clock,
		Tenant: req.Run.TenantID,
	})
	if err != nil {
		return normalizeRunfileError(err)
	}

	// Build a chain-aware adapter that implements the kernel interfaces backed
	// by the run's current state. This lets kernel.ReadAndMint compute the
	// correct seq/prevHash from the existing audit chain.
	sink := newRunfileChainSink(run, req.Run)

	reader, err := sourceReader(req.Reader)
	if err != nil {
		_ = run.Abort()
		return err
	}

	// Execute ReadAndMint. The sink captures PutToken/PutCapturedRead/AppendAuditEntry
	// in memory; the actual disk write happens via run.AppendEntry below.
	mintResult, err := kernel.ReadAndMint(
		ctx,
		sink, // TransactionalScope
		singleSourceResolver{source: req.Source},
		reader,
		sink, // Store (also implements CapturedReadStore)
		sink, // AuditSink
		clock,
		req.Run,
		req.Read,
	)
	if err != nil {
		_ = run.Abort()
		return newPacketError(packetErrorEvidenceFailed, err)
	}

	// Append the mint result (entry + token + captured read) to the runfile.
	capturedRead := mintResult.ReadResult.CapturedRead
	if err := run.AppendEntry(mintResult.AuditEntry, &mintResult.Token, &capturedRead); err != nil {
		_ = run.Abort()
		return normalizeRunfileError(err)
	}

	seq := run.LastSeq()
	if err := run.Close(); err != nil {
		return fmt.Errorf("runfile: close after read-mint: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(readMintResponse{
		Token:      mintResult.Token,
		AuditEntry: mintResult.AuditEntry,
		Seq:        seq,
	})
}

func validateReadMintVerbRequest(req readMintRequest) error {
	if req.Reader != readerExtractJSON {
		return fmt.Errorf("%w: %s", ErrUnsupportedReader, req.Reader)
	}
	if req.Run.ID == "" || req.Run.TenantID == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run id and tenant_id are required"))
	}
	if req.Read.SourcePath == "" || req.Read.OutputPath == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("read source_path and output_path are required"))
	}
	if req.Source.Source.Path == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("source path is required"))
	}
	if req.Source.Source.Path != req.Read.SourcePath {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("source path must match read source_path"))
	}
	if len(req.Source.Bytes) == 0 {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("source bytes are required"))
	}
	if len(req.Source.Bytes) > maxSourceBytes {
		return newPacketError(packetErrorRequestInvalid, ErrSourceTooLarge)
	}
	return nil
}

// singleSourceResolver satisfies kernel.SourceResolver for a single source.
type singleSourceResolver struct {
	source kernel.SourceBytes
}

func (s singleSourceResolver) ResolveSource(_ context.Context, path string) (kernel.SourceBytes, error) {
	if path != s.source.Source.Path {
		return kernel.SourceBytes{}, fmt.Errorf("source not found: %s", path)
	}
	return s.source, nil
}

// runfileChainSink implements kernel.TransactionalScope, kernel.Store,
// kernel.CapturedReadStore, and kernel.AuditSink backed by a runfile.Run.
//
// Reads (AuditEntries, Tokens, CapturedReads) are served from the run's current
// in-memory state so that the kernel can compute the correct seq/prevHash.
// Writes (PutToken, PutCapturedRead, AppendAuditEntry) are captured in-memory
// only; the caller is responsible for calling run.AppendEntry with the
// accumulated results.
type runfileChainSink struct {
	run     *runfile.Run
	kernRun kernel.Run
}

const runfileSubstrate = "provenant.substrate/runfile/v1"

type runfileTxSentinel struct{}

func newRunfileChainSink(run *runfile.Run, kernRun kernel.Run) *runfileChainSink {
	return &runfileChainSink{run: run, kernRun: kernRun}
}

func (s *runfileChainSink) TransactionalSubstrate() string { return runfileSubstrate }

func (s *runfileChainSink) WithTx(_ context.Context, fn func(context.Context, kernel.Tx) error) error {
	return fn(context.Background(), runfileTxSentinel{})
}

func (s *runfileChainSink) PutToken(_ context.Context, _ kernel.Tx, _ kernel.Token) error {
	return nil // captured via ReadAndMint return value
}

func (s *runfileChainSink) Tokens(_ context.Context, _ kernel.Tx, runID string) ([]kernel.Token, error) {
	if runID != s.kernRun.ID {
		return nil, nil
	}
	return s.run.Tokens(), nil
}

func (s *runfileChainSink) PutCapturedRead(_ context.Context, _ kernel.Tx, _ kernel.CapturedRead) error {
	return nil // captured via ReadAndMint return value
}

func (s *runfileChainSink) CapturedReads(_ context.Context, _ kernel.Tx, runID string) ([]kernel.CapturedRead, error) {
	if runID != s.kernRun.ID {
		return nil, nil
	}
	return s.run.CapturedReads(), nil
}

func (s *runfileChainSink) AppendAuditEntry(_ context.Context, _ kernel.Tx, entry kernel.AuditEntry) (kernel.AuditEntry, error) {
	return entry, nil // captured via ReadAndMint return value
}

func (s *runfileChainSink) AuditEntries(_ context.Context, _ kernel.Tx, tenantID, runID string) ([]kernel.AuditEntry, error) {
	if runID != s.kernRun.ID || tenantID != s.kernRun.TenantID {
		return nil, nil
	}
	return s.run.AuditEntries(), nil
}
