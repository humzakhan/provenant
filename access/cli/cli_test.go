package cli_test

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

	"github.com/provenant/provenant/access/cli"
	"github.com/provenant/provenant/access/corepath"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

func TestRunnerReadMintBundleSmoke(t *testing.T) {
	t.Parallel()

	fixture := newCLIRequestFixture()
	requestBytes, err := json.Marshal(fixture.Request())
	if err != nil {
		t.Fatalf("Marshal(request) error = %v", err)
	}

	var stdout bytes.Buffer
	runner := cli.Runner{
		Now: func() time.Time { return fixture.now },
	}
	if err := runner.Run(context.Background(), []string{"read-mint-bundle", "--request", "-"}, bytes.NewReader(requestBytes), &stdout); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var got corepath.ReadMintBundleResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v", err)
	}

	want, err := corepath.ExecuteReadMintBundle(context.Background(), fixture.Dependencies(), corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceCLI,
		Run:     fixture.run,
		Read:    fixture.read,
	})
	if err != nil {
		t.Fatalf("ExecuteReadMintBundle() error = %v", err)
	}

	if got.Surface != corepath.SurfaceCLI {
		t.Fatalf("Surface = %q, want %q", got.Surface, corepath.SurfaceCLI)
	}
	if got.BundleSHA256 != want.BundleSHA256 {
		t.Fatalf("BundleSHA256 = %s, want %s", got.BundleSHA256, want.BundleSHA256)
	}
	if !bytes.Equal(got.BundleBytes, want.BundleBytes) {
		t.Fatal("BundleBytes mismatch with canonical corepath output")
	}
	if got.ReadAndMint.Token.ID != want.ReadAndMint.Token.ID {
		t.Fatalf("Token.ID = %s, want %s", got.ReadAndMint.Token.ID, want.ReadAndMint.Token.ID)
	}
	if got.ReadAndMint.AuditEntry.Seq != 1 {
		t.Fatalf("AuditEntry.Seq = %d, want 1", got.ReadAndMint.AuditEntry.Seq)
	}

	replayed, err := bundle.Replay(got.BundleBytes)
	if err != nil {
		t.Fatalf("Replay(BundleBytes) error = %v", err)
	}
	if replayed.Run != fixture.run {
		t.Fatalf("Replay().Run = %#v, want %#v", replayed.Run, fixture.run)
	}
	if len(replayed.CapturedReads) != 1 || len(replayed.Tokens) != 1 || len(replayed.AuditEntries) != 1 {
		t.Fatalf("Replay counts = reads:%d tokens:%d audit:%d, want 1/1/1", len(replayed.CapturedReads), len(replayed.Tokens), len(replayed.AuditEntries))
	}
}

func TestRunnerReadMintBundleFailsClosedOnMalformedInput(t *testing.T) {
	t.Parallel()

	fixture := newCLIRequestFixture()
	validRequest := fixture.Request()

	tests := []struct {
		name    string
		request any
		raw     string
		wantErr error
	}{
		{
			name:    "unknown field",
			raw:     `{"reader":"extract.json","source":{"source":{"path":"fixture://invoice.json","sha256":"abc"},"bytes":"AQ=="},"run":{"id":"run_1","tenant_id":"tenant_1","status":"running"},"read":{"source_path":"fixture://invoice.json","selector":{"json_pointer":"/invoice/total"},"output_path":"$.value"},"extra":true}`,
			wantErr: cli.ErrInvalidRequest,
		},
		{
			name: "unsupported reader",
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
			wantErr: cli.ErrUnsupportedReader,
		},
		{
			name: "mismatched source path",
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
					SourcePath: "fixture://other.json",
					Selector:   validRequest.Read.Selector,
					OutputPath: validRequest.Read.OutputPath,
					Unit:       validRequest.Read.Unit,
				},
			},
			wantErr: cli.ErrInvalidRequest,
		},
		{
			name: "invalid run status",
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
			wantErr: cli.ErrInvalidRequest,
		},
		{
			name: "empty run status",
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
					Status:   "",
				},
				Read: validRequest.Read,
			},
			wantErr: cli.ErrInvalidRequest,
		},
		{
			name:    "missing run status",
			raw:     `{"reader":"extract.json","source":{"source":{"path":"fixture://invoice.json","sha256":"abc"},"bytes":"AQ=="},"run":{"id":"run_1","tenant_id":"tenant_1"},"read":{"source_path":"fixture://invoice.json","selector":{"json_pointer":"/invoice/total"},"output_path":"$.value"}}`,
			wantErr: cli.ErrInvalidRequest,
		},
		{
			name:    "trailing data",
			raw:     mustMarshalRequest(validRequest) + "\n{}",
			wantErr: cli.ErrInvalidRequest,
		},
	}

	runner := cli.Runner{
		Now: func() time.Time { return fixture.now },
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := tc.raw
			if raw == "" {
				raw = mustMarshalRequest(tc.request)
			}

			var stdout bytes.Buffer
			err := runner.Run(context.Background(), []string{"read-mint-bundle", "--request", "-"}, strings.NewReader(raw), &stdout)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tc.wantErr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout length = %d, want 0", stdout.Len())
			}
		})
	}
}

type cliRequestFixture struct {
	source kernel.SourceBytes
	run    kernel.Run
	read   kernel.ReadAndMintRequest
	now    time.Time
}

func newCLIRequestFixture() cliRequestFixture {
	sourceBytes := []byte(`{"invoice":{"total":123.45}}`)
	source := sourceWithHash("fixture://invoice.json", sourceBytes)

	return cliRequestFixture{
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

func (f cliRequestFixture) Request() cli.ReadMintBundleRequest {
	return cli.ReadMintBundleRequest{
		Reader: "extract.json",
		Source: f.source,
		Run:    f.run,
		Read:   f.read,
	}
}

func (f cliRequestFixture) Dependencies() corepath.Dependencies {
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
