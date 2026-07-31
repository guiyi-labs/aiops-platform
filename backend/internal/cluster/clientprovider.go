package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClientProvider implements Prober, Gateway, PatchGateway and CreateGateway
// using k8s.io/client-go instead of raw net/http. It replaces the legacy
// *Registry while preserving the exact same interface contracts and byte-level
// constraints (body size, content type, maxBytes, no-redirect).
type ClientProvider struct {
	mu      sync.RWMutex
	entries map[int64]*providerEntry
	timeout time.Duration
}

type providerEntry struct {
	restConfig *rest.Config
	clientset  kubernetes.Interface
	restClient rest.Interface
	transport  *http.Transport
}

func NewClientProvider(timeout time.Duration) *ClientProvider {
	return &ClientProvider{entries: make(map[int64]*providerEntry), timeout: timeout}
}

func (p *ClientProvider) Invalidate(clusterID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[clusterID]; ok && entry.transport != nil {
		entry.transport.CloseIdleConnections()
	}
	delete(p.entries, clusterID)
}

func (p *ClientProvider) Probe(ctx context.Context, clusterID int64, kubeconfig []byte) (string, error) {
	clientset, err := p.clientset(clusterID, kubeconfig)
	if err != nil {
		return "", err
	}
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", mapRestError(err)
	}
	if version.GitVersion == "" {
		return "", fmt.Errorf("decode Kubernetes version response")
	}
	return version.GitVersion, nil
}

func (p *ClientProvider) Get(ctx context.Context, clusterID int64, kubeconfig []byte, path string, query url.Values, maxBytes int64) ([]byte, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	restClient, err := p.restClient(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	req := restClient.Get().AbsPath(path)
	applyQuery(req, query)
	body, err := req.DoRaw(ctx)
	if err != nil {
		return nil, mapRestError(err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("Kubernetes API response exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func (p *ClientProvider) Patch(ctx context.Context, clusterID int64, kubeconfig []byte, path string, query url.Values, contentType string, body []byte, maxBytes int64) ([]byte, error) {
	if contentType != "application/strategic-merge-patch+json" {
		return nil, fmt.Errorf("unsupported Kubernetes patch content type")
	}
	if len(body) == 0 || len(body) > 4096 {
		return nil, fmt.Errorf("Kubernetes patch body must contain 1 to 4096 bytes")
	}
	return p.doWrite(ctx, clusterID, kubeconfig, http.MethodPatch, path, query, contentType, body, maxBytes)
}

func (p *ClientProvider) Create(ctx context.Context, clusterID int64, kubeconfig []byte, path string, query url.Values, contentType string, body []byte, maxBytes int64) ([]byte, error) {
	if contentType != "application/json" {
		return nil, fmt.Errorf("unsupported Kubernetes create content type")
	}
	if len(body) == 0 || len(body) > 262144 {
		return nil, fmt.Errorf("Kubernetes create body must contain 1 to 262144 bytes")
	}
	return p.doWrite(ctx, clusterID, kubeconfig, http.MethodPost, path, query, contentType, body, maxBytes)
}

func (p *ClientProvider) doWrite(ctx context.Context, clusterID int64, kubeconfig []byte, method, path string, query url.Values, contentType string, body []byte, maxBytes int64) ([]byte, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	restClient, err := p.restClient(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	var req *rest.Request
	switch method {
	case http.MethodPatch:
		req = restClient.Patch(typesPatchType(contentType)).AbsPath(path).Body(body)
	case http.MethodPost:
		req = restClient.Post().AbsPath(path).Body(body)
	default:
		return nil, fmt.Errorf("unsupported method %s", method)
	}
	applyQuery(req, query)
	response, err := req.DoRaw(ctx)
	if err != nil {
		return nil, mapRestError(err)
	}
	if int64(len(response)) > maxBytes {
		return nil, fmt.Errorf("Kubernetes API response exceeds %d bytes", maxBytes)
	}
	return response, nil
}

func (p *ClientProvider) restClient(clusterID int64, kubeconfig []byte) (rest.Interface, error) {
	entry, err := p.entry(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	return entry.restClient, nil
}

func (p *ClientProvider) clientset(clusterID int64, kubeconfig []byte) (kubernetes.Interface, error) {
	entry, err := p.entry(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	return entry.clientset, nil
}

func (p *ClientProvider) entry(clusterID int64, kubeconfig []byte) (*providerEntry, error) {
	p.mu.RLock()
	if entry, ok := p.entries[clusterID]; ok {
		p.mu.RUnlock()
		return entry, nil
	}
	p.mu.RUnlock()

	clientConfig, err := ParseKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	restConfig := buildRestConfig(clientConfig, p.timeout)
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("build Kubernetes clientset: %w", err)
	}
	restClient := clientset.RESTClient()
	// Preserve the legacy Registry invariant: never follow redirects. The
	// default http.Client follows up to 10 redirects, which would mask
	// 3xx responses from misconfigured clusters or intermediate proxies.
	restClient.(*rest.RESTClient).Client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	entry := &providerEntry{
		restConfig: restConfig,
		clientset:  clientset,
		restClient: restClient,
		transport:  clientConfig.Transport,
	}
	p.mu.Lock()
	p.entries[clusterID] = entry
	p.mu.Unlock()
	return entry, nil
}

// buildRestConfig converts the validated ClientConfig (from ParseKubeconfig)
// into a rest.Config. The TLS transport is reused directly so the validated
// CA pool, client certificates and server name are preserved without
// re-parsing. client-go wraps this transport with its rate limiter and retry
// logic via QPS/Burst.
func buildRestConfig(cc ClientConfig, timeout time.Duration) *rest.Config {
	return &rest.Config{
		Host:        cc.Server,
		BearerToken: cc.Token,
		Transport:   cc.Transport,
		Timeout:     timeout,
		QPS:         20,
		Burst:       40,
		UserAgent:   "k8s-aiops-platform/0.1",
	}
}

func validatePath(path string) error {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fmt.Errorf("invalid Kubernetes API path")
	}
	return nil
}

// applyQuery sets each query parameter on the rest.Request. client-go's
// rest.Request.Param method replaces any existing value for the same key,
// matching the behavior of url.Values.Encode for single-valued keys. For
// multi-valued keys (rare; only dryRun in practice) the last value wins,
// which is acceptable because all current call sites use at most one value
// per key.
func applyQuery(req *rest.Request, query url.Values) {
	for key, values := range query {
		for _, value := range values {
			req.Param(key, value)
		}
	}
}

// mapRestError converts a client-go error into the legacy APIStatusError type
// so that existing callers (mapGatewayError, mapCreateGatewayError,
// ResourceExists) continue to work without modification.
func mapRestError(err error) error {
	var statusError *apierrors.StatusError
	if errors.As(err, &statusError) {
		return APIStatusError{StatusCode: int(statusError.Status().Code)}
	}
	return err
}

// Discovery returns a discovery client for the given cluster. Used by probes
// and capability checks outside the hot read/write path.
func (p *ClientProvider) Discovery(clusterID int64, kubeconfig []byte) (discovery.DiscoveryInterface, error) {
	entry, err := p.entry(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	return entry.clientset.Discovery(), nil
}

// DynamicClient returns a dynamic client for the given cluster. Used by
// controlled CRD operations (Velero Backup/Restore) that need a typed GVR
// without a generated typed client.
func (p *ClientProvider) DynamicClient(clusterID int64, kubeconfig []byte) (dynamic.Interface, error) {
	entry, err := p.entry(clusterID, kubeconfig)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(entry.restConfig)
}

// Clientset returns the typed clientset for direct typed access by callers
// that have migrated off the generic Gateway interface (M33.2 write paths).
func (p *ClientProvider) Clientset(clusterID int64, kubeconfig []byte) (kubernetes.Interface, error) {
	return p.clientset(clusterID, kubeconfig)
}

// typesPatchType converts a content-type string to the PatchType used by
// client-go. Only strategic-merge-patch+json is accepted; the caller has
// already validated this.
func typesPatchType(contentType string) types.PatchType {
	if contentType == "application/strategic-merge-patch+json" {
		return types.StrategicMergePatchType
	}
	return types.PatchType(contentType)
}
