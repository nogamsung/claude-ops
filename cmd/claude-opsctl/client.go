// Package main provides the claude-opsctl CLI binary.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const defaultBaseURL = "http://127.0.0.1:8787"

// Client is an HTTP client for the claude-ops API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// newClient creates a Client using CLAUDE_OPS_URL env or the default base URL.
func newClient() *Client {
	base := os.Getenv("CLAUDE_OPS_URL")
	if base == "" {
		base = defaultBaseURL
	}
	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// get performs a GET request and decodes the JSON response into dest.
func (c *Client) get(ctx context.Context, path string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	return c.do(req, dest)
}

// post performs a POST request with a JSON body and decodes the response into dest.
func (c *Client) post(ctx context.Context, path string, body interface{}, dest interface{}) error {
	return c.sendJSON(ctx, http.MethodPost, path, body, dest)
}

// patch performs a PATCH request with a JSON body and decodes the response into dest.
func (c *Client) patch(ctx context.Context, path string, body interface{}, dest interface{}) error {
	return c.sendJSON(ctx, http.MethodPatch, path, body, dest)
}

func (c *Client) sendJSON(ctx context.Context, method, path string, body interface{}, dest interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, dest)
}

// do executes the request and decodes a JSON response; maps non-2xx to errors.
func (c *Client) do(req *http.Request, dest interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request to %s: %w", req.URL, err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on read path

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(raw, &apiErr); jsonErr == nil && apiErr.Error != "" {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, apiErr.Error)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(raw))
	}

	if dest != nil {
		if err := json.Unmarshal(raw, dest); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
