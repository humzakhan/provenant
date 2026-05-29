package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type installTargetSpec struct {
	name  string
	apply func(settings, mcp map[string]any)
}

var installTargetRegistry = []installTargetSpec{
	{
		name:  "claude",
		apply: applyClaudeInstallTarget,
	},
}

var installTargetByName = func() map[string]installTargetSpec {
	index := make(map[string]installTargetSpec, len(installTargetRegistry))
	for _, target := range installTargetRegistry {
		index[target.name] = target
	}
	return index
}()

func applyClaudeInstallTarget(settings, mcp map[string]any) {
	mergeClaudeSettings(settings)
	mergeClaudeMCP(mcp)
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

func applyInstallTargets(settings, mcp map[string]any, targetNames []string) error {
	for _, name := range targetNames {
		target, ok := installTargetByName[name]
		if !ok {
			return fmt.Errorf("unknown target %q", name)
		}
		target.apply(settings, mcp)
	}
	return nil
}

func writeInstallPreview(stdout io.Writer, settings, mcp map[string]any) error {
	payload := map[string]any{
		"settings": settings,
		"mcp":      mcp,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
