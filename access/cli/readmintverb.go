package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/provenant/provenant/access/sources"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/runfile"
)

// readMintRequest is the JSON body for the `read-mint` verb.
//
// Exactly one of the two source-identification modes must be used:
//   - Named-source mode: set source_name and supply --sources <registry-path>;
//     the registry resolves reader, path, selector, and output_path.
//   - Raw mode: set reader, source, and read fields directly (pre-Phase-06 behaviour).
type readMintRequest struct {
	RunFile    string                    `json:"run_file"`
	SourceName string                    `json:"source_name,omitempty"`
	Reader     string                    `json:"reader"`
	Source     kernel.SourceBytes        `json:"source"`
	Run        kernel.Run                `json:"run"`
	Read       kernel.ReadAndMintRequest `json:"read"`
}

// readMintResponse is written to stdout on `read-mint` success.
type readMintResponse struct {
	Token      kernel.Token      `json:"token"`
	AuditEntry kernel.AuditEntry `json:"audit_entry"`
	Seq        uint64            `json:"seq"`
}

func (r Runner) runReadMint(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	// Parse --sources flag before consuming stdin.
	sourcesPath, remainingArgs := extractSourcesFlag(args)

	req, err := decodeJSONRequest[readMintRequest](remainingArgs, stdin)
	if err != nil {
		return err
	}
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required for read-mint"))
	}

	// Resolve source_name if provided.
	if req.SourceName != "" {
		if sourcesPath == "" {
			return newPacketError(packetErrorRequestInvalid, fmt.Errorf("--sources is required when source_name is used"))
		}
		req, err = resolveSourceName(req, sourcesPath)
		if err != nil {
			return err
		}
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

	// Execute ReadAndMint. The kernel propagates req.Read.SourceName into
	// the captured read and audit payload automatically.
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

// resolveSourceName looks up the named source in the registry at sourcesPath
// and populates reader, source path, selector, output_path, and unit from the
// entry. The source bytes must already be set by the caller.
func resolveSourceName(req readMintRequest, sourcesPath string) (readMintRequest, error) {
	f, err := os.Open(sourcesPath)
	if err != nil {
		return req, newPacketError(packetErrorInputNotFound, fmt.Errorf("open sources registry %s: %w", sourcesPath, err))
	}
	defer f.Close()

	reg, err := sources.Load(f)
	if err != nil {
		return req, newPacketError(packetErrorRequestInvalid, fmt.Errorf("load sources registry: %w", err))
	}

	entry, ok := reg.Resolve(req.SourceName)
	if !ok {
		return req, newPacketError(packetErrorRequestInvalid, fmt.Errorf("source name %q not found in registry", req.SourceName))
	}

	// Populate reader and read fields from the registry entry.
	req.Reader = entry.Reader
	req.Read = kernel.ReadAndMintRequest{
		SourcePath: entry.Path,
		Selector:   entry.Selector,
		OutputPath: entry.OutputPath,
		Unit:       entry.Unit,
		SourceName: req.SourceName,
	}
	// The source bytes are caller-supplied; align the path to the registry entry.
	req.Source.Source.Path = entry.Path

	return req, nil
}

// extractSourcesFlag removes --sources <path> (or --sources=<path>) from args
// and returns the path value and the remaining args.
func extractSourcesFlag(args []string) (string, []string) {
	remaining := make([]string, 0, len(args))
	var value string
	for i := 0; i < len(args); i++ {
		if args[i] == "--sources" {
			if i+1 < len(args) {
				value = args[i+1]
				i++ // skip the value token
			}
			continue
		}
		if len(args[i]) > 10 && args[i][:10] == "--sources=" {
			value = args[i][10:]
			continue
		}
		remaining = append(remaining, args[i])
	}
	return value, remaining
}

func validateReadMintVerbRequest(req readMintRequest) error {
	if req.Reader == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("reader is required"))
	}
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
