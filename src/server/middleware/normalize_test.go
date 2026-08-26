package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestURLNormalize(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		lowercase  bool
		wantStatus int
		wantHeader string
		reason     string
	}{
		{
			name:       "trailing slash redirects",
			target:     "/records/",
			wantStatus: http.StatusMovedPermanently,
			wantHeader: "/records",
			reason:     "PART 16 canonical form has no trailing slash",
		},
		{
			name:       "trailing slash keeps the query string",
			target:     "/records/?page=2&sort=name",
			wantStatus: http.StatusMovedPermanently,
			wantHeader: "/records?page=2&sort=name",
			reason:     "dropping the query on a 301 would lose the client's request",
		},
		{
			name:       "root is exempt",
			target:     "/",
			wantStatus: http.StatusOK,
			reason:     "root has no non-slash form to redirect to",
		},
		{
			name:       "canonical path passes through",
			target:     "/records",
			wantStatus: http.StatusOK,
			reason:     "an already canonical path must not cost a redirect",
		},
		{
			name:       "file with a trailing slash is left alone",
			target:     "/assets/app.css/",
			wantStatus: http.StatusOK,
			reason:     "a final segment containing a dot is an explicit file request",
		},
		{
			name:       "nested collection redirects",
			target:     "/api/v1/records/",
			wantStatus: http.StatusMovedPermanently,
			wantHeader: "/api/v1/records",
			reason:     "the rule applies at every depth, not just the first segment",
		},
		{
			name:       "uppercase passes through when folding is off",
			target:     "/Records",
			wantStatus: http.StatusOK,
			reason:     "case folding is opt-in because asset names are case-sensitive",
		},
		{
			name:       "uppercase redirects when folding is on",
			target:     "/Records",
			lowercase:  true,
			wantStatus: http.StatusMovedPermanently,
			wantHeader: "/records",
			reason:     "LowercasePaths must fold the path and redirect once",
		},
		{
			name:       "folding and trailing slash collapse into one redirect",
			target:     "/Records/",
			lowercase:  true,
			wantStatus: http.StatusMovedPermanently,
			wantHeader: "/records",
			reason:     "two normalizations must not cost two round trips",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := URLNormalize(URLNormalizeOptions{LowercasePaths: tc.lowercase})
			rec := httptest.NewRecorder()
			mw(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, tc.reason)
			}
			if tc.wantHeader != "" {
				if got := rec.Header().Get("Location"); got != tc.wantHeader {
					t.Errorf("Location = %q, want %q: %s", got, tc.wantHeader, tc.reason)
				}
			}
		})
	}
}

func TestPathSecurity(t *testing.T) {
	cases := []struct {
		name       string
		rawPath    string
		maxLength  int
		wantStatus int
		wantPath   string
		reason     string
	}{
		{
			name:       "clean path passes",
			rawPath:    "/api/v1/records",
			wantStatus: http.StatusOK,
			wantPath:   "/api/v1/records",
			reason:     "an ordinary path must not be rejected or rewritten",
		},
		{
			name:       "dot dot traversal is rejected",
			rawPath:    "/api/../etc/passwd",
			wantStatus: http.StatusBadRequest,
			reason:     "traversal must be refused before any stage reads the path",
		},
		{
			name:       "trailing traversal is rejected",
			rawPath:    "/files/..",
			wantStatus: http.StatusBadRequest,
			reason:     "a traversal at the end of the path is still a traversal",
		},
		{
			name:       "encoded dot is rejected",
			rawPath:    "/api/%2e%2e/secret",
			wantStatus: http.StatusBadRequest,
			reason:     "%2e is the escaped dot an attacker uses to slip past a decoded-only filter",
		},
		{
			name:       "encoded slash is rejected",
			rawPath:    "/api/records%2fsecret",
			wantStatus: http.StatusBadRequest,
			reason:     "%2f smuggles a path separator into what looks like one segment",
		},
		{
			name:       "encoded backslash is rejected",
			rawPath:    "/api/%5cwindows",
			wantStatus: http.StatusBadRequest,
			reason:     "%5c is the Windows separator in escaped form",
		},
		{
			name:       "null byte is rejected",
			rawPath:    "/api/records%00.png",
			wantStatus: http.StatusBadRequest,
			reason:     "a NUL byte truncates the path in a C string handler downstream",
		},
		{
			name:       "literal backslash is rejected",
			rawPath:    `/api/records\admin`,
			wantStatus: http.StatusBadRequest,
			reason:     "a backslash is a separator on Windows and must never reach a file lookup",
		},
		{
			name:       "duplicate slashes are collapsed",
			rawPath:    "/api//v1///records",
			wantStatus: http.StatusOK,
			wantPath:   "/api/v1/records",
			reason:     "two spellings of one route would defeat prefix-based rules downstream",
		},
		{
			name:       "single dot segments are collapsed",
			rawPath:    "/api/./v1/./records",
			wantStatus: http.StatusOK,
			wantPath:   "/api/v1/records",
			reason:     "path.Clean must remove no-op segments",
		},
		{
			name:       "over-length path is rejected",
			rawPath:    "/" + strings.Repeat("a", 64),
			maxLength:  16,
			wantStatus: http.StatusBadRequest,
			reason:     "an unbounded path is a denial-of-service vector",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := PathSecurity(PathSecurityOptions{MaxPathLength: tc.maxLength})

			seen := ""
			handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				seen = req.URL.Path
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			mw(handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.rawPath, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, tc.reason)
			}
			if tc.wantPath != "" && seen != tc.wantPath {
				t.Errorf("path = %q, want %q: %s", seen, tc.wantPath, tc.reason)
			}
		})
	}
}
