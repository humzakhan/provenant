package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/humzakhan/provenant/access/cli"
	"github.com/humzakhan/provenant/access/corepath"
	"github.com/humzakhan/provenant/access/httpapi"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

func TestHandlerReadMintBundleSmoke(t *testing.T) {
	t.Parallel()

	fixture := newHTTPRequestFixture()
	requestBytes, err := json.Marshal(fixture.Request())
	if err != nil {
		t.Fatalf("Marshal(request) error = %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, httpapi.RouteReadMintBundle, bytes.NewReader(requestBytes))
	request.Header.Set("Content-Type", "application/json")

	handler := httpapi.Handler{
		Now: func() time.Time { return fixture.now },
	}
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var got corepath.ReadMintBundleResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(response) error = %v", err)
	}

	wantCorepath, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceHTTP,
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

	if got.Surface != corepath.SurfaceHTTP {
		t.Fatalf("Surface = %q, want %q", got.Surface, corepath.SurfaceHTTP)
	}
	if got.BundleSHA256 != wantCorepath.BundleSHA256 || got.BundleSHA256 != wantCLI.BundleSHA256 {
		t.Fatalf("BundleSHA256 = %s, want corepath %s and cli %s", got.BundleSHA256, wantCorepath.BundleSHA256, wantCLI.BundleSHA256)
	}
	if !bytes.Equal(got.BundleBytes, wantCorepath.BundleBytes) || !bytes.Equal(got.BundleBytes, wantCLI.BundleBytes) {
		t.Fatal("BundleBytes mismatch with canonical corepath or cli output")
	}
	if got.ReadAndMint.Token.ID != wantCorepath.ReadAndMint.Token.ID || got.ReadAndMint.Token.ID != wantCLI.ReadAndMint.Token.ID {
		t.Fatalf("Token.ID = %s, want corepath %s and cli %s", got.ReadAndMint.Token.ID, wantCorepath.ReadAndMint.Token.ID, wantCLI.ReadAndMint.Token.ID)
	}
	if got.ReadAndMint.AuditEntry.Seq != 1 {
		t.Fatalf("AuditEntry.Seq = %d, want 1", got.ReadAndMint.AuditEntry.Seq)
	}
}

func TestHandlerReadMintBundleRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	fixture := newHTTPRequestFixture()
	validRequest := fixture.Request()

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		request     any
		wantStatus  int
		wantCode    string
		wantErr     error
	}{
		{
			name:        "unknown field",
			method:      http.MethodPost,
			path:        httpapi.RouteReadMintBundle,
			contentType: "application/json",
			body:        `{"reader":"extract.json","source":{"source":{"path":"fixture://invoice.json","sha256":"abc"},"bytes":"AQ=="},"run":{"id":"run_1","tenant_id":"tenant_1","status":"running"},"read":{"source_path":"fixture://invoice.json","selector":{"json_pointer":"/invoice/total"},"output_path":"$.value"},"extra":true}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "validation_failed",
			wantErr:     httpapi.ErrInvalidRequest,
		},
		{
			name:        "unsupported reader",
			method:      http.MethodPost,
			path:        httpapi.RouteReadMintBundle,
			contentType: "application/json",
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
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantErr:    httpapi.ErrUnsupportedReader,
		},
		{
			name:        "invalid run status",
			method:      http.MethodPost,
			path:        httpapi.RouteReadMintBundle,
			contentType: "application/json",
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
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantErr:    httpapi.ErrInvalidRequest,
		},
		{
			name:        "missing source bytes",
			method:      http.MethodPost,
			path:        httpapi.RouteReadMintBundle,
			contentType: "application/json",
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
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantErr:    httpapi.ErrInvalidRequest,
		},
		{
			name:        "missing selector",
			method:      http.MethodPost,
			path:        httpapi.RouteReadMintBundle,
			contentType: "application/json",
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
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantErr:    httpapi.ErrInvalidRequest,
		},
		{
			name:       "missing content type",
			method:     http.MethodPost,
			path:       httpapi.RouteReadMintBundle,
			body:       mustMarshalRequest(validRequest),
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "unsupported_media_type",
			wantErr:    httpapi.ErrInvalidRequest,
		},
		{
			name:        "wrong content type",
			method:      http.MethodPost,
			path:        httpapi.RouteReadMintBundle,
			contentType: "text/plain",
			body:        mustMarshalRequest(validRequest),
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    "unsupported_media_type",
			wantErr:     httpapi.ErrInvalidRequest,
		},
		{
			name:       "wrong method",
			method:     http.MethodGet,
			path:       httpapi.RouteReadMintBundle,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "not found",
			method:     http.MethodPost,
			path:       "/v1/other",
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
	}

	handler := httpapi.Handler{
		Now: func() time.Time { return fixture.now },
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := tc.body
			if body == "" && tc.request != nil {
				body = mustMarshalRequest(tc.request)
			}

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			if tc.contentType != "" {
				request.Header.Set("Content-Type", tc.contentType)
			}

			handler.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("ServeHTTP() status = %d, want %d", recorder.Code, tc.wantStatus)
			}

			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("Unmarshal(error response) error = %v", err)
			}
			if got.Error.Code != tc.wantCode {
				t.Fatalf("error.code = %q, want %q", got.Error.Code, tc.wantCode)
			}
			if got.Error.Message == "" {
				t.Fatal("error.message is empty")
			}
			if tc.wantErr != nil && !strings.Contains(got.Error.Message, tc.wantErr.Error()) {
				t.Fatalf("error.message = %q, want substring %q", got.Error.Message, tc.wantErr.Error())
			}
		})
	}
}

type httpRequestFixture struct {
	source kernel.SourceBytes
	run    kernel.Run
	read   kernel.ReadAndMintRequest
	now    time.Time
}

func newHTTPRequestFixture() httpRequestFixture {
	sourceBytes := []byte(`{"invoice":{"total":123.45}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)

	return httpRequestFixture{
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

func (f httpRequestFixture) Request() httpapi.ReadMintBundleRequest {
	return httpapi.ReadMintBundleRequest{
		Reader: "extract.json",
		Source: f.source,
		Run:    f.run,
		Read:   f.read,
	}
}

func (f httpRequestFixture) Dependencies() corepath.Dependencies {
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

func mustUniqueSpan(data []byte, needle string) string {
	idx := bytes.Index(data, []byte(needle))
	if idx < 0 {
		panic("needle not found")
	}
	if bytes.Contains(data[idx+1:], []byte(needle)) {
		panic("needle must be unique")
	}
	return strings.Join([]string{
		strconv.Itoa(idx),
		strconv.Itoa(idx + len(needle)),
	}, "-")
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
