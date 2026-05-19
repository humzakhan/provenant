package corepath

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
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
	ErrExportRequiresExportSurface    = errors.New("corepath: export verification requires export surface")
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

	return ExportSourceResult{
		Surface:      request.Surface,
		State:        state,
		BundleSHA256: sha256Hex(request.BundleBytes),
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
