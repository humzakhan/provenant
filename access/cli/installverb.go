package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	provenantServerName      = "provenant"
	provenantMCPToolAllow    = "mcp__provenant__*"
	claudeSessionStartScript = "${CLAUDE_PROJECT_DIR}/hack/hooks/provenant-session-start.sh"
	claudeStopHookScript     = "${CLAUDE_PROJECT_DIR}/hack/hooks/answer-grounding-stop-hook.sh"
)

func (r Runner) runInstall(_ context.Context, args []string, _ io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet(commandInstall, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	claude := fs.Bool("claude", false, "install Claude Code integration")
	var targets csvTargets
	fs.Var(&targets, "target", "install target, csv list, or all")
	printConfig := fs.String("print-config", "", "print merged config for a target without writing files")
	if err := fs.Parse(args); err != nil {
		return userError(fmt.Errorf("%w: %v", ErrUsage, err))
	}
	if fs.NArg() != 0 {
		return userError(fmt.Errorf("%w: install takes no positional arguments", ErrUsage))
	}
	var selectedTargets []string
	var err error
	if *printConfig != "" {
		selectedTargets, err = resolveInstallTargetNames(false, csvTargets{*printConfig})
	} else {
		selectedTargets, err = resolveInstallTargetNames(*claude, targets)
	}
	if err != nil {
		return userError(fmt.Errorf("%w: %v", ErrUsage, err))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("install: getwd: %w", err)
	}

	if *printConfig != "" {
		if len(selectedTargets) != 1 {
			return fmt.Errorf("install: print-config requires exactly one target")
		}
		target, ok := installTargetByName[selectedTargets[0]]
		if !ok {
			return fmt.Errorf("install: unknown target %q", selectedTargets[0])
		}
		preview, err := target.preview(cwd)
		if err != nil {
			return fmt.Errorf("install: preview target: %w", err)
		}
		return writeInstallPreview(stdout, preview)
	}
	for _, targetName := range selectedTargets {
		target, ok := installTargetByName[targetName]
		if !ok {
			return fmt.Errorf("install: unknown target %q", targetName)
		}
		if err := target.apply(cwd); err != nil {
			return fmt.Errorf("install: apply %s: %w", targetName, err)
		}
	}

	return nil
}

func mergeClaudeSettings(doc map[string]any) {
	appendUniqueString(doc, "enabledMcpjsonServers", provenantServerName)

	permissions := nestedObject(doc, "permissions")
	appendUniqueString(permissions, "allow", provenantMCPToolAllow)

	hooks := nestedObject(doc, "hooks")
	sessionStartGroups := nestedObjectArray(hooks, "SessionStart")
	sessionStartGroups = upsertHookGroup(sessionStartGroups, "startup|resume|clear|compact", hookCommand{
		Command: claudeSessionStartScript,
		Args:    []string{},
	})
	hooks["SessionStart"] = sessionStartGroups

	stopGroups := nestedObjectArray(hooks, "Stop")
	stopGroups = upsertHookGroup(stopGroups, "", hookCommand{
		Command: claudeStopHookScript,
		Args:    []string{},
	})
	hooks["Stop"] = stopGroups
}

func mergeProvenantMCP(doc map[string]any) {
	mergeProvenantMCPWithType(doc, true)
}

func mergeProvenantMCPWithoutType(doc map[string]any) {
	mergeProvenantMCPWithType(doc, false)
}

func mergeProvenantMCPWithType(doc map[string]any, includeType bool) {
	servers := nestedObject(doc, "mcpServers")
	server := nestedObject(servers, provenantServerName)
	if includeType {
		setDefaultString(server, "type", "stdio")
	}
	setDefaultString(server, "command", "${PROVENANT_BIN:-provenant}")
	if _, ok := server["args"]; !ok {
		server["args"] = []any{"mcp"}
	}
	env := nestedObject(server, "env")
	setDefaultString(env, "PROVENANT_BUNDLE_PATH", "${PROVENANT_BUNDLE_PATH:-}")
}

func mergeOpenCodeMCP(doc map[string]any) {
	mcp := nestedObject(doc, "mcp")
	server := nestedObject(mcp, provenantServerName)
	setDefaultString(server, "type", "local")
	if _, ok := server["command"]; !ok {
		server["command"] = []any{"${PROVENANT_BIN:-provenant}", "mcp"}
	}
	if _, ok := server["enabled"]; !ok {
		server["enabled"] = true
	}
}

func mergeHermesConfig(existing string) string {
	mcpSection := strings.Join([]string{
		"mcp_servers:",
		"  provenant:",
		`    command: "${PROVENANT_BIN:-provenant}"`,
		"    args:",
		"      - mcp",
		"    env:",
		`      PROVENANT_BUNDLE_PATH: "${PROVENANT_BUNDLE_PATH:-}"`,
	}, "\n")
	toolsetSection := strings.Join([]string{
		"platform_toolsets:",
		"  cli:",
		"    - mcp-provenant",
	}, "\n")

	updated := replaceTopLevelYAMLSection(existing, "mcp_servers:", mcpSection)
	updated = replaceTopLevelYAMLSection(updated, "platform_toolsets:", toolsetSection)
	return normalizeTopLevelYAMLSpacing(updated)
}

func replaceTopLevelYAMLSection(existing, header, replacement string) string {
	if strings.TrimSpace(existing) == "" {
		return replacement
	}

	lines := strings.Split(existing, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != strings.TrimSpace(header) || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") && strings.HasSuffix(trimmed, ":") {
				end = j
				break
			}
		}
		merged := append([]string{}, lines[:i]...)
		merged = append(merged, strings.Split(replacement, "\n")...)
		merged = append(merged, lines[end:]...)
		return strings.Join(trimTrailingEmptyLine(merged), "\n")
	}

	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return replacement
	}
	return trimmed + "\n\n" + replacement
}

func normalizeTopLevelYAMLSpacing(existing string) string {
	lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return "\n"
	}

	out := make([]string, 0, len(lines)+len(lines)/2)
	for _, line := range lines {
		if len(out) > 0 && isTopLevelYAMLHeader(line) && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n"
}

func isTopLevelYAMLHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed != "" && trimmed == line && strings.HasSuffix(trimmed, ":")
}

type hookCommand struct {
	Command string
	Args    []string
}

func upsertHookGroup(groups []map[string]any, matcher string, cmd hookCommand) []map[string]any {
	for i := range groups {
		if groupMatcher(groups[i]) != matcher {
			continue
		}
		hooks := nestedObjectArray(groups[i], "hooks")
		if !containsHookCommand(hooks, cmd) {
			hooks = append(hooks, map[string]any{
				"type":    "command",
				"command": cmd.Command,
				"args":    stringSliceToAny(cmd.Args),
			})
			groups[i]["hooks"] = hooks
		}
		if matcher == "" {
			delete(groups[i], "matcher")
		} else {
			groups[i]["matcher"] = matcher
		}
		return groups
	}

	group := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": cmd.Command,
				"args":    stringSliceToAny(cmd.Args),
			},
		},
	}
	if matcher != "" {
		group["matcher"] = matcher
	}
	return append(groups, group)
}

func containsHookCommand(hooks []map[string]any, cmd hookCommand) bool {
	wantArgs := cmd.Args
	for _, hook := range hooks {
		if stringValue(hook["type"]) != "command" || stringValue(hook["command"]) != cmd.Command {
			continue
		}
		gotArgs := anySliceToStrings(hook["args"])
		if len(gotArgs) != len(wantArgs) {
			continue
		}
		match := true
		for i := range gotArgs {
			if gotArgs[i] != wantArgs[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func readJSONObject(path string) (map[string]any, error) {
	doc := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return doc, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return doc, nil
	}
	dec := json.NewDecoder(bytesReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func writeJSONObject(path string, doc map[string]any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func nestedObject(parent map[string]any, key string) map[string]any {
	if parent == nil {
		return map[string]any{}
	}
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	child := map[string]any{}
	parent[key] = child
	return child
}

func setDefaultString(parent map[string]any, key, value string) {
	if parent == nil {
		return
	}
	if _, ok := parent[key]; ok {
		return
	}
	parent[key] = value
}

func nestedObjectArray(parent map[string]any, key string) []map[string]any {
	raw, ok := parent[key]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func appendUniqueString(parent map[string]any, key, value string) {
	values := anySliceToStrings(parent[key])
	for _, existing := range values {
		if existing == value {
			parent[key] = stringSliceToAny(values)
			return
		}
	}
	values = append(values, value)
	parent[key] = stringSliceToAny(values)
}

func anySliceToStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return append([]string(nil), strings...)
		}
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

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func groupMatcher(group map[string]any) string {
	return stringValue(group["matcher"])
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
