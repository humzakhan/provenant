package corepath_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/provenant/provenant/access/corepath"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

func TestExecuteReadMintBundleSupportsPlannedAccessSurfaces(t *testing.T) {
	t.Parallel()

	for _, surface := range []corepath.Surface{
		corepath.SurfaceCLI,
		corepath.SurfaceHTTP,
		corepath.SurfaceMCP,
	} {
		surface := surface
		t.Run(string(surface), func(t *testing.T) {
			t.Parallel()

			fixture := newSmokeFixture(surface)
			result, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), fixture.Request())
			if err != nil {
				t.Fatalf("ExecuteReadMintBundle() error = %v", err)
			}
			if result.Surface != surface {
				t.Fatalf("Surface = %q, want %q", result.Surface, surface)
			}
			if result.ReadAndMint.Token.ID == "" {
				t.Fatal("Token.ID is empty")
			}
			if len(result.BundleBytes) == 0 {
				t.Fatal("BundleBytes is empty")
			}
			if result.BundleSHA256 != sha256Hex(result.BundleBytes) {
				t.Fatalf("BundleSHA256 = %s, want %s", result.BundleSHA256, sha256Hex(result.BundleBytes))
			}
			if err := bundle.Verify(result.BundleBytes); err != nil {
				t.Fatalf("Verify(BundleBytes) error = %v", err)
			}

			replayed, err := bundle.Replay(result.BundleBytes)
			if err != nil {
				t.Fatalf("Replay(BundleBytes) error = %v", err)
			}
			if replayed.Run != fixture.run {
				t.Fatalf("Replay().Run = %#v, want %#v", replayed.Run, fixture.run)
			}
			if len(replayed.CapturedReads) != 1 || len(replayed.Tokens) != 1 || len(replayed.AuditEntries) != 1 {
				t.Fatalf("Replay counts = reads:%d tokens:%d audit:%d, want 1/1/1", len(replayed.CapturedReads), len(replayed.Tokens), len(replayed.AuditEntries))
			}
			if replayed.Tokens[0].ID != result.ReadAndMint.Token.ID {
				t.Fatalf("Replay token ID = %s, want %s", replayed.Tokens[0].ID, result.ReadAndMint.Token.ID)
			}
			if replayed.Tokens[0].Source.CapturedReadID != replayed.CapturedReads[0].ID {
				t.Fatalf("Replay token captured read = %s, want %s", replayed.Tokens[0].Source.CapturedReadID, replayed.CapturedReads[0].ID)
			}
			if replayed.CapturedReads[0].OutputSHA256 != result.ReadAndMint.ReadResult.CapturedRead.OutputSHA256 {
				t.Fatalf("Replay output hash = %s, want %s", replayed.CapturedReads[0].OutputSHA256, result.ReadAndMint.ReadResult.CapturedRead.OutputSHA256)
			}
		})
	}
}

func TestExecuteReadMintBundleRejectsUnsupportedSurfaces(t *testing.T) {
	t.Parallel()

	for _, surface := range []corepath.Surface{"", "batch", corepath.SurfaceExport} {
		t.Run(string(surface), func(t *testing.T) {
			t.Parallel()

			_, err := corepath.ExecuteReadMintBundle(context.Background(), corepath.Dependencies{}, corepath.ReadMintBundleRequest{Surface: surface})
			if !errors.Is(err, corepath.ErrInvalidSurface) {
				t.Fatalf("ExecuteReadMintBundle() error = %v, want %v", err, corepath.ErrInvalidSurface)
			}
		})
	}
}

func TestSnapshotBundleFailsClosedOnDivergentDependencies(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceCLI)

	_, err := corepath.SnapshotBundle(
		context.Background(),
		fixture.binding,
		divergentStore{Binding: fixture.binding},
		fixture.binding,
		fixture.run,
	)
	if !errors.Is(err, corepath.ErrTransactionalSubstrateMismatch) {
		t.Fatalf("SnapshotBundle() error = %v, want %v", err, corepath.ErrTransactionalSubstrateMismatch)
	}
}

func TestSnapshotBundleFailsClosedWithoutCapturedReadStore(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceCLI)

	_, err := corepath.SnapshotBundle(
		context.Background(),
		fixture.binding,
		storeWithoutCapturedReads{binding: fixture.binding},
		fixture.binding,
		fixture.run,
	)
	if !errors.Is(err, corepath.ErrStoreMissingCapturedReads) {
		t.Fatalf("SnapshotBundle() error = %v, want %v", err, corepath.ErrStoreMissingCapturedReads)
	}
}

func TestVerifyExportSourceReplaysOnlyBundleBytes(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceHTTP)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), fixture.Request())
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	result, err := corepath.VerifyExportSource(context.Background(), corepath.ExportSourceRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
	})
	if err != nil {
		t.Fatalf("VerifyExportSource() error = %v", err)
	}
	if result.Surface != corepath.SurfaceExport {
		t.Fatalf("Surface = %q, want %q", result.Surface, corepath.SurfaceExport)
	}
	if result.BundleSHA256 != sha256Hex(executed.BundleBytes) {
		t.Fatalf("BundleSHA256 = %s, want %s", result.BundleSHA256, sha256Hex(executed.BundleBytes))
	}
	if result.State.Run != fixture.run {
		t.Fatalf("State.Run = %#v, want %#v", result.State.Run, fixture.run)
	}
	if len(result.State.CapturedReads) != 1 || len(result.State.Tokens) != 1 || len(result.State.AuditEntries) != 1 {
		t.Fatalf("State counts = reads:%d tokens:%d audit:%d, want 1/1/1", len(result.State.CapturedReads), len(result.State.Tokens), len(result.State.AuditEntries))
	}
}

func TestVerifyExportSourceFailsClosedOnInvalidInput(t *testing.T) {
	t.Parallel()

	if _, err := corepath.VerifyExportSource(context.Background(), corepath.ExportSourceRequest{Surface: corepath.SurfaceCLI, BundleBytes: []byte(`{}`)}); !errors.Is(err, corepath.ErrExportRequiresExportSurface) {
		t.Fatalf("VerifyExportSource(non-export) error = %v, want %v", err, corepath.ErrExportRequiresExportSurface)
	}
	if _, err := corepath.VerifyExportSource(context.Background(), corepath.ExportSourceRequest{Surface: corepath.SurfaceExport}); !errors.Is(err, corepath.ErrMissingBundleBytes) {
		t.Fatalf("VerifyExportSource(empty) error = %v, want %v", err, corepath.ErrMissingBundleBytes)
	}

	fixture := newSmokeFixture(corepath.SurfaceMCP)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), fixture.Request())
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	tampered := bytes.Replace(executed.BundleBytes, []byte("123.45"), []byte("999.99"), 1)
	if _, err := corepath.VerifyExportSource(context.Background(), corepath.ExportSourceRequest{Surface: corepath.SurfaceExport, BundleBytes: tampered}); !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("VerifyExportSource(tampered) error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestVerifyExportSourceRejectsReplayValidNonCanonicalBytes(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceExport)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceCLI,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	nonCanonical := append(append([]byte(nil), executed.BundleBytes...), '\n', '\t')
	replayed, err := bundle.Replay(nonCanonical)
	if err != nil {
		t.Fatalf("Replay(nonCanonical) error = %v", err)
	}
	if replayed.Run != fixture.run {
		t.Fatalf("Replay(nonCanonical).Run = %#v, want %#v", replayed.Run, fixture.run)
	}

	if _, err := corepath.VerifyExportSource(context.Background(), corepath.ExportSourceRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: nonCanonical,
	}); !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("VerifyExportSource(nonCanonical) error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestGenerateDerivedExportUsesCanonicalBundleBytesOnly(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceExport)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceCLI,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	result, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
	})
	if err != nil {
		t.Fatalf("GenerateDerivedExport() error = %v", err)
	}
	if result.Surface != corepath.SurfaceExport {
		t.Fatalf("Surface = %q, want %q", result.Surface, corepath.SurfaceExport)
	}
	if result.Document.FormatVersion == "" {
		t.Fatal("Document.FormatVersion is empty")
	}
	if result.Document.BundleSHA256 != sha256Hex(executed.BundleBytes) {
		t.Fatalf("Document.BundleSHA256 = %s, want %s", result.Document.BundleSHA256, sha256Hex(executed.BundleBytes))
	}
	if result.BundleSHA256 != sha256Hex(executed.BundleBytes) {
		t.Fatalf("BundleSHA256 = %s, want %s", result.BundleSHA256, sha256Hex(executed.BundleBytes))
	}
	if result.ExportSHA256 != sha256Hex(result.ExportBytes) {
		t.Fatalf("ExportSHA256 = %s, want %s", result.ExportSHA256, sha256Hex(result.ExportBytes))
	}
	if len(result.Document.Rows) != 1 {
		t.Fatalf("len(Document.Rows) = %d, want 1", len(result.Document.Rows))
	}
	row := result.Document.Rows[0]
	if row.TokenID != executed.ReadAndMint.Token.ID {
		t.Fatalf("row.TokenID = %s, want %s", row.TokenID, executed.ReadAndMint.Token.ID)
	}
	if row.CapturedReadID != executed.ReadAndMint.ReadResult.CapturedRead.ID {
		t.Fatalf("row.CapturedReadID = %s, want %s", row.CapturedReadID, executed.ReadAndMint.ReadResult.CapturedRead.ID)
	}
	if row.OutputSHA256 != executed.ReadAndMint.ReadResult.CapturedRead.OutputSHA256 {
		t.Fatalf("row.OutputSHA256 = %s, want %s", row.OutputSHA256, executed.ReadAndMint.ReadResult.CapturedRead.OutputSHA256)
	}
	if row.AuditSeq != 1 {
		t.Fatalf("row.AuditSeq = %d, want 1", row.AuditSeq)
	}
}

func TestValidateDerivedExportPassesAgainstCanonicalBundleFixture(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceExport)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceHTTP,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}
	exported, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
	})
	if err != nil {
		t.Fatalf("GenerateDerivedExport() error = %v", err)
	}

	validated, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
		ExportBytes: exported.ExportBytes,
	})
	if err != nil {
		t.Fatalf("ValidateDerivedExport() error = %v", err)
	}
	if validated.BundleSHA256 != exported.BundleSHA256 {
		t.Fatalf("BundleSHA256 = %s, want %s", validated.BundleSHA256, exported.BundleSHA256)
	}
	if validated.ExportSHA256 != exported.ExportSHA256 {
		t.Fatalf("ExportSHA256 = %s, want %s", validated.ExportSHA256, exported.ExportSHA256)
	}
	if validated.Document.BundleSHA256 != exported.Document.BundleSHA256 {
		t.Fatalf("Document.BundleSHA256 = %s, want %s", validated.Document.BundleSHA256, exported.Document.BundleSHA256)
	}
}

func TestGenerateDerivedExportFailsClosedOnMissingOrMismatchedCapturedProvenance(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceExport)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceCLI,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	var missingReadBundle struct {
		Format        string                `json:"format"`
		Run           kernel.Run            `json:"run"`
		CapturedReads []kernel.CapturedRead `json:"captured_reads"`
		Tokens        []kernel.Token        `json:"tokens"`
		AuditJSONL    string                `json:"audit_jsonl"`
	}
	if err := json.Unmarshal(executed.BundleBytes, &missingReadBundle); err != nil {
		t.Fatalf("Unmarshal(BundleBytes) error = %v", err)
	}
	missingReadBundle.CapturedReads = append([]kernel.CapturedRead(nil), missingReadBundle.CapturedReads...)
	missingReadBundle.CapturedReads[0].ID = "read_missing"
	missingReadBundleBytes, err := json.Marshal(missingReadBundle)
	if err != nil {
		t.Fatalf("Marshal(missingReadBundle) error = %v", err)
	}
	if _, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: missingReadBundleBytes,
	}); !errors.Is(err, bundle.ErrMissingArtifact) {
		t.Fatalf("GenerateDerivedExport(missing read) error = %v, want %v", err, bundle.ErrMissingArtifact)
	}

	var mismatchedBundle struct {
		Format        string                `json:"format"`
		Run           kernel.Run            `json:"run"`
		CapturedReads []kernel.CapturedRead `json:"captured_reads"`
		Tokens        []kernel.Token        `json:"tokens"`
		AuditJSONL    string                `json:"audit_jsonl"`
	}
	if err := json.Unmarshal(executed.BundleBytes, &mismatchedBundle); err != nil {
		t.Fatalf("Unmarshal(BundleBytes) error = %v", err)
	}
	mismatchedBundle.Tokens = append([]kernel.Token(nil), mismatchedBundle.Tokens...)
	mismatchedBundle.Tokens[0].Source.SourcePath = "fixture://other.json"
	mismatchedBundleBytes, err := json.Marshal(mismatchedBundle)
	if err != nil {
		t.Fatalf("Marshal(mismatchedBundle) error = %v", err)
	}
	if _, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: mismatchedBundleBytes,
	}); !errors.Is(err, bundle.ErrTamperDetected) {
		t.Fatalf("GenerateDerivedExport(mismatched provenance) error = %v, want %v", err, bundle.ErrTamperDetected)
	}
}

func TestValidateDerivedExportFailsClosedOnTamperedOrNonCanonicalExport(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceExport)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceMCP,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}
	exported, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
	})
	if err != nil {
		t.Fatalf("GenerateDerivedExport() error = %v", err)
	}

	tamperedExport := bytes.Replace(exported.ExportBytes, []byte(exported.Document.Rows[0].ValueCanonical), []byte("999.000000000"), 1)
	if _, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
		ExportBytes: tamperedExport,
	}); !errors.Is(err, corepath.ErrDerivedExportMismatch) {
		t.Fatalf("ValidateDerivedExport(tampered export) error = %v, want %v", err, corepath.ErrDerivedExportMismatch)
	}

	nonCanonicalExport := append(append([]byte(nil), exported.ExportBytes...), '\n')
	if _, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
		Surface:     corepath.SurfaceExport,
		BundleBytes: executed.BundleBytes,
		ExportBytes: nonCanonicalExport,
	}); !errors.Is(err, corepath.ErrDerivedExportMismatch) {
		t.Fatalf("ValidateDerivedExport(non-canonical export) error = %v, want %v", err, corepath.ErrDerivedExportMismatch)
	}
}

func TestDerivedExportPathsFailClosedOnMissingOrMalformedInput(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture(corepath.SurfaceExport)
	executed, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceCLI,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{
			name: "generate requires export surface",
			run: func() error {
				_, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
					Surface:     corepath.SurfaceCLI,
					BundleBytes: executed.BundleBytes,
				})
				return err
			},
			wantErr: corepath.ErrExportRequiresExportSurface,
		},
		{
			name: "generate requires bundle bytes",
			run: func() error {
				_, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
					Surface: corepath.SurfaceExport,
				})
				return err
			},
			wantErr: corepath.ErrMissingBundleBytes,
		},
		{
			name: "validate requires export bytes",
			run: func() error {
				_, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
					Surface:     corepath.SurfaceExport,
					BundleBytes: executed.BundleBytes,
				})
				return err
			},
			wantErr: corepath.ErrMissingExportBytes,
		},
		{
			name: "validate rejects malformed export json",
			run: func() error {
				_, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
					Surface:     corepath.SurfaceExport,
					BundleBytes: executed.BundleBytes,
					ExportBytes: []byte(`{"format_version":`),
				})
				return err
			},
			wantErr: corepath.ErrDerivedExportMismatch,
		},
		{
			name: "validate rejects unknown export fields",
			run: func() error {
				_, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
					Surface:     corepath.SurfaceExport,
					BundleBytes: executed.BundleBytes,
					ExportBytes: []byte(`{"format_version":"phase3.derived_export/v1","run":{"id":"run_1","tenant_id":"tenant_1","status":"running"},"bundle_sha256":"` + sha256Hex(executed.BundleBytes) + `","rows":[],"extra":true}`),
				})
				return err
			},
			wantErr: corepath.ErrDerivedExportMismatch,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.run(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

type smokeFixture struct {
	surface corepath.Surface
	run     kernel.Run
	read    kernel.ReadAndMintRequest
	reader  kernel.SourceReader
	binding *memio.Binding
	clock   fixedClock
}

func newSmokeFixture(surface corepath.Surface) smokeFixture {
	sourceBytes := []byte(`{"invoice":{"total":123.45}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)

	return smokeFixture{
		surface: surface,
		run: kernel.Run{
			ID:       "run_1",
			TenantID: "tenant_1",
			Status:   kernel.RunRunning,
		},
		read: kernel.ReadAndMintRequest{
			SourcePath: source.Source.Path,
			Selector: kernel.SourceSelector{
				JSONPointer: "/invoice/total",
				JSONPath:    "$.invoice.total",
				ByteSpan:    mustUniqueSpan(sourceBytes, "123.45"),
			},
			OutputPath: "$.value",
			Unit:       "USD",
		},
		reader:  extractjson.Reader{},
		binding: memio.NewBinding(source),
		clock: fixedClock{
			t: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		},
	}
}

func (f smokeFixture) Dependencies() corepath.Dependencies {
	return corepath.Dependencies{
		Scope:     f.binding,
		Resolver:  f.binding,
		Reader:    f.reader,
		Store:     f.binding,
		AuditSink: f.binding,
		Clock:     f.clock,
	}
}

func (f smokeFixture) Request() corepath.ReadMintBundleRequest {
	return corepath.ReadMintBundleRequest{
		Surface: f.surface,
		Run:     f.run,
		Read:    f.read,
	}
}

func sourceWithHash(path string, data []byte) kernel.SourceBytes {
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:   path,
			SHA256: sha256Hex(data),
		},
		Bytes: append([]byte(nil), data...),
	}
}

func mustUniqueSpan(source []byte, raw string) string {
	start := strings.Index(string(source), raw)
	if start < 0 {
		panic(fmt.Sprintf("raw value %q not found in %s", raw, source))
	}
	if strings.Count(string(source), raw) != 1 {
		panic(fmt.Sprintf("raw value %q appears multiple times in %s", raw, source))
	}
	return fmt.Sprintf("%d-%d", start, start+len(raw))
}

func sha256Hex(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}

type fixedClock struct {
	t time.Time
}

func (f fixedClock) Now() time.Time {
	return f.t
}

type divergentStore struct {
	*memio.Binding
}

func (s divergentStore) TransactionalSubstrate() string {
	return "provenant.substrate/test/divergent/v1"
}

type storeWithoutCapturedReads struct {
	binding *memio.Binding
}

func (s storeWithoutCapturedReads) TransactionalSubstrate() string {
	return s.binding.TransactionalSubstrate()
}

func (s storeWithoutCapturedReads) PutToken(ctx context.Context, tx kernel.Tx, token kernel.Token) error {
	return s.binding.PutToken(ctx, tx, token)
}

func (s storeWithoutCapturedReads) Tokens(ctx context.Context, tx kernel.Tx, runID string) ([]kernel.Token, error) {
	return s.binding.Tokens(ctx, tx, runID)
}
