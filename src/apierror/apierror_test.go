package apierror

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		code string
		want int
	}{
		{name: "bad request", code: CodeBadRequest, want: 400},
		{name: "validation failed", code: CodeValidationFailed, want: 400},
		{name: "unauthorized", code: CodeUnauthorized, want: 401},
		{name: "token expired", code: CodeTokenExpired, want: 401},
		{name: "token invalid", code: CodeTokenInvalid, want: 401},
		{name: "2fa required", code: CodeTwoFactorRequired, want: 401},
		{name: "2fa invalid", code: CodeTwoFactorInvalid, want: 401},
		{name: "forbidden", code: CodeForbidden, want: 403},
		{name: "account locked", code: CodeAccountLocked, want: 403},
		{name: "not found", code: CodeNotFound, want: 404},
		{name: "method not allowed", code: CodeMethodNotAllowed, want: 405},
		{name: "conflict", code: CodeConflict, want: 409},
		{name: "rate limited", code: CodeRateLimited, want: 429},
		{name: "server error", code: CodeServerError, want: 500},
		{name: "maintenance", code: CodeMaintenance, want: 503},
		{name: "unknown code", code: "TOTALLY_MADE_UP", want: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPStatus(tt.code); got != tt.want {
				t.Fatalf("HTTPStatus(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestDefaultMessage(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{name: "bad request", code: CodeBadRequest, want: "Invalid request format"},
		{name: "validation failed", code: CodeValidationFailed, want: "Validation failed"},
		{name: "unauthorized", code: CodeUnauthorized, want: "Authentication required"},
		{name: "token expired", code: CodeTokenExpired, want: "Token has expired"},
		{name: "token invalid", code: CodeTokenInvalid, want: "Invalid token"},
		{name: "2fa required", code: CodeTwoFactorRequired, want: "Two-factor authentication required"},
		{name: "2fa invalid", code: CodeTwoFactorInvalid, want: "Invalid 2FA code"},
		{name: "forbidden", code: CodeForbidden, want: "Permission denied"},
		{name: "account locked", code: CodeAccountLocked, want: "Account locked"},
		{name: "not found", code: CodeNotFound, want: "Resource not found"},
		{name: "method not allowed", code: CodeMethodNotAllowed, want: "Method not allowed"},
		{name: "conflict", code: CodeConflict, want: "Resource already exists"},
		{name: "rate limited", code: CodeRateLimited, want: "Too many requests"},
		{name: "server error", code: CodeServerError, want: "Internal server error"},
		{name: "maintenance", code: CodeMaintenance, want: "Service unavailable"},
		{name: "unknown code", code: "TOTALLY_MADE_UP", want: "Internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultMessage(tt.code); got != tt.want {
				t.Fatalf("DefaultMessage(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestSendOK(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{name: "nil data", data: nil, want: `{"ok":true}`},
		{name: "map data", data: map[string]any{"id": "abc"}, want: `{"ok":true,"data":{"id":"abc"}}`},
		{name: "slice data", data: []int{1, 2}, want: `{"ok":true,"data":[1,2]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := SendOK(rec, tt.data); err != nil {
				t.Fatalf("SendOK returned error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Fatalf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSendError(t *testing.T) {
	tests := []struct {
		name       string
		err        *Error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "plain not found",
			err:        New(CodeNotFound),
			wantStatus: 404,
			wantBody:   `{"ok":false,"error":"NOT_FOUND","message":"Resource not found"}`,
		},
		{
			name: "validation with details",
			err: Newf(CodeValidationFailed, "Invalid email address").
				WithDetails(map[string]any{"field": "email"}),
			wantStatus: 400,
			wantBody:   `{"ok":false,"error":"VALIDATION_FAILED","message":"Invalid email address","details":{"field":"email"}}`,
		},
		{
			name:       "server error",
			err:        New(CodeServerError),
			wantStatus: 500,
			wantBody:   `{"ok":false,"error":"SERVER_ERROR","message":"Internal server error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := SendError(rec, tt.err); err != nil {
				t.Fatalf("SendError returned error: %v", err)
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Fatalf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

func TestSendErrorNeverLeaksInternal(t *testing.T) {
	const secret = "pq: connection refused on 10.0.0.5"
	tests := []struct {
		name string
		err  *Error
	}{
		{name: "wrapped server error", err: Wrap(CodeServerError, errors.New(secret))},
		{name: "wrapped not found", err: Wrap(CodeNotFound, errors.New(secret))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := SendError(rec, tt.err); err != nil {
				t.Fatalf("SendError returned error: %v", err)
			}
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("internal error text leaked into response body: %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "internal") {
				t.Fatalf("internal field present in response body: %s", rec.Body.String())
			}
		})
	}
}

func TestSendErrorCode(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := SendErrorCode(rec, CodeRateLimited); err != nil {
		t.Fatalf("SendErrorCode returned error: %v", err)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	want := `{"ok":false,"error":"RATE_LIMITED","message":"Too many requests"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want slog.Level
	}{
		{name: "server error is error", err: New(CodeServerError), want: slog.LevelError},
		{name: "maintenance is error", err: New(CodeMaintenance), want: slog.LevelError},
		{name: "not found is warn", err: New(CodeNotFound), want: slog.LevelWarn},
		{name: "unauthorized is warn", err: New(CodeUnauthorized), want: slog.LevelWarn},
		{name: "sub 400 is info", err: &Error{Code: "OK", HTTPStatusCode: 200}, want: slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LogLevel(tt.err); got != tt.want {
				t.Fatalf("LogLevel = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogAttrs(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want map[string]string
	}{
		{
			name: "no request id no internal",
			err:  New(CodeNotFound),
			want: map[string]string{"error_code": "NOT_FOUND", "http_status": "404"},
		},
		{
			name: "with request id",
			err:  New(CodeForbidden).WithRequestID("req_abc123"),
			want: map[string]string{"error_code": "FORBIDDEN", "request_id": "req_abc123", "http_status": "403"},
		},
		{
			name: "with internal",
			err:  Wrap(CodeServerError, errors.New("boom")).WithRequestID("req_1"),
			want: map[string]string{"error_code": "SERVER_ERROR", "request_id": "req_1", "http_status": "500", "internal": "boom"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := LogAttrs(tt.err)
			got := make(map[string]string, len(attrs))
			for _, a := range attrs {
				got[a.Key] = a.Value.String()
			}
			if len(got) != len(tt.want) {
				t.Fatalf("attrs = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("attr %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFromAndUnwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	existing := Wrap(CodeConflict, sentinel)

	tests := []struct {
		name         string
		in           error
		wantNil      bool
		wantCode     string
		wantSentinel bool
	}{
		{name: "nil error", in: nil, wantNil: true},
		{name: "already app error", in: existing, wantCode: CodeConflict, wantSentinel: true},
		{name: "wrapped app error", in: errors.Join(errors.New("outer"), existing), wantCode: CodeConflict, wantSentinel: true},
		{name: "plain error", in: sentinel, wantCode: CodeServerError, wantSentinel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := From(tt.in)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("From(nil) = %v, want nil", got)
				}
				return
			}
			if got.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Code, tt.wantCode)
			}
			if tt.wantSentinel && !errors.Is(got, sentinel) {
				t.Fatalf("errors.Is(got, sentinel) = false, want true")
			}
			if got.Unwrap() == nil {
				t.Fatalf("Unwrap() = nil, want the internal error")
			}
		})
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "without internal", err: New(CodeNotFound), want: "NOT_FOUND: Resource not found"},
		{name: "with internal", err: Wrap(CodeServerError, errors.New("boom")), want: "SERVER_ERROR: Internal server error: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
