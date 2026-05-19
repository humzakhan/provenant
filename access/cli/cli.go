package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/provenant/provenant/access/corepath"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
)

const (
	commandReadMintBundle = "read-mint-bundle"
	readerExtractJSON     = "extract.json"
	maxRequestBytes       = 2 << 20
	maxSourceBytes        = 1 << 20
)

var (
	ErrUnsupportedCommand = errors.New("cli: unsupported command")
	ErrMissingRequestPath = errors.New("cli: request path is required")
	ErrInvalidRequest     = errors.New("cli: invalid request")
	ErrUnsupportedReader  = errors.New("cli: unsupported reader")
	ErrRequestTooLarge    = errors.New("cli: request exceeds size limit")
	ErrSourceTooLarge     = errors.New("cli: source bytes exceed size limit")
)

type Runner struct {
	Now func() time.Time
}

type ReadMintBundleRequest struct {
	Reader string                    `json:"reader"`
	Source kernel.SourceBytes        `json:"source"`
	Run    kernel.Run                `json:"run"`
	Read   kernel.ReadAndMintRequest `json:"read"`
}

func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	runner := Runner{Now: time.Now}
	if err := runner.Run(context.Background(), args, stdin, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "provenant: %v\n", err)
		return 1
	}
	return 0
}

func (r Runner) Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: expected %s", ErrUnsupportedCommand, commandReadMintBundle)
	}

	switch args[0] {
	case commandReadMintBundle:
		return r.runReadMintBundle(ctx, args[1:], stdin, stdout)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedCommand, args[0])
	}
}

func (r Runner) runReadMintBundle(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet(commandReadMintBundle, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	requestPath := fs.String("request", "", "path to JSON request, or - for stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *requestPath == "" {
		return ErrMissingRequestPath
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%w: unexpected trailing arguments", ErrInvalidRequest)
	}

	requestReader, closeFn, err := openRequest(*requestPath, stdin)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	request, err := decodeRequest(requestReader)
	if err != nil {
		return err
	}
	deps, err := buildDependencies(request, r.now())
	if err != nil {
		return err
	}

	result, err := corepath.ExecuteReadMintBundle(ctx, deps, corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceCLI,
		Run:     request.Run,
		Read:    request.Read,
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(result)
}

func decodeRequest(in io.Reader) (ReadMintBundleRequest, error) {
	limited := &io.LimitedReader{R: in, N: maxRequestBytes + 1}
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()

	var request ReadMintBundleRequest
	if err := dec.Decode(&request); err != nil {
		return ReadMintBundleRequest{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if err := rejectTrailingJSON(dec); err != nil {
		return ReadMintBundleRequest{}, err
	}
	if limited.N == 0 {
		return ReadMintBundleRequest{}, ErrRequestTooLarge
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
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return fmt.Errorf("%w: trailing data", ErrInvalidRequest)
}

func validateRequest(request ReadMintBundleRequest) error {
	if request.Reader != readerExtractJSON {
		return fmt.Errorf("%w: %s", ErrUnsupportedReader, request.Reader)
	}
	if request.Run.ID == "" || request.Run.TenantID == "" {
		return fmt.Errorf("%w: run id and tenant_id are required", ErrInvalidRequest)
	}
	if !isAllowedRunStatus(request.Run.Status) {
		return fmt.Errorf("%w: run status must be one of pending, running, completed, failed", ErrInvalidRequest)
	}
	if request.Read.SourcePath == "" || request.Read.OutputPath == "" {
		return fmt.Errorf("%w: read source_path and output_path are required", ErrInvalidRequest)
	}
	if request.Source.Source.Path == "" {
		return fmt.Errorf("%w: source path is required", ErrInvalidRequest)
	}
	if request.Source.Source.Path != request.Read.SourcePath {
		return fmt.Errorf("%w: source path must match read source_path", ErrInvalidRequest)
	}
	if len(request.Source.Bytes) == 0 {
		return fmt.Errorf("%w: source bytes are required", ErrInvalidRequest)
	}
	if len(request.Source.Bytes) > maxSourceBytes {
		return ErrSourceTooLarge
	}
	if strings.TrimSpace(request.Source.Source.SHA256) == "" {
		return fmt.Errorf("%w: source sha256 is required", ErrInvalidRequest)
	}
	if err := request.Read.Selector.ValidatePhaseTwoSelector(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
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

func openRequest(path string, stdin io.Reader) (io.Reader, func() error, error) {
	if path == "-" {
		return stdin, nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

type fixedClock struct {
	t time.Time
}

func (f fixedClock) Now() time.Time {
	return f.t
}
