package request

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxRequestBytes = 2 << 20

func DecodeRequest(r io.Reader) (*Request, error) {
	limited := &io.LimitedReader{R: r, N: maxRequestBytes + 1}
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()

	var req Request
	if err := dec.Decode(&req); err != nil {
		if limited.N == 0 {
			return nil, requestTooLargeError()
		}
		return nil, mapDecodeError(err)
	}
	if err := ensureNoTrailingJSON(dec); err != nil {
		if limited.N == 0 {
			return nil, requestTooLargeError()
		}
		return nil, err
	}
	if limited.N == 0 {
		return nil, requestTooLargeError()
	}
	if err := Validate(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func decodeStrictJSON(data []byte, dest any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return mapStrictJSONError(err)
	}
	if err := ensureDecoderEOF(dec); err != nil {
		return err
	}
	return nil
}

func ensureNoTrailingJSON(dec *json.Decoder) error {
	var trailing struct{}
	if err := dec.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return mapDecodeError(err)
	}
	return &DecodeError{Code: CodeTrailingData, Message: "trailing JSON data is not allowed"}
}

func ensureDecoderEOF(dec *json.Decoder) error {
	var trailing struct{}
	if err := dec.Decode(&trailing); err == io.EOF {
		return nil
	}
	return &DecodeError{Code: CodeTrailingData, Message: "trailing JSON data is not allowed"}
}

func requestTooLargeError() *DecodeError {
	return &DecodeError{Code: CodeRequestTooLarge, Message: fmt.Sprintf("request exceeds %d bytes", maxRequestBytes)}
}

func mapDecodeError(err error) error {
	if err == io.EOF {
		return &DecodeError{Code: CodeInvalidJSON, Message: "request body is empty"}
	}
	return mapStrictJSONError(err)
}

func mapStrictJSONError(err error) error {
	message := err.Error()
	switch {
	case strings.HasPrefix(message, "json: unknown field "):
		return &DecodeError{Code: CodeUnknownField, Message: message}
	default:
		return &DecodeError{Code: CodeInvalidJSON, Message: message}
	}
}
