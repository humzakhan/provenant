package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/humzakhan/provenant/kernel/public/bundle"
	"github.com/humzakhan/provenant/kernel/public/grounding"
)

const maxAnswerBytes = 1 << 20

var ErrUngroundedClaims = errors.New("check-answer: ungrounded numeric claims")

type checkAnswerRequest struct {
	BundlePath string `json:"bundle_path"`
	AnswerPath string `json:"answer_path"`
}

func (r Runner) runCheckAnswer(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	requestPath, err := parseRequestFlag(args)
	if err != nil {
		return userError(err)
	}

	req, err := decodeJSONRequest[checkAnswerRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.BundlePath == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("bundle_path is required"))
	}
	if req.AnswerPath == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("answer_path is required"))
	}
	if req.AnswerPath == "-" && requestPath == "-" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("answer_path \"-\" requires request JSON from a file"))
	}

	bundleBytes, err := os.ReadFile(req.BundlePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newPacketError(packetErrorInputNotFound, err)
		}
		return fmt.Errorf("check-answer: read bundle: %w", err)
	}

	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		return err
	}

	answer, err := readAnswer(req.AnswerPath, stdin)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newPacketError(packetErrorInputNotFound, err)
		}
		if errors.Is(err, ErrAnswerTooLarge) {
			return newPacketError(packetErrorRequestInvalid, err)
		}
		return fmt.Errorf("check-answer: read answer: %w", err)
	}

	verdict, err := grounding.CheckAnswer(answer, state.Tokens)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(verdict); err != nil {
		return fmt.Errorf("check-answer: encode verdict: %w", err)
	}
	if verdict.Fail {
		return ErrUngroundedClaims
	}
	return nil
}

func readAnswer(path string, stdin io.Reader) (string, error) {
	rc, closeFn, err := openRequest(path, stdin)
	if err != nil {
		return "", err
	}
	if closeFn != nil {
		defer ignoreCloseError(closeFn)
	}

	limited := &io.LimitedReader{R: rc, N: maxAnswerBytes + 1}
	answerBytes, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if limited.N == 0 {
		return "", ErrAnswerTooLarge
	}
	return string(answerBytes), nil
}
