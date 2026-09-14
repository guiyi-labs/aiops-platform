package mcpserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxBackendResponseBytes bounds one platform response so a pathological reply
// cannot exhaust the server's memory.
const maxBackendResponseBytes = 4 << 20

// HTTPBackend calls the platform's REST API with a bearer token.
//
// The token is the entire authorisation story. This server holds no privilege
// of its own, so an agent reaches exactly what the token's owner may read; the
// two controls compose, and both have to fail before anything leaks. A token
// with generous read access still cannot make this server write, and a
// minimally-scoped token still cannot be widened by a clever tool call.
type HTTPBackend struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewHTTPBackend builds a backend against baseURL. A non-positive timeout
// falls back to 30 seconds.
func NewHTTPBackend(baseURL, token string, timeout time.Duration) *HTTPBackend {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HTTPBackend{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: timeout},
	}
}

// Get performs the one request shape this backend can make.
func (b *HTTPBackend) Get(ctx context.Context, path string, query map[string]string) ([]byte, error) {
	// Paths come from the compiled-in catalogue, never from caller input. The
	// check is belt-and-braces against a malformed catalogue entry turning into
	// a request to somewhere else entirely: a leading "//" would be read as a
	// protocol-relative authority by some URL parsers, and an embedded scheme
	// would replace the base outright.
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "://") {
		return nil, fmt.Errorf("refusing non-relative path %q", path)
	}

	endpoint := b.baseURL + path
	if len(query) > 0 {
		values := url.Values{}
		for name, value := range query {
			values.Set(name, value)
		}
		endpoint += "?" + values.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if b.token != "" {
		request.Header.Set("Authorization", "Bearer "+b.token)
	}
	request.Header.Set("Accept", "application/json")

	reply, err := b.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call platform API: %w", err)
	}
	defer reply.Body.Close()

	body, err := io.ReadAll(io.LimitReader(reply.Body, maxBackendResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read platform response: %w", err)
	}
	if len(body) > maxBackendResponseBytes {
		return nil, fmt.Errorf("platform response exceeds %d bytes", maxBackendResponseBytes)
	}
	if reply.StatusCode < 200 || reply.StatusCode >= 300 {
		return nil, fmt.Errorf("platform API returned HTTP %d", reply.StatusCode)
	}
	return body, nil
}
