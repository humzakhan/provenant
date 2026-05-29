package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/humzakhan/provenant/access/corepath"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

const (
	ToolReadMintBundle    = "provenant.read_mint_bundle"
	PromptReadMintBundle  = "provenant.read_mint_bundle"
	ResourceRequestSchema = "provenant://access/mcp/read-mint-bundle/request-schema"

	readerExtractJSON = "extract.json"
	maxRequestBytes   = 2 << 20
	maxSourceBytes    = 1 << 20
)

// ServerInstructions is the MCP initialize soft assist layer. Hard enforcement
// lives in the release hook/CheckAnswer path.
const ServerInstructions = "Every number in a released answer must originate from a Provenant tool/token, not prose computation. Route numeric claims through Provenant tool paths such as provenant.read_mint_bundle. Ungrounded numbers are blocked at release by the hard hook/CheckAnswer path."

var (
	ErrUnsupportedTool     = errors.New("mcpapi: unsupported tool")
	ErrUnsupportedPrompt   = errors.New("mcpapi: unsupported prompt")
	ErrUnsupportedResource = errors.New("mcpapi: unsupported resource")
	ErrInvalidArguments    = errors.New("mcpapi: invalid arguments")
	ErrUnsupportedReader   = errors.New("mcpapi: unsupported reader")
	ErrRequestTooLarge     = errors.New("mcpapi: request exceeds size limit")
	ErrSourceTooLarge      = errors.New("mcpapi: source bytes exceed size limit")
)

type Surface struct {
	Now func() time.Time
}

type InitializeResult struct {
	ServerInstructions string `json:"serverInstructions"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type Prompt struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mime_type"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolResult struct {
	Content           []ContentBlock                `json:"content"`
	StructuredContent corepath.ReadMintBundleResult `json:"structured_content"`
}

type PromptResult struct {
	Description string `json:"description"`
	Text        string `json:"text"`
}

type ResourceResult struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mime_type"`
	Text     string `json:"text"`
}

type ReadMintBundleRequest struct {
	Reader string                    `json:"reader"`
	Source kernel.SourceBytes        `json:"source"`
	Run    kernel.Run                `json:"run"`
	Read   kernel.ReadAndMintRequest `json:"read"`
}

func Tools() []Tool {
	return []Tool{
		{
			Name:        ToolReadMintBundle,
			Description: "Run the verified read, mint, and bundle core path for extract.json input.",
			InputSchema: json.RawMessage(readMintBundleInputSchema),
		},
	}
}

func Prompts() []Prompt {
	return []Prompt{
		{
			Name:        PromptReadMintBundle,
			Description: "Describe the MCP read/mint/bundle tool contract and request-shaping limits.",
		},
	}
}

func Resources() []Resource {
	return []Resource{
		{
			URI:         ResourceRequestSchema,
			Name:        "read-mint-bundle request schema",
			Description: "JSON schema for the thin MCP read/mint/bundle tool.",
			MIMEType:    "application/json",
		},
	}
}

func (Surface) Initialize() InitializeResult {
	return InitializeResult{
		ServerInstructions: ServerInstructions,
	}
}

func (s Surface) CallTool(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error) {
	if name != ToolReadMintBundle {
		return ToolResult{}, fmt.Errorf("%w: %s", ErrUnsupportedTool, name)
	}

	request, err := decodeRequest(arguments)
	if err != nil {
		return ToolResult{}, err
	}
	deps, err := buildDependencies(request, s.now())
	if err != nil {
		return ToolResult{}, err
	}

	result, err := corepath.ExecuteReadMintBundle(ctx, deps, corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceMCP,
		Run:     request.Run,
		Read:    request.Read,
	})
	if err != nil {
		return ToolResult{}, err
	}

	return ToolResult{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: fmt.Sprintf(
					"surface=%s bundle_sha256=%s token_id=%s",
					result.Surface,
					result.BundleSHA256,
					result.ReadAndMint.Token.ID,
				),
			},
		},
		StructuredContent: result,
	}, nil
}

func GetPrompt(name string) (PromptResult, error) {
	if name != PromptReadMintBundle {
		return PromptResult{}, fmt.Errorf("%w: %s", ErrUnsupportedPrompt, name)
	}
	return PromptResult{
		Description: "Thin MCP prompt for the verified read/mint/bundle path.",
		Text:        "Use the tool provenant.read_mint_bundle with a strict JSON object matching the request schema at provenant://access/mcp/read-mint-bundle/request-schema. Supported reader: extract.json. The MCP surface calls access/corepath directly and rejects malformed or unsupported input.",
	}, nil
}

func ReadResource(uri string) (ResourceResult, error) {
	if uri != ResourceRequestSchema {
		return ResourceResult{}, fmt.Errorf("%w: %s", ErrUnsupportedResource, uri)
	}
	return ResourceResult{
		URI:      ResourceRequestSchema,
		MIMEType: "application/json",
		Text:     readMintBundleInputSchema,
	}, nil
}

func decodeRequest(input json.RawMessage) (ReadMintBundleRequest, error) {
	if len(input) == 0 {
		return ReadMintBundleRequest{}, fmt.Errorf("%w: arguments are required", ErrInvalidArguments)
	}
	if len(input) > maxRequestBytes {
		return ReadMintBundleRequest{}, ErrRequestTooLarge
	}

	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()

	var request ReadMintBundleRequest
	if err := dec.Decode(&request); err != nil {
		return ReadMintBundleRequest{}, fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	if err := rejectTrailingJSON(dec); err != nil {
		return ReadMintBundleRequest{}, err
	}
	if err := validateRequest(request); err != nil {
		return ReadMintBundleRequest{}, err
	}
	return request, nil
}

func rejectTrailingJSON(dec *json.Decoder) error {
	var trailing any
	if err := dec.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	return fmt.Errorf("%w: trailing data", ErrInvalidArguments)
}

func validateRequest(request ReadMintBundleRequest) error {
	if request.Reader != readerExtractJSON {
		return fmt.Errorf("%w: %s", ErrUnsupportedReader, request.Reader)
	}
	if request.Run.ID == "" || request.Run.TenantID == "" {
		return fmt.Errorf("%w: run id and tenant_id are required", ErrInvalidArguments)
	}
	if !isAllowedRunStatus(request.Run.Status) {
		return fmt.Errorf("%w: run status must be one of pending, running, completed, failed", ErrInvalidArguments)
	}
	if request.Read.SourcePath == "" || request.Read.OutputPath == "" {
		return fmt.Errorf("%w: read source_path and output_path are required", ErrInvalidArguments)
	}
	if request.Source.Source.Path == "" {
		return fmt.Errorf("%w: source path is required", ErrInvalidArguments)
	}
	if request.Source.Source.Path != request.Read.SourcePath {
		return fmt.Errorf("%w: source path must match read source_path", ErrInvalidArguments)
	}
	if len(request.Source.Bytes) == 0 {
		return fmt.Errorf("%w: source bytes are required", ErrInvalidArguments)
	}
	if len(request.Source.Bytes) > maxSourceBytes {
		return ErrSourceTooLarge
	}
	if strings.TrimSpace(request.Source.Source.SHA256) == "" {
		return fmt.Errorf("%w: source sha256 is required", ErrInvalidArguments)
	}
	if err := request.Read.Selector.ValidatePhaseTwoSelector(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	return nil
}

func isAllowedRunStatus(status kernel.RunStatus) bool {
	switch status {
	case kernel.RunPending, kernel.RunRunning, kernel.RunCompleted, kernel.RunFailed:
		return true
	default:
		return false
	}
}

func buildDependencies(request ReadMintBundleRequest, now time.Time) (corepath.Dependencies, error) {
	reader, err := sourceReader(request.Reader)
	if err != nil {
		return corepath.Dependencies{}, err
	}

	binding := memio.NewBinding(request.Source)
	return corepath.Dependencies{
		Scope:     binding,
		Resolver:  binding,
		Reader:    reader,
		Store:     binding,
		AuditSink: binding,
		Clock:     fixedClock{t: now.UTC()},
	}, nil
}

func sourceReader(name string) (kernel.SourceReader, error) {
	switch name {
	case readerExtractJSON:
		return extractjson.Reader{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedReader, name)
	}
}

func (s Surface) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type fixedClock struct {
	t time.Time
}

func (f fixedClock) Now() time.Time {
	return f.t
}

const readMintBundleInputSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["reader", "source", "run", "read"],
  "properties": {
    "reader": {
      "type": "string",
      "enum": ["extract.json"]
    },
    "source": {
      "type": "object"
    },
    "run": {
      "type": "object"
    },
    "read": {
      "type": "object"
    }
  }
}`
