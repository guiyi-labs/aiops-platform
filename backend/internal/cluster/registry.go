package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type registryEntry struct {
	client *http.Client
	config ClientConfig
}

type Registry struct {
	mu      sync.RWMutex
	clients map[int64]registryEntry
	timeout time.Duration
}

func NewRegistry(timeout time.Duration) *Registry {
	return &Registry{clients: make(map[int64]registryEntry), timeout: timeout}
}

func (r *Registry) Invalidate(clusterID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.clients[clusterID]; ok {
		entry.config.Transport.CloseIdleConnections()
	}
	delete(r.clients, clusterID)
}

func (r *Registry) Probe(ctx context.Context, clusterID int64, kubeconfig []byte) (string, error) {
	body, err := r.Get(ctx, clusterID, kubeconfig, "/version", nil, 1<<20)
	if err != nil {
		return "", err
	}
	var version struct {
		GitVersion string `json:"gitVersion"`
	}
	if err := json.Unmarshal(body, &version); err != nil || version.GitVersion == "" {
		return "", fmt.Errorf("decode Kubernetes version response")
	}
	return version.GitVersion, nil
}

type APIStatusError struct{ StatusCode int }

func (e APIStatusError) Error() string {
	return fmt.Sprintf("Kubernetes API returned status %d", e.StatusCode)
}

func (r *Registry) Get(ctx context.Context, clusterID int64, kubeconfig []byte, path string, query url.Values, maxBytes int64) ([]byte, error) {
	return r.request(ctx, clusterID, kubeconfig, http.MethodGet, path, query, "", nil, maxBytes)
}

func (r *Registry) Patch(ctx context.Context, clusterID int64, kubeconfig []byte, path string, query url.Values, contentType string, body []byte, maxBytes int64) ([]byte, error) {
	if contentType != "application/strategic-merge-patch+json" {
		return nil, fmt.Errorf("unsupported Kubernetes patch content type")
	}
	if len(body) == 0 || len(body) > 4096 {
		return nil, fmt.Errorf("Kubernetes patch body must contain 1 to 4096 bytes")
	}
	return r.request(ctx, clusterID, kubeconfig, http.MethodPatch, path, query, contentType, body, maxBytes)
}

func (r *Registry) request(ctx context.Context, clusterID int64, kubeconfig []byte, method, path string, query url.Values, contentType string, body []byte, maxBytes int64) ([]byte, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, fmt.Errorf("invalid Kubernetes API path")
	}
	entry, err := r.client(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	target := entry.config.Server + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Header.Set("User-Agent", "k8s-aiops-platform/0.1")
	if entry.config.Token != "" {
		request.Header.Set("Authorization", "Bearer "+entry.config.Token)
	}
	response, err := entry.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("contact Kubernetes API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, APIStatusError{StatusCode: response.StatusCode}
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes API response: %w", err)
	}
	if int64(len(responseBody)) > maxBytes {
		return nil, fmt.Errorf("Kubernetes API response exceeds %d bytes", maxBytes)
	}
	return responseBody, nil
}

func (r *Registry) client(clusterID int64, kubeconfig []byte) (registryEntry, error) {
	r.mu.RLock()
	entry, ok := r.clients[clusterID]
	r.mu.RUnlock()
	if ok {
		return entry, nil
	}
	config, err := ParseKubeconfig(kubeconfig)
	if err != nil {
		return registryEntry{}, err
	}
	entry = registryEntry{client: &http.Client{Transport: config.Transport, Timeout: r.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, config: config}
	r.mu.Lock()
	r.clients[clusterID] = entry
	r.mu.Unlock()
	return entry, nil
}
