package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/runfile"
)

// runStartRequest is the JSON body for `run start`.
type runStartRequest struct {
	Run     kernel.Run `json:"run"`
	RunFile string     `json:"run_file"`
}

// runStartResponse is written to stdout on `run start` success.
type runStartResponse struct {
	RunFile string `json:"run_file"`
	Seq     uint64 `json:"seq"`
}

// runFinishRequest is the JSON body for `run finish`.
type runFinishRequest struct {
	RunFile string `json:"run_file"`
}

// runBundleResponse is written to stdout on `run finish` or `run fail` success.
type runBundleResponse struct {
	BundleSHA256 string `json:"bundle_sha256"`
	BundlePath   string `json:"bundle_path"`
}

// runFailRequest is the JSON body for `run fail`.
type runFailRequest struct {
	RunFile string `json:"run_file"`
	Reason  string `json:"reason"`
}

func (r Runner) runStart(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[runStartRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required"))
	}
	if req.Run.ID == "" || req.Run.TenantID == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run.id and run.tenant_id are required"))
	}

	run, err := runfile.Start(runfile.Options{
		Path:   req.RunFile,
		Run:    req.Run,
		Clock:  fixedClock{t: r.now()},
		Tenant: req.Run.TenantID,
	})
	if err != nil {
		return normalizeRunfileError(err)
	}
	seq := run.LastSeq()
	// Release the lock without deleting the partfile. Subsequent verbs
	// (read-mint, calc, submit-answer) will call Start() which replays the
	// existing non-empty partfile and acquires the lock.
	if err := run.Close(); err != nil {
		return fmt.Errorf("runfile: close after start: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(runStartResponse{RunFile: req.RunFile, Seq: seq})
}

func (r Runner) runFinish(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[runFinishRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required"))
	}

	run, err := runfile.Recover(req.RunFile)
	if err != nil {
		return normalizeRunfileError(err)
	}
	run.SetClock(fixedClock{t: r.now()})
	if err := run.Finish(); err != nil {
		return normalizeRunfileError(err)
	}

	bundlePath := partfileToBundlePath(req.RunFile)
	bundleBytes, readErr := os.ReadFile(bundlePath)
	if readErr != nil {
		return fmt.Errorf("runfile: read finalized bundle: %w", readErr)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(runBundleResponse{
		BundleSHA256: sha256HexBytes(bundleBytes),
		BundlePath:   bundlePath,
	})
}

func (r Runner) runFail(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[runFailRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required"))
	}

	run, err := runfile.Recover(req.RunFile)
	if err != nil {
		return normalizeRunfileError(err)
	}
	run.SetClock(fixedClock{t: r.now()})
	if err := run.Fail(req.Reason); err != nil {
		return normalizeRunfileError(err)
	}

	bundlePath := partfileToBundlePath(req.RunFile)
	bundleBytes, readErr := os.ReadFile(bundlePath)
	if readErr != nil {
		return fmt.Errorf("runfile: read finalized bundle: %w", readErr)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(runBundleResponse{
		BundleSHA256: sha256HexBytes(bundleBytes),
		BundlePath:   bundlePath,
	})
}

// partfileToBundlePath derives the final bundle path from a partfile path.
// The partfile is ".{base}.part" in some directory; the bundle is at the
// path the caller used for Options.Path (i.e. the non-.part file).
// However, the runfile package derives the .part from the final path as:
//
//	partPath = dir + "/." + base + ".part"
//
// So we need to reverse that. The `run start` request carries the run_file
// which is the final bundle path (not the .part path). `run finish` and
// `run fail` receive the same run_file, so we just use it directly.
//
// For Recover, the argument is the final bundle path (it converts internally).
func partfileToBundlePath(runFilePath string) string {
	// runFilePath is the final bundle path as supplied by the caller.
	// After Finish/Fail, the bundle is written to this path.
	return runFilePath
}

func normalizeRunfileError(err error) error {
	switch {
	case errors.Is(err, runfile.ErrConcurrentAppender):
		return newPacketError(packetErrorConcurrentAppender, err)
	case errors.Is(err, runfile.ErrAlreadyFinalized):
		return newPacketError(packetErrorRequestInvalid, err)
	case errors.Is(err, runfile.ErrCorruptedPart):
		return newPacketError(packetErrorOutputWriteFailed, err)
	case errors.Is(err, runfile.ErrRunFinalized):
		return newPacketError(packetErrorRequestInvalid, err)
	case errors.Is(err, runfile.ErrSeqOutOfOrder):
		return fmt.Errorf("runfile: %w", err)
	case errors.Is(err, os.ErrNotExist):
		return newPacketError(packetErrorInputNotFound, err)
	default:
		return err
	}
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// decodeJSONRequest decodes a JSON request from --request flag or stdin.
// It uses DisallowUnknownFields to fail closed on extra fields.
func decodeJSONRequest[T any](args []string, stdin io.Reader) (T, error) {
	var zero T
	requestPath, err := parseRequestFlag(args)
	if err != nil {
		return zero, userError(err)
	}

	rc, closeFn, err := openRequest(requestPath, stdin)
	if err != nil {
		return zero, newPacketError(packetErrorRequestInvalid, err)
	}
	if closeFn != nil {
		defer closeFn()
	}

	limited := &io.LimitedReader{R: rc, N: maxRequestBytes + 1}
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()

	var req T
	if err := dec.Decode(&req); err != nil {
		return zero, newPacketError(packetErrorRequestInvalid, fmt.Errorf("decode request: %w", err))
	}
	if err := rejectTrailingJSON(dec); err != nil {
		return zero, newPacketError(packetErrorRequestInvalid, err)
	}
	if limited.N == 0 {
		return zero, newPacketError(packetErrorRequestInvalid, ErrRequestTooLarge)
	}
	return req, nil
}

func parseRequestFlag(args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--request" {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%w: --request requires a value", ErrUsage)
			}
			return args[i+1], nil
		}
		if len(args[i]) > 10 && args[i][:10] == "--request=" {
			return args[i][10:], nil
		}
	}
	return "", ErrMissingRequestPath
}

