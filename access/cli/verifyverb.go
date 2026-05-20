package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/provenant/provenant/kernel/public/bundle"
)

// verifyRequest is the JSON body for `verify`.
type verifyRequest struct {
	BundlePath string `json:"bundle_path"`
}

// verifyResponse is written to stdout on `verify` success.
type verifyResponse struct {
	OK bool `json:"ok"`
}

// verifyErrorResponse is written to stdout on `verify` failure.
type verifyErrorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func (r Runner) runVerify(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[verifyRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.BundlePath == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("bundle_path is required"))
	}

	bundleBytes, err := os.ReadFile(req.BundlePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newPacketError(packetErrorInputNotFound, err)
		}
		return fmt.Errorf("verify: read bundle: %w", err)
	}

	verifyErr := bundle.Verify(bundleBytes)

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if verifyErr != nil {
		_ = enc.Encode(verifyErrorResponse{OK: false, Error: verifyErr.Error()})
		return verifyErr
	}
	return enc.Encode(verifyResponse{OK: true})
}
