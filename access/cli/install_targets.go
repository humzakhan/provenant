package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type installTargetSpec struct {
	name    string
	apply   func(cwd string) error
	preview func(cwd string) (string, error)
}

var installTargetRegistry = []installTargetSpec{
	{
		name:    "claude",
		apply:   applyClaudeInstallTarget,
		preview: previewClaudeInstallTarget,
	},
	{
		name:    "cursor",
		apply:   applyCursorInstallTarget,
		preview: previewCursorInstallTarget,
	},
	{
		name:    "codex",
		apply:   applyCodexInstallTarget,
		preview: previewCodexInstallTarget,
	},
	{
		name:    "gemini",
		apply:   applyGeminiInstallTarget,
		preview: previewGeminiInstallTarget,
	},
	{
		name:    "kiro",
		apply:   applyKiroInstallTarget,
		preview: previewKiroInstallTarget,
	},
}

var installTargetByName = func() map[string]installTargetSpec {
	index := make(map[string]installTargetSpec, len(installTargetRegistry))
	for _, target := range installTargetRegistry {
		index[target.name] = target
	}
	return index
}()

func applyClaudeInstallTarget(cwd string) error {
	settingsPath := filepath.Join(cwd, ".claude", "settings.json")
	if err := ensureDir(filepath.Dir(settingsPath)); err != nil {
		return err
	}
	settings, err := readJSONObject(settingsPath)
	if err != nil {
		return err
	}
	mergeClaudeSettings(settings)
	if err := writeJSONObject(settingsPath, settings); err != nil {
		return err
	}

	mcpPath := filepath.Join(cwd, ".mcp.json")
	mcpDoc, err := readJSONObject(mcpPath)
	if err != nil {
		return err
	}
	mergeProvenantMCP(mcpDoc)
	return writeJSONObject(mcpPath, mcpDoc)
}

func previewClaudeInstallTarget(cwd string) (string, error) {
	settingsPath := filepath.Join(cwd, ".claude", "settings.json")
	settings, err := readJSONObject(settingsPath)
	if err != nil {
		return "", err
	}
	mergeClaudeSettings(settings)

	mcpPath := filepath.Join(cwd, ".mcp.json")
	mcpDoc, err := readJSONObject(mcpPath)
	if err != nil {
		return "", err
	}
	mergeProvenantMCP(mcpDoc)

	payload := map[string]any{
		"settings": settings,
		"mcp":      mcpDoc,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(data, '\n')), nil
}

func applyCursorInstallTarget(cwd string) error {
	return applyJSONInstallTarget(filepath.Join(cwd, ".cursor", "mcp.json"))
}

func previewCursorInstallTarget(cwd string) (string, error) {
	return previewJSONInstallTarget(filepath.Join(cwd, ".cursor", "mcp.json"))
}

func applyGeminiInstallTarget(cwd string) error {
	return applyJSONInstallTarget(filepath.Join(cwd, ".gemini", "settings.json"))
}

func previewGeminiInstallTarget(cwd string) (string, error) {
	return previewJSONInstallTarget(filepath.Join(cwd, ".gemini", "settings.json"))
}

func applyKiroInstallTarget(cwd string) error {
	return applyJSONInstallTarget(filepath.Join(cwd, ".kiro", "settings", "mcp.json"))
}

func previewKiroInstallTarget(cwd string) (string, error) {
	return previewJSONInstallTarget(filepath.Join(cwd, ".kiro", "settings", "mcp.json"))
}

func applyCodexInstallTarget(cwd string) error {
	path := codexConfigPath(cwd)
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(mergeCodexConfig(string(existing))), 0o644)
}

func previewCodexInstallTarget(cwd string) (string, error) {
	path := codexConfigPath(cwd)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return mergeCodexConfig(string(existing)), nil
}

func applyJSONInstallTarget(path string) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	doc, err := readJSONObject(path)
	if err != nil {
		return err
	}
	mergeProvenantMCP(doc)
	return writeJSONObject(path, doc)
}

func previewJSONInstallTarget(path string) (string, error) {
	doc, err := readJSONObject(path)
	if err != nil {
		return "", err
	}
	mergeProvenantMCP(doc)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(data, '\n')), nil
}

func codexConfigPath(cwd string) string {
	return filepath.Join(cwd, ".codex", "config.toml")
}

func mergeCodexConfig(existing string) string {
	section := codexMCPSection()
	if strings.TrimSpace(existing) == "" {
		return section + "\n"
	}

	lines := strings.Split(existing, "\n")
	start, end, ok := findTomlSection(lines, "[mcp_servers.provenant]")
	if ok {
		merged := make([]string, 0, len(lines)-((end-start)+2)+3)
		merged = append(merged, lines[:start]...)
		merged = append(merged, strings.Split(section, "\n")...)
		merged = append(merged, lines[end:]...)
		return strings.Join(trimTrailingEmptyLine(merged), "\n") + "\n"
	}

	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return section + "\n"
	}
	return trimmed + "\n\n" + section + "\n"
}

func findTomlSection(lines []string, header string) (start, end int, ok bool) {
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		start = i
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				end = j
				break
			}
		}
		return start, end, true
	}
	return 0, 0, false
}

func trimTrailingEmptyLine(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}

func codexMCPSection() string {
	return strings.Join([]string{
		"[mcp_servers.provenant]",
		`command = "${PROVENANT_BIN:-provenant}"`,
		`args = ["mcp"]`,
	}, "\n")
}

type csvTargets []string

func (t *csvTargets) String() string {
	return strings.Join(*t, ",")
}

func (t *csvTargets) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("target cannot be empty")
	}
	for _, raw := range strings.Split(value, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			return fmt.Errorf("target cannot be empty")
		}
		*t = append(*t, name)
	}
	return nil
}

func resolveInstallTargetNames(includeClaude bool, targets csvTargets) ([]string, error) {
	raw := append([]string(nil), targets...)
	if includeClaude {
		raw = append([]string{"claude"}, raw...)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("install requires --claude or --target")
	}

	seen := make(map[string]struct{}, len(raw))
	resolved := make([]string, 0, len(raw))
	addTarget := func(name string) error {
		if _, ok := installTargetByName[name]; !ok {
			return fmt.Errorf("unknown target %q", name)
		}
		if _, ok := seen[name]; ok {
			return nil
		}
		seen[name] = struct{}{}
		resolved = append(resolved, name)
		return nil
	}

	for _, name := range raw {
		if name == "all" {
			for _, target := range installTargetRegistry {
				if err := addTarget(target.name); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := addTarget(name); err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

func writeInstallPreview(stdout io.Writer, preview string) error {
	if _, err := io.WriteString(stdout, preview); err != nil {
		return err
	}
	if !strings.HasSuffix(preview, "\n") {
		_, err := io.WriteString(stdout, "\n")
		return err
	}
	return nil
}
