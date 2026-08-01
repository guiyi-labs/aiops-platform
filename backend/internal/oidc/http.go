package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// maxOIDCResponseSize bounds discovery and JWKS responses to 1 MiB, matching
// the offline identity-readiness gate.
const maxOIDCResponseSize = 1 << 20

// newBoundedHTTPClient returns an HTTP client that enforces the OIDC runtime
// SSRF controls: a bounded timeout and no redirects. Callers must still
// validate that target URLs are absolute HTTPS before dispatching.
func newBoundedHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("oidc: redirects are not allowed")
		},
	}
}

// requireHTTPSURL validates that raw is an absolute HTTPS URL without
// userinfo. It is the runtime SSRF guard for provider endpoints.
func requireHTTPSURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("oidc: invalid URL %q: %w", raw, err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("oidc: endpoint %q must be an absolute HTTPS URL without userinfo", raw)
	}
	return nil
}

// fetchJSON performs a bounded GET against an HTTPS endpoint and decodes the
// response into target. Unknown fields are permitted so the runtime can
// interoperate with providers that advertise extensions; required fields are
// validated explicitly by the caller.
func fetchJSON(ctx context.Context, client *http.Client, rawURL string, target any) error {
	if err := requireHTTPSURL(rawURL); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("oidc: request %s failed: %w", rawURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: %s returned status %d", rawURL, response.StatusCode)
	}
	if err := decodeLimitedJSON(response, target); err != nil {
		return fmt.Errorf("oidc: decode %s response: %w", rawURL, err)
	}
	return nil
}

// decodeLimitedJSON reads an HTTP response body capped at
// maxOIDCResponseSize+1 bytes and decodes a single JSON value into target. It
// rejects empty bodies, oversized bodies and payloads that contain trailing
// JSON values. Unknown fields are permitted so the runtime interoperates with
// providers that advertise extensions.
func decodeLimitedJSON(response *http.Response, target any) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOIDCResponseSize+1))
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("response body is empty")
	}
	if len(body) > maxOIDCResponseSize {
		return fmt.Errorf("response body exceeds %d bytes", maxOIDCResponseSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("response must contain a single JSON value")
	}
	return nil
}
