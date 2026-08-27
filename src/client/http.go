package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPClient wraps net/http with the identity and timeout rules AI.md
// PART 33 "HTTP Client Identity" requires: the User-Agent always names
// the compiled project, never the (possibly renamed) binary, and every
// request carries the resolved bearer token.
type HTTPClient struct {
	BaseURL string
	Token   string
	client  *http.Client
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

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}
