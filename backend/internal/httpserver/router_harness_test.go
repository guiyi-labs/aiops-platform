package httpserver

import (
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/aiinvestigator"
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
	"k8s-aiops.local/backend/internal/federation"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/gitops"
	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/golden"
	"k8s-aiops.local/backend/internal/incident"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/maintenance"
	"k8s-aiops.local/backend/internal/monitoring"
	"k8s-aiops.local/backend/internal/namespaceposture"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/oidc"
	"k8s-aiops.local/backend/internal/optimization"
	"k8s-aiops.local/backend/internal/posture"
	"k8s-aiops.local/backend/internal/promotion"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/restore"
	"k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/slo"
	"k8s-aiops.local/backend/internal/topology"
	"k8s-aiops.local/backend/internal/workspace"
)

// buildFullEngine constructs the production router with every optional
// service enabled, mirroring main.go. Used by route contract tests so the
// routeTable covers the full production route surface.
func buildFullEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authzRepo := &openAPITestAuthzRepo{}
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe:      probeStub{},
		Auth:       &auth.Service{},
		Clusters:   &cluster.Service{},
		Kubernetes: &k8sgateway.Service{},
		Diagnosis:  &diagnosis.Service{},
		// M98 incident workspace: non-nil so the incident routes are
		// registered and covered by the route contract test.
		Incidents:        incident.NewService(nil),
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
		// M64 optimization analyzers: non-nil so the /optimization analyze
		// routes are registered and covered by the route contract test.
		Optimization: optimization.NewService(finops.DefaultCostRate(), nil),
		// M80 posture evaluator: non-nil so the /optimization/posture route is
		// registered and covered by the route contract test. The route test
		// only inspects route presence, not behavior, so a nil-collector
		// evaluator is enough.
		Posture: posture.New(nil),
		Version: "route-contract-test",
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}
	return engine
}
