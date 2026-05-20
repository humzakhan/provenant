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
	"path/filepath"
	"strings"
	"time"

	"github.com/provenant/provenant/access/corepath"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/memio"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/provenance"
	"github.com/provenant/provenant/reconcile/report"
	reconrequest "github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"
)

const (
	commandReadMintBundle = "read-mint-bundle"
	commandReconcile      = "reconcile"
	commandRun            = "run"
	commandReadMint       = "read-mint"
	commandCalc           = "calc"
	commandSubmitAnswer   = "submit-answer"
	commandVerify         = "verify"
	commandReplay         = "replay"
	commandPack           = "pack"
	commandTrace          = "trace"
	readerExtractJSON     = "extract.json"
	maxRequestBytes       = 2 << 20
	maxSourceBytes        = 1 << 20
)

var (
	ErrUnsupportedCommand = errors.New("cli: unsupported command")
	ErrUsage              = errors.New("cli: usage error")
	ErrMissingRequestPath = errors.New("cli: request path is required")
	ErrMissingOutputPath  = errors.New("cli: output path is required")
	ErrInvalidRequest     = errors.New("cli: invalid request")
	ErrUnsupportedReader  = errors.New("cli: unsupported reader")
	ErrRequestTooLarge    = errors.New("cli: request exceeds size limit")
	ErrSourceTooLarge     = errors.New("cli: source bytes exceed size limit")
	ErrTokenNotFound      = errors.New("cli: token not found in run")
)

type packetErrorCategory string

const (
	packetErrorRequestInvalid       packetErrorCategory = "request_invalid"
	packetErrorInputNotFound        packetErrorCategory = "input_not_found"
	packetErrorInputUnsupported     packetErrorCategory = "input_unsupported"
	packetErrorSelectorInvalid      packetErrorCategory = "selector_invalid"
	packetErrorReconciliationFailed packetErrorCategory = "reconciliation_failed"
	packetErrorEvidenceFailed       packetErrorCategory = "evidence_failed"
	packetErrorReportFailed         packetErrorCategory = "report_failed"
	packetErrorOutputExists         packetErrorCategory = "output_exists"
	packetErrorOutputWriteFailed    packetErrorCategory = "output_write_failed"
	packetErrorTokenNotFound        packetErrorCategory = "token_not_found"
	packetErrorConcurrentAppender   packetErrorCategory = "concurrent_appender"
)

type packetError struct {
	category packetErrorCategory
	err      error
}

func (e packetError) Error() string {
	if e.err == nil {
		return string(e.category)
	}
	return string(e.category) + ": " + e.err.Error()
}

func (e packetError) Unwrap() error {
	return e.err
}

func newPacketError(category packetErrorCategory, err error) error {
	return userError(packetError{category: category, err: err})
}

type outputWriteError struct {
	err error
}

func (e outputWriteError) Error() string { return e.err.Error() }

func (e outputWriteError) Unwrap() error { return e.err }

type outputExistsError struct {
	err error
}

func (e outputExistsError) Error() string { return e.err.Error() }

func (e outputExistsError) Unwrap() error { return e.err }

type ErrorCategory string

const (
	ErrorCategoryUser ErrorCategory = "user"
)

type Error struct {
	Category ErrorCategory
	Err      error
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}

func userError(err error) error {
	return &Error{Category: ErrorCategoryUser, Err: err}
}

func isCategory(err error, category ErrorCategory) bool {
	var cliErr *Error
	return errors.As(err, &cliErr) && cliErr.Category == category
}

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
		if isCategory(err, ErrorCategoryUser) {
			return 2
		}
		return 1
	}
	return 0
}

func (r Runner) Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: expected a command", ErrUnsupportedCommand)
	}

	switch args[0] {
	case commandReadMintBundle:
		return r.runReadMintBundle(ctx, args[1:], stdin, stdout)
	case commandReconcile:
		return r.runReconcile(ctx, args[1:], stdin, stdout)
	case commandRun:
		return r.runRunSubcommand(ctx, args[1:], stdin, stdout)
	case commandReadMint:
		return r.runReadMint(ctx, args[1:], stdin, stdout)
	case commandCalc:
		return r.runCalc(ctx, args[1:], stdin, stdout)
	case commandSubmitAnswer:
		return r.runSubmitAnswer(ctx, args[1:], stdin, stdout)
	case commandVerify:
		return r.runVerify(ctx, args[1:], stdin, stdout)
	case commandReplay:
		return r.runReplay(ctx, args[1:], stdin, stdout)
	case commandPack:
		return r.runPack(ctx, args[1:], stdin, stdout)
	case commandTrace:
		return r.runTrace(ctx, args[1:], stdin, stdout)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedCommand, args[0])
	}
}

func (r Runner) runRunSubcommand(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: run requires a subcommand (start, finish, fail)", ErrUnsupportedCommand)
	}
	switch args[0] {
	case "start":
		return r.runStart(ctx, args[1:], stdin, stdout)
	case "finish":
		return r.runFinish(ctx, args[1:], stdin, stdout)
	case "fail":
		return r.runFail(ctx, args[1:], stdin, stdout)
	default:
		return fmt.Errorf("%w: run %s (expected start, finish, or fail)", ErrUnsupportedCommand, args[0])
	}
}

type reconcileFlags struct {
	request   string
	out       string
	workdir   string
	overwrite bool
}

func (r Runner) runReconcile(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	_, _ = stdin, stdout

	flags, err := parseReconcileFlags(args)
	if err != nil {
		return userError(err)
	}
	if err := ensureOutputWritable(flags.out, flags.overwrite); err != nil {
		return err
	}

	file, err := os.Open(flags.request)
	if err != nil {
		return newPacketError(packetErrorRequestInvalid, err)
	}
	defer file.Close()

	req, err := reconrequest.DecodeRequest(file)
	if err != nil {
		return newPacketError(packetErrorRequestInvalid, err)
	}
	baseDir := flags.workdir
	if baseDir == "" {
		baseDir = filepath.Dir(flags.request)
	}

	files, err := router.Route(ctx, *req, router.Options{BaseDir: baseDir})
	if err != nil {
		return normalizeRouteError(err)
	}
	results, err := engine.Reconcile(ctx, *req, files)
	if err != nil {
		return newPacketError(packetErrorReconciliationFailed, err)
	}
	bound, err := provenance.Bind(ctx, *req, files, results, provenance.Options{})
	if err != nil {
		return newPacketError(packetErrorEvidenceFailed, err)
	}
	if err := writeReconcileReportFile(ctx, flags.out, flags.overwrite, bound); err != nil {
		return normalizeReportError(err)
	}
	return nil
}

func ensureOutputWritable(path string, overwrite bool) error {
	_, err := os.Stat(path)
	if err == nil {
		if !overwrite {
			return newPacketError(packetErrorOutputExists, fmt.Errorf("%s already exists", path))
		}
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return newPacketError(packetErrorOutputWriteFailed, err)
}

func normalizeRouteError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return newPacketError(packetErrorInputNotFound, err)
	case strings.Contains(err.Error(), "unsupported format") || strings.Contains(err.Error(), "unsupported reader") || strings.Contains(err.Error(), "incompatible_reader"):
		return newPacketError(packetErrorInputUnsupported, err)
	case strings.Contains(err.Error(), "selector") || strings.Contains(err.Error(), "path_invalid"):
		return newPacketError(packetErrorSelectorInvalid, err)
	default:
		return newPacketError(packetErrorInputUnsupported, err)
	}
}

func normalizeReportError(err error) error {
	var existsErr outputExistsError
	if errors.As(err, &existsErr) {
		return newPacketError(packetErrorOutputExists, err)
	}
	var writeErr outputWriteError
	if errors.As(err, &writeErr) {
		return newPacketError(packetErrorOutputWriteFailed, err)
	}
	if strings.Contains(err.Error(), "reconcile_report_failed") {
		return newPacketError(packetErrorReportFailed, err)
	}
	return newPacketError(packetErrorOutputWriteFailed, err)
}

func writeReconcileReportFile(ctx context.Context, path string, overwrite bool, result *provenance.ProvenancedResultSet) error {
	var buf bytes.Buffer
	if err := report.Write(ctx, &buf, result); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return outputWriteError{err: fmt.Errorf("create temp report: %w", err)}
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return outputWriteError{err: fmt.Errorf("write temp report: %w", err)}
	}
	if err := tmp.Close(); err != nil {
		return outputWriteError{err: fmt.Errorf("close temp report: %w", err)}
	}
	if err := finalizeReconcileReportFile(tmpPath, path, overwrite); err != nil {
		return err
	}
	ok = true
	return nil
}

func finalizeReconcileReportFile(tmpPath, path string, overwrite bool) error {
	if overwrite {
		if err := os.Rename(tmpPath, path); err != nil {
			return outputWriteError{err: fmt.Errorf("rename temp report: %w", err)}
		}
		return nil
	}

	in, err := os.Open(tmpPath)
	if err != nil {
		return outputWriteError{err: fmt.Errorf("open temp report: %w", err)}
	}
	defer in.Close()

	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return outputExistsError{err: fmt.Errorf("%s already exists", path)}
		}
		return outputWriteError{err: fmt.Errorf("create report: %w", err)}
	}
	removeOut := true
	defer func() {
		if removeOut {
			_ = os.Remove(path)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return outputWriteError{err: fmt.Errorf("write report: %w", err)}
	}
	if err := out.Close(); err != nil {
		return outputWriteError{err: fmt.Errorf("close report: %w", err)}
	}
	removeOut = false
	return nil
}

func parseReconcileFlags(args []string) (reconcileFlags, error) {
	var flags reconcileFlags
	seen := map[string]bool{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return reconcileFlags{}, fmt.Errorf("%w: unexpected trailing argument %q", ErrUsage, arg)
		}

		name := strings.TrimPrefix(arg, "--")
		if name == "" {
			return reconcileFlags{}, fmt.Errorf("%w: unknown flag %q", ErrUsage, arg)
		}
		if strings.Contains(name, "=") {
			parts := strings.SplitN(name, "=", 2)
			name = parts[0]
			value := parts[1]
			if err := setReconcileFlag(&flags, seen, name, value, true); err != nil {
				return reconcileFlags{}, err
			}
			continue
		}

		if name == "overwrite" {
			if err := setReconcileFlag(&flags, seen, name, "true", false); err != nil {
				return reconcileFlags{}, err
			}
			continue
		}

		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return reconcileFlags{}, fmt.Errorf("%w: --%s requires a value", ErrUsage, name)
		}
		i++
		if err := setReconcileFlag(&flags, seen, name, args[i], false); err != nil {
			return reconcileFlags{}, err
		}
	}

	if flags.request == "" {
		return reconcileFlags{}, ErrMissingRequestPath
	}
	if flags.out == "" {
		return reconcileFlags{}, ErrMissingOutputPath
	}
	return flags, nil
}

func setReconcileFlag(flags *reconcileFlags, seen map[string]bool, name, value string, inline bool) error {
	switch name {
	case "request", "out", "workdir", "overwrite":
	default:
		return fmt.Errorf("%w: unknown flag --%s", ErrUsage, name)
	}
	if seen[name] {
		return fmt.Errorf("%w: duplicate flag --%s", ErrUsage, name)
	}
	seen[name] = true

	switch name {
	case "request":
		flags.request = value
	case "out":
		flags.out = value
	case "workdir":
		flags.workdir = value
	case "overwrite":
		if inline {
			return fmt.Errorf("%w: --overwrite does not take a value", ErrUsage)
		}
		flags.overwrite = true
	}
	return nil
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
