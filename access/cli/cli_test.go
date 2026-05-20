package cli_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/provenant/provenant/access/cli"
	"github.com/provenant/provenant/access/corepath"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
	reconrequest "github.com/provenant/provenant/reconcile/request"
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
			name: "missing source bytes",
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
			wantErr: cli.ErrInvalidRequest,
		},
		{
			name: "missing selector",
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

func TestRunnerReconcileFlagValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{
			name:    "missing request",
			args:    []string{"reconcile", "--out", "report.json"},
			wantErr: cli.ErrMissingRequestPath,
		},
		{
			name:    "missing out",
			args:    []string{"reconcile", "--request", "request.json"},
			wantErr: cli.ErrMissingOutputPath,
		},
		{
			name:    "unknown flag",
			args:    []string{"reconcile", "--request", "request.json", "--out", "report.json", "--format", "json"},
			wantErr: cli.ErrUsage,
		},
		{
			name:    "duplicate flag",
			args:    []string{"reconcile", "--request", "request.json", "--request", "other.json", "--out", "report.json"},
			wantErr: cli.ErrUsage,
		},
		{
			name:    "trailing arg",
			args:    []string{"reconcile", "--request", "request.json", "--out", "report.json", "extra"},
			wantErr: cli.ErrUsage,
		},
		{
			name:    "missing flag value",
			args:    []string{"reconcile", "--request", "--out", "report.json"},
			wantErr: cli.ErrUsage,
		},
		{
			name:    "overwrite value rejected",
			args:    []string{"reconcile", "--request", "request.json", "--out", "report.json", "--overwrite=false"},
			wantErr: cli.ErrUsage,
		},
		{
			name:    "overwrite true value rejected",
			args:    []string{"reconcile", "--request", "request.json", "--out", "report.json", "--overwrite=true"},
			wantErr: cli.ErrUsage,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			err := cli.Runner{}.Run(context.Background(), tc.args, strings.NewReader(""), &stdout)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tc.wantErr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout length = %d, want 0", stdout.Len())
			}
		})
	}
}

func TestRunnerReconcilePipelineFailures(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON fails before source reads", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		requestPath := filepath.Join(dir, "request.json")
		outPath := filepath.Join(dir, "report.zip")
		writeTestFile(t, requestPath, []byte(`{`))

		var stdout bytes.Buffer
		err := cli.Runner{}.Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath}, strings.NewReader(""), &stdout)
		assertCLIErrorPrefix(t, err, "request_invalid: ")
		assertNoOutput(t, stdout, outPath)
	})

	t.Run("request validation error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		requestPath := filepath.Join(dir, "request.json")
		outPath := filepath.Join(dir, "report.zip")
		writeTestFile(t, requestPath, []byte(`{"run":{"id":"run_1","tenant_id":"tenant_1"},"files":[],"reconcile":{}}`))

		var stdout bytes.Buffer
		err := cli.Runner{}.Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath}, strings.NewReader(""), &stdout)
		assertCLIErrorPrefix(t, err, "request_invalid: ")
		assertNoOutput(t, stdout, outPath)
	})

	t.Run("missing source file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		requestPath := filepath.Join(dir, "request.json")
		outPath := filepath.Join(dir, "report.zip")
		writeReconcileRequest(t, requestPath, validReconcileRequest())

		var stdout bytes.Buffer
		err := cli.Runner{}.Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath, "--workdir", dir}, strings.NewReader(""), &stdout)
		assertCLIErrorPrefix(t, err, "input_not_found: ")
		assertNoOutput(t, stdout, outPath)
	})

	t.Run("existing output without overwrite", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		requestPath := filepath.Join(dir, "missing-request.json")
		outPath := filepath.Join(dir, "report.zip")
		writeTestFile(t, outPath, []byte("existing"))

		var stdout bytes.Buffer
		err := cli.Runner{}.Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath}, strings.NewReader(""), &stdout)
		assertCLIErrorPrefix(t, err, "output_exists: ")
		if stdout.Len() != 0 {
			t.Fatalf("stdout length = %d, want 0", stdout.Len())
		}
	})

	t.Run("workdir defaults to request directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		requestPath := filepath.Join(dir, "request.json")
		outPath := filepath.Join(dir, "report.zip")
		writeReconcileFixture(t, dir, requestPath)

		var stdout bytes.Buffer
		if err := (cli.Runner{}).Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath}, strings.NewReader(""), &stdout); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		assertNoStdoutAndReportExists(t, stdout, outPath)
	})

	t.Run("overwrite replaces existing output", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		requestPath := filepath.Join(dir, "request.json")
		outPath := filepath.Join(dir, "report.zip")
		writeReconcileFixture(t, dir, requestPath)
		writeTestFile(t, outPath, []byte("existing"))

		var stdout bytes.Buffer
		if err := (cli.Runner{}).Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath, "--workdir", dir, "--overwrite"}, strings.NewReader(""), &stdout); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		assertNoStdoutAndReportExists(t, stdout, outPath)
	})
}

func TestRunnerReconcileWritesReportPackage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	outPath := filepath.Join(dir, "report.zip")
	writeReconcileFixture(t, dir, requestPath)

	var stdout bytes.Buffer
	if err := (cli.Runner{}).Run(context.Background(), []string{"reconcile", "--request", requestPath, "--out", outPath, "--workdir", dir}, strings.NewReader(""), &stdout); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertNoStdoutAndReportExists(t, stdout, outPath)

	entries := readZipEntries(t, outPath)
	wantNames := []string{"report.md", "report.json", "mismatches.csv", "captured_reads.json", "tokens.json", "audit.jsonl", "bundle.json", "manifest.json"}
	slices.Sort(wantNames)
	if got := zipEntryNames(entries); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("zip entries = %#v, want %#v", got, wantNames)
	}
	if len(entries["manifest.json"]) == 0 {
		t.Fatal("manifest.json is empty")
	}
}

func TestRunnerReconcileExampleFixture(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "examples", "reconciliation")
	workdir := t.TempDir()
	copyFixtureFiles(t, fixtureDir, workdir, "reconcile.json", "bank.csv", "ledger.xlsx", "invoices.csv")

	outPath := filepath.Join(workdir, "report.zip")
	var stdout bytes.Buffer
	if err := (cli.Runner{}).Run(context.Background(), []string{"reconcile", "--request", filepath.Join(workdir, "reconcile.json"), "--out", outPath, "--workdir", workdir}, strings.NewReader(""), &stdout); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertNoStdoutAndReportExists(t, stdout, outPath)

	entries := readZipEntries(t, outPath)
	wantNames := []string{"report.md", "report.json", "mismatches.csv", "captured_reads.json", "tokens.json", "audit.jsonl", "bundle.json", "manifest.json"}
	slices.Sort(wantNames)
	if got := zipEntryNames(entries); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("zip entries = %#v, want %#v", got, wantNames)
	}

	var reportPayload struct {
		ResultCount   int            `json:"result_count"`
		OutcomeCounts map[string]int `json:"outcome_counts"`
	}
	if err := json.Unmarshal(entries["report.json"], &reportPayload); err != nil {
		t.Fatalf("Unmarshal(report.json) error = %v", err)
	}
	if reportPayload.ResultCount != 4 {
		t.Fatalf("result_count = %d, want 4", reportPayload.ResultCount)
	}
	for outcome, want := range map[string]int{"matched": 1, "mismatch": 1, "missing_left": 1, "missing_right": 1} {
		if got := reportPayload.OutcomeCounts[outcome]; got != want {
			t.Fatalf("outcome_counts[%s] = %d, want %d", outcome, got, want)
		}
	}

	assertManifestHashes(t, entries)
	if err := bundle.Verify(entries["bundle.json"]); err != nil {
		t.Fatalf("bundle.Verify(bundle.json) error = %v", err)
	}
	if _, err := bundle.Replay(entries["bundle.json"]); err != nil {
		t.Fatalf("bundle.Replay(bundle.json) error = %v", err)
	}
}

func TestRunnerReconcileFailureFixtures(t *testing.T) {
	t.Parallel()

	fixtureRoot := filepath.Join("..", "..", "examples", "reconciliation", "failures")
	tests := []struct {
		name       string
		wantPrefix string
	}{
		{name: "missing_required_source", wantPrefix: "selector_invalid: "},
		{name: "malformed_amount", wantPrefix: "evidence_failed: "},
		{name: "unsupported_format", wantPrefix: "request_invalid: "},
		{name: "invalid_selector", wantPrefix: "request_invalid: "},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixtureDir := filepath.Join(fixtureRoot, tc.name)
			outPath := filepath.Join(t.TempDir(), "report.zip")
			var stdout bytes.Buffer
			err := (cli.Runner{}).Run(context.Background(), []string{"reconcile", "--request", filepath.Join(fixtureDir, "reconcile.json"), "--out", outPath, "--workdir", fixtureDir}, strings.NewReader(""), &stdout)
			assertCLIErrorPrefix(t, err, tc.wantPrefix)
			assertNoOutput(t, stdout, outPath)
		})
	}
}

func TestRunnerReconcileOutcomeFixtures(t *testing.T) {
	t.Parallel()

	fixtureRoot := filepath.Join("..", "..", "examples", "reconciliation", "failures")
	tests := []struct {
		name       string
		wantCounts map[string]int
	}{
		{name: "duplicate_key", wantCounts: map[string]int{"duplicate_key": 2}},
		{name: "invalid_record", wantCounts: map[string]int{"invalid_record": 1, "missing_left": 1}},
		{name: "currency_mismatch", wantCounts: map[string]int{"mismatch": 1}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixtureDir := filepath.Join(fixtureRoot, tc.name)
			outPath := filepath.Join(t.TempDir(), "report.zip")
			var stdout bytes.Buffer
			if err := (cli.Runner{}).Run(context.Background(), []string{"reconcile", "--request", filepath.Join(fixtureDir, "reconcile.json"), "--out", outPath, "--workdir", fixtureDir}, strings.NewReader(""), &stdout); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			assertNoStdoutAndReportExists(t, stdout, outPath)

			entries := readZipEntries(t, outPath)
			var reportPayload struct {
				OutcomeCounts map[string]int `json:"outcome_counts"`
			}
			if err := json.Unmarshal(entries["report.json"], &reportPayload); err != nil {
				t.Fatalf("Unmarshal(report.json) error = %v", err)
			}
			if !reflect.DeepEqual(reportPayload.OutcomeCounts, tc.wantCounts) {
				t.Fatalf("outcome_counts = %#v, want %#v", reportPayload.OutcomeCounts, tc.wantCounts)
			}
		})
	}
}

func TestMainExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want int
	}{
		{
			name: "reconcile usage error exits 2",
			args: []string{"reconcile", "--out", "report.json"},
			want: 2,
		},
		{
			name: "read mint bundle flag behavior unchanged",
			args: []string{"read-mint-bundle", "--unknown"},
			want: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if got := cli.Main(tc.args, strings.NewReader(""), &stdout, &stderr); got != tc.want {
				t.Fatalf("Main() = %d, want %d", got, tc.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout length = %d, want 0", stdout.Len())
			}
			if stderr.Len() == 0 {
				t.Fatal("stderr length = 0, want diagnostic")
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

func writeReconcileFixture(t *testing.T, dir, requestPath string) {
	t.Helper()

	writeTestFile(t, filepath.Join(dir, "left.csv"), []byte("ID,Amount,Currency\nA1,10.25,USD\n"))
	writeTestFile(t, filepath.Join(dir, "right.csv"), []byte("ID,Amount,Currency\nA1,10.25,USD\n"))
	writeReconcileRequest(t, requestPath, validReconcileRequest())
}

func validReconcileRequest() reconrequest.Request {
	return reconrequest.Request{
		Run: reconrequest.RunConfig{ID: "run_1", TenantID: "tenant_1"},
		Files: []reconrequest.FileConfig{
			validReconcileCSVFile("left", "left.csv"),
			validReconcileCSVFile("right", "right.csv"),
		},
		Reconcile: reconrequest.ReconcileConfig{
			Mode:    reconrequest.ModeExactMatch,
			Left:    "left",
			Right:   "right",
			MatchOn: []string{"transaction_id"},
			Compare: []string{"amount", "currency"},
		},
	}
}

func validReconcileCSVFile(id, path string) reconrequest.FileConfig {
	return reconrequest.FileConfig{
		ID:      id,
		Purpose: id,
		Format:  reconrequest.FormatCSV,
		Path:    path,
		Reader:  reconrequest.ReaderExtractCSVRows,
		Selector: reconrequest.SelectorConfig{
			RowWindow: "2:",
			Encoding:  "utf-8",
			Dialect:   "excel",
		},
		Columns: reconrequest.Columns{
			"transaction_id": {Source: "ID", Role: reconrequest.RoleKey, Required: true},
			"amount":         {Source: "Amount", Role: reconrequest.RoleAmount, Required: true, Numeric: &reconrequest.NumericConfig{Scale: 2, CurrencyField: "currency"}},
			"currency":       {Source: "Currency", Role: reconrequest.RoleCurrency, Required: true},
		},
	}
}

func writeReconcileRequest(t *testing.T, path string, req reconrequest.Request) {
	t.Helper()

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal(reconcile request) error = %v", err)
	}
	writeTestFile(t, path, data)
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func copyFixtureFiles(t *testing.T, srcDir, dstDir string, names ...string) {
	t.Helper()

	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		writeTestFile(t, filepath.Join(dstDir, name), data)
	}
}

func assertManifestHashes(t *testing.T, entries map[string][]byte) {
	t.Helper()

	var manifest struct {
		BundleHash string `json:"bundle_hash"`
		Artifacts  []struct {
			Name   string `json:"name"`
			Size   int    `json:"size"`
			SHA256 string `json:"sha256"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("Unmarshal(manifest.json) error = %v", err)
	}
	if got, want := manifest.BundleHash, sha256Hex(entries["bundle.json"]); got != want {
		t.Fatalf("manifest bundle_hash = %s, want %s", got, want)
	}
	if len(manifest.Artifacts) == 0 {
		t.Fatal("manifest artifacts empty")
	}
	for _, artifact := range manifest.Artifacts {
		data, ok := entries[artifact.Name]
		if !ok {
			t.Fatalf("manifest artifact %q missing from zip", artifact.Name)
		}
		if got := len(data); got != artifact.Size {
			t.Fatalf("manifest artifact %q size = %d, want %d", artifact.Name, got, artifact.Size)
		}
		if got := sha256Hex(data); got != artifact.SHA256 {
			t.Fatalf("manifest artifact %q sha256 = %s, want %s", artifact.Name, got, artifact.SHA256)
		}
	}
}

func assertCLIErrorPrefix(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("Run() error = %v, want prefix %q", err, want)
	}
}

func assertNoOutput(t *testing.T, stdout bytes.Buffer, outPath string) {
	t.Helper()

	if stdout.Len() != 0 {
		t.Fatalf("stdout length = %d, want 0", stdout.Len())
	}
	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%s) error = %v, want not exist", outPath, err)
	}
}

func assertNoStdoutAndReportExists(t *testing.T, stdout bytes.Buffer, outPath string) {
	t.Helper()

	if stdout.Len() != 0 {
		t.Fatalf("stdout length = %d, want 0", stdout.Len())
	}
	if info, err := os.Stat(outPath); err != nil {
		t.Fatalf("Stat(%s) error = %v", outPath, err)
	} else if info.Size() == 0 {
		t.Fatalf("Stat(%s).Size = 0, want non-empty report", outPath)
	}
}

func readZipEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()

	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader(%s) error = %v", path, err)
	}
	defer reader.Close()

	entries := map[string][]byte{}
	for _, file := range reader.File {
		body, err := file.Open()
		if err != nil {
			t.Fatalf("open %s error = %v", file.Name, err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatalf("read %s error = %v", file.Name, err)
		}
		entries[file.Name] = data
	}
	return entries
}

func zipEntryNames(entries map[string][]byte) []string {
	got := make([]string, 0, len(entries))
	for name := range entries {
		got = append(got, name)
	}
	slices.Sort(got)
	return got
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
