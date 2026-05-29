package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/humzakhan/provenant/access/cli"
	"github.com/humzakhan/provenant/access/mcpapi"
	kernel "github.com/humzakhan/provenant/kernel/public"
)

func TestInstallClaudeWritesSettingsAndMCPJSON(t *testing.T) {
	dir := t.TempDir()
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.claude) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{
  "permissions": {"deny": ["Read(./.env)"]},
  "hooks": {"PostToolUse": [{"matcher":"Edit|Write","hooks":[{"type":"command","command":"./existing.sh"}]}]},
  "custom": true
}
`), 0o600); err != nil {
		t.Fatalf("WriteFile(settings.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{
  "mcpServers": {
    "existing": {"type":"stdio","command":"existing","args":[],"env":{}}
  }
}
`), 0o600); err != nil {
		t.Fatalf("WriteFile(.mcp.json) error = %v", err)
	}

	chdir(t, dir)
	if code := cli.Main([]string{"install", "--claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install exit code = %d, want 0", code)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		t.Fatalf("Unmarshal(settings.json) error = %v", err)
	}
	if !containsString(anyStrings(settings["enabledMcpjsonServers"]), "provenant") {
		t.Fatalf("enabledMcpjsonServers = %#v, want provenant", settings["enabledMcpjsonServers"])
	}
	permissions := settings["permissions"].(map[string]any)
	if !containsString(anyStrings(permissions["allow"]), "mcp__provenant__*") {
		t.Fatalf("permissions.allow = %#v, want mcp__provenant__*", permissions["allow"])
	}
	if !containsHook(settings, "SessionStart", "startup|resume|clear|compact", "${CLAUDE_PROJECT_DIR}/hack/hooks/provenant-session-start.sh") {
		t.Fatalf("settings SessionStart hook missing: %s", settingsBytes)
	}
	if !containsHook(settings, "Stop", "", "${CLAUDE_PROJECT_DIR}/hack/hooks/answer-grounding-stop-hook.sh") {
		t.Fatalf("settings Stop hook missing: %s", settingsBytes)
	}
	if custom, _ := settings["custom"].(bool); !custom {
		t.Fatalf("custom = %#v, want true", settings["custom"])
	}

	mcpPath := filepath.Join(dir, ".mcp.json")
	mcpBytes, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(.mcp.json) error = %v", err)
	}
	var mcpDoc map[string]any
	if err := json.Unmarshal(mcpBytes, &mcpDoc); err != nil {
		t.Fatalf("Unmarshal(.mcp.json) error = %v", err)
	}
	servers := mcpDoc["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Fatalf("existing mcp server missing: %s", mcpBytes)
	}
	provenant := servers["provenant"].(map[string]any)
	if got, _ := provenant["command"].(string); got != "${PROVENANT_BIN:-provenant}" {
		t.Fatalf("provenant command = %q, want ${PROVENANT_BIN:-provenant}", got)
	}
	if got, _ := provenant["type"].(string); got != "stdio" {
		t.Fatalf("provenant type = %q, want stdio", got)
	}
	if args := anyStrings(provenant["args"]); len(args) != 1 || args[0] != "mcp" {
		t.Fatalf("provenant args = %#v, want [mcp]", provenant["args"])
	}
	env := provenant["env"].(map[string]any)
	if got, _ := env["PROVENANT_BUNDLE_PATH"].(string); got != "${PROVENANT_BUNDLE_PATH:-}" {
		t.Fatalf("provenant env.PROVENANT_BUNDLE_PATH = %q, want ${PROVENANT_BUNDLE_PATH:-}", got)
	}

	firstSettings := append([]byte(nil), settingsBytes...)
	firstMCP := append([]byte(nil), mcpBytes...)
	if code := cli.Main([]string{"install", "--claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("second install exit code = %d, want 0", code)
	}
	secondSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings.json second) error = %v", err)
	}
	secondMCP, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(.mcp.json second) error = %v", err)
	}
	if !bytes.Equal(firstSettings, secondSettings) {
		t.Fatalf("settings.json changed on second install\nfirst:\n%s\nsecond:\n%s", firstSettings, secondSettings)
	}
	if !bytes.Equal(firstMCP, secondMCP) {
		t.Fatalf(".mcp.json changed on second install\nfirst:\n%s\nsecond:\n%s", firstMCP, secondMCP)
	}
}

func TestInstallClaudePreservesExistingProvenantMCPServerFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{
  "mcpServers": {
    "provenant": {
      "command": "custom-provenant",
      "args": ["mcp", "--stdio"],
      "env": {
        "EXISTING_KEY": "existing-value",
        "PROVENANT_BUNDLE_PATH": "/custom/bundle.json"
      },
      "custom": true
    }
  }
}
`), 0o600); err != nil {
		t.Fatalf("WriteFile(.mcp.json) error = %v", err)
	}

	chdir(t, dir)
	if code := cli.Main([]string{"install", "--claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install exit code = %d, want 0", code)
	}

	mcpBytes, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("ReadFile(.mcp.json) error = %v", err)
	}
	var mcpDoc map[string]any
	if err := json.Unmarshal(mcpBytes, &mcpDoc); err != nil {
		t.Fatalf("Unmarshal(.mcp.json) error = %v", err)
	}
	provenant := mcpDoc["mcpServers"].(map[string]any)["provenant"].(map[string]any)
	if got, _ := provenant["type"].(string); got != "stdio" {
		t.Fatalf("provenant type = %q, want stdio", got)
	}
	if got, _ := provenant["command"].(string); got != "custom-provenant" {
		t.Fatalf("provenant command = %q, want preserved custom value", got)
	}
	if args := anyStrings(provenant["args"]); len(args) != 2 || args[0] != "mcp" || args[1] != "--stdio" {
		t.Fatalf("provenant args = %#v, want preserved custom args", provenant["args"])
	}
	env := provenant["env"].(map[string]any)
	if got, _ := env["EXISTING_KEY"].(string); got != "existing-value" {
		t.Fatalf("provenant env.EXISTING_KEY = %q, want preserved value", got)
	}
	if got, _ := env["PROVENANT_BUNDLE_PATH"].(string); got != "/custom/bundle.json" {
		t.Fatalf("provenant env.PROVENANT_BUNDLE_PATH = %q, want preserved custom path", got)
	}
	if custom, _ := provenant["custom"].(bool); !custom {
		t.Fatalf("provenant custom = %#v, want true", provenant["custom"])
	}
}

func TestInstallLocalJSONTargetsPreserveSiblingServers(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		configPath string
	}{
		{name: "cursor", target: "cursor", configPath: filepath.Join(".cursor", "mcp.json")},
		{name: "gemini", target: "gemini", configPath: filepath.Join(".gemini", "settings.json")},
		{name: "kiro", target: "kiro", configPath: filepath.Join(".kiro", "settings", "mcp.json")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.configPath)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("MkdirAll(config dir) error = %v", err)
			}
			if err := os.WriteFile(path, []byte(`{
  "mcpServers": {
    "existing": {"type":"stdio","command":"existing","args":[],"env":{}}
  },
  "custom": true
}
`), 0o600); err != nil {
				t.Fatalf("WriteFile(config) error = %v", err)
			}

			chdir(t, dir)
			if code := cli.Main([]string{"install", "--target", tc.target}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
				t.Fatalf("install exit code = %d, want 0", code)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(config) error = %v", err)
			}
			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatalf("Unmarshal(config) error = %v", err)
			}
			servers := doc["mcpServers"].(map[string]any)
			if _, ok := servers["existing"]; !ok {
				t.Fatalf("existing mcp server missing: %s", data)
			}
			provenant := servers["provenant"].(map[string]any)
			if got, _ := provenant["command"].(string); got != "${PROVENANT_BIN:-provenant}" {
				t.Fatalf("provenant command = %q, want ${PROVENANT_BIN:-provenant}", got)
			}
			if got, _ := provenant["type"].(string); got != "stdio" {
				t.Fatalf("provenant type = %q, want stdio", got)
			}
			if args := anyStrings(provenant["args"]); len(args) != 1 || args[0] != "mcp" {
				t.Fatalf("provenant args = %#v, want [mcp]", provenant["args"])
			}
			env := provenant["env"].(map[string]any)
			if got, _ := env["PROVENANT_BUNDLE_PATH"].(string); got != "${PROVENANT_BUNDLE_PATH:-}" {
				t.Fatalf("provenant env.PROVENANT_BUNDLE_PATH = %q, want ${PROVENANT_BUNDLE_PATH:-}", got)
			}
			if custom, _ := doc["custom"].(bool); !custom {
				t.Fatalf("custom = %#v, want true", doc["custom"])
			}
		})
	}
}

func TestInstallMixedTargetsWriteMultipleConfigs(t *testing.T) {
	dir := t.TempDir()
	cursorPath := filepath.Join(dir, ".cursor", "mcp.json")
	geminiPath := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(cursor dir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(geminiPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(gemini dir) error = %v", err)
	}
	if err := os.WriteFile(cursorPath, []byte(`{"mcpServers":{"existing":{"type":"stdio","command":"existing","args":[],"env":{}}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(cursor) error = %v", err)
	}
	if err := os.WriteFile(geminiPath, []byte(`{"mcpServers":{"existing":{"type":"stdio","command":"existing","args":[],"env":{}}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(gemini) error = %v", err)
	}

	chdir(t, dir)
	if code := cli.Main([]string{"install", "--target", "cursor,gemini"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install exit code = %d, want 0", code)
	}

	for _, path := range []string{cursorPath, geminiPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", path, err)
		}
		servers := doc["mcpServers"].(map[string]any)
		if _, ok := servers["existing"]; !ok {
			t.Fatalf("existing mcp server missing from %s: %s", path, data)
		}
		if _, ok := servers["provenant"]; !ok {
			t.Fatalf("provenant mcp server missing from %s: %s", path, data)
		}
	}
}

func TestInstallCodexWritesTOMLAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(codex dir) error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`# keep this
[other]
value = "preserved"
`), 0o600); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	chdir(t, dir)
	if code := cli.Main([]string{"install", "--target", "codex"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install exit code = %d, want 0", code)
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(first)
	if !strings.Contains(text, `[mcp_servers.provenant]`) {
		t.Fatalf("codex config missing provenance section: %s", text)
	}
	if !strings.Contains(text, `command = "${PROVENANT_BIN:-provenant}"`) {
		t.Fatalf("codex command missing: %s", text)
	}
	if !strings.Contains(text, `args = ["mcp"]`) {
		t.Fatalf("codex args missing: %s", text)
	}
	if !strings.Contains(text, `[other]`) || !strings.Contains(text, `value = "preserved"`) {
		t.Fatalf("codex unrelated content lost: %s", text)
	}

	if code := cli.Main([]string{"install", "--target", "codex"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("second install exit code = %d, want 0", code)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config.toml second) error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("config.toml changed on second install\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestInstallTargetParsing(t *testing.T) {
	t.Run("no target fails", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		code := cli.Main([]string{"install"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		if code != 2 {
			t.Fatalf("install exit code = %d, want 2", code)
		}
	})

	t.Run("--claude succeeds", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		if code := cli.Main([]string{"install", "--claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("install exit code = %d, want 0", code)
		}
	})

	t.Run("--target claude succeeds", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		if code := cli.Main([]string{"install", "--target", "claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("install exit code = %d, want 0", code)
		}
	})

	t.Run("--target all succeeds", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		if code := cli.Main([]string{"install", "--target", "all"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("install exit code = %d, want 0", code)
		}
	})

	t.Run("duplicate targets do not duplicate output", func(t *testing.T) {
		single := t.TempDir()
		chdir(t, single)
		if code := cli.Main([]string{"install", "--target", "claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("single-target install exit code = %d, want 0", code)
		}
		singleSettings, err := os.ReadFile(filepath.Join(single, ".claude", "settings.json"))
		if err != nil {
			t.Fatalf("ReadFile(single settings) error = %v", err)
		}
		singleMCP, err := os.ReadFile(filepath.Join(single, ".mcp.json"))
		if err != nil {
			t.Fatalf("ReadFile(single .mcp.json) error = %v", err)
		}

		dupe := t.TempDir()
		chdir(t, dupe)
		if code := cli.Main([]string{"install", "--target", "claude,claude"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("duplicate-target install exit code = %d, want 0", code)
		}
		dupeSettings, err := os.ReadFile(filepath.Join(dupe, ".claude", "settings.json"))
		if err != nil {
			t.Fatalf("ReadFile(duplicate settings) error = %v", err)
		}
		dupeMCP, err := os.ReadFile(filepath.Join(dupe, ".mcp.json"))
		if err != nil {
			t.Fatalf("ReadFile(duplicate .mcp.json) error = %v", err)
		}

		if !bytes.Equal(singleSettings, dupeSettings) {
			t.Fatalf("settings.json changed for duplicate targets\nsingle:\n%s\nduplicate:\n%s", singleSettings, dupeSettings)
		}
		if !bytes.Equal(singleMCP, dupeMCP) {
			t.Fatalf(".mcp.json changed for duplicate targets\nsingle:\n%s\nduplicate:\n%s", singleMCP, dupeMCP)
		}
	})

	t.Run("unknown target fails", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		code := cli.Main([]string{"install", "--target", "unknown"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		if code != 2 {
			t.Fatalf("install exit code = %d, want 2", code)
		}
	})

	t.Run("positional args fail", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		code := cli.Main([]string{"install", "--claude", "extra"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		if code != 2 {
			t.Fatalf("install exit code = %d, want 2", code)
		}
	})
}

func TestProvenantSessionStartHookWritesBundlePath(t *testing.T) {
	repoRoot := repoRoot(t)
	cwd := t.TempDir()
	envPath := filepath.Join(t.TempDir(), "claude.env")

	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "provenant-session-start.sh")
	input := map[string]any{
		"cwd":        cwd,
		"session_id": "session-123",
	}
	result := runHookScript(t, scriptPath, input, map[string]string{
		"CLAUDE_ENV_FILE": envPath,
	})
	if result.exitCode != 0 {
		t.Fatalf("session-start exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}

	expectedBundle := filepath.Join(cwd, ".provenant", "claude-code", "session-123", "current.bundle.json")
	if _, err := os.Stat(filepath.Dir(expectedBundle)); err != nil {
		t.Fatalf("Stat(bundle dir) error = %v", err)
	}
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(claude.env) error = %v", err)
	}
	envText := string(envBytes)
	if !strings.Contains(envText, "PROVENANT_BUNDLE_PATH=") || !strings.Contains(envText, expectedBundle) {
		t.Fatalf("CLAUDE_ENV_FILE = %q, want bundle path %q", envText, expectedBundle)
	}
}

func TestMCPReadMintBundleWritesSessionBundleForStopHook(t *testing.T) {
	repoRoot := repoRoot(t)
	provenantBin := buildProvenantBinary(t)
	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "answer-grounding-stop-hook.sh")

	dir := t.TempDir()
	sessionID := "session-from-mcp"
	bundlePath := filepath.Join(dir, ".provenant", "claude-code", sessionID, "current.bundle.json")
	t.Setenv("PROVENANT_BUNDLE_PATH", bundlePath)

	src := testSourceBytes("fixture://stop-hook-mcp.json", []byte(`{"amount":42}`))
	request := cli.ReadMintBundleRequest{
		Reader: "extract.json",
		Source: src,
		Run: kernel.Run{
			ID:       "run_stop_hook_mcp",
			TenantID: "tenant_stop_hook_mcp",
			Status:   kernel.RunRunning,
		},
		Read: kernel.ReadAndMintRequest{
			SourcePath: src.Source.Path,
			Selector: kernel.SourceSelector{
				JSONPointer: "/amount",
			},
			OutputPath: "$.value",
			Unit:       "USD",
		},
	}

	inputBytes, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      mcpapi.ToolReadMintBundle,
			"arguments": request,
			"_meta": map[string]any{
				"progressToken": "claude-code-params",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(mcp request) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Main([]string{"mcp"}, bytes.NewReader(append(inputBytes, '\n')), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mcp exit code = %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	var response struct {
		Result struct {
			StructuredContent struct {
				BundleBytes []byte `json:"bundle_bytes"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal(mcp stdout) error = %v, stdout=%q", err, stdout.String())
	}
	bundleBytes, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundlePath) error = %v", err)
	}
	if !bytes.Equal(bundleBytes, response.Result.StructuredContent.BundleBytes) {
		t.Fatalf("persisted bundle differs from mcp response bytes")
	}

	result := runHookScript(t, scriptPath, map[string]any{
		"cwd":                    dir,
		"session_id":             sessionID,
		"last_assistant_message": "The amount is 42.",
	}, map[string]string{
		"PROVENANT_BIN": provenantBin,
	})
	if result.exitCode != 0 {
		t.Fatalf("stop hook exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	if strings.TrimSpace(result.stdout) != "" {
		t.Fatalf("stop hook stdout = %q, want empty", result.stdout)
	}
}

func TestMCPSubmitAnswerUsesEnvBundlePath(t *testing.T) {
	dir := t.TempDir()
	bundlePath, _ := buildCheckAnswerBundle(t, dir, "42")
	t.Setenv("PROVENANT_BUNDLE_PATH", bundlePath)

	requestBytes, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": mcpapi.ToolSubmitAnswer,
			"arguments": map[string]any{
				"answer": "The amount is 42.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(mcp request) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Main([]string{"mcp"}, bytes.NewReader(append(requestBytes, '\n')), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mcp exit code = %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	var response struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal(mcp stdout) error = %v, stdout=%q", err, stdout.String())
	}
	if response.Error != nil {
		t.Fatalf("mcp returned error = %#v, stdout=%s", response.Error, stdout.String())
	}
	if got, _ := response.Result.StructuredContent["attested"].(bool); !got {
		t.Fatalf("attested = %#v, want true", response.Result.StructuredContent["attested"])
	}
	if got, _ := response.Result.StructuredContent["ok"].(bool); !got {
		t.Fatalf("ok = %#v, want true", response.Result.StructuredContent["ok"])
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Type != "text" || !strings.Contains(response.Result.Content[0].Text, "PROVENANT-ATTESTED") {
		t.Fatalf("content = %#v, want attestation marker", response.Result.Content)
	}
	if _, ok := response.Result.StructuredContent["bundle_sha256"].(string); !ok {
		t.Fatalf("bundle_sha256 = %#v, want string", response.Result.StructuredContent["bundle_sha256"])
	}
}

func TestAnswerGroundingStopHookWithRealCLI(t *testing.T) {
	repoRoot := repoRoot(t)
	provenantBin := buildProvenantBinary(t)
	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "answer-grounding-stop-hook.sh")

	t.Run("grounded allows", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath, _ := buildCheckAnswerBundle(t, dir, "42")
		result := runHookScript(t, scriptPath, map[string]any{
			"cwd":                    dir,
			"session_id":             "grounded",
			"last_assistant_message": "The amount is 42.",
			"provenant_bundle_path":  bundlePath,
		}, map[string]string{
			"PROVENANT_BIN": provenantBin,
		})
		if result.exitCode != 0 {
			t.Fatalf("grounded exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
		}
		if strings.TrimSpace(result.stdout) != "" {
			t.Fatalf("grounded stdout = %q, want empty", result.stdout)
		}
	})

	t.Run("ungrounded blocks", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath, _ := buildCheckAnswerBundle(t, dir, "42")
		result := runHookScript(t, scriptPath, map[string]any{
			"cwd":                    dir,
			"session_id":             "ungrounded",
			"last_assistant_message": "The amount is 43.",
			"provenant_bundle_path":  bundlePath,
		}, map[string]string{
			"PROVENANT_BIN": provenantBin,
		})
		if result.exitCode != 0 {
			t.Fatalf("ungrounded exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
			t.Fatalf("Unmarshal(stop hook stdout) error = %v, stdout = %q", err, result.stdout)
		}
		if got, _ := resp["decision"].(string); got != "block" {
			t.Fatalf("decision = %q, want block", got)
		}
		reason, _ := resp["reason"].(string)
		if !strings.Contains(reason, "43.000000000") || !strings.Contains(reason, "raw=43") || !strings.Contains(reason, "span=") {
			t.Fatalf("reason = %q, want value/raw/span details", reason)
		}
	})
}

func TestAnswerGroundingStopHookMalformedInputBlocks(t *testing.T) {
	repoRoot := repoRoot(t)
	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "answer-grounding-stop-hook.sh")

	result := runHookScriptRaw(t, scriptPath, []byte("{"), nil)
	if result.exitCode != 0 {
		t.Fatalf("malformed input exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%q", err, result.stdout)
	}
	if got, _ := resp["decision"].(string); got != "block" {
		t.Fatalf("decision = %q, want block", got)
	}
	reason, _ := resp["reason"].(string)
	if !strings.Contains(reason, "could not parse input") {
		t.Fatalf("reason = %q, want parse failure", reason)
	}
}

func TestAnswerGroundingStopHookMissingBundleBehavior(t *testing.T) {
	repoRoot := repoRoot(t)
	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "answer-grounding-stop-hook.sh")
	dir := t.TempDir()

	t.Run("digits block", func(t *testing.T) {
		result := runHookScript(t, scriptPath, map[string]any{
			"cwd":                    dir,
			"session_id":             "missing-bundle",
			"last_assistant_message": "The answer is 42.",
		}, nil)
		if result.exitCode != 0 {
			t.Fatalf("digits exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
			t.Fatalf("Unmarshal(stdout) error = %v, stdout=%q", err, result.stdout)
		}
		if got, _ := resp["decision"].(string); got != "block" {
			t.Fatalf("decision = %q, want block", got)
		}
		reason, _ := resp["reason"].(string)
		if !strings.Contains(reason, "no Provenant bundle") {
			t.Fatalf("reason = %q, want missing bundle message", reason)
		}
	})

	t.Run("no digits allows", func(t *testing.T) {
		result := runHookScript(t, scriptPath, map[string]any{
			"cwd":                    dir,
			"session_id":             "missing-bundle-no-digits",
			"last_assistant_message": "The amount is unavailable.",
		}, nil)
		if result.exitCode != 0 {
			t.Fatalf("no-digits exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
		}
		if strings.TrimSpace(result.stdout) != "" {
			t.Fatalf("no-digits stdout = %q, want empty", result.stdout)
		}
	})
}

func TestAnswerGroundingStopHookSkipsReentrantInvocation(t *testing.T) {
	repoRoot := repoRoot(t)
	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "answer-grounding-stop-hook.sh")

	result := runHookScript(t, scriptPath, map[string]any{
		"cwd":                    t.TempDir(),
		"session_id":             "reentrant-stop-hook",
		"last_assistant_message": "The answer is 42.",
		"stop_hook_active":       true,
	}, nil)
	if result.exitCode != 0 {
		t.Fatalf("reentrant exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	if strings.TrimSpace(result.stdout) != "" {
		t.Fatalf("reentrant stdout = %q, want empty", result.stdout)
	}
}

func TestAnswerGroundingStopHookRejectsNonBooleanStopHookActive(t *testing.T) {
	repoRoot := repoRoot(t)
	scriptPath := filepath.Join(repoRoot, "hack", "hooks", "answer-grounding-stop-hook.sh")

	result := runHookScript(t, scriptPath, map[string]any{
		"cwd":                    t.TempDir(),
		"session_id":             "non-boolean-stop-hook",
		"last_assistant_message": "The answer is 42.",
		"stop_hook_active":       "false",
	}, nil)
	if result.exitCode != 0 {
		t.Fatalf("non-boolean stop_hook_active exit code = %d, stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%q", err, result.stdout)
	}
	if got, _ := resp["decision"].(string); got != "block" {
		t.Fatalf("decision = %q, want block", got)
	}
	reason, _ := resp["reason"].(string)
	if !strings.Contains(reason, "stop_hook_active must be a JSON boolean") {
		t.Fatalf("reason = %q, want stop_hook_active type failure", reason)
	}
}

type hookResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func runHookScript(t *testing.T, scriptPath string, input map[string]any, extraEnv map[string]string) hookResult {
	t.Helper()

	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal(input) error = %v", err)
	}
	return runHookScriptRaw(t, scriptPath, inputBytes, extraEnv)
}

func runHookScriptRaw(t *testing.T, scriptPath string, input []byte, extraEnv map[string]string) hookResult {
	t.Helper()

	cmd := exec.Command(scriptPath)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "executable file not found") {
			t.Fatalf("Run(%s) error = %v", scriptPath, err)
		}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("Run(%s) error = %v", scriptPath, err)
		}
	}
	return hookResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func containsHook(settings map[string]any, event, matcher, command string) bool {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}
	groups, ok := hooks[event].([]any)
	if !ok {
		return false
	}
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			continue
		}
		groupMatcher, _ := group["matcher"].(string)
		if groupMatcher != matcher {
			if !(matcher == "" && groupMatcher == "") {
				continue
			}
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, rawHook := range hookList {
			hook, ok := rawHook.(map[string]any)
			if !ok {
				continue
			}
			if got, _ := hook["command"].(string); got == command {
				return true
			}
		}
	}
	return false
}

func anyStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd %s error = %v", previous, err)
		}
	})
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

var (
	buildProvenantOnce sync.Once
	buildProvenantPath string
	buildProvenantErr  error
)

func buildProvenantBinary(t *testing.T) string {
	t.Helper()
	buildProvenantOnce.Do(func() {
		root := repoRoot(t)
		outDir, err := os.MkdirTemp("", "provenant-cli-tests-*")
		buildProvenantErr = err
		if buildProvenantErr != nil {
			return
		}
		buildProvenantPath = filepath.Join(outDir, "provenant-test-bin")
		cmd := exec.Command("go", "build", "-o", buildProvenantPath, "./cmd/provenant")
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		buildProvenantErr = cmd.Run()
		if buildProvenantErr != nil {
			buildProvenantErr = fmt.Errorf("%w: %s", buildProvenantErr, strings.TrimSpace(stderr.String()))
		}
	})
	if buildProvenantErr != nil {
		t.Fatalf("build provenant binary error = %v", buildProvenantErr)
	}
	return buildProvenantPath
}
