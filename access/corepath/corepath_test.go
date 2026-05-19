package corepath_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
