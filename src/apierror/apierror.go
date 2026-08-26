// Package apierror implements the unified API response envelope and the
// error-code taxonomy defined by AI.md PART 9 (ERROR HANDLING & CACHING).
//
// Every HTTP handler in the project answers with the same JSON shape: a
// success envelope carrying data, or an error envelope carrying a stable
// machine-readable code, a human-readable message, and optional structured
// details. Internal error text is reserved for logs and never reaches a
// client.
package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// Error codes from the PART 9 error-code table. These strings are part of the
// public API contract: clients switch on them, so they must never change.
const (
	// CodeBadRequest reports a malformed or unparsable request.
	CodeBadRequest = "BAD_REQUEST"
	// CodeValidationFailed reports a well-formed request that failed field validation.
	CodeValidationFailed = "VALIDATION_FAILED"
	// CodeUnauthorized reports a request that carried no usable credentials.
	CodeUnauthorized = "UNAUTHORIZED"
	// CodeTokenExpired reports a credential that is no longer valid due to age.
	CodeTokenExpired = "TOKEN_EXPIRED"
	// CodeTokenInvalid reports a credential that failed verification.
	CodeTokenInvalid = "TOKEN_INVALID"
	// CodeTwoFactorRequired reports that a second authentication factor is needed.
	CodeTwoFactorRequired = "2FA_REQUIRED"
	// CodeTwoFactorInvalid reports a rejected second-factor code.
	CodeTwoFactorInvalid = "2FA_INVALID"
	// CodeForbidden reports an authenticated caller lacking permission.
	CodeForbidden = "FORBIDDEN"
	// CodeAccountLocked reports an account that has been administratively or automatically locked.
	CodeAccountLocked = "ACCOUNT_LOCKED"
	// CodeNotFound reports a missing resource.
	CodeNotFound = "NOT_FOUND"
	// CodeMethodNotAllowed reports an HTTP method the route does not accept.
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	// CodeConflict reports a uniqueness or state conflict with an existing resource.
	CodeConflict = "CONFLICT"
	// CodeRateLimited reports that the caller exceeded its request budget.
	CodeRateLimited = "RATE_LIMITED"
	// CodeServerError reports an unexpected server-side failure.
	CodeServerError = "SERVER_ERROR"
	// CodeMaintenance reports that the service is temporarily unavailable.
	CodeMaintenance = "MAINTENANCE"
)

// Response is the canonical API envelope used by every endpoint.
//
// A success response sets OK and Data; an error response sets Error, Message
// and, when the failure carries structured context, Details.
type Response struct {
	OK      bool           `json:"ok"`
	Data    any            `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
	Details map[string]any `json:"details,omitempty"`

	// Debug carries developer-only diagnostics. The PART 11 output
	// sanitization pipeline strips every dev_only field whenever the
	// debug flag is off, which is why the tag is present here rather
	// than the field being conditionally populated by each handler.
	Debug any `json:"_debug,omitempty" dev_only:"true"`
}

// HTTPStatus maps an error code to its HTTP status code. Unknown codes map to
// http.StatusInternalServerError.
func HTTPStatus(code string) int {
	switch code {
	case CodeBadRequest, CodeValidationFailed:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeTokenExpired, CodeTokenInvalid, CodeTwoFactorRequired, CodeTwoFactorInvalid:
		return http.StatusUnauthorized
	case CodeForbidden, CodeAccountLocked:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case CodeConflict:
		return http.StatusConflict
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeMaintenance:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// DefaultMessage returns the user-facing message for an error code. Unknown
// codes fall back to the generic server-error message so that no internal
// detail can leak through an unmapped code.
func DefaultMessage(code string) string {
	switch code {
	case CodeBadRequest:
		return "Invalid request format"
	case CodeValidationFailed:
		return "Validation failed"
	case CodeUnauthorized:
		return "Authentication required"
	case CodeTokenExpired:
		return "Token has expired"
	case CodeTokenInvalid:
		return "Invalid token"
	case CodeTwoFactorRequired:
		return "Two-factor authentication required"
	case CodeTwoFactorInvalid:
		return "Invalid 2FA code"
	case CodeForbidden:
		return "Permission denied"
	case CodeAccountLocked:
		return "Account locked"
	case CodeNotFound:
		return "Resource not found"
	case CodeMethodNotAllowed:
		return "Method not allowed"
	case CodeConflict:
		return "Resource already exists"
	case CodeRateLimited:
		return "Too many requests"
	case CodeServerError:
		return "Internal server error"
	case CodeMaintenance:
		return "Service unavailable"
	default:
		return "Internal server error"
	}
}

// Error is the application error type carried through handlers and middleware.
//
// Code, Message, Details and HTTPStatusCode describe what the client is told;
// Internal holds the underlying cause and is for logging only.
type Error struct {
	Code           string
	Message        string
	HTTPStatusCode int
	RequestID      string
	Details        map[string]any
	Internal       error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Internal != nil {
		return e.Code + ": " + e.Message + ": " + e.Internal.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap returns the wrapped internal error so errors.Is and errors.As can
// inspect the underlying cause.
func (e *Error) Unwrap() error {
	return e.Internal
}

// New builds an Error for a code, filling in the default message and HTTP
// status for that code.
func New(code string) *Error {
	return &Error{
		Code:           code,
		Message:        DefaultMessage(code),
		HTTPStatusCode: HTTPStatus(code),
	}
}

// Newf builds an Error for a code with a formatted user-facing message.
func Newf(code, format string, args ...any) *Error {
	e := New(code)
	e.Message = fmt.Sprintf(format, args...)
	return e
}

// Wrap builds an Error for a code that carries internal as its underlying
// cause. The internal error is never shown to clients.
func Wrap(code string, internal error) *Error {
	e := New(code)
	e.Internal = internal
	return e
}

// WithRequestID attaches a request identifier used in logs and support lookups.
func (e *Error) WithRequestID(id string) *Error {
	e.RequestID = id
	return e
}

// WithDetails attaches structured context, such as the failing field and rule
// for a validation error.
func (e *Error) WithDetails(d map[string]any) *Error {
	e.Details = d
	return e
}

// From converts any error into an *Error. An error that already is (or wraps)
// an *Error is returned as-is; anything else becomes a SERVER_ERROR wrapping
// the original.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Wrap(CodeServerError, err)
}

// SendOK writes a success envelope with the supplied payload.
func SendOK(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json")
	return WriteJSON(w, Response{OK: true, Data: data})
}

// WriteJSON writes v as the indented JSON document AI.md PART 14
// mandates for every API response: two-space indentation followed by a
// single trailing newline, so piping a response into a file or a diff
// produces a well-formed text file.
func WriteJSON(w io.Writer, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	_, err = w.Write(body)
	return err
}

// SendError writes an error envelope with the error's status code.
//
// Security rule from PART 9: the Internal error is NEVER serialized into the
// response. Only Code, Message and Details cross the wire; the internal cause
// belongs in the logs, where LogAttrs puts it.
func SendError(w http.ResponseWriter, e *Error) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatusCode)
	return WriteJSON(w, Response{
		OK:      false,
		Error:   e.Code,
		Message: e.Message,
		Details: e.Details,
	})
}

// SendErrorCode writes an error envelope built from a bare error code.
func SendErrorCode(w http.ResponseWriter, code string) error {
	return SendError(w, New(code))
}

// LogAttrs returns the structured log attributes for an error.
//
// The internal attribute carries the underlying cause's text and exists purely
// for debugging: it is written to logs only and must never be copied into an
// API response. request_id is omitted when the error carries none.
func LogAttrs(e *Error) []slog.Attr {
	attrs := []slog.Attr{slog.String("error_code", e.Code)}
	if e.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", e.RequestID))
	}
	attrs = append(attrs, slog.Int("http_status", e.HTTPStatusCode))
	if e.Internal != nil {
		attrs = append(attrs, slog.String("internal", e.Internal.Error()))
	}
	return attrs
}

// LogLevel returns the log level an error should be recorded at: server
// failures are errors, client failures are warnings, everything else is info.
func LogLevel(e *Error) slog.Level {
	switch {
	case e.HTTPStatusCode >= 500:
		return slog.LevelError
	case e.HTTPStatusCode >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
