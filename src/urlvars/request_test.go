package urlvars

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

const (
	sampleUUID = "550e8400-e29b-41d4-a716-446655440000"
	otherUUID  = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
)

func TestRequestID(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		want       string
		wantRandom bool
	}{
		{
			name:    "request id header wins",
			headers: map[string]string{HeaderRequestID: sampleUUID, HeaderCorrelationID: otherUUID},
			want:    sampleUUID,
		},
		{
			name:    "correlation id is the first fallback",
			headers: map[string]string{HeaderCorrelationID: otherUUID, HeaderTraceID: sampleUUID},
			want:    otherUUID,
		},
		{
			name:    "trace id is the last fallback",
			headers: map[string]string{HeaderTraceID: sampleUUID},
			want:    sampleUUID,
		},
		{
			name:    "uppercase uuid is normalized",
			headers: map[string]string{HeaderRequestID: "550E8400-E29B-41D4-A716-446655440000"},
			want:    sampleUUID,
		},
		{
			name:    "surrounding whitespace is trimmed",
			headers: map[string]string{HeaderRequestID: "  " + sampleUUID + "  "},
			want:    sampleUUID,
		},
		{
			name:    "invalid value falls through to the next header",
			headers: map[string]string{HeaderRequestID: "not-a-uuid", HeaderCorrelationID: otherUUID},
			want:    otherUUID,
		},
		{
			name:       "invalid values everywhere generate a new id",
			headers:    map[string]string{HeaderRequestID: "not-a-uuid", HeaderCorrelationID: "12345"},
			wantRandom: true,
		},
		{
			name:       "urn form is rejected",
			headers:    map[string]string{HeaderRequestID: "urn:uuid:" + sampleUUID},
			wantRandom: true,
		},
		{
			name:       "braced form is rejected",
			headers:    map[string]string{HeaderRequestID: "{" + sampleUUID + "}"},
			wantRandom: true,
		},
		{
			name:       "no headers generate a new id",
			wantRandom: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}

			got := RequestID(req)
			if tc.wantRandom {
				if !IsValidUUID(got) {
					t.Fatalf("generated request ID %q is not a valid UUID", got)
				}
				if got == RequestID(req) {
					t.Fatal("generated request IDs must not repeat")
				}
				return
			}
			if got != tc.want {
				t.Fatalf("RequestID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequestIDNilRequest(t *testing.T) {
	if got := RequestID(nil); !IsValidUUID(got) {
		t.Fatalf("RequestID(nil) = %q, want a generated UUID", got)
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: sampleUUID, want: true},
		{in: uuid.NewString(), want: true},
		{in: "", want: false},
		{in: "not-a-uuid", want: false},
		{in: "550e8400e29b41d4a716446655440000", want: false},
		{in: "urn:uuid:" + sampleUUID, want: false},
		{in: "550e8400-e29b-41d4-a716-44665544000z", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := IsValidUUID(tc.in); got != tc.want {
				t.Fatalf("IsValidUUID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	var seen string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seen = RequestIDFromContext(req.Context())
	}))

	t.Run("client id is preserved", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		req.Header.Set(HeaderRequestID, sampleUUID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if seen != sampleUUID {
			t.Fatalf("context request ID = %q, want %q", seen, sampleUUID)
		}
		if got := rec.Header().Get(HeaderRequestID); got != sampleUUID {
			t.Fatalf("response header = %q, want %q", got, sampleUUID)
		}
	})

	t.Run("missing id is generated and echoed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if !IsValidUUID(seen) {
			t.Fatalf("context request ID = %q, want a generated UUID", seen)
		}
		if got := rec.Header().Get(HeaderRequestID); got != seen {
			t.Fatalf("response header = %q, want %q", got, seen)
		}
	})

	t.Run("original request is not mutated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if got := RequestIDFromContext(req.Context()); got != "" {
			t.Fatalf("original request context = %q, want empty", got)
		}
	})
}

func TestRequestIDFromContext(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("empty context = %q, want empty", got)
	}
	if got := RequestIDFromContext(context.WithValue(context.Background(), requestIDKey, sampleUUID)); got != sampleUUID {
		t.Fatalf("stored ID = %q, want %q", got, sampleUUID)
	}
	if got := RequestIDFromContext(WithRequestID(context.Background(), otherUUID)); got != otherUUID {
		t.Fatalf("WithRequestID = %q, want %q", got, otherUUID)
	}
}

func TestAuthToken(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		headers    map[string]string
		wantToken  string
		wantSource string
	}{
		{
			name:       "bearer wins over every other source",
			target:     "http://example.com/?token=querytoken",
			headers:    map[string]string{HeaderAuthorization: "Bearer authtoken", HeaderXAPIKey: "apikey", HeaderAuthToken: "authheader"},
			wantToken:  "authtoken",
			wantSource: SourceBearer,
		},
		{
			name:       "basic credentials",
			headers:    map[string]string{HeaderAuthorization: "Basic dXNlcjpwYXNz"},
			wantToken:  "dXNlcjpwYXNz",
			wantSource: SourceBasic,
		},
		{
			name:       "digest credentials",
			headers:    map[string]string{HeaderAuthorization: `Digest username="u", realm="r"`},
			wantToken:  `username="u", realm="r"`,
			wantSource: SourceDigest,
		},
		{
			name:       "scheme is case insensitive",
			headers:    map[string]string{HeaderAuthorization: "bearer authtoken"},
			wantToken:  "authtoken",
			wantSource: SourceBearer,
		},
		{
			name:       "unknown scheme falls through",
			headers:    map[string]string{HeaderAuthorization: "Weird sometoken", HeaderXAPIKey: "apikey"},
			wantToken:  "apikey",
			wantSource: HeaderXAPIKey,
		},
		{
			name:       "schemeless authorization falls through",
			headers:    map[string]string{HeaderAuthorization: "rawtoken", HeaderXAPIKey: "apikey"},
			wantToken:  "apikey",
			wantSource: HeaderXAPIKey,
		},
		{
			name:       "empty bearer credentials fall through",
			headers:    map[string]string{HeaderAuthorization: "Bearer ", HeaderXAPIKey: "apikey"},
			wantToken:  "apikey",
			wantSource: HeaderXAPIKey,
		},
		{
			name:       "api key without the x prefix",
			headers:    map[string]string{HeaderAPIKey: "plainapikey", HeaderAuthToken: "authheader"},
			wantToken:  "plainapikey",
			wantSource: HeaderAPIKey,
		},
		{
			name:       "unhyphenated api key",
			headers:    map[string]string{HeaderAPIKeyCompact: "compactapikey", HeaderAuthToken: "authheader"},
			wantToken:  "compactapikey",
			wantSource: HeaderAPIKeyCompact,
		},
		{
			name:       "auth token beats access token",
			headers:    map[string]string{HeaderAuthToken: "authheader", HeaderAccessToken: "accessheader"},
			wantToken:  "authheader",
			wantSource: HeaderAuthToken,
		},
		{
			name:       "access token beats the short forms",
			headers:    map[string]string{HeaderAccessToken: "accessheader", HeaderXToken: "shorttoken", HeaderToken: "minimaltoken"},
			wantToken:  "accessheader",
			wantSource: HeaderAccessToken,
		},
		{
			name:       "x token beats token",
			headers:    map[string]string{HeaderXToken: "shorttoken", HeaderToken: "minimaltoken"},
			wantToken:  "shorttoken",
			wantSource: HeaderXToken,
		},
		{
			name:       "minimal token header",
			headers:    map[string]string{HeaderToken: "minimaltoken"},
			wantToken:  "minimaltoken",
			wantSource: HeaderToken,
		},
		{
			name:       "query parameter is the last resort",
			target:     "http://example.com/?token=querytoken",
			wantToken:  "querytoken",
			wantSource: SourceQuery,
		},
		{
			name:   "no credentials at all",
			target: "http://example.com/",
		},
		{
			name:   "empty query token is ignored",
			target: "http://example.com/?token=",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := tc.target
			if target == "" {
				target = "http://example.com/"
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}

			token, source := AuthToken(req)
			if token != tc.wantToken || source != tc.wantSource {
				t.Fatalf("AuthToken = (%q, %q), want (%q, %q)", token, source, tc.wantToken, tc.wantSource)
			}
		})
	}
}

func TestAuthTokenNilRequest(t *testing.T) {
	token, source := AuthToken(nil)
	if token != "" || source != "" {
		t.Fatalf("AuthToken(nil) = (%q, %q), want empty", token, source)
	}
}

func TestAuthTokenSourceNeverLeaksTheToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set(HeaderAuthorization, "Bearer supersecretvalue")

	token, source := AuthToken(req)
	if token != "supersecretvalue" {
		t.Fatalf("token = %q, want supersecretvalue", token)
	}
	if source != SourceBearer {
		t.Fatalf("source = %q, want %q", source, SourceBearer)
	}
}
