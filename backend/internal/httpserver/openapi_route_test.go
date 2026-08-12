package httpserver

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/capability"
	"k8s-aiops.local/backend/internal/eventstream"
	"k8s-aiops.local/backend/internal/metricshistory"
)

type openAPIDocument struct {
	Paths map[string]map[string]yaml.Node `yaml:"paths"`
}

var ginPathParameter = regexp.MustCompile(`:([A-Za-z0-9_]+)`)

func TestRegisteredRoutesMatchOpenAPI(t *testing.T) {
	documentPath := filepath.Join(repositoryRoot(t), "docs", "api", "openapi.yaml")
	contents, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}
	var document openAPIDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}

	engine := buildFullEngine(t)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+normalizeOpenAPIPath(route.Path)] = struct{}{}
	}
	documented := make(map[string]struct{})
	for path, operations := range document.Paths {
		for method := range operations {
			if !isHTTPMethod(method) {
				continue
			}
			documented[strings.ToUpper(method)+" "+path] = struct{}{}
		}
	}

	if missing := routeSetDifference(documented, registered); len(missing) > 0 {
		t.Fatalf("OpenAPI documents routes not registered by Gin: %s", strings.Join(missing, ", "))
	}
	if undocumented := routeSetDifference(registered, documented); len(undocumented) > 0 {
		t.Fatalf("Gin registers routes missing from OpenAPI: %s", strings.Join(undocumented, ", "))
	}
}

func mustMetricsHistoryService(t *testing.T) *metricshistory.Service {
	t.Helper()
	service, err := metricshistory.NewService(metricshistory.Config{}, &metricsHistoryRepositoryStub{})
	if err != nil {
		t.Fatalf("create metrics history service: %v", err)
	}
	return service
}

func mustAlertService(t *testing.T) *alert.Service {
	t.Helper()
	return alert.NewService(nil, nil, nil, 60*time.Second)
}

func mustEventStreamService(t *testing.T) *eventstream.Service {
	t.Helper()
	svc, err := eventstream.NewService(eventstream.Config{}, nil)
	if err != nil {
		t.Fatalf("create eventstream service: %v", err)
	}
	return svc
}

// mustProviderRegistryForContract builds a minimal capability registry with
// the stable provider catalog. Route contract tests only assert route
// presence, not runtime state, so providers are registered with nil
// lifecycle adapters and reported as not-configured where appropriate.
func mustProviderRegistryForContract(t *testing.T) *capability.Registry {
	t.Helper()
	reg := capability.NewRegistry(
		[]string{
			capability.ClusterRoleStandalone,
			capability.ClusterRoleHost,
			capability.ClusterRoleMember,
		},
		1*time.Second,
	)
	entries := []capability.ProviderDescriptor{
		{Name: "metrics_prometheus", Description: "contract", Kind: "capability"},
		{Name: "logs_loki", Description: "contract", Kind: "capability"},
		{Name: "federation", Description: "contract", Kind: "federation"},
		{Name: "inspection_scheduler", Description: "contract", Kind: "inspection"},
		{Name: "service_mesh_readonly", Description: "contract", Kind: "mesh"},
		{Name: "gitops_argocd", Description: "contract", Kind: "gitops"},
		{Name: "copyops_cross_cluster", Description: "contract", Kind: "copyops"},
		{Name: "app_catalog_helm", Description: "contract", Kind: "appcatalog"},
		{Name: "backup_restore_velero", Description: "contract", Kind: "backup"},
		{Name: "ai_investigator", Description: "contract", Kind: "ai"},
	}
	for _, entry := range entries {
		if err := reg.Register(entry); err != nil {
			t.Fatalf("register %s provider: %v", entry.Name, err)
		}
	}
	return reg
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate route contract test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func normalizeOpenAPIPath(path string) string {
	return ginPathParameter.ReplaceAllString(path, `{$1}`)
}

func isHTTPMethod(method string) bool {
	switch method {
	case "get", "post", "put", "patch", "delete", "options", "head", "trace":
		return true
	default:
		return false
	}
}

func routeSetDifference(left, right map[string]struct{}) []string {
	values := make([]string, 0)
	for value := range left {
		if _, ok := right[value]; !ok {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

// openAPITestAuthzRepo is a no-op authz.Repository so the OpenAPI parity test
// can wire the Authz service and GrantManager, exercising the access-grants
// routes without a database.
type openAPITestAuthzRepo struct{}

func (openAPITestAuthzRepo) CreateClusterGrant(context.Context, int64, int64) (authz.ClusterGrant, error) {
	return authz.ClusterGrant{}, nil
}
func (openAPITestAuthzRepo) DeleteClusterGrant(context.Context, int64, int64) error { return nil }
func (openAPITestAuthzRepo) ListClusterGrants(context.Context, int64) ([]authz.ClusterGrant, error) {
	return nil, nil
}
func (openAPITestAuthzRepo) CreateNamespaceGrant(context.Context, int64, int64, string) (authz.NamespaceGrant, error) {
	return authz.NamespaceGrant{}, nil
}
func (openAPITestAuthzRepo) DeleteNamespaceGrant(context.Context, int64, int64, string) error {
	return nil
}
func (openAPITestAuthzRepo) ListNamespaceGrants(context.Context, int64) ([]authz.NamespaceGrant, error) {
	return nil, nil
}
func (openAPITestAuthzRepo) ClusterScope(context.Context, int64, int64) (authz.ClusterScope, error) {
	return authz.ClusterScope{}, nil
}
func (openAPITestAuthzRepo) VisibleClusters(context.Context, int64) ([]int64, error) { return nil, nil }
func (openAPITestAuthzRepo) HasClusterGrant(context.Context, int64, int64) (bool, error) {
	return false, nil
}
