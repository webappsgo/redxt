package i18n

import (
	"net/http"
	"strings"
	"time"
)

// CookieName is the default cookie LangFromRequest reads and
// SetLanguageCookie writes, per PART 31's fallback chain. Callers that
// configure a different name (server.i18n.cookie_name) pass it via
// CookieNamed instead.
const CookieName = "lang"

// DefaultCookieMaxAge is one year, matching the PART 31 example
// (`Max-Age=31536000`).
const DefaultCookieMaxAge = 365 * 24 * time.Hour

// LangFromRequest resolves the active language for r using the PART 31
// fallback chain: `?lang=` query parameter, then the lang cookie, then
// Accept-Language, then DefaultLanguage. It never returns an
// unsupported code — resolve() folds any unsupported value to
// DefaultLanguage.
func LangFromRequest(r *http.Request) string {
	return LangFromRequestCookie(r, CookieName)
}

// LangFromRequestCookie is LangFromRequest with a configurable cookie
// name, for deployments that set server.i18n.cookie_name.
func LangFromRequestCookie(r *http.Request, cookieName string) string {
	if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" {
		return resolve(q)
	}
	if c, err := r.Cookie(cookieName); err == nil {
		if v := strings.TrimSpace(c.Value); v != "" {
			return resolve(v)
		}
	}
	if lang := parseAcceptLanguage(r.Header.Get("Accept-Language")); lang != "" {
		return resolve(lang)
	}
	return DefaultLanguage
}

// SetLanguageCookie persists lang as the visitor's language cookie so
// the `?lang=` choice survives past the current request, per PART 31.
func SetLanguageCookie(w http.ResponseWriter, lang string) {
	SetLanguageCookieNamed(w, CookieName, lang, DefaultCookieMaxAge)
}

// SetLanguageCookieNamed is SetLanguageCookie with a configurable
// cookie name and max age, for server.i18n.cookie_name /
// cookie_max_age.
func SetLanguageCookieNamed(w http.ResponseWriter, cookieName, lang string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    resolve(lang),
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
}

// parseAcceptLanguage picks the highest-quality supported language tag
// out of a raw Accept-Language header, ignoring region subtags (e.g.
// "es-MX" matches "es") and quality-value ordering. It returns "" when
// nothing in the header matches a supported language, letting the
// caller fall through to DefaultLanguage.
func parseAcceptLanguage(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	type candidate struct {
		lang string
		q    float64
	}

	var candidates []candidate
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		q := 1.0
		if i := strings.Index(part, ";"); i >= 0 {
			tag = strings.TrimSpace(part[:i])
			if qv, ok := parseQValue(part[i+1:]); ok {
				q = qv
			}
		}
		base := strings.ToLower(tag)
		if i := strings.IndexAny(base, "-_"); i >= 0 {
			base = base[:i]
		}
		if base == "" || base == "*" {
			continue
		}
		candidates = append(candidates, candidate{lang: base, q: q})
	}

	best := ""
	bestQ := -1.0
	for _, c := range candidates {
		if !IsSupported(c.lang) {
			continue
		}
		if c.q > bestQ {
			bestQ = c.q
			best = c.lang
		}
	}
	return best
}

// parseQValue parses the "q=0.8" attribute of one Accept-Language
// segment.
func parseQValue(attr string) (float64, bool) {
	attr = strings.TrimSpace(attr)
	if !strings.HasPrefix(attr, "q=") {
		return 0, false
	}
	raw := strings.TrimPrefix(attr, "q=")
	var whole, frac int
	var fracDigits int
	sawDigit := false
	negOrInvalid := false
	i := 0
	for ; i < len(raw) && raw[i] >= '0' && raw[i] <= '9'; i++ {
		whole = whole*10 + int(raw[i]-'0')
		sawDigit = true
	}
	if i < len(raw) && raw[i] == '.' {
		i++
		for ; i < len(raw) && raw[i] >= '0' && raw[i] <= '9'; i++ {
			frac = frac*10 + int(raw[i]-'0')
			fracDigits++
			sawDigit = true
		}
	}
	if !sawDigit || negOrInvalid {
		return 0, false
	}
	q := float64(whole)
	if fracDigits > 0 {
		q += float64(frac) / pow10(fracDigits)
	}
	return q, true
}

// pow10 returns 10^n for the small non-negative n values parseQValue
// ever calls it with.
func pow10(n int) float64 {
	v := 1.0
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}
