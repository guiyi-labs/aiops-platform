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

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/aiinvestigator"
	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/alertroute"
	"k8s-aiops.local/backend/internal/appcatalog"
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/automation"
	"k8s-aiops.local/backend/internal/backup"
	"k8s-aiops.local/backend/internal/capability"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/copyops"
	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/eventstream"
	"k8s-aiops.local/backend/internal/federation"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/gitops"
	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/golden"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/maintenance"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/monitoring"
	"k8s-aiops.local/backend/internal/namespaceposture"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/oidc"
	"k8s-aiops.local/backend/internal/promotion"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/restore"
	"k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/slo"
	"k8s-aiops.local/backend/internal/topology"
	"k8s-aiops.local/backend/internal/workspace"
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

	gin.SetMode(gin.TestMode)
	authzRepo := &openAPITestAuthzRepo{}
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe:            probeStub{},
		Auth:             &auth.Service{},
		Clusters:         &cluster.Service{},
		Kubernetes:       &k8sgateway.Service{},
		Diagnosis:        &diagnosis.Service{},
		Audit:            &audit.Service{},
		AIExplanation:    &aiexplain.Service{},
		Notifications:    notification.NewService(notification.ServiceConfig{}, nil, nil),
		Remediation:      remediation.NewService(nil, nil, nil),
		Promotion:        promotion.NewService(nil, nil),
		Fleet:            fleet.NewService(fleet.Config{}, nil, nil),
		GlobalSearch:     globalsearch.NewService(globalsearch.Config{}, nil, nil),
		SavedFilters:     globalsearch.NewSavedFilterService(nil),
		MetricsHistory:   mustMetricsHistoryService(t),
		Alert:            mustAlertService(t),
		Backup:           backup.NewService(nil, nil),
		Maintenance:      maintenance.NewService(nil, nil),
		NamespacePosture: namespaceposture.NewService(nil),
		Restore:          restore.NewService(nil, nil),
		Authz:            authz.NewService(authzRepo),
		GrantManager:     authz.NewGrantManager(authzRepo),
		// A non-nil SessionManager registers the OIDC routes so the route
		// contract test covers them. The zero-value Provider is sufficient
		// because the test only inspects route registration, not behavior.
		OIDC:           oidc.NewSessionManager(&oidc.Provider{}, nil, nil, oidc.SessionManagerConfig{}),
		OIDCPostLogout: "https://platform.example.com/post-logout",
		// M37A capability providers: non-nil so the capability routes are
		// registered and covered by the route contract test.
		CapabilityMetricsProvider: capability.NopMetricsProvider{},
		CapabilityLogProvider:     capability.NopLogProvider{},
		// M37B alert-route service: non-nil so the alert-routes are registered
		// and covered by the route contract test.
		AlertRouteService: alertroute.NewService(nil, nil),
		// M39 signal service: non-nil so the aiops routes are registered and
		// covered by the route contract test.
		SignalService: signal.NewService(signal.ServiceOptions{}),
		// M40 topology service: non-nil so the topology routes are registered
		// and covered by the route contract test.
		TopologyService: topology.NewService(nil, topology.NopRepository{}, nil),
		// M41 SLO service: non-nil so the SLO routes are registered and
		// covered by the route contract test. NopRepository avoids a
		// database dependency during route registration.
		SLOService: slo.NewService(slo.NopRepository{}, nil),
		// M42 correlation service: non-nil so the correlation routes are
		// registered and covered by the route contract test. NopRepository
		// avoids a database dependency during route registration.
		CorrelationService: correlation.NewService(correlation.NopRepository{}, nil, nil),
		// M43 AI investigator service: non-nil so the investigator routes are
		// registered and covered by the route contract test. NopRepository
		// avoids a database dependency during route registration.
		AIInvestigatorService: aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil),
		// M44 policy-constrained automation service: non-nil so the automation
		// routes are registered and covered by the route contract test.
		// NopRepository avoids a database dependency during route registration.
		AutomationService: automation.NewService(automation.NopRepository{}, nil, nil),
		// M46 workspace multi-tenancy service: non-nil so the workspace routes
		// are registered and covered by the route contract test. A nil
		// repository is acceptable because the test only inspects route
		// registration, not behavior.
		WorkspaceService: workspace.NewService(nil),
		// M48 federation service: non-nil so the federation routes are
		// registered and covered by the route contract test. A nil
		// repository is acceptable because the test only inspects route
		// registration, not behavior.
		FederationService: federation.NewService(nil, nil),
		// M50 monitoring service: non-nil so the monitoring dashboard and
		// logs/query routes are registered and covered by the route contract
		// test. A nil workspaceLister is acceptable for route registration.
		Monitoring: monitoring.NewService(monitoring.Config{}, nil, nil),
		// M51 event-stream service: non-nil so the SSE events route is
		// registered and covered by the route contract test. A nil lister is
		// acceptable for route registration (the handler returns 503 at call
		// time, but the route is still registered).
		EventStreamService: mustEventStreamService(t),
		// M56 golden quality-report service: non-nil so the quality-report
		// routes are registered and covered by the route contract test.
		// NopReportStorage avoids filesystem writes during route registration.
		GoldenService: golden.NewService(golden.EngineContracts{}, golden.NopReportStorage{}, zaptest.NewLogger(t)),
		// M57 app-catalog service: non-nil so the app-catalog routes are
		// registered and covered by the route contract test. Nil dependencies
		// are acceptable because the test only inspects route registration.
		AppCatalogService: appcatalog.NewService(nil, nil),
		// M58 GitOps + copyops services. Non-nil so the M58 routes are
		// registered for the route contract test.
		GitOpsService:  gitops.NewService(nil),
		CopyOpsService: copyops.NewService(nil, nil),
		// M60 provider registry: non-nil so the capability/providers routes
		// are registered and covered by the route contract test. The route
		// contract test does not call into registry state, only checks route
		// presence, so an empty registry with the full role set is enough.
		CapabilityRegistry: mustProviderRegistryForContract(t),
		Version:            "route-contract-test",
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}

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
