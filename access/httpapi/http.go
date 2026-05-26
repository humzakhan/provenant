package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/humzakhan/provenant/access/corepath"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

const (
	RouteReadMintBundle = "/v1/read-mint-bundle"

	readerExtractJSON = "extract.json"
	maxRequestBytes   = 2 << 20
	maxSourceBytes    = 1 << 20
)

var (
	ErrInvalidRequest    = errors.New("httpapi: invalid request")
	ErrUnsupportedReader = errors.New("httpapi: unsupported reader")
	ErrRequestTooLarge   = errors.New("httpapi: request exceeds size limit")
	ErrSourceTooLarge    = errors.New("httpapi: source bytes exceed size limit")
)

type Handler struct {
	Now func() time.Time
}

type ReadMintBundleRequest struct {
	Reader string                    `json:"reader"`
	Source kernel.SourceBytes        `json:"source"`
	Run    kernel.Run                `json:"run"`
	Read   kernel.ReadAndMintRequest `json:"read"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != RouteReadMintBundle {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method must be POST")
		return
	}
	if err := requireJSONContentType(r.Header.Get("Content-Type")); err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", err.Error())
		return
	}

	request, err := decodeRequest(r.Body)
	if err != nil {
		writeRequestError(w, err)
		return
	}

	result, err := execute(r.Context(), request, h.now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_mint_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func execute(ctx context.Context, request ReadMintBundleRequest, now time.Time) (corepath.ReadMintBundleResult, error) {
	deps, err := buildDependencies(request, now)
	if err != nil {
		return corepath.ReadMintBundleResult{}, err
	}
	return corepath.ExecuteReadMintBundle(ctx, deps, corepath.ReadMintBundleRequest{
		Surface: corepath.SurfaceHTTP,
		Run:     request.Run,
		Read:    request.Read,
	})
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

func requireJSONContentType(contentType string) error {
	if contentType == "" {
		return fmt.Errorf("%w: content type must be application/json", ErrInvalidRequest)
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("%w: malformed content type", ErrInvalidRequest)
	}
	if mediaType != "application/json" {
		return fmt.Errorf("%w: content type must be application/json", ErrInvalidRequest)
	}
	return nil
}

func writeRequestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRequestTooLarge), errors.Is(err, ErrSourceTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", err.Error())
	case errors.Is(err, ErrUnsupportedReader), errors.Is(err, ErrInvalidRequest):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{
		Error: errorBody{
			Code:    code,
			Message: message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

type fixedClock struct {
	t time.Time
}

func (f fixedClock) Now() time.Time {
	return f.t
}
