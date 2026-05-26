package corepath

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/gowebpki/jcs"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/bundle"
)

type Surface string

const (
	SurfaceCLI    Surface = "cli"
	SurfaceHTTP   Surface = "http"
	SurfaceMCP    Surface = "mcp"
	SurfaceExport Surface = "export"
)

var (
	ErrInvalidSurface                 = errors.New("corepath: invalid surface")
	ErrNilDependencies                = errors.New("corepath: dependencies are required")
	ErrTransactionalSubstrateMismatch = errors.New("corepath: transactional substrate mismatch")
	ErrStoreMissingCapturedReads      = errors.New("corepath: store does not expose captured reads")
	ErrMissingBundleBytes             = errors.New("corepath: bundle bytes are required")
	ErrMissingExportBytes             = errors.New("corepath: export bytes are required")
	ErrExportRequiresExportSurface    = errors.New("corepath: export verification requires export surface")
	ErrDerivedExportMismatch          = errors.New("corepath: derived export does not match canonical bundle bytes")
	ErrDerivedExportProvenance        = errors.New("corepath: derived export requires matching captured provenance")
)

type Dependencies struct {
	Scope     kernel.TransactionalScope
	Resolver  kernel.SourceResolver
	Reader    kernel.SourceReader
	Store     kernel.Store
	AuditSink kernel.AuditSink
	Clock     kernel.Clock
}

type ReadMintBundleRequest struct {
	Surface Surface                   `json:"surface"`
	Run     kernel.Run                `json:"run"`
	Read    kernel.ReadAndMintRequest `json:"read"`
}

type ReadMintBundleResult struct {
	Surface      Surface                  `json:"surface"`
	ReadAndMint  kernel.ReadAndMintResult `json:"read_and_mint"`
	BundleBytes  []byte                   `json:"bundle_bytes"`
	BundleSHA256 string                   `json:"bundle_sha256"`
}

type ExportSourceRequest struct {
	Surface     Surface `json:"surface"`
	BundleBytes []byte  `json:"bundle_bytes"`
}

type ExportSourceResult struct {
	Surface      Surface      `json:"surface"`
	State        bundle.State `json:"state"`
	BundleSHA256 string       `json:"bundle_sha256"`
}

type DerivedExportRequest struct {
	Surface     Surface `json:"surface"`
	BundleBytes []byte  `json:"bundle_bytes"`
}

type DerivedExportResult struct {
	Surface      Surface               `json:"surface"`
	Document     DerivedExportDocument `json:"document"`
	ExportBytes  []byte                `json:"export_bytes"`
	BundleSHA256 string                `json:"bundle_sha256"`
	ExportSHA256 string                `json:"export_sha256"`
}

type ValidateDerivedExportRequest struct {
	Surface     Surface `json:"surface"`
	BundleBytes []byte  `json:"bundle_bytes"`
	ExportBytes []byte  `json:"export_bytes"`
}

type ValidateDerivedExportResult struct {
	Surface      Surface               `json:"surface"`
	Document     DerivedExportDocument `json:"document"`
	BundleSHA256 string                `json:"bundle_sha256"`
	ExportSHA256 string                `json:"export_sha256"`
}

type DerivedExportDocument struct {
	FormatVersion string             `json:"format_version"`
	Run           kernel.Run         `json:"run"`
	BundleSHA256  string             `json:"bundle_sha256"`
	Rows          []DerivedExportRow `json:"rows"`
}

type DerivedExportRow struct {
	TokenID        string                `json:"token_id"`
	CapturedReadID string                `json:"captured_read_id"`
	SourcePath     string                `json:"source_path"`
	SourceSelector kernel.SourceSelector `json:"source_selector"`
	OutputPath     string                `json:"output_path"`
	Origin         string                `json:"origin,omitempty"`
	ReaderName     string                `json:"reader_name"`
	ReaderVersion  string                `json:"reader_version"`
	ReaderOptions  json.RawMessage       `json:"reader_options,omitempty"`
	Locator        json.RawMessage       `json:"locator,omitempty"`
	SourceSHA256   string                `json:"source_sha256"`
	OutputSHA256   string                `json:"output_sha256"`
	ValueCanonical string                `json:"value_canonical"`
	Unit           string                `json:"unit,omitempty"`
	AuditSeq       uint64                `json:"audit_seq"`
	AuditHash      string                `json:"audit_hash"`
}

func ExecuteReadMintBundle(ctx context.Context, deps Dependencies, request ReadMintBundleRequest) (ReadMintBundleResult, error) {
	if err := validateAccessSurface(request.Surface); err != nil {
		return ReadMintBundleResult{}, err
	}
	if err := validateDependencies(deps); err != nil {
		return ReadMintBundleResult{}, err
	}

	readAndMint, err := kernel.ReadAndMint(
		ctx,
		deps.Scope,
		deps.Resolver,
		deps.Reader,
		deps.Store,
		deps.AuditSink,
		deps.Clock,
		request.Run,
		request.Read,
	)
	if err != nil {
		return ReadMintBundleResult{}, err
	}

	bundleBytes, err := SnapshotBundle(ctx, deps.Scope, deps.Store, deps.AuditSink, request.Run)
	if err != nil {
		return ReadMintBundleResult{}, err
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		return ReadMintBundleResult{}, err
	}

	return ReadMintBundleResult{
		Surface:      request.Surface,
		ReadAndMint:  readAndMint,
		BundleBytes:  append([]byte(nil), bundleBytes...),
		BundleSHA256: sha256Hex(bundleBytes),
	}, nil
}

func SnapshotBundle(ctx context.Context, scope kernel.TransactionalScope, store kernel.Store, auditSink kernel.AuditSink, run kernel.Run) ([]byte, error) {
	if scope == nil || store == nil || auditSink == nil {
		return nil, ErrNilDependencies
	}
	if scope.TransactionalSubstrate() != store.TransactionalSubstrate() || scope.TransactionalSubstrate() != auditSink.TransactionalSubstrate() {
		return nil, ErrTransactionalSubstrateMismatch
	}
	capturedReadStore, ok := store.(kernel.CapturedReadStore)
	if !ok {
		return nil, ErrStoreMissingCapturedReads
	}

	var state bundle.State
	if err := scope.WithTx(ctx, func(ctx context.Context, tx kernel.Tx) error {
		capturedReads, err := capturedReadStore.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		tokens, err := store.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		auditEntries, err := auditSink.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		state = bundle.State{
			Run:           run,
			CapturedReads: capturedReads,
			Tokens:        tokens,
			AuditEntries:  auditEntries,
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return bundle.Emit(state)
}

func VerifyExportSource(_ context.Context, request ExportSourceRequest) (ExportSourceResult, error) {
	if request.Surface != SurfaceExport {
		return ExportSourceResult{}, ErrExportRequiresExportSurface
	}
	if len(request.BundleBytes) == 0 {
		return ExportSourceResult{}, ErrMissingBundleBytes
	}

	state, err := bundle.Replay(request.BundleBytes)
	if err != nil {
		return ExportSourceResult{}, err
	}
	canonicalBundleBytes, err := bundle.Emit(state)
	if err != nil {
		return ExportSourceResult{}, err
	}
	if !bytes.Equal(canonicalBundleBytes, request.BundleBytes) {
		return ExportSourceResult{}, fmt.Errorf("%w: bundle bytes are not canonical", bundle.ErrTamperDetected)
	}

	return ExportSourceResult{
		Surface:      request.Surface,
		State:        state,
		BundleSHA256: sha256Hex(request.BundleBytes),
	}, nil
}

func GenerateDerivedExport(ctx context.Context, request DerivedExportRequest) (DerivedExportResult, error) {
	source, err := VerifyExportSource(ctx, ExportSourceRequest{
		Surface:     request.Surface,
		BundleBytes: request.BundleBytes,
	})
	if err != nil {
		return DerivedExportResult{}, err
	}

	document, err := deriveExportDocument(source.State, source.BundleSHA256)
	if err != nil {
		return DerivedExportResult{}, err
	}
	exportBytes, err := emitDerivedExport(document)
	if err != nil {
		return DerivedExportResult{}, err
	}

	return DerivedExportResult{
		Surface:      request.Surface,
		Document:     document,
		ExportBytes:  exportBytes,
		BundleSHA256: source.BundleSHA256,
		ExportSHA256: sha256Hex(exportBytes),
	}, nil
}

func ValidateDerivedExport(ctx context.Context, request ValidateDerivedExportRequest) (ValidateDerivedExportResult, error) {
	if len(request.ExportBytes) == 0 {
		return ValidateDerivedExportResult{}, ErrMissingExportBytes
	}

	expected, err := GenerateDerivedExport(ctx, DerivedExportRequest{
		Surface:     request.Surface,
		BundleBytes: request.BundleBytes,
	})
	if err != nil {
		return ValidateDerivedExportResult{}, err
	}

	document, err := parseDerivedExportDocument(request.ExportBytes)
	if err != nil {
		return ValidateDerivedExportResult{}, err
	}
	canonicalExportBytes, err := emitDerivedExport(document)
	if err != nil {
		return ValidateDerivedExportResult{}, err
	}
	if !bytes.Equal(canonicalExportBytes, request.ExportBytes) {
		return ValidateDerivedExportResult{}, fmt.Errorf("%w: export bytes are not canonical", ErrDerivedExportMismatch)
	}
	if !bytes.Equal(expected.ExportBytes, canonicalExportBytes) {
		return ValidateDerivedExportResult{}, fmt.Errorf("%w: export artifact differs from canonical replay", ErrDerivedExportMismatch)
	}

	return ValidateDerivedExportResult{
		Surface:      request.Surface,
		Document:     document,
		BundleSHA256: expected.BundleSHA256,
		ExportSHA256: sha256Hex(canonicalExportBytes),
	}, nil
}

func validateAccessSurface(surface Surface) error {
	switch surface {
	case SurfaceCLI, SurfaceHTTP, SurfaceMCP:
		return nil
	case SurfaceExport:
		return fmt.Errorf("%w: export uses VerifyExportSource", ErrInvalidSurface)
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSurface, surface)
	}
}

func validateDependencies(deps Dependencies) error {
	if deps.Scope == nil || deps.Resolver == nil || deps.Reader == nil || deps.Store == nil || deps.AuditSink == nil || deps.Clock == nil {
		return ErrNilDependencies
	}
	return nil
}

func sha256Hex(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}

const derivedExportFormatVersion = "phase3.derived_export/v1"

type tokenMintExportPayload struct {
	TokenID        string                `json:"token_id"`
	CapturedReadID string                `json:"captured_read_id"`
	SourcePath     string                `json:"source_path"`
	SourceSelector kernel.SourceSelector `json:"source_selector"`
	OutputPath     string                `json:"output_path"`
	Origin         string                `json:"origin,omitempty"`
	ReaderName     string                `json:"reader_name"`
	ReaderVersion  string                `json:"reader_version"`
	ReaderOptions  json.RawMessage       `json:"reader_options,omitempty"`
	Locator        json.RawMessage       `json:"locator,omitempty"`
	SourceSHA256   string                `json:"source_sha256"`
	OutputSHA256   string                `json:"output_sha256"`
	SourceValueRaw string                `json:"source_value_raw"`
	ValueCanonical string                `json:"value_canonical"`
	Unit           string                `json:"unit,omitempty"`
}

func deriveExportDocument(state bundle.State, bundleSHA256 string) (DerivedExportDocument, error) {
	readsByID := make(map[string]kernel.CapturedRead, len(state.CapturedReads))
	for _, read := range state.CapturedReads {
		if read.ID == "" {
			return DerivedExportDocument{}, fmt.Errorf("%w: captured read id", ErrDerivedExportProvenance)
		}
		readsByID[read.ID] = read
	}

	tokensByID := make(map[string]kernel.Token, len(state.Tokens))
	for _, token := range state.Tokens {
		if token.ID == "" {
			return DerivedExportDocument{}, fmt.Errorf("%w: token id", ErrDerivedExportProvenance)
		}
		if token.Source.CapturedReadID == "" {
			return DerivedExportDocument{}, fmt.Errorf("%w: token %s captured read id", ErrDerivedExportProvenance, token.ID)
		}
		tokensByID[token.ID] = token
	}

	rows := make([]DerivedExportRow, 0, len(state.AuditEntries))
	seenTokenIDs := make(map[string]struct{}, len(state.AuditEntries))
	for _, entry := range state.AuditEntries {
		payload, err := decodeTokenMintExportPayload(entry.Payload.Deterministic)
		if err != nil {
			return DerivedExportDocument{}, fmt.Errorf("%w: audit seq %d: %v", ErrDerivedExportProvenance, entry.Seq, err)
		}

		token, ok := tokensByID[payload.TokenID]
		if !ok {
			return DerivedExportDocument{}, fmt.Errorf("%w: token %s", ErrDerivedExportProvenance, payload.TokenID)
		}
		read, ok := readsByID[payload.CapturedReadID]
		if !ok {
			return DerivedExportDocument{}, fmt.Errorf("%w: captured read %s", ErrDerivedExportProvenance, payload.CapturedReadID)
		}
		if token.Source.CapturedReadID != read.ID {
			return DerivedExportDocument{}, fmt.Errorf("%w: token %s captured read mismatch", ErrDerivedExportProvenance, token.ID)
		}
		if payload.CapturedReadID != read.ID {
			return DerivedExportDocument{}, fmt.Errorf("%w: audit seq %d captured read mismatch", ErrDerivedExportProvenance, entry.Seq)
		}
		if payload.TokenID != token.ID {
			return DerivedExportDocument{}, fmt.Errorf("%w: audit seq %d token mismatch", ErrDerivedExportProvenance, entry.Seq)
		}

		rows = append(rows, DerivedExportRow{
			TokenID:        token.ID,
			CapturedReadID: read.ID,
			SourcePath:     read.SourcePath,
			SourceSelector: read.Selector,
			OutputPath:     token.Source.OutputPath,
			Origin:         read.Origin,
			ReaderName:     read.ReaderName,
			ReaderVersion:  read.ReaderVersion,
			ReaderOptions:  cloneRawMessage(read.ReaderOptions),
			Locator:        cloneRawMessage(read.Locator),
			SourceSHA256:   read.SourceSHA256,
			OutputSHA256:   read.OutputSHA256,
			ValueCanonical: token.ValueCanonical,
			Unit:           token.Unit,
			AuditSeq:       entry.Seq,
			AuditHash:      hex.EncodeToString(entry.Hash),
		})
		seenTokenIDs[token.ID] = struct{}{}
	}

	if len(rows) != len(state.Tokens) || len(rows) != len(state.CapturedReads) {
		return DerivedExportDocument{}, fmt.Errorf("%w: expected matching token, captured read, and audit counts", ErrDerivedExportProvenance)
	}
	if len(seenTokenIDs) != len(state.Tokens) {
		return DerivedExportDocument{}, fmt.Errorf("%w: duplicate or missing token coverage", ErrDerivedExportProvenance)
	}

	return DerivedExportDocument{
		FormatVersion: derivedExportFormatVersion,
		Run:           state.Run,
		BundleSHA256:  bundleSHA256,
		Rows:          rows,
	}, nil
}

func parseDerivedExportDocument(exportBytes []byte) (DerivedExportDocument, error) {
	dec := json.NewDecoder(bytes.NewReader(exportBytes))
	dec.DisallowUnknownFields()

	var document DerivedExportDocument
	if err := dec.Decode(&document); err != nil {
		return DerivedExportDocument{}, fmt.Errorf("%w: parse export: %v", ErrDerivedExportMismatch, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return DerivedExportDocument{}, fmt.Errorf("%w: trailing export bytes", ErrDerivedExportMismatch)
	}
	if document.FormatVersion != derivedExportFormatVersion {
		return DerivedExportDocument{}, fmt.Errorf("%w: format_version %q", ErrDerivedExportMismatch, document.FormatVersion)
	}
	return document, nil
}

func emitDerivedExport(document DerivedExportDocument) ([]byte, error) {
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal export: %v", ErrDerivedExportMismatch, err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical export: %v", ErrDerivedExportMismatch, err)
	}
	return canonical, nil
}

func decodeTokenMintExportPayload(deterministic []byte) (tokenMintExportPayload, error) {
	dec := json.NewDecoder(bytes.NewReader(deterministic))
	dec.DisallowUnknownFields()

	var payload tokenMintExportPayload
	if err := dec.Decode(&payload); err != nil {
		return tokenMintExportPayload{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return tokenMintExportPayload{}, errors.New("trailing payload bytes")
	}
	if err := payload.SourceSelector.ValidatePhaseTwoSelector(); err != nil {
		return tokenMintExportPayload{}, fmt.Errorf("invalid source_selector: %w", err)
	}
	switch {
	case payload.TokenID == "":
		return tokenMintExportPayload{}, errors.New("missing token_id")
	case payload.CapturedReadID == "":
		return tokenMintExportPayload{}, errors.New("missing captured_read_id")
	case payload.SourcePath == "":
		return tokenMintExportPayload{}, errors.New("missing source_path")
	case payload.OutputPath == "":
		return tokenMintExportPayload{}, errors.New("missing output_path")
	case payload.ReaderName == "":
		return tokenMintExportPayload{}, errors.New("missing reader_name")
	case payload.ReaderVersion == "":
		return tokenMintExportPayload{}, errors.New("missing reader_version")
	case payload.SourceSHA256 == "":
		return tokenMintExportPayload{}, errors.New("missing source_sha256")
	case payload.OutputSHA256 == "":
		return tokenMintExportPayload{}, errors.New("missing output_sha256")
	case payload.SourceValueRaw == "":
		return tokenMintExportPayload{}, errors.New("missing source_value_raw")
	case payload.ValueCanonical == "":
		return tokenMintExportPayload{}, errors.New("missing value_canonical")
	default:
		return payload, nil
	}
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
