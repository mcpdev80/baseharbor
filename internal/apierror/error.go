package apierror

import (
	"errors"
	"net/http"
)

// Config sentinels. These are matched by internal/deployment and recognized by
// the classify layer (internal/machine). Declaring them here keeps the classify
// layer able to recognize the family without importing the higher-level
// deployment package. internal/deployment re-exports the identical sentinel
// values so callers that match them via errors.Is against the deployment
// package keep working.
var (
	// ErrConfigMissing is returned when the config root is populated but the
	// authoritative config.yaml is absent.
	ErrConfigMissing = errors.New("BaseHarbor config root is configured but the config.yaml is missing")
	// ErrConfigEmpty is returned when an existing config.yaml carries no
	// loadable content (zero bytes or whitespace only). It belongs to the
	// ErrConfigMissing family.
	ErrConfigEmpty = errors.New("BaseHarbor config root is configured but the config.yaml is empty")
)

// Code is a stable machine-readable API error code.
type Code string

const (
	CodeBadRequest   Code = "bad_request"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeInternal     Code = "internal_error"
)

// Error is safe to serialize to API clients.
// Cause is retained for internal handling and is never serialized.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	Cause   error  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code Code, message string, status int) *Error {
	if message == "" {
		message = defaultMessage(code)
	}
	if status == 0 {
		status = defaultStatus(code)
	}
	return &Error{Code: code, Message: message, Status: status}
}

func Wrap(code Code, cause error) *Error {
	err := New(code, "", 0)
	err.Cause = cause
	return err
}

func defaultStatus(code Code) int {
	switch code {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func defaultMessage(code Code) string {
	switch code {
	case CodeBadRequest:
		return "invalid request"
	case CodeUnauthorized:
		return "authentication required"
	case CodeForbidden:
		return "forbidden"
	case CodeNotFound:
		return "resource not found"
	case CodeConflict:
		return "resource conflict"
	default:
		return "internal error"
	}
}
