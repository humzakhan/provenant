package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/humzakhan/provenant/kernel/public/bundle"
)

// replayRequest is the JSON body for `replay`.
type replayRequest struct {
	BundlePath string `json:"bundle_path"`
}

func (r Runner) runReplay(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[replayRequest](args, stdin)
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
		return fmt.Errorf("replay: read bundle: %w", err)
	}

	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(state)
}
