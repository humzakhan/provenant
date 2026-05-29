package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/humzakhan/provenant/access/mcpapi"
)

const (
	mcpProtocolVersion = "2025-06-18"
	mcpServerName      = "provenant"
	mcpServerVersion   = "dev"
)

type mcpEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r Runner) runMCP(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 0 {
		return userError(fmt.Errorf("%w: mcp takes no subcommand", ErrUsage))
	}

	server := mcpStdioServer{
		surface:    mcpapi.Surface{Now: r.now, Dependencies: mcpapi.NewStatefulDependencyBuilder()},
		stdin:      stdin,
		stdout:     stdout,
		bundlePath: os.Getenv("PROVENANT_BUNDLE_PATH"),
	}
	return server.serve(ctx)
}

type mcpStdioServer struct {
	surface    mcpapi.Surface
	stdin      io.Reader
	stdout     io.Writer
	bundlePath string
}

func (s mcpStdioServer) serve(ctx context.Context) error {
	reader := bufio.NewReader(s.stdin)
	enc := json.NewEncoder(s.stdout)
	enc.SetEscapeHTML(false)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if strings.TrimSpace(line) == "" {
					return nil
				}
			} else {
				return err
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			if err == io.EOF {
				return nil
			}
			continue
		}

		response, hasResponse := s.handleMessage(ctx, []byte(line))
		if hasResponse {
			if err := enc.Encode(response); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

func (s mcpStdioServer) handleMessage(ctx context.Context, raw []byte) (map[string]any, bool) {
	var env mcpEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"error": map[string]any{
				"code":    -32700,
				"message": fmt.Sprintf("parse error: %v", err),
			},
		}, true
	}
	if env.Method == "" || len(env.ID) == 0 {
		s.handleNotification(env)
		return nil, false
	}

	result, code, message := s.dispatch(ctx, env)
	if message != "" {
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      rawID(env.ID),
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		}, true
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      rawID(env.ID),
		"result":  result,
	}, true
}

func (s mcpStdioServer) handleNotification(env mcpEnvelope) {
	_ = env
}

func (s mcpStdioServer) dispatch(ctx context.Context, env mcpEnvelope) (map[string]any, int, string) {
	switch env.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{
					"listChanged": false,
				},
				"prompts": map[string]any{
					"listChanged": false,
				},
				"resources": map[string]any{
					"listChanged": false,
					"subscribe":   false,
				},
			},
			"serverInfo": map[string]any{
				"name":    mcpServerName,
				"version": mcpServerVersion,
			},
			"instructions": mcpapi.ServerInstructions,
		}, 0, ""
	case "ping":
		return map[string]any{}, 0, ""
	case "tools/list":
		tools := make([]any, 0, len(mcpapi.Tools()))
		for _, tool := range mcpapi.Tools() {
			tools = append(tools, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": json.RawMessage(tool.InputSchema),
			})
		}
		return map[string]any{"tools": tools}, 0, ""
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := decodeMCPParams(env.Params, &params); err != nil {
			return nil, -32602, err.Error()
		}
		args := params.Arguments
		if len(args) == 0 {
			args = json.RawMessage([]byte("{}"))
		}
		result, err := s.surface.CallTool(ctx, params.Name, args)
		if err != nil {
			return nil, -32000, err.Error()
		}
		if err := s.persistToolBundle(params.Name, result); err != nil {
			return nil, -32000, err.Error()
		}
		content := make([]any, 0, len(result.Content))
		for _, block := range result.Content {
			content = append(content, map[string]any{
				"type": block.Type,
				"text": block.Text,
			})
		}
		return map[string]any{
			"content":           content,
			"structuredContent": result.StructuredContent,
		}, 0, ""
	case "prompts/list":
		prompts := make([]any, 0, len(mcpapi.Prompts()))
		for _, prompt := range mcpapi.Prompts() {
			prompts = append(prompts, map[string]any{
				"name":        prompt.Name,
				"description": prompt.Description,
			})
		}
		return map[string]any{"prompts": prompts}, 0, ""
	case "prompts/get":
		var params struct {
			Name string `json:"name"`
		}
		if err := decodeMCPParams(env.Params, &params); err != nil {
			return nil, -32602, err.Error()
		}
		prompt, err := mcpapi.GetPrompt(params.Name)
		if err != nil {
			return nil, -32000, err.Error()
		}
		return map[string]any{
			"description": prompt.Description,
			"messages": []any{
				map[string]any{
					"role": "user",
					"content": map[string]any{
						"type": "text",
						"text": prompt.Text,
					},
				},
			},
		}, 0, ""
	case "resources/list":
		resources := make([]any, 0, len(mcpapi.Resources()))
		for _, resource := range mcpapi.Resources() {
			resources = append(resources, map[string]any{
				"uri":         resource.URI,
				"name":        resource.Name,
				"description": resource.Description,
				"mimeType":    resource.MIMEType,
			})
		}
		return map[string]any{"resources": resources}, 0, ""
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := decodeMCPParams(env.Params, &params); err != nil {
			return nil, -32602, err.Error()
		}
		resource, err := mcpapi.ReadResource(params.URI)
		if err != nil {
			return nil, -32000, err.Error()
		}
		return map[string]any{
			"contents": []any{
				map[string]any{
					"uri":      resource.URI,
					"mimeType": resource.MIMEType,
					"text":     resource.Text,
				},
			},
		}, 0, ""
	default:
		return nil, -32601, fmt.Sprintf("method not found: %s", env.Method)
	}
}

func (s mcpStdioServer) persistToolBundle(name string, result mcpapi.ToolResult) error {
	if s.bundlePath == "" || name != mcpapi.ToolReadMintBundle {
		return nil
	}
	if len(result.StructuredContent.BundleBytes) == 0 {
		return fmt.Errorf("mcp: %s returned an empty bundle", name)
	}
	if err := writeFileAtomically(s.bundlePath, result.StructuredContent.BundleBytes, 0o600); err != nil {
		return fmt.Errorf("mcp: write bundle %s: %w", s.bundlePath, err)
	}
	return nil
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func decodeMCPParams(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return fmt.Errorf("missing params")
	}
	raw, err := stripMCPMeta(raw)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func stripMCPMeta(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if _, ok := obj["_meta"]; !ok {
		return raw, nil
	}
	delete(obj, "_meta")
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func rawID(id json.RawMessage) any {
	var value any
	if err := json.Unmarshal(id, &value); err != nil {
		return nil
	}
	return value
}
