package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humzakhan/provenant/access/cli"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/grounding"
)

func TestCheckAnswerVerbGroundedPass(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath, tokenID := buildCheckAnswerBundle(t, dir, "42")
	answerPath := filepath.Join(dir, "answer.txt")
	if err := os.WriteFile(answerPath, []byte("The amount is 42."), 0o600); err != nil {
		t.Fatalf("WriteFile(answer) error = %v", err)
	}

	code, out := runCLI(t, newPhase05Runner(), []string{"check-answer", "--request", "-"}, map[string]any{
		"bundle_path": bundlePath,
		"answer_path": answerPath,
	})
	if code != 0 {
		t.Fatalf("check-answer exit code = %d, output = %s", code, out)
	}

	verdict := decodeJSON[grounding.Verdict](t, out)
	if !verdict.OK || !verdict.Pass || verdict.Fail {
		t.Fatalf("verdict = %#v, want ok/pass true and fail false", verdict)
	}
	if len(verdict.GroundedClaims) != 1 {
		t.Fatalf("len(GroundedClaims) = %d, want 1", len(verdict.GroundedClaims))
	}
	if verdict.GroundedClaims[0].TokenID != tokenID {
		t.Fatalf("GroundedClaims[0].TokenID = %q, want %q", verdict.GroundedClaims[0].TokenID, tokenID)
	}
}

func TestCheckAnswerVerbUngroundedFailFromStdin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bundlePath, _ := buildCheckAnswerBundle(t, dir, "42")
	reqPath := filepath.Join(dir, "request.json")
	reqBytes, err := json.Marshal(map[string]any{
		"bundle_path": bundlePath,
		"answer_path": "-",
	})
	if err != nil {
		t.Fatalf("Marshal(request) error = %v", err)
	}
	if err := os.WriteFile(reqPath, reqBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(request) error = %v", err)
	}

	answer := "The amount is 43."
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Main([]string{"check-answer", "--request", reqPath}, strings.NewReader(answer), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("check-answer exit code = %d, want 1; stdout = %s; stderr = %s", code, stdout.String(), stderr.String())
	}

	verdict := decodeJSON[grounding.Verdict](t, stdout.Bytes())
	if verdict.OK || verdict.Pass || !verdict.Fail {
		t.Fatalf("verdict = %#v, want ok/pass false and fail true", verdict)
	}
	if len(verdict.UngroundedClaims) != 1 {
		t.Fatalf("len(UngroundedClaims) = %d, want 1", len(verdict.UngroundedClaims))
	}
	claim := verdict.UngroundedClaims[0]
	if claim.Value != "43.000000000" || claim.Raw != "43" {
		t.Fatalf("UngroundedClaims[0] = %#v, want value=43.000000000 raw=43", claim)
	}
	if claim.Reason == "" {
		t.Fatal("UngroundedClaims[0].Reason is empty")
	}
	if answer[claim.Start:claim.End] != claim.Raw {
		t.Fatalf("UngroundedClaims[0] span = %q, want %q", answer[claim.Start:claim.End], claim.Raw)
	}
}

func buildCheckAnswerBundle(t *testing.T, dir, amount string) (string, string) {
	t.Helper()

	bundlePath := filepath.Join(dir, "check-answer.bundle.json")
	runner := newPhase05Runner()
	run := kernel.Run{ID: "run_check_answer", TenantID: "tenant_check_answer", Status: kernel.RunRunning}

	code, out := runCLI(t, runner, []string{"run", "start", "--request", "-"}, map[string]any{
		"run":      run,
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run start exit code = %d, output = %s", code, out)
	}

	src := testSourceBytes("fixture://check-answer.json", []byte(`{"amount":`+amount+`}`))
	code, out = runCLI(t, runner, []string{"read-mint", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
		"reader":   "extract.json",
		"source":   src,
		"run":      run,
		"read": kernel.ReadAndMintRequest{
			SourcePath: src.Source.Path,
			Selector:   kernel.SourceSelector{JSONPointer: "/amount"},
			OutputPath: "$.value",
			Unit:       "USD",
		},
	})
	if code != 0 {
		t.Fatalf("read-mint exit code = %d, output = %s", code, out)
	}
	resp := decodeJSON[readMintResp](t, out)

	code, out = runCLI(t, runner, []string{"run", "finish", "--request", "-"}, map[string]any{
		"run_file": bundlePath,
	})
	if code != 0 {
		t.Fatalf("run finish exit code = %d, output = %s", code, out)
	}

	return bundlePath, resp.Token.ID
}
