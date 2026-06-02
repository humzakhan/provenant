package kernel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gowebpki/jcs"

	"github.com/humzakhan/provenant/kernel/public/internal/phaseone"
)

var (
	ErrNilTransactionalScope          = errors.New("kernel: transactional scope is required")
	ErrNilSourceResolver              = errors.New("kernel: source resolver is required")
	ErrNilSourceReader                = errors.New("kernel: source reader is required")
	ErrNilStore                       = errors.New("kernel: store is required")
	ErrNilAuditSink                   = errors.New("kernel: audit sink is required")
	ErrNilClock                       = errors.New("kernel: clock is required")
	ErrStoreMissingCapturedReadStore  = errors.New("kernel: store does not persist captured reads")
	ErrMissingRunID                   = errors.New("kernel: run id is required")
	ErrMissingTenantID                = errors.New("kernel: tenant id is required")
	ErrMissingSourcePath              = errors.New("kernel: source path is required")
	ErrMissingOutputPath              = errors.New("kernel: output path is required")
	ErrUnsupportedOutputPath          = errors.New("kernel: unsupported output path")
	ErrOutputPathNotFound             = errors.New("kernel: output path not found")
	ErrMintValueNotNumeric            = errors.New("kernel: mint value is not numeric")
	ErrTransactionalScopeMismatch     = errors.New("kernel: transactional substrate mismatch")
	ErrCapturedReadRunIDMismatch      = errors.New("kernel: captured read run id mismatch")
	ErrCapturedReadPathMismatch       = errors.New("kernel: captured read source path mismatch")
	ErrCapturedReadSelectorMismatch   = errors.New("kernel: captured read selector mismatch")
	ErrCapturedReadSourceHashMismatch = errors.New("kernel: captured read source hash mismatch")
	ErrCapturedReadOutputHashMismatch = errors.New("kernel: captured read output hash mismatch")
	ErrCapturedReadOutputMismatch     = errors.New("kernel: captured read output mismatch")
)

// ReadAndMintRequest identifies one source read plus the mintable output path.
type ReadAndMintRequest struct {
	SourcePath string         `json:"source_path"`
	Selector   SourceSelector `json:"selector"`
	OutputPath string         `json:"output_path"`
	Unit       string         `json:"unit,omitempty"`
	// SourceName is optional. When non-empty it records the logical source name
	// (from a named-source registry) that produced this read. It is propagated
	// into the CapturedRead and into the deterministic token_mint audit payload.
	SourceName string `json:"source_name,omitempty"`
}

// ReadAndMintResult returns the captured read, minted token, and appended audit entry.
type ReadAndMintResult struct {
	ReadResult ReadResult `json:"read_result"`
	Token      Token      `json:"token"`
	AuditEntry AuditEntry `json:"audit_entry"`
}

// MintFromCapturedReadRequest identifies one already captured source read plus the mintable output path.
type MintFromCapturedReadRequest struct {
	CapturedRead CapturedRead `json:"captured_read"`
	Source       Source       `json:"source"`
	OutputPath   string       `json:"output_path"`
	Unit         string       `json:"unit,omitempty"`
}

// MintFromCapturedReadResult returns the minted token and appended audit entry.
type MintFromCapturedReadResult struct {
	Token      Token      `json:"token"`
	AuditEntry AuditEntry `json:"audit_entry"`
}

// ReadAndMint resolves one injected source, runs the reader, and mints a token from the selected output path.
func ReadAndMint(
	ctx context.Context,
	scope TransactionalScope,
	resolver SourceResolver,
	reader SourceReader,
	store Store,
	auditSink AuditSink,
	clock Clock,
	run Run,
	request ReadAndMintRequest,
) (ReadAndMintResult, error) {
	if scope == nil {
		return ReadAndMintResult{}, ErrNilTransactionalScope
	}
	if resolver == nil {
		return ReadAndMintResult{}, ErrNilSourceResolver
	}
	if reader == nil {
		return ReadAndMintResult{}, ErrNilSourceReader
	}
	if store == nil {
		return ReadAndMintResult{}, ErrNilStore
	}
	if auditSink == nil {
		return ReadAndMintResult{}, ErrNilAuditSink
	}
	if clock == nil {
		return ReadAndMintResult{}, ErrNilClock
	}
	if run.ID == "" {
		return ReadAndMintResult{}, ErrMissingRunID
	}
	if run.TenantID == "" {
		return ReadAndMintResult{}, ErrMissingTenantID
	}
	if request.SourcePath == "" {
		return ReadAndMintResult{}, ErrMissingSourcePath
	}
	if request.OutputPath == "" {
		return ReadAndMintResult{}, ErrMissingOutputPath
	}
	if scope.TransactionalSubstrate() != store.TransactionalSubstrate() || scope.TransactionalSubstrate() != auditSink.TransactionalSubstrate() {
		return ReadAndMintResult{}, ErrTransactionalScopeMismatch
	}
	capturedReadStore, ok := store.(CapturedReadStore)
	if !ok {
		return ReadAndMintResult{}, ErrStoreMissingCapturedReadStore
	}

	var result ReadAndMintResult
	if err := scope.WithTx(ctx, func(ctx context.Context, tx Tx) error {
		source, err := resolver.ResolveSource(ctx, request.SourcePath)
		if err != nil {
			return err
		}

		read, err := reader.Read(ctx, run, source, request.Selector)
		if err != nil {
			return err
		}
		// Propagate the optional logical source name from the request into
		// the captured read so the audit payload and stored read agree.
		read.CapturedRead.SourceName = request.SourceName
		if err := validateReadAndMintCapturedRead(source, read, run, request); err != nil {
			return err
		}
		if err := capturedReadStore.PutCapturedRead(ctx, tx, read.CapturedRead); err != nil {
			return err
		}
		minted, err := mintFromCapturedReadTx(ctx, tx, store, auditSink, clock, run, read.CapturedRead, request.OutputPath, request.Unit)
		if err != nil {
			return err
		}

		result = ReadAndMintResult{
			ReadResult: read,
			Token:      minted.Token,
			AuditEntry: minted.AuditEntry,
		}

		return nil
	}); err != nil {
		return ReadAndMintResult{}, err
	}

	return result, nil
}

// MintFromCapturedRead mints a token from supplied captured-read evidence after verifying its source and output hashes.
func MintFromCapturedRead(
	ctx context.Context,
	scope TransactionalScope,
	store Store,
	auditSink AuditSink,
	clock Clock,
	run Run,
	request MintFromCapturedReadRequest,
) (MintFromCapturedReadResult, error) {
	if scope == nil {
		return MintFromCapturedReadResult{}, ErrNilTransactionalScope
	}
	if store == nil {
		return MintFromCapturedReadResult{}, ErrNilStore
	}
	if auditSink == nil {
		return MintFromCapturedReadResult{}, ErrNilAuditSink
	}
	if clock == nil {
		return MintFromCapturedReadResult{}, ErrNilClock
	}
	if run.ID == "" {
		return MintFromCapturedReadResult{}, ErrMissingRunID
	}
	if run.TenantID == "" {
		return MintFromCapturedReadResult{}, ErrMissingTenantID
	}
	if request.Source.Path == "" {
		return MintFromCapturedReadResult{}, ErrMissingSourcePath
	}
	if request.OutputPath == "" {
		return MintFromCapturedReadResult{}, ErrMissingOutputPath
	}
	if scope.TransactionalSubstrate() != store.TransactionalSubstrate() || scope.TransactionalSubstrate() != auditSink.TransactionalSubstrate() {
		return MintFromCapturedReadResult{}, ErrTransactionalScopeMismatch
	}

	var result MintFromCapturedReadResult
	if err := scope.WithTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := validateCapturedReadForMint(request.CapturedRead, request.Source, run); err != nil {
			return err
		}

		minted, err := mintFromCapturedReadTx(ctx, tx, store, auditSink, clock, run, request.CapturedRead, request.OutputPath, request.Unit)
		if err != nil {
			return err
		}
		result = minted
		return nil
	}); err != nil {
		return MintFromCapturedReadResult{}, err
	}

	return result, nil
}

type tokenMintPayload struct {
	TokenID        string          `json:"token_id"`
	CapturedReadID string          `json:"captured_read_id"`
	SourcePath     string          `json:"source_path"`
	SourceSelector SourceSelector  `json:"source_selector"`
	OutputPath     string          `json:"output_path"`
	Origin         string          `json:"origin,omitempty"`
	SourceName     string          `json:"source_name,omitempty"`
	ReaderName     string          `json:"reader_name"`
	ReaderVersion  string          `json:"reader_version"`
	ReaderOptions  json.RawMessage `json:"reader_options,omitempty"`
	Locator        json.RawMessage `json:"locator,omitempty"`
	SourceSHA256   string          `json:"source_sha256"`
	OutputSHA256   string          `json:"output_sha256"`
	SourceValueRaw string          `json:"source_value_raw"`
	ValueCanonical string          `json:"value_canonical"`
	Unit           string          `json:"unit,omitempty"`
}

func extractMintValue(output []byte, outputPath string) (string, error) {
	value, err := phaseone.ExtractMintValue(output, outputPath)
	return value, mapPhaseOneMintError(err)
}

func validateReadAndMintCapturedRead(source SourceBytes, read ReadResult, run Run, request ReadAndMintRequest) error {
	if read.CapturedRead.RunID != run.ID {
		return ErrCapturedReadRunIDMismatch
	}
	if read.CapturedRead.SourcePath != request.SourcePath || read.CapturedRead.SourcePath != source.Source.Path {
		return ErrCapturedReadPathMismatch
	}
	if read.CapturedRead.Selector != request.Selector {
		return ErrCapturedReadSelectorMismatch
	}

	sourceSHA256 := sha256Hex(source.Bytes)
	if read.CapturedRead.SourceSHA256 != sourceSHA256 {
		return ErrCapturedReadSourceHashMismatch
	}

	outputSHA256 := sha256Hex(read.Output)
	if read.CapturedRead.OutputSHA256 != outputSHA256 {
		return ErrCapturedReadOutputHashMismatch
	}

	if !bytes.Equal(read.CapturedRead.Output, read.Output) {
		return ErrCapturedReadOutputMismatch
	}

	return nil
}

func validateCapturedReadForMint(read CapturedRead, source Source, run Run) error {
	if read.RunID != run.ID {
		return ErrCapturedReadRunIDMismatch
	}
	if read.SourcePath != source.Path {
		return ErrCapturedReadPathMismatch
	}
	if read.SourceSHA256 == "" || source.SHA256 == "" || read.SourceSHA256 != source.SHA256 {
		return ErrCapturedReadSourceHashMismatch
	}
	if read.OutputSHA256 == "" || read.OutputSHA256 != sha256Hex(read.Output) {
		return ErrCapturedReadOutputHashMismatch
	}
	return nil
}

func mintFromCapturedReadTx(
	ctx context.Context,
	tx Tx,
	store Store,
	auditSink AuditSink,
	clock Clock,
	run Run,
	read CapturedRead,
	outputPath string,
	unit string,
) (MintFromCapturedReadResult, error) {
	rawValue, err := extractMintValue(read.Output, outputPath)
	if err != nil {
		return MintFromCapturedReadResult{}, err
	}
	sourceValueRaw, err := normalizeSourceValueRaw(rawValue)
	if err != nil {
		return MintFromCapturedReadResult{}, err
	}

	valueCanonical, err := canonicalizeNumeric(rawValue)
	if err != nil {
		return MintFromCapturedReadResult{}, err
	}

	now := clock.Now().UTC()
	token := Token{
		ID:             tokenID(run.ID, read.ID, outputPath, unit, sourceValueRaw, valueCanonical),
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: valueCanonical,
		Unit:           unit,
		Source: TokenSource{
			CapturedReadID: read.ID,
			SourcePath:     read.SourcePath,
			SourceSelector: read.Selector,
			OutputPath:     outputPath,
			Origin:         read.Origin,
		},
		CreatedAt: now,
	}

	entries, err := auditSink.AuditEntries(ctx, tx, run.TenantID, run.ID)
	if err != nil {
		return MintFromCapturedReadResult{}, err
	}

	deterministic, err := canonicalizeDeterministicPayload(tokenMintPayload{
		TokenID:        token.ID,
		CapturedReadID: read.ID,
		SourcePath:     read.SourcePath,
		SourceSelector: read.Selector,
		OutputPath:     outputPath,
		Origin:         read.Origin,
		SourceName:     read.SourceName,
		ReaderName:     read.ReaderName,
		ReaderVersion:  read.ReaderVersion,
		ReaderOptions:  read.ReaderOptions,
		Locator:        read.Locator,
		SourceSHA256:   read.SourceSHA256,
		OutputSHA256:   read.OutputSHA256,
		SourceValueRaw: sourceValueRaw,
		ValueCanonical: valueCanonical,
		Unit:           unit,
	})
	if err != nil {
		return MintFromCapturedReadResult{}, err
	}

	prevHash := genesisHash(run.TenantID)
	seq := uint64(1)
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		prevHash = append([]byte(nil), last.Hash...)
		seq = last.Seq + 1
	}

	hashInput := append([]byte(nil), deterministic...)
	entry := AuditEntry{
		Seq:      seq,
		RunID:    run.ID,
		TenantID: run.TenantID,
		Kind:     EntryKindTokenMint,
		At:       now,
		PrevHash: prevHash,
		Hash:     auditHash(prevHash, seq, run.ID, run.TenantID, EntryKindTokenMint, hashInput),
		Payload: PayloadEnvelope{
			Deterministic:      deterministic,
			HashInputCanonical: hashInput,
		},
	}

	if err := store.PutToken(ctx, tx, token); err != nil {
		return MintFromCapturedReadResult{}, err
	}

	appended, err := auditSink.AppendAuditEntry(ctx, tx, entry)
	if err != nil {
		return MintFromCapturedReadResult{}, err
	}

	return MintFromCapturedReadResult{Token: token, AuditEntry: appended}, nil
}

func normalizeSourceValueRaw(raw string) (string, error) {
	value, err := phaseone.NormalizeSourceValueRaw(raw)
	return value, mapPhaseOneMintError(err)
}

func canonicalizeNumeric(raw string) (string, error) {
	value, err := phaseone.CanonicalizeNumeric(raw)
	return value, mapPhaseOneMintError(err)
}

func canonicalizeDeterministicPayload(payload tokenMintPayload) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}

func tokenID(runID, capturedReadID, outputPath, unit, rawValue, valueCanonical string) string {
	return phaseone.TokenID(runID, capturedReadID, outputPath, unit, rawValue, valueCanonical)
}

func mapPhaseOneMintError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, phaseone.ErrUnsupportedOutputPath):
		return fmt.Errorf("%w: %s", ErrUnsupportedOutputPath, errorSuffix(err))
	case errors.Is(err, phaseone.ErrOutputPathNotFound):
		return fmt.Errorf("%w: %s", ErrOutputPathNotFound, errorSuffix(err))
	case errors.Is(err, phaseone.ErrMintValueNotNumeric):
		return fmt.Errorf("%w: %s", ErrMintValueNotNumeric, errorSuffix(err))
	default:
		return err
	}
}

func errorSuffix(err error) string {
	message := err.Error()
	index := strings.LastIndex(message, ": ")
	if index < 0 {
		return message
	}
	return message[index+2:]
}

func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

func auditHash(prevHash []byte, seq uint64, runID, tenantID string, kind EntryKind, hashInput []byte) []byte {
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
