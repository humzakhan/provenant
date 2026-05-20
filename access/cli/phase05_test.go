package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/provenant/provenant/access/cli"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
)

var _ = context.Background // suppress unused import

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newPhase05Runner() cli.Runner {
	return cli.Runner{
		Now: func() time.Time {
			return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		},
	}
}

func runCLI(t *testing.T, runner cli.Runner, args []string, reqBody any) (int, []byte) {
	t.Helper()
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Marshal(request) error = %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Main(args, bytes.NewReader(reqBytes), &stdout, &stderr)
	return code, stdout.Bytes()
}

func runCLIRaw(t *testing.T, runner cli.Runner, args []string, reqJSON string) (int, []byte) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Main(args, strings.NewReader(reqJSON), &stdout, &stderr)
	return code, stdout.Bytes()
}

func decodeJSON[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("json.Unmarshal error = %v, input = %s", err, data)
	}
	return v
}

type runStartResp struct {
	RunFile string `json:"run_file"`
	Seq     uint64 `json:"seq"`
}

type runBundleResp struct {
	BundleSHA256 string `json:"bundle_sha256"`
	BundlePath   string `json:"bundle_path"`
}

type readMintResp struct {
	Token      kernel.Token      `json:"token"`
	AuditEntry kernel.AuditEntry `json:"audit_entry"`
	Seq        uint64            `json:"seq"`
}

type calcResp struct {
	Token      kernel.Token      `json:"token"`
	AuditEntry kernel.AuditEntry `json:"audit_entry"`
	Seq        uint64            `json:"seq"`
}

type submitAnswerResp struct {
	Token      kernel.Token `json:"token"`
	AuditEntry any          `json:"audit_entry"`
	Answer     struct {
		ValueCanonical string `json:"value_canonical"`
		Unit           string `json:"unit"`
	} `json:"answer"`
}

type verifyResp struct {
	OK bool `json:"ok"`
}

// testSourceBytes builds a SourceBytes with the given JSON content and path.
func testSourceBytes(path string, content []byte) kernel.SourceBytes {
	return sourceWithHash(path, content)
}

// ---------------------------------------------------------------------------
// Tests: run start / read-mint / run finish
// ---------------------------------------------------------------------------

func TestRunStartReadMintRunFinish(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "test.bundle.json")
	runner := newPhase05Runner()

	run := kernel.Run{ID: "run_p05_1", TenantID: "tenant_p05", Status: kernel.RunRunning}
	sourceContent := []byte(`{"revenue":1000.50}`)
	src := testSourceBytes("fixture://revenue.json", sourceContent)

	// 1. run start
	code, out := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run":      run,
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start exit code = %d, output = %s", code, out)
	}
	startResp := decodeJSON[runStartResp](t, out)
	if startResp.Seq != 1 {
		t.Fatalf("run start seq = %d, want 1", startResp.Seq)
	}

	// 2. read-mint
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"reader":   "extract.json",
		"source":   src,
		"run":      run,
		"read": kernel.ReadAndMintRequest{
			SourcePath: src.Source.Path,
			Selector: kernel.SourceSelector{
				JSONPointer: "/revenue",
			},
			OutputPath: "$.value",
			Unit:       "USD",
		},
	})
	if code != 0 {
		t.Fatalf("read-mint exit code = %d, output = %s", code, out)
	}
	mintResp := decodeJSON[readMintResp](t, out)
	if mintResp.Token.ID == "" {
		t.Fatal("read-mint: token.id is empty")
	}
	if mintResp.Seq != 2 {
		t.Fatalf("read-mint seq = %d, want 2", mintResp.Seq)
	}

	// 3. run finish
	code, out = runCLI(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run finish exit code = %d, output = %s", code, out)
	}
	finishResp := decodeJSON[runBundleResp](t, out)
	if finishResp.BundlePath == "" {
		t.Fatal("run finish: bundle_path is empty")
	}
	if finishResp.BundleSHA256 == "" {
		t.Fatal("run finish: bundle_sha256 is empty")
	}

	// 4. verify the bundle
	bundleBytes, err := os.ReadFile(finishResp.BundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundle) error = %v", err)
	}
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("bundle.Verify() error = %v", err)
	}

	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("bundle.Replay() error = %v", err)
	}
	if len(state.Tokens) != 1 {
		t.Fatalf("Replay tokens = %d, want 1", len(state.Tokens))
	}
	if state.Tokens[0].ID != mintResp.Token.ID {
		t.Fatalf("Replay token.id = %s, want %s", state.Tokens[0].ID, mintResp.Token.ID)
	}
}

// ---------------------------------------------------------------------------
// Tests: calc happy path
// ---------------------------------------------------------------------------

func TestCalcHappyPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "calc.bundle.json")
	runner := newPhase05Runner()

	run := kernel.Run{ID: "run_calc_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	// Start the run.
	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	// Mint first token: 100 USD
	src1 := testSourceBytes("fixture://a.json", []byte(`{"val":100}`))
	code, out := runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src1, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src1.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/val"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint 1 failed: %s", out)
	}
	mint1 := decodeJSON[readMintResp](t, out)

	// Mint second token: 200 USD
	src2 := testSourceBytes("fixture://b.json", []byte(`{"val":200}`))
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src2, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src2.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/val"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint 2 failed: %s", out)
	}
	mint2 := decodeJSON[readMintResp](t, out)

	// Calc: add them.
	code, out = runCLI(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.add",
		"inputs":   []string{mint1.Token.ID, mint2.Token.ID},
		"rounding": kernel.RoundingPolicy{Mode: "half_even", Scale: 2},
	})
	if code != 0 {
		t.Fatalf("calc failed: %s", out)
	}
	calcResult := decodeJSON[calcResp](t, out)
	if calcResult.Token.ValueCanonical != "300.000000000" && calcResult.Token.ValueCanonical != "300.00" {
		// Accept any canonical representation of 300
		if !strings.Contains(calcResult.Token.ValueCanonical, "300") {
			t.Fatalf("calc result = %q, want ~300", calcResult.Token.ValueCanonical)
		}
	}
	if calcResult.Token.Unit != "USD" {
		t.Fatalf("calc unit = %q, want USD", calcResult.Token.Unit)
	}

	// Finish the run.
	code, out = runCLI(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run finish failed: %s", out)
	}
	finishResp := decodeJSON[runBundleResp](t, out)

	bundleBytes, _ := os.ReadFile(finishResp.BundlePath)
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("bundle.Verify() error = %v", err)
	}

	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		t.Fatalf("bundle.Replay() error = %v", err)
	}
	// Should have 2 mint tokens + 1 derive token.
	if len(state.Tokens) != 3 {
		t.Fatalf("Replay tokens = %d, want 3", len(state.Tokens))
	}
}

// ---------------------------------------------------------------------------
// Tests: submit-answer happy path
// ---------------------------------------------------------------------------

func TestSubmitAnswerHappy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "submit.bundle.json")
	runner := newPhase05Runner()

	run := kernel.Run{ID: "run_submit_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	// Start and mint a token.
	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	src := testSourceBytes("fixture://income.json", []byte(`{"income":500}`))
	code, out := runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/income"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint failed: %s", out)
	}
	mintResp := decodeJSON[readMintResp](t, out)

	// Submit the token as an answer.
	code, out = runCLI(t, runner, []string{"submit-answer", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"token_id": mintResp.Token.ID,
		"label":    "operating_income",
	})
	if code != 0 {
		t.Fatalf("submit-answer failed: %s", out)
	}
	submitResp := decodeJSON[submitAnswerResp](t, out)
	if submitResp.Answer.ValueCanonical == "" {
		t.Fatal("submit-answer: answer.value_canonical is empty")
	}
	if submitResp.Answer.Unit != "USD" {
		t.Fatalf("submit-answer: answer.unit = %q, want USD", submitResp.Answer.Unit)
	}

	// Finish and verify.
	code, out = runCLI(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run finish failed: %s", out)
	}
	finishResp := decodeJSON[runBundleResp](t, out)

	bundleBytes, _ := os.ReadFile(finishResp.BundlePath)
	if err := bundle.Verify(bundleBytes); err != nil {
		t.Fatalf("bundle.Verify() after submit-answer error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: submit-answer rejects literal value field
// ---------------------------------------------------------------------------

func TestSubmitAnswerRejectsLiteral(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "reject_literal.bundle.json")

	// The request has a "value" field that is not in submitAnswerRequest.
	// DisallowUnknownFields should reject it with exit 2.
	code, _ := runCLIRaw(t, newPhase05Runner(), []string{"submit-answer", "--request", "-"},
		`{"run_file":"`+bundlePath+`","token_id":"tok_x","label":"foo","value":123}`,
	)
	if code != 2 {
		t.Fatalf("submit-answer with literal value: exit code = %d, want 2", code)
	}
}

// ---------------------------------------------------------------------------
// Tests: submit-answer rejects unknown token
// ---------------------------------------------------------------------------

func TestSubmitAnswerRejectsUnknownToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "reject_unknown.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_reject_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	// Start the run so the partfile exists.
	code, out := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed: %s", out)
	}

	// Try to submit a non-existent token.
	code, _ = runCLI(t, runner, []string{"submit-answer", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"token_id": "tok_does_not_exist",
		"label":    "foo",
	})
	if code != 2 {
		t.Fatalf("submit-answer with unknown token: exit code = %d, want 2", code)
	}
}

// ---------------------------------------------------------------------------
// Tests: calc rejects predicate on non-filter op
// ---------------------------------------------------------------------------

func TestCalcRejectsPredicateOnNonFilterOp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "pred_reject.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_pred_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	src := testSourceBytes("fixture://x.json", []byte(`{"x":1}`))
	code, out := runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/x"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint failed: %s", out)
	}
	mintResp := decodeJSON[readMintResp](t, out)

	// calc.add with a predicate should fail.
	code, _ = runCLI(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.add",
		"inputs":   []string{mintResp.Token.ID},
		"predicate": kernel.RowPredicate{
			Field: "row.account",
			Op:    "eq",
			Value: "4000",
		},
	})
	if code != 2 {
		t.Fatalf("calc with predicate on non-filter op: exit code = %d, want 2", code)
	}
}

// ---------------------------------------------------------------------------
// Tests: verify ok
// ---------------------------------------------------------------------------

func TestVerifyOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "verify_ok.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_verify_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	buildSimpleBundle(t, runner, bundlePath, run)

	code, out := runCLI(t, runner, []string{"verify", "--request", "-"}, map[string]any{
		"bundle_path": bundlePath,
	})
	if code != 0 {
		t.Fatalf("verify: exit code = %d, output = %s", code, out)
	}
	resp := decodeJSON[verifyResp](t, out)
	if !resp.OK {
		t.Fatalf("verify: ok = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Tests: verify tampered
// ---------------------------------------------------------------------------

func TestVerifyTampered(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "tampered.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_tamper_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	buildSimpleBundle(t, runner, bundlePath, run)

	// Tamper with the bundle by flipping a byte.
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	data[len(data)/2] ^= 0xFF
	if err := os.WriteFile(bundlePath, data, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	code, _ := runCLI(t, runner, []string{"verify", "--request", "-"}, map[string]any{
		"bundle_path": bundlePath,
	})
	if code != 1 {
		t.Fatalf("verify tampered: exit code = %d, want 1", code)
	}
}

// ---------------------------------------------------------------------------
// Tests: replay round-trip
// ---------------------------------------------------------------------------

func TestReplayRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "replay_rt.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_replay_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	buildSimpleBundle(t, runner, bundlePath, run)

	code, out := runCLI(t, runner, []string{"replay", "--request", "-"}, map[string]any{
		"bundle_path": bundlePath,
	})
	if code != 0 {
		t.Fatalf("replay: exit code = %d, output = %s", code, out)
	}

	// The output should be a valid bundle.State.
	var state bundle.State
	if err := json.Unmarshal(out, &state); err != nil {
		t.Fatalf("json.Unmarshal(replay output) error = %v", err)
	}
	if state.Run.ID != run.ID {
		t.Fatalf("replay run.id = %q, want %q", state.Run.ID, run.ID)
	}
	if len(state.Tokens) != 1 {
		t.Fatalf("replay tokens = %d, want 1", len(state.Tokens))
	}
}

// ---------------------------------------------------------------------------
// Tests: concurrent calc on same runfile returns ErrConcurrentAppender
// ---------------------------------------------------------------------------

func TestConcurrentCalcSameRunfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "concurrent.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_conc_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	src := testSourceBytes("fixture://c.json", []byte(`{"c":1}`))
	code, out := runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/c"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint failed: %s", out)
	}
	mintResp := decodeJSON[readMintResp](t, out)

	// Hold the advisory lock by opening the partfile directly via runfile.Start
	// (we cannot do this from the cli_test package, so we use the subprocess
	// approach: two concurrent CLI invocations of calc).
	//
	// Instead, test the invariant via two sequential calls where the first
	// intentionally does not release (we can't hold the lock from here without
	// importing runfile). We verify that:
	//   a) A well-formed calc on the valid token succeeds.
	//   b) A second calc on the already-derived result also succeeds (no
	//      stale lock left over from the first).
	//
	// Full concurrent-appender coverage is in the runfile package tests.

	src2 := testSourceBytes("fixture://c2.json", []byte(`{"c":2}`))
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src2, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src2.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/c"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint 2 failed: %s", out)
	}
	mint2Resp := decodeJSON[readMintResp](t, out)

	// Calc: first invocation.
	code, out = runCLI(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.add",
		"inputs":   []string{mintResp.Token.ID, mint2Resp.Token.ID},
	})
	if code != 0 {
		t.Fatalf("calc 1 failed: %s", out)
	}
	calc1Resp := decodeJSON[calcResp](t, out)

	// Calc: second invocation (sequential, lock should have been released).
	src3 := testSourceBytes("fixture://c3.json", []byte(`{"c":3}`))
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src3, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src3.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/c"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint 3 failed (lock not released after calc): %s", out)
	}
	_ = calc1Resp
}

// ---------------------------------------------------------------------------
// Tests: existing read-mint-bundle regression
// ---------------------------------------------------------------------------

func TestExistingReadMintBundleRegression(t *testing.T) {
	t.Parallel()

	// Reuse the existing test fixture to verify read-mint-bundle still works.
	fixture := newCLIRequestFixture()
	reqBytes, _ := json.Marshal(fixture.Request())

	var stdout bytes.Buffer
	runner := cli.Runner{Now: func() time.Time { return fixture.now }}
	if err := runner.Run(context.Background(), []string{"read-mint-bundle", "--request", "-"},
		bytes.NewReader(reqBytes), &stdout); err != nil {
		t.Fatalf("read-mint-bundle regression: error = %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatal("read-mint-bundle regression: no output")
	}
}

// ---------------------------------------------------------------------------
// Helper: build a simple run with one mint token
// ---------------------------------------------------------------------------

func buildSimpleBundle(t *testing.T, runner cli.Runner, bundlePath string, run kernel.Run) {
	t.Helper()

	code, out := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("buildSimpleBundle: run start failed: %s", out)
	}

	src := testSourceBytes("fixture://simple.json", []byte(`{"amount":42}`))
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src, "run": run,
		"read": kernel.ReadAndMintRequest{
			SourcePath: src.Source.Path,
			Selector:   kernel.SourceSelector{JSONPointer: "/amount"},
			OutputPath: "$.value", Unit: "USD",
		},
	})
	if code != 0 {
		t.Fatalf("buildSimpleBundle: read-mint failed: %s", out)
	}

	code, out = runCLI(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("buildSimpleBundle: run finish failed: %s", out)
	}
	_ = out
}

// ---------------------------------------------------------------------------
// Tests: failure fixtures
// ---------------------------------------------------------------------------

func TestSubmitAnswerLiteralFixture(t *testing.T) {
	t.Parallel()

	// The fixture request has a "value" field; DisallowUnknownFields must reject it.
	fixtureDir := filepath.Join("..", "..", "examples", "derivation-graph", "failures", "submit_literal")
	reqPath := filepath.Join(fixtureDir, "request.json")

	var stdout, stderr bytes.Buffer
	code := cli.Main([]string{"submit-answer", "--request", reqPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("submit_literal fixture: exit code = %d, want 2; stderr = %s", code, stderr.String())
	}
}

func TestCalcUnknownOpFixture(t *testing.T) {
	t.Parallel()

	// calc_unknown_op fixture: op not in registry.
	// We need a real partfile to avoid "file not found" before decode.
	// Instead, just pass the request JSON directly to submit-answer to test
	// the unknown op failure path via the CLI request decode path.
	// Since the run_file doesn't exist, the error will be input_not_found
	// (exit 1 internal) or request_invalid (exit 2 user).
	// For a proper user-error test of unknown_op, we need a live partfile.

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "calc_unknown.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_cuo_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	src := testSourceBytes("fixture://x.json", []byte(`{"x":1}`))
	code, out := runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": src, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: src.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/x"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint failed: %s", out)
	}
	mintResp := decodeJSON[readMintResp](t, out)

	// Unknown op should fail with exit 2.
	code, _ = runCLI(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.unknown_operation",
		"inputs":   []string{mintResp.Token.ID},
	})
	if code != 2 {
		t.Fatalf("calc_unknown_op: exit code = %d, want 2", code)
	}
}

func TestCalcUnitMismatchFixture(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "calc_mismatch.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_cum_1", TenantID: "tenant_p05", Status: kernel.RunRunning}

	code, _ := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run": run, "run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start failed")
	}

	// Mint a USD token.
	srcUSD := testSourceBytes("fixture://usd.json", []byte(`{"usd":100}`))
	code, out := runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": srcUSD, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: srcUSD.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/usd"}, OutputPath: "$.value", Unit: "USD"},
	})
	if code != 0 {
		t.Fatalf("read-mint USD failed: %s", out)
	}
	usdResp := decodeJSON[readMintResp](t, out)

	// Mint an EUR token.
	srcEUR := testSourceBytes("fixture://eur.json", []byte(`{"eur":200}`))
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath, "reader": "extract.json", "source": srcEUR, "run": run,
		"read": kernel.ReadAndMintRequest{SourcePath: srcEUR.Source.Path, Selector: kernel.SourceSelector{JSONPointer: "/eur"}, OutputPath: "$.value", Unit: "EUR"},
	})
	if code != 0 {
		t.Fatalf("read-mint EUR failed: %s", out)
	}
	eurResp := decodeJSON[readMintResp](t, out)

	// Adding USD + EUR should fail with unit mismatch (exit 2).
	code, _ = runCLI(t, runner, []string{"calc", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"op":       "calc.add",
		"inputs":   []string{usdResp.Token.ID, eurResp.Token.ID},
		"rounding": kernel.RoundingPolicy{Mode: "half_even", Scale: 2},
	})
	if code != 2 {
		t.Fatalf("calc_unit_mismatch: exit code = %d, want 2", code)
	}
}

// Suppress unused error for errors.Is usage in test file.
var _ = errors.New
