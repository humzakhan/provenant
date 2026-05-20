package request

import (
	"fmt"
	"sort"
	"strings"
)

type ErrorCode string

const (
	CodeInvalidJSON          ErrorCode = "invalid_json"
	CodeUnknownField         ErrorCode = "unknown_field"
	CodeTrailingData         ErrorCode = "trailing_data"
	CodeRequestTooLarge      ErrorCode = "request_too_large"
	CodeMissingRequired      ErrorCode = "missing_required"
	CodeUnsupportedEnum      ErrorCode = "unsupported_enum"
	CodeDuplicateID          ErrorCode = "duplicate_id"
	CodeDuplicateDeclaration ErrorCode = "duplicate_declaration"
	CodeInvalidReference     ErrorCode = "invalid_reference"
	CodeInvalidModeConfig    ErrorCode = "invalid_mode_config"
	CodeIncompatibleReader   ErrorCode = "incompatible_reader"
	CodeAmbiguousRole        ErrorCode = "ambiguous_role"
)

type DecodeError struct {
	Code    ErrorCode
	Message string
}

func (e *DecodeError) Error() string {
	return string(e.Code) + ": " + e.Message
}

type ValidationError struct {
	Path    string
	Code    ErrorCode
	Message string
}

func (e ValidationError) String() string {
	if e.Path == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Path, e.Code, e.Message)
}

type ValidationErrors struct {
	Errors []ValidationError
}

func (e *ValidationErrors) Error() string {
	parts := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		parts = append(parts, item.String())
	}
	return strings.Join(parts, "; ")
}

func (e *ValidationErrors) add(path string, code ErrorCode, message string) {
	e.Errors = append(e.Errors, ValidationError{Path: path, Code: code, Message: message})
}

func (e *ValidationErrors) sort() {
	sort.Slice(e.Errors, func(i, j int) bool {
		return lessValidationError(e.Errors[i], e.Errors[j])
	})
}

func lessValidationError(a, b ValidationError) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Message < b.Message
}

func (e *ValidationErrors) errOrNil() error {
	if len(e.Errors) == 0 {
		return nil
	}
	e.sort()
	return e
}
