package mcpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/humzakhan/provenant/access/cli"
	"github.com/humzakhan/provenant/access/corepath"
	"github.com/humzakhan/provenant/access/httpapi"
	"github.com/humzakhan/provenant/access/mcpapi"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

func TestSurfaceReadMintBundleSmoke(t *testing.T) {
	t.Parallel()

	fixture := newMCPRequestFixture()
	requestBytes, err := json.Marshal(fixture.Request())
	if err != nil {
		t.Fatalf("Marshal(request) error = %v", err)
	}

	surface := mcpapi.Surface{
		Now: func() time.Time { return fixture.now },
	}
	got, err := surface.CallTool(context.Background(), mcpapi.ToolReadMintBundle, requestBytes)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}

	wantCorepath, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceMCP,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	var cliStdout bytes.Buffer
	cliRunner := cli.Runner{
		Now: func() time.Time { return fixture.now },
	}
	if err := cliRunner.Run(context.Background(), []string{"read-mint-bundle", "--request", "-"}, bytes.NewReader(requestBytes), &cliStdout); err != nil {
		t.Fatalf("CLI Run() error = %v", err)
	}

	var wantCLI corepath.ReadMintBundleResult
	if err := json.Unmarshal(cliStdout.Bytes(), &wantCLI); err != nil {
		t.Fatalf("Unmarshal(cli stdout) error = %v", err)
	}

	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, httpapi.RouteReadMintBundle, bytes.NewReader(requestBytes))
	httpRequest.Header.Set("Content-Type", "application/json")
	httpHandler := httpapi.Handler{
		Now: func() time.Time { return fixture.now },
	}
	httpHandler.ServeHTTP(recorder, httpRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP ServeHTTP() status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var wantHTTP corepath.ReadMintBundleResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &wantHTTP); err != nil {
		t.Fatalf("Unmarshal(http response) error = %v", err)
	}

	if got.StructuredContent.Surface != corepath.SurfaceMCP {
		t.Fatalf("Surface = %q, want %q", got.StructuredContent.Surface, corepath.SurfaceMCP)
	}
	if got.StructuredContent.BundleSHA256 != wantCorepath.BundleSHA256 || got.StructuredContent.BundleSHA256 != wantCLI.BundleSHA256 || got.StructuredContent.BundleSHA256 != wantHTTP.BundleSHA256 {
		t.Fatalf("BundleSHA256 = %s, want corepath %s cli %s http %s", got.StructuredContent.BundleSHA256, wantCorepath.BundleSHA256, wantCLI.BundleSHA256, wantHTTP.BundleSHA256)
	}
	if !bytes.Equal(got.StructuredContent.BundleBytes, wantCorepath.BundleBytes) || !bytes.Equal(got.StructuredContent.BundleBytes, wantCLI.BundleBytes) || !bytes.Equal(got.StructuredContent.BundleBytes, wantHTTP.BundleBytes) {
		t.Fatal("BundleBytes mismatch with canonical corepath, cli, or http output")
	}
	if got.StructuredContent.ReadAndMint.Token.ID != wantCorepath.ReadAndMint.Token.ID || got.StructuredContent.ReadAndMint.Token.ID != wantCLI.ReadAndMint.Token.ID || got.StructuredContent.ReadAndMint.Token.ID != wantHTTP.ReadAndMint.Token.ID {
		t.Fatalf("Token.ID = %s, want corepath %s cli %s http %s", got.StructuredContent.ReadAndMint.Token.ID, wantCorepath.ReadAndMint.Token.ID, wantCLI.ReadAndMint.Token.ID, wantHTTP.ReadAndMint.Token.ID)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "text" || !strings.Contains(got.Content[0].Text, got.StructuredContent.BundleSHA256) {
		t.Fatalf("content = %#v, want one summary block containing bundle sha", got.Content)
	}
}

func TestSurfaceRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	fixture := newMCPRequestFixture()
	validRequest := fixture.Request()
	surface := mcpapi.Surface{
		Now: func() time.Time { return fixture.now },
	}

	tests := []struct {
		name      string
		toolName  string
		raw       string
		request   any
		wantError error
	}{
		{
			name:      "unsupported tool",
			toolName:  "provenant.other",
			raw:       mustMarshalRequest(validRequest),
			wantError: mcpapi.ErrUnsupportedTool,
		},
		{
			name:      "unknown field",
			toolName:  mcpapi.ToolReadMintBundle,
			raw:       `{"reader":"extract.json","source":{"source":{"path":"fixture://invoice.json","sha256":"abc"},"bytes":"AQ=="},"run":{"id":"run_1","tenant_id":"tenant_1","status":"running"},"read":{"source_path":"fixture://invoice.json","selector":{"json_pointer":"/invoice/total"},"output_path":"$.value"},"extra":true}`,
			wantError: mcpapi.ErrInvalidArguments,
		},
		{
			name:     "unsupported reader",
			toolName: mcpapi.ToolReadMintBundle,
			request: struct {
				Reader string                    `json:"reader"`
				Source kernel.SourceBytes        `json:"source"`
				Run    kernel.Run                `json:"run"`
				Read   kernel.ReadAndMintRequest `json:"read"`
			}{
				Reader: "extract.csv_rows",
				Source: validRequest.Source,
				Run:    validRequest.Run,
				Read:   validRequest.Read,
			},
			wantError: mcpapi.ErrUnsupportedReader,
		},
		{
			name:      "missing arguments",
			toolName:  mcpapi.ToolReadMintBundle,
			raw:       "",
			wantError: mcpapi.ErrInvalidArguments,
		},
		{
			name:     "invalid run status",
			toolName: mcpapi.ToolReadMintBundle,
			request: struct {
				Reader string                    `json:"reader"`
				Source kernel.SourceBytes        `json:"source"`
				Run    kernel.Run                `json:"run"`
				Read   kernel.ReadAndMintRequest `json:"read"`
			}{
				Reader: validRequest.Reader,
				Source: validRequest.Source,
				Run: kernel.Run{
					ID:       validRequest.Run.ID,
					TenantID: validRequest.Run.TenantID,
					Status:   kernel.RunStatus("paused"),
				},
				Read: validRequest.Read,
			},
			wantError: mcpapi.ErrInvalidArguments,
		},
		{
			name:     "missing source bytes",
			toolName: mcpapi.ToolReadMintBundle,
			request: struct {
				Reader string                    `json:"reader"`
				Source kernel.SourceBytes        `json:"source"`
				Run    kernel.Run                `json:"run"`
				Read   kernel.ReadAndMintRequest `json:"read"`
			}{
				Reader: validRequest.Reader,
				Source: kernel.SourceBytes{
					Source: validRequest.Source.Source,
				},
				Run:  validRequest.Run,
				Read: validRequest.Read,
			},
			wantError: mcpapi.ErrInvalidArguments,
		},
		{
			name:     "missing selector",
			toolName: mcpapi.ToolReadMintBundle,
			request: struct {
				Reader string                    `json:"reader"`
				Source kernel.SourceBytes        `json:"source"`
				Run    kernel.Run                `json:"run"`
				Read   kernel.ReadAndMintRequest `json:"read"`
			}{
				Reader: validRequest.Reader,
				Source: validRequest.Source,
				Run:    validRequest.Run,
				Read: kernel.ReadAndMintRequest{
					SourcePath: validRequest.Read.SourcePath,
					OutputPath: validRequest.Read.OutputPath,
					Unit:       validRequest.Read.Unit,
				},
			},
			wantError: mcpapi.ErrInvalidArguments,
		},
		{
			name:      "trailing data",
			toolName:  mcpapi.ToolReadMintBundle,
			raw:       mustMarshalRequest(validRequest) + "\n{}",
			wantError: mcpapi.ErrInvalidArguments,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := tc.raw
			if raw == "" && tc.request != nil {
				raw = mustMarshalRequest(tc.request)
			}

			_, err := surface.CallTool(context.Background(), tc.toolName, json.RawMessage(raw))
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("CallTool() error = %v, want %v", err, tc.wantError)
			}
		})
	}
}

func TestMetadataExposure(t *testing.T) {
	t.Parallel()

	tools := mcpapi.Tools()
	if len(tools) != 1 {
		t.Fatalf("len(Tools()) = %d, want 1", len(tools))
	}
	if tools[0].Name != mcpapi.ToolReadMintBundle {
		t.Fatalf("Tools()[0].Name = %q, want %q", tools[0].Name, mcpapi.ToolReadMintBundle)
	}
	if !json.Valid(tools[0].InputSchema) {
		t.Fatal("Tools()[0].InputSchema is not valid JSON")
	}

	prompts := mcpapi.Prompts()
	if len(prompts) != 1 {
		t.Fatalf("len(Prompts()) = %d, want 1", len(prompts))
	}
	prompt, err := mcpapi.GetPrompt(prompts[0].Name)
	if err != nil {
		t.Fatalf("GetPrompt() error = %v", err)
	}
	if !strings.Contains(prompt.Text, mcpapi.ToolReadMintBundle) || !strings.Contains(prompt.Text, mcpapi.ResourceRequestSchema) {
		t.Fatalf("Prompt text = %q, want tool and resource references", prompt.Text)
	}

	resources := mcpapi.Resources()
	if len(resources) != 1 {
		t.Fatalf("len(Resources()) = %d, want 1", len(resources))
	}
	resource, err := mcpapi.ReadResource(resources[0].URI)
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if resource.URI != mcpapi.ResourceRequestSchema {
		t.Fatalf("Resource URI = %q, want %q", resource.URI, mcpapi.ResourceRequestSchema)
	}
	if resource.MIMEType != "application/json" || !json.Valid([]byte(resource.Text)) {
		t.Fatalf("Resource = %#v, want application/json with valid schema", resource)
	}
}

type mcpRequestFixture struct {
	source kernel.SourceBytes
	run    kernel.Run
	read   kernel.ReadAndMintRequest
	now    time.Time
}

func newMCPRequestFixture() mcpRequestFixture {
	sourceBytes := []byte(`{"invoice":{"total":123.45}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)

	return mcpRequestFixture{
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

func (f mcpRequestFixture) Request() mcpapi.ReadMintBundleRequest {
	return mcpapi.ReadMintBundleRequest{
		Reader: "extract.json",
		Source: f.source,
		Run:    f.run,
		Read:   f.read,
	}
}

func (f mcpRequestFixture) Dependencies() corepath.Dependencies {
	binding := memio.NewBinding(f.source)
	return corepath.Dependencies{
		Scope:     binding,
		Resolver:  binding,
		Reader:    extractjson.Reader{},
		Store:     binding,
		AuditSink: binding,
		Clock: fixedClock{
			t: f.now,
		},
	}
}

func mustMarshalRequest(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
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
