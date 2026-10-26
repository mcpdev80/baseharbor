package apierror

import "net/http"

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
