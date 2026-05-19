package smoke_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/provenant/provenant/access/cli"
	"github.com/provenant/provenant/access/corepath"
	"github.com/provenant/provenant/access/httpapi"
	"github.com/provenant/provenant/access/mcpapi"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

func TestPhaseThreeCrossSurfaceSmokeMatrix(t *testing.T) {
	t.Parallel()

	fixture := newSmokeFixture()
	requestBytes, err := json.Marshal(fixture.cliRequest())
	if err != nil {
		t.Fatalf("Marshal(request) error = %v", err)
	}

	type lane struct {
		name    string
		surface corepath.Surface
		run     func(context.Context) (corepath.ReadMintBundleResult, error)
	}

	lanes := []lane{
		{
			name:    "corepath-cli-lane",
			surface: corepath.SurfaceCLI,
			run: func(ctx context.Context) (corepath.ReadMintBundleResult, error) {
				return corepath.ExecuteReadMintBundle(ctx, fixture.dependencies(), corepath.ReadMintBundleRequest{
					Surface: corepath.SurfaceCLI,
					Run:     fixture.run,
					Read:    fixture.read,
				})
			},
		},
		{
			name:    "cli",
			surface: corepath.SurfaceCLI,
			run: func(ctx context.Context) (corepath.ReadMintBundleResult, error) {
				var stdout bytes.Buffer
				runner := cli.Runner{
					Now: func() time.Time { return fixture.now },
				}
				if err := runner.Run(ctx, []string{"read-mint-bundle", "--request", "-"}, bytes.NewReader(requestBytes), &stdout); err != nil {
					return corepath.ReadMintBundleResult{}, err
				}

				var result corepath.ReadMintBundleResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					return corepath.ReadMintBundleResult{}, err
				}
				return result, nil
			},
		},
		{
			name:    "http",
			surface: corepath.SurfaceHTTP,
			run: func(_ context.Context) (corepath.ReadMintBundleResult, error) {
				var result corepath.ReadMintBundleResult
				handler := httpapi.Handler{
					Now: func() time.Time { return fixture.now },
				}

				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, httpapi.RouteReadMintBundle, bytes.NewReader(requestBytes))
				request.Header.Set("Content-Type", "application/json")
				handler.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusOK {
					return corepath.ReadMintBundleResult{}, &httpStatusError{status: recorder.Code, body: recorder.Body.String()}
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
					return corepath.ReadMintBundleResult{}, err
				}
				return result, nil
			},
		},
		{
			name:    "mcp",
			surface: corepath.SurfaceMCP,
			run: func(ctx context.Context) (corepath.ReadMintBundleResult, error) {
				surface := mcpapi.Surface{
					Now: func() time.Time { return fixture.now },
				}
				result, err := surface.CallTool(ctx, mcpapi.ToolReadMintBundle, requestBytes)
				if err != nil {
					return corepath.ReadMintBundleResult{}, err
				}
				return result.StructuredContent, nil
			},
		},
	}

	var baseline corepath.ReadMintBundleResult
	var baselineExport corepath.DerivedExportResult
	for idx, lane := range lanes {
		lane := lane
		t.Run(lane.name, func(t *testing.T) {
			result, err := lane.run(context.Background())
			if err != nil {
				t.Fatalf("%s run() error = %v", lane.name, err)
			}
			if result.Surface != lane.surface {
				t.Fatalf("%s surface = %q, want %q", lane.name, result.Surface, lane.surface)
			}

			source, err := corepath.VerifyExportSource(context.Background(), corepath.ExportSourceRequest{
				Surface:     corepath.SurfaceExport,
				BundleBytes: result.BundleBytes,
			})
			if err != nil {
				t.Fatalf("%s VerifyExportSource() error = %v", lane.name, err)
			}

			exported, err := corepath.GenerateDerivedExport(context.Background(), corepath.DerivedExportRequest{
				Surface:     corepath.SurfaceExport,
				BundleBytes: result.BundleBytes,
			})
			if err != nil {
				t.Fatalf("%s GenerateDerivedExport() error = %v", lane.name, err)
			}

			validated, err := corepath.ValidateDerivedExport(context.Background(), corepath.ValidateDerivedExportRequest{
				Surface:     corepath.SurfaceExport,
				BundleBytes: result.BundleBytes,
				ExportBytes: exported.ExportBytes,
			})
			if err != nil {
				t.Fatalf("%s ValidateDerivedExport() error = %v", lane.name, err)
			}

			if idx == 0 {
				baseline = result
				baselineExport = exported
				return
			}

			if !bytes.Equal(result.BundleBytes, baseline.BundleBytes) {
				t.Fatalf("%s BundleBytes mismatch with baseline lane", lane.name)
			}
			if result.BundleSHA256 != baseline.BundleSHA256 {
				t.Fatalf("%s BundleSHA256 = %s, want %s", lane.name, result.BundleSHA256, baseline.BundleSHA256)
			}
			if result.ReadAndMint.Token.ID != baseline.ReadAndMint.Token.ID {
				t.Fatalf("%s Token.ID = %s, want %s", lane.name, result.ReadAndMint.Token.ID, baseline.ReadAndMint.Token.ID)
			}
			if source.BundleSHA256 != baseline.BundleSHA256 {
				t.Fatalf("%s export source bundle sha = %s, want %s", lane.name, source.BundleSHA256, baseline.BundleSHA256)
			}
			if !bytes.Equal(exported.ExportBytes, baselineExport.ExportBytes) {
				t.Fatalf("%s ExportBytes mismatch with baseline lane", lane.name)
			}
			if exported.ExportSHA256 != baselineExport.ExportSHA256 {
				t.Fatalf("%s ExportSHA256 = %s, want %s", lane.name, exported.ExportSHA256, baselineExport.ExportSHA256)
			}
			if validated.BundleSHA256 != exported.BundleSHA256 {
				t.Fatalf("%s validated BundleSHA256 = %s, want %s", lane.name, validated.BundleSHA256, exported.BundleSHA256)
			}
			if validated.ExportSHA256 != exported.ExportSHA256 {
				t.Fatalf("%s validated ExportSHA256 = %s, want %s", lane.name, validated.ExportSHA256, exported.ExportSHA256)
			}
		})
	}
}

type smokeFixture struct {
	source kernel.SourceBytes
	run    kernel.Run
	read   kernel.ReadAndMintRequest
	now    time.Time
}

func newSmokeFixture() smokeFixture {
	sourceBytes := []byte(`{"invoice":{"total":123.45}}`)
	source := kernel.SourceBytes{
		Source: kernel.Source{
			Path:   "fixture://invoice.json",
			SHA256: sha256Hex(sourceBytes),
		},
		Bytes: sourceBytes,
	}

	return smokeFixture{
		source: source,
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
		now: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
	}
}

func (f smokeFixture) cliRequest() cli.ReadMintBundleRequest {
	return cli.ReadMintBundleRequest{
		Reader: "extract.json",
		Source: f.source,
		Run:    f.run,
		Read:   f.read,
	}
}

func (f smokeFixture) dependencies() corepath.Dependencies {
	binding := memio.NewBinding(f.source)
	return corepath.Dependencies{
		Scope:     binding,
		Resolver:  binding,
		Reader:    extractjson.Reader{},
		Store:     binding,
		AuditSink: binding,
		Clock:     fixedClock{t: f.now},
	}
}

type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("unexpected http status %d: %s", e.status, e.body)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mustUniqueSpan(source []byte, needle string) string {
	idx := bytes.Index(source, []byte(needle))
	if idx < 0 {
		panic("needle not found")
	}
	if strings.Count(string(source), needle) != 1 {
		panic("needle must be unique")
	}
	return fmt.Sprintf("%d-%d", idx, idx+len(needle))
}

type fixedClock struct {
	t time.Time
}

func (f fixedClock) Now() time.Time {
	return f.t
}
