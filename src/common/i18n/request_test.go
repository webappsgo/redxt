package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLangFromRequestFallbackChain(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		cookie         string
		acceptLanguage string
		want           string
	}{
		{
			name: "no signal falls back to default",
			url:  "/",
			want: "en",
		},
		{
			name: "accept-language wins over nothing",
			url:  "/", acceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8",
			want: "fr",
		},
		{
			name: "cookie beats accept-language",
			url:  "/", cookie: "es", acceptLanguage: "fr-FR",
			want: "es",
		},
		{
			name: "query param beats cookie and accept-language",
			url:  "/?lang=de", cookie: "es", acceptLanguage: "fr-FR",
			want: "de",
		},
		{
			name: "unsupported query param falls back to default",
			url:  "/?lang=xx",
			want: "en",
		},
		{
			name: "unsupported cookie falls back to default",
			url:  "/", cookie: "xx",
			want: "en",
		},
		{
			name: "unsupported accept-language falls back to default",
			url:  "/", acceptLanguage: "xx-XX",
			want: "en",
		},
		{
			name: "accept-language region subtag matches base language",
			url:  "/", acceptLanguage: "zh-Hans-CN",
			want: "zh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tt.cookie})
			}
			if tt.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tt.acceptLanguage)
			}
			if got := LangFromRequest(req); got != tt.want {
				t.Errorf("LangFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetLanguageCookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetLanguageCookie(w, "ja")

	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != CookieName {
		t.Errorf("cookie name = %q, want %q", c.Name, CookieName)
	}
	if c.Value != "ja" {
		t.Errorf("cookie value = %q, want %q", c.Value, "ja")
	}
	if c.Path != "/" {
		t.Errorf("cookie path = %q, want %q", c.Path, "/")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", c.SameSite)
	}
	if c.MaxAge != int(DefaultCookieMaxAge.Seconds()) {
		t.Errorf("cookie MaxAge = %d, want %d", c.MaxAge, int(DefaultCookieMaxAge.Seconds()))
	}
}

func TestSetLanguageCookieFoldsUnsupportedToDefault(t *testing.T) {
	w := httptest.NewRecorder()
	SetLanguageCookie(w, "xx")

	got := w.Result().Cookies()[0].Value
	if got != DefaultLanguage {
		t.Errorf("cookie value = %q, want %q", got, DefaultLanguage)
	}
}

func TestParseAcceptLanguagePrefersHighestQValue(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"fr;q=0.5, de;q=0.9, en;q=0.1", "de"},
		{"", ""},
		{"*", ""},
		{"xx;q=1.0", ""},
		{"xx;q=1.0, es;q=0.3", "es"},
	}
	for _, tt := range tests {
		if got := parseAcceptLanguage(tt.header); got != tt.want {
			t.Errorf("parseAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
