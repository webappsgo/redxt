package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/apierror"
)

// ErrTokenRevoked is returned by Get/PostJSON when the server answers a
// request with 401 TOKEN_REVOKED, per AI.md PART 33 "CLI Token Revocation
// Handling". Callers use errors.Is(err, ErrTokenRevoked) to detect it and
// exit with the documented authentication-error exit code instead of a
// generic failure.
var ErrTokenRevoked = errors.New("api: token has been revoked")

// ErrTokenExpired is returned by Get/PostJSON when the server answers a
// request with 401 TOKEN_EXPIRED. AI.md's "CLI Token Revocation Handling"
// section requires the same cached-token deletion on TOKEN_EXPIRED as on
// TOKEN_REVOKED, so callers handle both errors the same way.
var ErrTokenExpired = errors.New("api: token has expired")

// HTTPClient wraps net/http with the identity and timeout rules AI.md
// PART 33 "HTTP Client Identity" requires: the User-Agent always names
// the compiled project, never the (possibly renamed) binary, and every
// request carries the resolved bearer token.
type HTTPClient struct {
	BaseURL string
	Token   string
	client  *http.Client

	// ConfigPath is the cli.yml path to clear a cached token from when
	// the server reports it revoked. Left empty (as in most tests),
	// revocation is still detected and reported but no file is touched.
	ConfigPath string
}

// NewHTTPClient builds an HTTPClient for baseURL, trimming any trailing
// slash so path joins never produce a double slash.
func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Get issues a GET request against path (which must start with "/") and
// decodes a JSON response into out.
func (c *HTTPClient) Get(path string, out any) (*http.Response, error) {
	return c.do(http.MethodGet, path, nil, out)
}

// PostJSON issues a POST request with a JSON-encoded body and decodes a
// JSON response into out.
func (c *HTTPClient) PostJSON(path string, body, out any) (*http.Response, error) {
	var reader *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(data))
	} else {
		reader = strings.NewReader("")
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.send(req, out)
}

func (c *HTTPClient) do(method, path string, body any, out any) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.send(req, out)
}

func (c *HTTPClient) send(req *http.Request, out any) (*http.Response, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("no server configured (use --server or set server.url in cli.yml)")
	}
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, fmt.Errorf("read response: %w", err)
	}

	// AI.md PART 33 "CLI Token Revocation Handling": on 401 TOKEN_REVOKED
	// the cached token must be dropped and the caller told to re-auth,
	// rather than the error envelope being silently decoded into out (or
	// failing decode) like any other response. The section explicitly
	// requires "the same behavior on 401 TOKEN_EXPIRED", so both codes
	// clear the cached token the same way.
	if resp.StatusCode == http.StatusUnauthorized {
		var envelope apierror.Response
		if jsonErr := json.Unmarshal(body, &envelope); jsonErr == nil {
			switch envelope.Error {
			case apierror.CodeTokenRevoked:
				if c.ConfigPath != "" {
					_ = DeleteCachedToken(c.ConfigPath)
				}
				return resp, ErrTokenRevoked
			case apierror.CodeTokenExpired:
				if c.ConfigPath != "" {
					_ = DeleteCachedToken(c.ConfigPath)
				}
				return resp, ErrTokenExpired
			}
		}
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}
