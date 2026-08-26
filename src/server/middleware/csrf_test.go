package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// csrfHost is the origin every CSRF test treats as the server's own.
const (
	csrfHost       = "app.example.test"
	csrfSelfSite   = "http://" + csrfHost
	csrfOtherSite  = "http://evil.example.test"
	csrfSession    = "redxt_session"
	csrfTokenValue = "8f14e45fceea167a5a36dedd4bea2543"
)

// csrfOptions returns the settings a session-cookie deployment runs with.
func csrfOptions() CSRFOptions {
	return CSRFOptions{
		Enabled:            true,
		ExemptPaths:        []string{"/api/v1/webhooks/"},
		SessionCookieNames: []string{csrfSession},
		IsPublicPath:       func(path string) bool { return strings.HasPrefix(path, "/api/v1/public/") },
	}
}

// csrfRequest builds a state-changing request from the named origin,
// carrying a session cookie unless withSession is false.
func csrfRequest(method, path, origin string, withSession bool) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Host = csrfHost
	if origin != "" {
		req.Header.Set(HeaderOrigin, origin)
	}
	if withSession {
		req.AddCookie(&http.Cookie{Name: csrfSession, Value: "session-value"})
	}
	return req
}

func TestCSRFRequired(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*CSRFOptions)
		request func() *http.Request
		want    bool
		reason  string
	}{
		{
			name:    "cross-site post with a session cookie is checked",
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true) },
			want:    true,
			reason:  "this is the exact shape of a forged request and nothing else",
		},
		{
			name:    "cross-site put is checked",
			request: func() *http.Request { return csrfRequest(http.MethodPut, "/api/v1/records/1", csrfOtherSite, true) },
			want:    true,
			reason:  "every state-changing method is forgeable, not just POST",
		},
		{
			name:    "cross-site delete is checked",
			request: func() *http.Request { return csrfRequest(http.MethodDelete, "/api/v1/records/1", csrfOtherSite, true) },
			want:    true,
			reason:  "a forged DELETE destroys data just as effectively as a forged POST",
		},
		{
			name:    "missing origin and referer is treated as unknown",
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", "", true) },
			want:    true,
			reason:  "an unknown origin must get the same answer as a hostile one",
		},
		{
			name:    "disabled stage checks nothing",
			mutate:  func(o *CSRFOptions) { o.Enabled = false },
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true) },
			want:    false,
			reason:  "an operator who turned CSRF off must not have requests rejected by it",
		},
		{
			name:    "get is safe",
			request: func() *http.Request { return csrfRequest(http.MethodGet, "/api/v1/records", csrfOtherSite, true) },
			want:    false,
			reason:  "RFC 9110 safe methods cannot change state",
		},
		{
			name:    "head is safe",
			request: func() *http.Request { return csrfRequest(http.MethodHead, "/api/v1/records", csrfOtherSite, true) },
			want:    false,
			reason:  "HEAD is a GET without a body",
		},
		{
			name:    "options is safe",
			request: func() *http.Request { return csrfRequest(http.MethodOptions, "/api/v1/records", csrfOtherSite, true) },
			want:    false,
			reason:  "a preflight cannot carry a token and must not be rejected for lacking one",
		},
		{
			name: "websocket upgrade is exempt",
			request: func() *http.Request {
				req := csrfRequest(http.MethodPost, "/api/v1/stream", csrfOtherSite, true)
				req.Header.Set(HeaderUpgrade, SchemeWebSocket)
				return req
			},
			want:   false,
			reason: "the handshake performs its own origin check and the token does not fit the WS lifecycle",
		},
		{
			name: "exempt path prefix is skipped",
			request: func() *http.Request {
				return csrfRequest(http.MethodPost, "/api/v1/webhooks/stripe", csrfOtherSite, true)
			},
			want:   false,
			reason: "a webhook sender cannot know a per-session token",
		},
		{
			name: "public endpoint is skipped",
			request: func() *http.Request {
				return csrfRequest(http.MethodPost, "/api/v1/public/report", csrfOtherSite, true)
			},
			want:   false,
			reason: "there is no session to abuse on an endpoint open to everyone",
		},
		{
			name: "api token header bypasses",
			request: func() *http.Request {
				req := csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true)
				req.Header.Set(HeaderAPIToken, "programmatic")
				return req
			},
			want:   false,
			reason: "a cross-site page cannot make the browser attach an explicit header",
		},
		{
			name: "bearer authorization bypasses",
			request: func() *http.Request {
				req := csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true)
				req.Header.Set("Authorization", "Bearer abc123")
				return req
			},
			want:   false,
			reason: "Bearer auth is a different auth model with no ambient credential to forge",
		},
		{
			name:    "no session cookie means no ambient authority",
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, false) },
			want:    false,
			reason:  "with no cookie attached there is nothing for a forged request to ride on",
		},
		{
			name:    "same-origin request is inherently safe",
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", csrfSelfSite, true) },
			want:    false,
			reason:  "the same-origin policy already prevents a third-party page from sending this",
		},
		{
			name: "same-site referer counts when origin is absent",
			request: func() *http.Request {
				req := csrfRequest(http.MethodPost, "/api/v1/records", "", true)
				req.Header.Set(HeaderReferer, csrfSelfSite+"/records")
				return req
			},
			want:   false,
			reason: "Referer is the documented fallback when the browser sends no Origin",
		},
		{
			name: "cross-site referer is checked",
			request: func() *http.Request {
				req := csrfRequest(http.MethodPost, "/api/v1/records", "", true)
				req.Header.Set(HeaderReferer, csrfOtherSite+"/attack")
				return req
			},
			want:   true,
			reason: "the fallback must classify a foreign Referer as cross-site",
		},
		{
			name:    "strict cookies make a header-less request same-site",
			mutate:  func(o *CSRFOptions) { o.SessionSameSiteStrict = true },
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", "", true) },
			want:    false,
			reason:  "under SameSite=Strict the cookie's arrival is itself proof the request is same-site",
		},
		{
			name:    "strict cookies do not excuse a declared foreign origin",
			mutate:  func(o *CSRFOptions) { o.SessionSameSiteStrict = true },
			request: func() *http.Request { return csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true) },
			want:    true,
			reason:  "a cookie that reached us from a stated cross-site origin was not withheld, so Strict did not apply",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := csrfOptions()
			if tc.mutate != nil {
				tc.mutate(&opts)
			}
			if got := CSRFRequired(opts, tc.request()); got != tc.want {
				t.Errorf("CSRFRequired = %v, want %v: %s", got, tc.want, tc.reason)
			}
		})
	}
}

func TestCSRFValidation(t *testing.T) {
	cases := []struct {
		name       string
		withCookie bool
		cookie     string
		header     string
		wantStatus int
		reason     string
	}{
		{
			name:       "matching token passes",
			withCookie: true,
			cookie:     csrfTokenValue,
			header:     csrfTokenValue,
			wantStatus: http.StatusOK,
			reason:     "the double-submit pair matched, which only the real origin can produce",
		},
		{
			name:       "missing cookie is refused",
			header:     csrfTokenValue,
			wantStatus: http.StatusForbidden,
			reason:     "with no cookie there is nothing to compare against",
		},
		{
			name:       "missing header is refused",
			withCookie: true,
			cookie:     csrfTokenValue,
			wantStatus: http.StatusForbidden,
			reason:     "an attacker's page can cause the cookie to be sent but cannot echo its value",
		},
		{
			name:       "mismatched token is refused",
			withCookie: true,
			cookie:     csrfTokenValue,
			header:     "0000000000000000000000000000000f",
			wantStatus: http.StatusForbidden,
			reason:     "a guessed token must not be accepted",
		},
		{
			name:       "empty cookie value is refused",
			withCookie: true,
			cookie:     "",
			header:     csrfTokenValue,
			wantStatus: http.StatusForbidden,
			reason:     "an empty cookie must never compare equal to anything",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true)
			if tc.withCookie {
				req.AddCookie(&http.Cookie{Name: DefaultCSRFCookie, Value: tc.cookie})
			}
			if tc.header != "" {
				req.Header.Set("X-CSRF-Token", tc.header)
			}

			rec := httptest.NewRecorder()
			CSRF(csrfOptions())(okHandler).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, tc.reason)
			}
		})
	}
}

func TestCSRFAcceptsTheFormField(t *testing.T) {
	body := strings.NewReader(DefaultCSRFField + "=" + csrfTokenValue + "&title=hello")
	req := httptest.NewRequest(http.MethodPost, "/server/administration/records", body)
	req.Host = csrfHost
	req.Header.Set(HeaderOrigin, csrfOtherSite)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfSession, Value: "session-value"})
	req.AddCookie(&http.Cookie{Name: DefaultCSRFCookie, Value: csrfTokenValue})

	seen := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.PostFormValue("title")
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	CSRF(csrfOptions())(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: a no-JavaScript form carries its token in a hidden field", rec.Code, http.StatusOK)
	}
	if seen != "hello" {
		t.Errorf("handler read title = %q, want %q: reading the token must not consume the body the handler needs", seen, "hello")
	}
}

func TestCSRFRejectionLeaksNoReason(t *testing.T) {
	req := csrfRequest(http.MethodPost, "/api/v1/records", csrfOtherSite, true)

	rec := httptest.NewRecorder()
	CSRF(csrfOptions())(okHandler).ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	for _, probe := range []string{"cookie", "header", "mismatch", DefaultCSRFCookie} {
		if strings.Contains(strings.ToLower(body), probe) {
			t.Errorf("body %q names %q: naming the failed check hands an attacker a probe", body, probe)
		}
	}
}
