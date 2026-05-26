package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gowebpki/jcs"
	"github.com/humzakhan/provenant/kernel/public/trace"
)

// traceRequest is the JSON body for `trace`.
type traceRequest struct {
	BundlePath string `json:"bundle_path"`
	TokenID    string `json:"token_id"`
}

func (r Runner) runTrace(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[traceRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.BundlePath == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("bundle_path is required"))
	}
	if req.TokenID == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("token_id is required"))
	}

	bundleBytes, err := os.ReadFile(req.BundlePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newPacketError(packetErrorInputNotFound, err)
		}
		return fmt.Errorf("trace: read bundle: %w", err)
	}

	g, err := trace.Trace(bundleBytes, req.TokenID)
	if err != nil {
		if errors.Is(err, trace.ErrTokenNotFound) {
			return newPacketError(packetErrorTokenNotFound, err)
		}
		return fmt.Errorf("trace: %w", err)
	}

	// JCS-canonicalize for deterministic output.
	raw, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("trace: marshal graph: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return fmt.Errorf("trace: canonicalize graph: %w", err)
	}
	canonical = append(canonical, '\n')
	_, err = stdout.Write(canonical)
	return err
}
