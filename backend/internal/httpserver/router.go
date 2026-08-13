package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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
	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/inspection"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/maintenance"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/monitoring"
	"k8s-aiops.local/backend/internal/namespaceposture"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/oidc"
	"k8s-aiops.local/backend/internal/optimization"
	"k8s-aiops.local/backend/internal/posture"
	"k8s-aiops.local/backend/internal/promotion"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/restore"
	"k8s-aiops.local/backend/internal/servicemesh"
	"k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/slo"
	"k8s-aiops.local/backend/internal/topology"
	"k8s-aiops.local/backend/internal/workspace"
)

type Options struct {
	Probe      readinessProbe
	Auth       *auth.Service
	Clusters   *cluster.Service
	Kubernetes *k8sgateway.Service
	Diagnosis  *diagnosis.Service
	// M98 incident workspace service. When nil the incident routes are not
	// registered.
	Incidents        *incident.Service
	Audit            *audit.Service
	AIExplanation    *aiexplain.Service
	SecureCookies    bool
	RefreshTTL       time.Duration
	Version          string
	Metrics          *Metrics
	Notifications    *notification.Service
	Remediation      *remediation.Service
	Fleet            *fleet.Service
	GlobalSearch     *globalsearch.Service
	SavedFilters     *globalsearch.SavedFilterService
	MetricsHistory   *metricshistory.Service
	Promotion        *promotion.Service
	Alert            *alert.Service
	Backup           *backup.Service
	Maintenance      *maintenance.Service
	NamespacePosture *namespaceposture.Service
	Restore          *restore.Service
	Authz            *authz.Service
	GrantManager     *authz.GrantManager
	// OIDC is non-nil only when OIDC is enabled in configuration. When nil,
	// the OIDC routes are not registered and the server behaves as a
	// local-only deployment.
	OIDC           *oidc.SessionManager
	OIDCPostLogout string
	// M37A capability providers. Either may be nil; the corresponding route
	// returns 503 when its provider is unset. Both being nil is allowed: no
	// capability routes are registered.
	CapabilityMetricsProvider capability.MetricsProvider
	CapabilityLogProvider     capability.LogProvider
	// M37B alert-route service. When nil the alert-routes are not registered.
	AlertRouteService *alertroute.Service
	// M39 signal service. When nil the aiops routes are not registered.
	SignalService *signal.Service
	// M39 signal source reader for the overview. Optional; when nil the
	// overview reports zero active diagnoses and empty source completeness.
	SignalSourceReader signal.SourceReader
	// M40 topology service. When nil the topology routes are not registered.
	TopologyService *topology.Service
	// M41 SLO service. When nil the SLO routes are not registered.
	SLOService *slo.Service
	// M42 correlation service. When nil the correlation routes are not
	// registered.
	CorrelationService *correlation.Service
	// M43 AI investigator service. When nil the investigator routes are not
	// registered.
	AIInvestigatorService *aiinvestigator.Service
	// M44 policy-constrained automation service. When nil the automation
	// routes are not registered.
	AutomationService *automation.Service
	// M46 workspace multi-tenancy service. When nil the workspace routes
	// are not registered.
	WorkspaceService *workspace.Service
	// M48 multi-cluster federation service. When nil the federation routes
	// are not registered.
	FederationService *federation.Service
	// M50 monitoring service. When nil the monitoring dashboard routes are
	// not registered.
	Monitoring *monitoring.Service
	// M51 event-stream service. When nil the SSE events route is not
	// registered. The service wraps the read-only Kubernetes gateway via
	// an adapter (ADR 0066).
	EventStreamService *eventstream.Service
	// M52 inspection service. When nil the inspection routes are not
	// registered. Includes compile-time rule catalog, ad-hoc runs, plans
	// tasks and results.
	InspectionService *inspection.Service
	// M52 service-mesh read-only service. When nil the mesh routes are not
	// registered. Exposes VirtualService/DestinationRule and traffic metrics
	// projections; strictly read-only (no write surface).
	ServiceMeshService *servicemesh.Service
	// M56 golden quality-report service. When nil the quality-report routes
	// are not registered. Exposes the latest quality report (GET) and an
	// async replay trigger (POST, SystemOpsAdmin only).
	GoldenService *golden.Service
	// M57 Helm application catalog service. When nil the app-catalog routes
	// are not registered. Exposes Helm repository CRUD, chart listing/detail
	// (read-only index.yaml fetch), and M19 controlled-operation deploy
	// plans (preview + execute). Credentials are never returned in API
	// responses (ADR 0008).
	AppCatalogService *appcatalog.Service
	// M58 GitOps (ArgoCD Application read-only) service. When nil the
	// GitOps browse routes are not registered. Exposes cluster-level
	// Application list/detail and a capability probe.
	GitOpsService *gitops.Service
	// M58 interactive cross-cluster copy service. When nil the copy-ops
	// routes are not registered. Exposes M19 controlled-operation preview/
	// execute for the operator-curated copy GVR whitelist.
	CopyOpsService *copyops.Service
	// M60 compile-time provider registry (ADR 0075). When non-nil, the
	// /capability/providers read-only surface is registered under the
	// capability route group. The registry carries the compile-time
	// provider catalog, per-provider lifecycle state and health probes.
	// Nil disables the provider surface — safe for tests and minimal builds.
	CapabilityRegistry *capability.Registry
	// M64 optimization analyzers (M61 FinOps, M62 CIS, M63 deprecated-API).
	// When non-nil, the /optimization analyze endpoints are registered. The
	// analyzers are read-only pure functions over a caller-supplied
	// observation bundle (ADR 0004); the server never reaches into a cluster.
	// Nil disables the optimization routes — safe for tests and minimal builds.
	Optimization *optimization.Service
	// Posture evaluator aggregates all M61-M78 analyzers into a unified
	// cluster governance posture report. Nil disables the posture endpoint.
	Posture *posture.Evaluator
}

func New(logger *zap.Logger, options Options) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	resetRouteTable()
	router := gin.New()
	metrics := options.Metrics
	if metrics == nil {
		metrics = NewMetrics()
	}
	router.Use(gin.Recovery(), withRequestID(), requestLogger(logger), requestMetrics(metrics))
	router.GET("/metrics", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(metrics.Render()))
	})
	if options.Audit != nil {
		router.Use(auditTrail(logger, auditServiceAdapter{service: options.Audit}))
	}

	health := healthHandler{
		probe:   options.Probe,
		version: options.Version,
		now:     time.Now,
	}
	authAPI := authHandler{service: options.Auth, secureCookies: options.SecureCookies, refreshTTL: options.RefreshTTL}
	reg := newRouteRegistrar(options.Auth)

	v1 := router.Group("/api/v1")
	{
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/health/live", Handler: health.live})
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/health/ready", Handler: health.ready})
		if options.Auth != nil {
			if options.Fleet != nil {
				fleetAPI := fleetHandler{service: options.Fleet, authz: options.Authz}
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/fleet/health", AuthRequired: true, Handler: fleetAPI.health})
			}
			if options.GlobalSearch != nil {
				searchAPI := globalSearchHandler{service: options.GlobalSearch, authz: options.Authz}
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/fleet/resources/search", AuthRequired: true, Handler: searchAPI.search})
			}
			if options.SavedFilters != nil {
				filtersAPI := savedGlobalSearchFilterHandler{service: options.SavedFilters}
				filterRoutes := v1.Group("/fleet/resources/search/filters", withAuthentication(options.Auth))
				reg.register(filterRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: filtersAPI.list})
				reg.register(filterRoutes, RouteDescriptor{Method: "POST", Path: "", Handler: filtersAPI.create, AuditAction: "global_search_filter.create", AuditResource: "GlobalSearchFilter"})
				reg.register(filterRoutes, RouteDescriptor{Method: "PATCH", Path: "/:filter_id", Handler: filtersAPI.update, AuditAction: "global_search_filter.update", AuditResource: "GlobalSearchFilter"})
				reg.register(filterRoutes, RouteDescriptor{Method: "DELETE", Path: "/:filter_id", Handler: filtersAPI.delete, AuditAction: "global_search_filter.delete", AuditResource: "GlobalSearchFilter"})
			}
			authRoutes := v1.Group("/auth")
			reg.register(authRoutes, RouteDescriptor{Method: "POST", Path: "/login", Handler: authAPI.login, AuditAction: "auth.login", AuditResource: "Session"})
			reg.register(authRoutes, RouteDescriptor{Method: "POST", Path: "/refresh", Handler: authAPI.refresh, AuditAction: "auth.refresh", AuditResource: "Session"})
			reg.register(authRoutes, RouteDescriptor{Method: "POST", Path: "/logout", Handler: authAPI.logout, AuditAction: "auth.logout", AuditResource: "Session"})
			reg.register(authRoutes, RouteDescriptor{Method: "GET", Path: "/me", AuthRequired: true, Handler: authAPI.me})
			reg.register(authRoutes, RouteDescriptor{Method: "POST", Path: "/password-change", AuthRequired: true, Handler: authAPI.changePassword, AuditAction: "auth.password.change", AuditResource: "UserCredential"})
			reg.register(authRoutes, RouteDescriptor{Method: "GET", Path: "/sessions", AuthRequired: true, Handler: authAPI.sessions})
			reg.register(authRoutes, RouteDescriptor{Method: "DELETE", Path: "/sessions/:session_id", AuthRequired: true, Handler: authAPI.revokeSession, AuditAction: "auth.session.revoke", AuditResource: "Session"})
			reg.register(authRoutes, RouteDescriptor{Method: "POST", Path: "/sessions/revoke-others", AuthRequired: true, Handler: authAPI.revokeOtherSessions, AuditAction: "auth.sessions.revoke_others", AuditResource: "Session"})
			if options.OIDC != nil {
				oidcAPI := oidcHandler{authHandler: authAPI, manager: options.OIDC, authSessionTTL: oidcAuthSessionTTL, postLogoutURI: options.OIDCPostLogout}
				oidcRoutes := v1.Group("/auth/oidc")
				reg.register(oidcRoutes, RouteDescriptor{Method: "GET", Path: "/login", Handler: oidcAPI.login, AuditAction: "auth.oidc.login", AuditResource: "Session"})
				reg.register(oidcRoutes, RouteDescriptor{Method: "GET", Path: "/callback", Handler: oidcAPI.callback, AuditAction: "auth.oidc.callback", AuditResource: "Session"})
				reg.register(oidcRoutes, RouteDescriptor{Method: "POST", Path: "/logout", AuthRequired: true, Handler: oidcAPI.logout, AuditAction: "auth.oidc.logout", AuditResource: "Session"})
			}
			usersAPI := userHandler{service: options.Auth}
			reg.register(v1, RouteDescriptor{Method: "GET", Path: "/users/assignable", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: usersAPI.assignable})
			reg.register(v1, RouteDescriptor{Method: "GET", Path: "/users", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: usersAPI.list})
			reg.register(v1, RouteDescriptor{Method: "POST", Path: "/users", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: usersAPI.create, AuditAction: "user.create", AuditResource: "User"})
			reg.register(v1, RouteDescriptor{Method: "PATCH", Path: "/users/:user_id", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: usersAPI.update, AuditAction: "user.update", AuditResource: "User"})
			reg.register(v1, RouteDescriptor{Method: "POST", Path: "/users/:user_id/password-reset", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: usersAPI.resetPassword, AuditAction: "user.password.reset", AuditResource: "User"})
			if options.GrantManager != nil {
				grantsAPI := grantHandler{manager: options.GrantManager}
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/users/:user_id/cluster-grants", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: grantsAPI.listClusterGrants, AuditAction: "user.cluster_grant.list", AuditResource: "ClusterGrant"})
				reg.register(v1, RouteDescriptor{Method: "POST", Path: "/users/:user_id/cluster-grants", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: grantsAPI.createClusterGrant, AuditAction: "user.cluster_grant.create", AuditResource: "ClusterGrant"})
				reg.register(v1, RouteDescriptor{Method: "DELETE", Path: "/users/:user_id/cluster-grants/:cluster_id", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: grantsAPI.deleteClusterGrant, AuditAction: "user.cluster_grant.delete", AuditResource: "ClusterGrant"})
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/users/:user_id/namespace-grants", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: grantsAPI.listNamespaceGrants, AuditAction: "user.namespace_grant.list", AuditResource: "NamespaceGrant"})
				reg.register(v1, RouteDescriptor{Method: "POST", Path: "/users/:user_id/namespace-grants", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: grantsAPI.createNamespaceGrant, AuditAction: "user.namespace_grant.create", AuditResource: "NamespaceGrant"})
				reg.register(v1, RouteDescriptor{Method: "DELETE", Path: "/users/:user_id/namespace-grants/:cluster_id/:namespace", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: grantsAPI.deleteNamespaceGrant, AuditAction: "user.namespace_grant.delete", AuditResource: "NamespaceGrant"})
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/auth/me/grants", AuthRequired: true, Handler: grantsAPI.myGrants})
			}
			if options.Audit != nil {
				auditAPI := auditHandler{service: options.Audit}
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/audit-logs", AuthRequired: true, RequiredRoles: rolesSystemSecurityAudit, Handler: auditAPI.list})
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/audit-logs/export", AuthRequired: true, RequiredRoles: rolesSystemSecurityAudit, Handler: auditAPI.export, AuditAction: "audit.export", AuditResource: "AuditExport"})
			}
			if options.Notifications != nil {
				notificationsAPI := notificationHandler{service: options.Notifications}
				reg.register(v1, RouteDescriptor{Method: "GET", Path: "/notification-deliveries", AuthRequired: true, RequiredRoles: rolesSystemSecurityAudit, Handler: notificationsAPI.list})
				reg.register(v1, RouteDescriptor{Method: "POST", Path: "/notification-deliveries/:delivery_id/retry", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: notificationsAPI.retry, AuditAction: "notification.delivery.retry", AuditResource: "NotificationDelivery"})
			}
			if options.Clusters != nil {
				clustersAPI := clusterHandler{service: options.Clusters}
				clusterRoutes := v1.Group("/clusters", withAuthentication(options.Auth))
				reg.register(clusterRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: clustersAPI.list})
				reg.register(clusterRoutes, RouteDescriptor{Method: "GET", Path: "/:cluster_id", Handler: clustersAPI.get})
				reg.register(clusterRoutes, RouteDescriptor{Method: "POST", Path: "", RequiredRoles: rolesSystemAdmin, Handler: clustersAPI.create, AuditAction: "cluster.create", AuditResource: "Cluster"})
				reg.register(clusterRoutes, RouteDescriptor{Method: "PATCH", Path: "/:cluster_id", RequiredRoles: rolesSystemAdmin, Handler: clustersAPI.setEnabled, AuditAction: "cluster.enabled.update", AuditResource: "Cluster"})
				reg.register(clusterRoutes, RouteDescriptor{Method: "PUT", Path: "/:cluster_id/credentials", RequiredRoles: rolesSystemAdmin, Handler: clustersAPI.updateCredential, AuditAction: "cluster.credentials.rotate", AuditResource: "ClusterCredential"})
				reg.register(clusterRoutes, RouteDescriptor{Method: "POST", Path: "/:cluster_id/probe", RequiredRoles: rolesSystemOpsAdmin, Handler: clustersAPI.probe, AuditAction: "cluster.probe", AuditResource: "Cluster"})
				reg.register(clusterRoutes, RouteDescriptor{Method: "DELETE", Path: "/:cluster_id", RequiredRoles: rolesSystemAdmin, Handler: clustersAPI.delete, AuditAction: "cluster.delete", AuditResource: "Cluster"})
			}
			// Metrics history is cluster-scoped and depends on withClusterContext
			// to populate currentClusterID. Without it the ClusterID would be 0
			// and normalizeQuery would reject every request with INVALID_QUERY.
			// Registered independently of options.Kubernetes so the surface stays
			// available even when the Kubernetes gateway is nil (tests, minimal
			// builds).
			if options.MetricsHistory != nil {
				historyAPI := metricsHistoryHandler{service: options.MetricsHistory}
				metricsHistoryRoutes := v1.Group("/clusters/:cluster_id", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
				reg.register(metricsHistoryRoutes, RouteDescriptor{Method: "GET", Path: "/metrics/history", AuthRequired: true, Handler: historyAPI.series})
				reg.register(metricsHistoryRoutes, RouteDescriptor{Method: "GET", Path: "/metrics/history/evaluate", AuthRequired: true, Handler: historyAPI.evaluate})
			}
			if options.Kubernetes != nil {
				resourcesAPI := kubernetesHandler{service: options.Kubernetes}
				// api-resources is cluster-scoped only (no namespace dimension),
				// so it skips the namespace middleware. It still requires cluster
				// access (404 > 403 anti-leakage via requireClusterAccess).
				clusterScopedRoutes := v1.Group("/clusters/:cluster_id", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
				reg.register(clusterScopedRoutes, RouteDescriptor{Method: "GET", Path: "/api-resources", AuthRequired: true, Handler: resourcesAPI.apiResources, AuditAction: "kubernetes.api_resources.read", AuditResource: "APIResource"})
				// M52 per-cluster effective inspection rules (compile-time defaults
				// merged with runtime overrides from inspection_rules table).
				// Cluster-scoped so requireClusterAccess gates visibility.
				if options.InspectionService != nil {
					inspAPI := inspectionHandler{service: options.InspectionService}
					reg.register(clusterScopedRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/rules", AuthRequired: true, Handler: inspAPI.effectiveRules, AuditAction: "aiops.inspection.rules_effective.list", AuditResource: "InspectionRuleEffective"})
				}
				resourceRoutes := v1.Group("/clusters/:cluster_id", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz), requireNamespaceAccess(options.Authz, "namespace"), requireNamespaceQueryAccess(options.Authz), withWorkspaceNamespaceFilter(options.WorkspaceService))
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/namespaces", Handler: resourcesAPI.namespaces})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/nodes", Handler: resourcesAPI.nodes})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/metrics/nodes", Handler: resourcesAPI.nodeMetrics})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/metrics/pods", Handler: resourcesAPI.podMetrics})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/nodes/:name", Handler: resourcesAPI.node})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/pods", Handler: resourcesAPI.pods})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/pods/:namespace/:name", Handler: resourcesAPI.pod})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/pods/:namespace/:name/logs", Handler: resourcesAPI.logs})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/pods/:namespace/:name/logs_since", Handler: resourcesAPI.logsSince})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/pods/:namespace/:name/all_logs", Handler: resourcesAPI.allContainerLogs})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/pods/:namespace/:name/containers", Handler: resourcesAPI.containers})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/events", Handler: resourcesAPI.events})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/deployments", Handler: resourcesAPI.deployments})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/deployments/:namespace/:name", Handler: resourcesAPI.deployment})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/statefulsets", Handler: resourcesAPI.statefulSets})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/statefulsets/:namespace/:name", Handler: resourcesAPI.statefulSet})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/daemonsets", Handler: resourcesAPI.daemonSets})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/daemonsets/:namespace/:name", Handler: resourcesAPI.daemonSet})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/replicasets", Handler: resourcesAPI.replicaSets})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/replicasets/:namespace/:name", Handler: resourcesAPI.replicaSet})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/jobs", Handler: resourcesAPI.jobs})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/jobs/:namespace/:name", Handler: resourcesAPI.job})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/cronjobs", Handler: resourcesAPI.cronJobs})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/cronjobs/:namespace/:name", Handler: resourcesAPI.cronJob})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/horizontalpodautoscalers", Handler: resourcesAPI.horizontalPodAutoscalers})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/horizontalpodautoscalers/:namespace/:name", Handler: resourcesAPI.horizontalPodAutoscaler})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/resourcequotas", Handler: resourcesAPI.resourceQuotas})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/resourcequotas/:namespace/:name", Handler: resourcesAPI.resourceQuota})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/limitranges", Handler: resourcesAPI.limitRanges})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/limitranges/:namespace/:name", Handler: resourcesAPI.limitRange})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/secrets", Handler: resourcesAPI.secrets})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/secrets/:namespace/:name", Handler: resourcesAPI.secret})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/services", Handler: resourcesAPI.services})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/services/:namespace/:name", Handler: resourcesAPI.serviceDetail})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/ingresses", Handler: resourcesAPI.ingresses})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/ingresses/:namespace/:name", Handler: resourcesAPI.ingress})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/endpointslices", Handler: resourcesAPI.endpointSlices})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/persistentvolumeclaims", Handler: resourcesAPI.persistentVolumeClaims})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/persistentvolumeclaims/:namespace/:name", Handler: resourcesAPI.persistentVolumeClaim})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/storageclasses", Handler: resourcesAPI.storageClasses})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/storageclasses/:name", Handler: resourcesAPI.storageClass})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/configmaps", Handler: resourcesAPI.configMaps})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/configmaps/:namespace/:name", Handler: resourcesAPI.configMap})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/persistentvolumes", Handler: resourcesAPI.persistentVolumes})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/persistentvolumes/:name", Handler: resourcesAPI.persistentVolume})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/poddisruptionbudgets", Handler: resourcesAPI.podDisruptionBudgets})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/poddisruptionbudgets/:namespace/:name", Handler: resourcesAPI.podDisruptionBudget})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/networkpolicies", Handler: resourcesAPI.networkPolicies})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/networkpolicies/:namespace/:name", Handler: resourcesAPI.networkPolicy})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/serviceaccounts", Handler: resourcesAPI.serviceAccounts})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/serviceaccounts/:namespace/:name", Handler: resourcesAPI.serviceAccount})
				// RBAC inventory: read-only Role/ClusterRole/RoleBinding/ClusterRoleBinding.
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/roles", Handler: resourcesAPI.roles})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/roles/:namespace/:name", Handler: resourcesAPI.role})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/clusterroles", Handler: resourcesAPI.clusterRoles})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/clusterroles/:name", Handler: resourcesAPI.clusterRole})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/rolebindings", Handler: resourcesAPI.roleBindings})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/rolebindings/:namespace/:name", Handler: resourcesAPI.roleBinding})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/clusterrolebindings", Handler: resourcesAPI.clusterRoleBindings})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/clusterrolebindings/:name", Handler: resourcesAPI.clusterRoleBinding})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/resources/:kind/:namespace/:name/manifest", Handler: resourcesAPI.manifest})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/velero/capability", Handler: resourcesAPI.veleroCapability})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/backups", Handler: resourcesAPI.backups})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/backups/:namespace/:name", Handler: resourcesAPI.backup})
				// M49 read-only CRD browsing. Namespaced CRDs fan out across the
				// caller's authorized namespace scope (M35) and honor the
				// ?workspace_id visibility filter (M47) applied by the middleware
				// chain; cluster-scoped CRDs are listed cluster-wide. The
				// ?namespace= query is authz-checked by requireNamespaceQueryAccess.
				// Non-whitelisted GVRs return 404 (anti-leakage, ADR 0064).
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/custom-resources/:group/:version/:resource", Handler: resourcesAPI.customResources, AuditAction: "kubernetes.custom_resources.list", AuditResource: "CustomResource"})
				reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/custom-resources/:group/:version/:resource/:name", Handler: resourcesAPI.customResource, AuditAction: "kubernetes.custom_resources.read", AuditResource: "CustomResource"})
				// M50 monitoring dashboard + log explorer. The cluster dashboard
				// returns fixed template panels (no PromQL); logs/query reuses the
				// M37A capability.LogProvider with M35 namespace scope re-check.
				// Both inherit the resourceRoutes middleware chain (cluster +
				// namespace access). The handler returns 503 when the service /
				// provider is nil (ADR 0065 §3).
				if options.Monitoring != nil {
					monitoringAPI := monitoringHandler{service: options.Monitoring, logProvider: options.CapabilityLogProvider}
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/monitoring/dashboard/:template", Handler: monitoringAPI.clusterDashboard, AuditAction: "monitoring.dashboard.read", AuditResource: "MonitoringDashboard"})
					if options.CapabilityLogProvider != nil {
						reg.register(resourceRoutes, RouteDescriptor{Method: "POST", Path: "/logs/query", Handler: monitoringAPI.queryLogs, AuditAction: "monitoring.logs.query", AuditResource: "LogQuery"})
					}
				}
				// M51 bounded SSE event stream. Registered under resourceRoutes
				// so it inherits requireClusterAccess + requireNamespaceQueryAccess
				// (M35 scope). The handler resolves the authorized namespace set
				// and passes it to the bounded poller. Returns 503 when the
				// service is nil (ADR 0066).
				if options.EventStreamService != nil {
					eventstreamAPI := eventstreamHandler{service: options.EventStreamService}
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/events/stream", Handler: eventstreamAPI.streamEvents, AuditAction: "kubernetes.events.stream", AuditResource: "EventStream"})
				}
				// M52 service-mesh read-only access. VirtualService/
				// DestinationRule list+detail and per-cluster traffic metrics.
				// All routes are strictly GET (structural read-only); there is
				// no write surface for mesh resources (ADR 0067 §4). Inherits
				// the resourceRoutes middleware chain including requireClusterAccess
				// and the M47 workspace_id namespace filter.
				if options.ServiceMeshService != nil {
					meshAPI := servicemeshHandler{service: options.ServiceMeshService}
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/service-mesh/virtual-services", Handler: meshAPI.listVirtualServices, AuditAction: "servicemesh.virtualservices.list", AuditResource: "VirtualService"})
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/service-mesh/virtual-services/:namespace/:name", Handler: meshAPI.getVirtualService, AuditAction: "servicemesh.virtualservices.read", AuditResource: "VirtualService"})
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/service-mesh/destination-rules", Handler: meshAPI.listDestinationRules, AuditAction: "servicemesh.destinationrules.list", AuditResource: "DestinationRule"})
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/service-mesh/destination-rules/:namespace/:name", Handler: meshAPI.getDestinationRule, AuditAction: "servicemesh.destinationrules.read", AuditResource: "DestinationRule"})
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/service-mesh/traffic-metrics", Handler: meshAPI.trafficMetrics, AuditAction: "servicemesh.traffic_metrics.read", AuditResource: "ServiceMeshTraffic"})
				}
				if options.NamespacePosture != nil {
					postureAPI := namespacePostureHandler{service: options.NamespacePosture}
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/namespace-postures", Handler: postureAPI.list})
					reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/namespace-postures/:namespace", Handler: postureAPI.get})
				}
				if options.Diagnosis != nil {
					diagnosisAPI := diagnosisHandler{service: options.Diagnosis, users: options.Auth, explanations: options.AIExplanation, remediations: options.Remediation}
					reg.register(resourceRoutes, RouteDescriptor{Method: "POST", Path: "/diagnoses", Handler: diagnosisAPI.create, AuditAction: "diagnosis.run", AuditResource: "Diagnosis"})
					reg.register(resourceRoutes, RouteDescriptor{Method: "POST", Path: "/diagnoses/node_metrics", Handler: diagnosisAPI.diagnoseNodeMetrics})
					reg.register(v1, RouteDescriptor{Method: "GET", Path: "/diagnoses", AuthRequired: true, Handler: diagnosisAPI.list})
					reg.register(v1, RouteDescriptor{Method: "GET", Path: "/diagnoses/summary", AuthRequired: true, Handler: diagnosisAPI.summary})
					reg.register(v1, RouteDescriptor{Method: "GET", Path: "/diagnoses/:diagnosis_id", AuthRequired: true, Handler: diagnosisAPI.get})
					reg.register(v1, RouteDescriptor{Method: "GET", Path: "/diagnoses/:diagnosis_id/replay", AuthRequired: true, Handler: diagnosisAPI.replay, AuditAction: "diagnosis.replay.read", AuditResource: "Diagnosis"})
					reg.register(v1, RouteDescriptor{Method: "PATCH", Path: "/diagnoses/:diagnosis_id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: diagnosisAPI.transition, AuditAction: "diagnosis.status.update", AuditResource: "Diagnosis"})
					reg.register(v1, RouteDescriptor{Method: "POST", Path: "/diagnoses/:diagnosis_id/feedback", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: diagnosisAPI.feedback, AuditAction: "diagnosis.feedback.create", AuditResource: "Diagnosis"})
					reg.register(v1, RouteDescriptor{Method: "PATCH", Path: "/diagnoses/:diagnosis_id/assignment", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: diagnosisAPI.assign, AuditAction: "diagnosis.assignment.update", AuditResource: "Diagnosis"})
					if options.Remediation != nil {
						remediationAPI := remediationHandler{service: options.Remediation}
						reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/operations", Handler: remediationAPI.listOperations})
						reg.register(resourceRoutes, RouteDescriptor{Method: "POST", Path: "/operations/preview", RequiredRoles: rolesSystemOpsAdmin, Handler: remediationAPI.previewOperation, AuditAction: "operation.preview", AuditResource: "ControlledOperation"})
						reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/deployments/:namespace/:name/rollout/history", Handler: remediationAPI.rolloutHistory})
						reg.register(resourceRoutes, RouteDescriptor{Method: "GET", Path: "/deployments/:namespace/:name/rollout/status", Handler: remediationAPI.rolloutStatus})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/diagnoses/:diagnosis_id/remediations", AuthRequired: true, Handler: remediationAPI.list})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/diagnoses/:diagnosis_id/remediations/preview", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: remediationAPI.preview, AuditAction: "remediation.preview", AuditResource: "RemediationPlan"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/remediations/:remediation_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: remediationAPI.execute, AuditAction: "remediation.execute", AuditResource: "RemediationPlan"})
					}
					if options.Promotion != nil {
						promotionAPI := promotionHandler{service: options.Promotion}
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/promotions", AuthRequired: true, Handler: promotionAPI.list})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/promotions/preview", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: promotionAPI.preview})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/promotions/:promotion_id", AuthRequired: true, Handler: promotionAPI.get})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/promotions/:promotion_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: promotionAPI.execute})
					}
					if options.Alert != nil {
						alertAPI := alertHandler{service: options.Alert, users: options.Auth}
						alertRuleRoutes := v1.Group("/clusters/:cluster_id/alert-rules", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(alertRuleRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: alertAPI.listRules})
						reg.register(alertRuleRoutes, RouteDescriptor{Method: "POST", Path: "", RequiredRoles: rolesSystemOpsAdmin, Handler: alertAPI.createRule, AuditAction: "alert_rule.create", AuditResource: "AlertRule"})
						reg.register(alertRuleRoutes, RouteDescriptor{Method: "GET", Path: "/:rule_id", Handler: alertAPI.getRule})
						reg.register(alertRuleRoutes, RouteDescriptor{Method: "PATCH", Path: "/:rule_id", RequiredRoles: rolesSystemOpsAdmin, Handler: alertAPI.patchRule, AuditAction: "alert_rule.update", AuditResource: "AlertRule"})
						reg.register(alertRuleRoutes, RouteDescriptor{Method: "DELETE", Path: "/:rule_id", RequiredRoles: rolesSystemOpsAdmin, Handler: alertAPI.deleteRule, AuditAction: "alert_rule.delete", AuditResource: "AlertRule"})
						alertRoutes := v1.Group("/clusters/:cluster_id/alerts", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(alertRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: alertAPI.listInstances})
						reg.register(alertRoutes, RouteDescriptor{Method: "GET", Path: "/:alert_id", Handler: alertAPI.getInstance})
					}
					if options.Backup != nil {
						backupAPI := backupHandler{service: options.Backup, kubernetes: options.Kubernetes}
						backupPlanRoutes := v1.Group("/clusters/:cluster_id/backup-plans", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(backupPlanRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: backupAPI.list})
						reg.register(backupPlanRoutes, RouteDescriptor{Method: "POST", Path: "/preview", RequiredRoles: rolesSystemOpsAdmin, Handler: backupAPI.preview, AuditAction: "backup.preview", AuditResource: "BackupPlan"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/backup-plans/:plan_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: backupAPI.execute, AuditAction: "backup.execute", AuditResource: "BackupPlan"})
						// M58: live Velero Backup CRs (read-only).
						veleroBackupRoutes := v1.Group("/clusters/:cluster_id/velero/backups", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(veleroBackupRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: backupAPI.listBackups})
						reg.register(veleroBackupRoutes, RouteDescriptor{Method: "GET", Path: "/:namespace/:name", Handler: backupAPI.getBackup})
					}
					if options.Maintenance != nil {
						maintAPI := maintenanceHandler{service: options.Maintenance}
						maintRoutes := v1.Group("/clusters/:cluster_id/maintenance-plans", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(maintRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: maintAPI.list})
						reg.register(maintRoutes, RouteDescriptor{Method: "POST", Path: "/preview", RequiredRoles: rolesSystemOpsAdmin, Handler: maintAPI.preview, AuditAction: "maintenance.preview", AuditResource: "MaintenancePlan"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/maintenance-plans/:plan_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: maintAPI.execute, AuditAction: "maintenance.execute", AuditResource: "MaintenancePlan"})
					}
					if options.Restore != nil {
						restoreAPI := restoreHandler{service: options.Restore, kubernetes: options.Kubernetes}
						restoreRoutes := v1.Group("/clusters/:cluster_id/restore-plans", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(restoreRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: restoreAPI.list})
						reg.register(restoreRoutes, RouteDescriptor{Method: "POST", Path: "/preview", RequiredRoles: rolesSystemOpsAdmin, Handler: restoreAPI.preview, AuditAction: "restore.preview", AuditResource: "RestorePlan"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/restore-plans/:plan_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: restoreAPI.execute, AuditAction: "restore.execute", AuditResource: "RestorePlan"})
						// M58: live Velero Restore CRs (read-only).
						veleroRestoreRoutes := v1.Group("/clusters/:cluster_id/velero/restores", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(veleroRestoreRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: restoreAPI.listRestores})
						reg.register(veleroRestoreRoutes, RouteDescriptor{Method: "GET", Path: "/:namespace/:name", Handler: restoreAPI.getRestore})
					}
					// M58 GitOps (ArgoCD Application read-only) browse routes.
					if options.GitOpsService != nil {
						gitopsAPI := gitopsHandler{service: options.GitOpsService}
						gitopsRoutes := v1.Group("/clusters/:cluster_id/gitops", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(gitopsRoutes, RouteDescriptor{Method: "GET", Path: "/capability", Handler: gitopsAPI.capability})
						reg.register(gitopsRoutes, RouteDescriptor{Method: "GET", Path: "/applications", Handler: gitopsAPI.list})
						reg.register(gitopsRoutes, RouteDescriptor{Method: "GET", Path: "/applications/:name", Handler: gitopsAPI.get})
					}
					// M58 interactive cross-cluster copy routes.
					if options.CopyOpsService != nil {
						copyopsAPI := copyopsHandler{service: options.CopyOpsService}
						// Preview accepts either source_cluster_id in body or picks
						// up the URL param from the per-cluster route group.
						copyopsClusterRoutes := v1.Group("/clusters/:cluster_id/copy-plans", withAuthentication(options.Auth), withClusterContext(), requireClusterAccess(options.Authz))
						reg.register(copyopsClusterRoutes, RouteDescriptor{Method: "POST", Path: "/preview", RequiredRoles: rolesSystemOpsAdmin, Handler: copyopsAPI.preview, AuditAction: "copyops.preview", AuditResource: "CopyPlan"})
						reg.register(copyopsClusterRoutes, RouteDescriptor{Method: "GET", Path: "", Handler: copyopsAPI.listByCluster})
						// Plan-level routes are global (plan ID in URL, no cluster in URL)
						// so execute + get don't require cluster context in the URL.
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/copy-plans/:plan_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: copyopsAPI.execute, AuditAction: "copyops.execute", AuditResource: "CopyPlan"})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/copy-plans/:plan_id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: copyopsAPI.get})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/copy-plans", AuthRequired: true, Handler: copyopsAPI.listCurrentUser})
					}
					if options.AIExplanation != nil {
						aiAPI := aiExplanationHandler{service: options.AIExplanation}
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/ai/status", AuthRequired: true, Handler: aiAPI.status})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/ai/quality", AuthRequired: true, Handler: aiAPI.quality})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/ai/explanations/:explanation_id/feedback", AuthRequired: true, Handler: aiAPI.feedback, AuditAction: "ai_explanation.feedback.create", AuditResource: "AIExplanationFeedback"})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/diagnoses/:diagnosis_id/explanations", AuthRequired: true, Handler: aiAPI.list})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/diagnoses/:diagnosis_id/explanations", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: aiAPI.generate, AuditAction: "diagnosis.ai_explanation.create", AuditResource: "DiagnosisAIExplanation"})
					}
					// M98 incident workspace: collaborative wrapper around a
					// diagnosis or client-observed finding with a stable number,
					// assignee, followers, timeline, status machine and a
					// read-only postmortem view.
					if options.Incidents != nil {
						incidentAPI := incidentHandler{service: options.Incidents}
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/incidents", AuthRequired: true, Handler: incidentAPI.list})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/incidents/summary", AuthRequired: true, Handler: incidentAPI.summary})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/incidents/:incident_id", AuthRequired: true, Handler: incidentAPI.get})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/incidents/:incident_id/evidence", AuthRequired: true, Handler: incidentAPI.evidence, AuditAction: "incident.evidence.get", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "GET", Path: "/incidents/:incident_id/export", AuthRequired: true, Handler: incidentAPI.export, AuditAction: "incident.export", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/incidents", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.create, AuditAction: "incident.create", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/incidents/batch-assign", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.batchAssign, AuditAction: "incident.assignment.batch", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "PATCH", Path: "/incidents/:incident_id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.transition, AuditAction: "incident.status.update", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "PATCH", Path: "/incidents/:incident_id/assignment", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.assign, AuditAction: "incident.assignment.update", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/incidents/:incident_id/followers", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.addFollower, AuditAction: "incident.follower.add", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "DELETE", Path: "/incidents/:incident_id/followers/:user_id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.removeFollower, AuditAction: "incident.follower.remove", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "POST", Path: "/incidents/:incident_id/notes", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.addNote, AuditAction: "incident.note.create", AuditResource: "Incident"})
						reg.register(v1, RouteDescriptor{Method: "PUT", Path: "/incidents/:incident_id/postmortem", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: incidentAPI.setPostmortem, AuditAction: "incident.postmortem.update", AuditResource: "Incident"})
					}
				}
			}
		}
	}

	// M37A capability adapters: fixed-template SLI metrics and read-only
	// historical logs. Routes are registered only when at least one provider
	// is configured; an unset provider returns 503 from its endpoint.
	// M60 adds the compile-time provider catalog surface (/providers) when
	// the CapabilityRegistry is non-nil. The registry surface carries its
	// own route descriptor and can exist independently of metrics/logs.
	if options.CapabilityMetricsProvider != nil || options.CapabilityLogProvider != nil || options.CapabilityRegistry != nil {
		capabilityAPI := capabilityHandler{
			metricsProvider: options.CapabilityMetricsProvider,
			logProvider:     options.CapabilityLogProvider,
			registry:        options.CapabilityRegistry,
		}
		capabilityRoutes := v1.Group("/capability", withAuthentication(options.Auth))
		if options.CapabilityMetricsProvider != nil {
			reg.register(capabilityRoutes, RouteDescriptor{Method: "GET", Path: "/metrics", AuthRequired: true, Handler: capabilityAPI.queryMetrics, AuditAction: "capability.metrics.query", AuditResource: "Metrics"})
		}
		if options.CapabilityLogProvider != nil {
			reg.register(capabilityRoutes, RouteDescriptor{Method: "POST", Path: "/logs", AuthRequired: true, Handler: capabilityAPI.queryLogs, AuditAction: "capability.logs.query", AuditResource: "Logs"})
		}
		if options.CapabilityRegistry != nil {
			reg.register(capabilityRoutes, RouteDescriptor{Method: "GET", Path: "/providers", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: capabilityAPI.listProviders, AuditAction: "capability.providers.list", AuditResource: "Provider"})
			reg.register(capabilityRoutes, RouteDescriptor{Method: "GET", Path: "/providers/:name", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: capabilityAPI.getProvider, AuditAction: "capability.providers.get", AuditResource: "Provider"})
		}
	}

	// M64 optimization analyzers (M61 FinOps right-sizing, M62 CIS posture,
	// M63 deprecated-API check, M67 network reachability). The endpoints are
	// read-only: the caller supplies an already-collected observation bundle
	// — or lets the M65 collector gather one — and the server returns findings
	// (ADR 0004). Routes are registered only when the optimization service is
	// configured.
	if options.Optimization != nil {
		optAPI := optimizationHandler{svc: options.Optimization, posture: options.Posture}
		optRoutes := v1.Group("/optimization")
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/cis/analyze", AuthRequired: true, Handler: optAPI.cisAnalyze, AuditAction: "optimization.cis.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/finops/analyze", AuthRequired: true, Handler: optAPI.finopsAnalyze, AuditAction: "optimization.finops.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/deprecated-api/analyze", AuthRequired: true, Handler: optAPI.deprecatedAPIAnalyze, AuditAction: "optimization.deprecated_api.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/network/analyze", AuthRequired: true, Handler: optAPI.networkAnalyze, AuditAction: "optimization.network.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/image/analyze", AuthRequired: true, Handler: optAPI.imageAnalyze, AuditAction: "optimization.image.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/gitops/analyze", AuthRequired: true, Handler: optAPI.gitopsAnalyze, AuditAction: "optimization.gitops.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/capacity/analyze", AuthRequired: true, Handler: optAPI.capacityAnalyze, AuditAction: "optimization.capacity.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/policy/analyze", AuthRequired: true, Handler: optAPI.policyAnalyze, AuditAction: "optimization.policy.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/hpa/analyze", AuthRequired: true, Handler: optAPI.hpaAnalyze, AuditAction: "optimization.hpa.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/pdb/analyze", AuthRequired: true, Handler: optAPI.pdbAnalyze, AuditAction: "optimization.pdb.analyze", AuditResource: "Cluster"})
		reg.register(optRoutes, RouteDescriptor{Method: "POST", Path: "/ingress/analyze", AuthRequired: true, Handler: optAPI.ingressAnalyze, AuditAction: "optimization.ingress.analyze", AuditResource: "Cluster"})
		// M80 unified governance posture: aggregates all M61-M78 analyzers.
		if options.Posture != nil {
			postureRoutes := optRoutes.Group("/posture")
			reg.register(postureRoutes, RouteDescriptor{Method: "GET", Path: "/cluster", AuthRequired: true, Handler: optAPI.postureReport, AuditAction: "posture.cluster.report", AuditResource: "Cluster"})
		}
	}

	// M37B alert routes: webhook receivers, exact-match routes, bounded
	// silences and delivery records. Routes are registered only when the
	// alert-route service is configured.
	if options.AlertRouteService != nil {
		alertrouteAPI := alertrouteHandler{service: options.AlertRouteService}
		alertrouteRoutes := v1.Group("/alert-routes", withAuthentication(options.Auth))
		// Receivers
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "GET", Path: "/receivers", AuthRequired: true, Handler: alertrouteAPI.listReceivers})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "POST", Path: "/receivers", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.createReceiver, AuditAction: "alert_route.receiver.create", AuditResource: "AlertReceiver"})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "DELETE", Path: "/receivers/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.deleteReceiver, AuditAction: "alert_route.receiver.delete", AuditResource: "AlertReceiver"})
		// Routes
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "GET", Path: "", AuthRequired: true, Handler: alertrouteAPI.listRoutes})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "POST", Path: "", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.createRoute, AuditAction: "alert_route.route.create", AuditResource: "AlertRoute"})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "PATCH", Path: "/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.updateRoute, AuditAction: "alert_route.route.update", AuditResource: "AlertRoute"})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "DELETE", Path: "/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.deleteRoute, AuditAction: "alert_route.route.delete", AuditResource: "AlertRoute"})
		// Silences
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "GET", Path: "/silences", AuthRequired: true, Handler: alertrouteAPI.listSilences})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "POST", Path: "/silences", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.createSilence, AuditAction: "alert_route.silence.create", AuditResource: "AlertSilence"})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "DELETE", Path: "/silences/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.deleteSilence, AuditAction: "alert_route.silence.delete", AuditResource: "AlertSilence"})
		// Inhibits (M51)
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "GET", Path: "/inhibits", AuthRequired: true, Handler: alertrouteAPI.listInhibits})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "POST", Path: "/inhibits", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.createInhibit, AuditAction: "alert_route.inhibit.create", AuditResource: "AlertInhibit"})
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "DELETE", Path: "/inhibits/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: alertrouteAPI.deleteInhibit, AuditAction: "alert_route.inhibit.delete", AuditResource: "AlertInhibit"})
		// Deliveries
		reg.register(alertrouteRoutes, RouteDescriptor{Method: "GET", Path: "/deliveries", AuthRequired: true, RequiredRoles: rolesSystemSecurityAudit, Handler: alertrouteAPI.listDeliveries})
	}

	// M39–M56 AIOps routes. The /aiops group is always created; each
	// sub-service gates its own routes via options.XxxService (see the
	// per-service if blocks below). Previously the entire group was nested
	// under `if options.SignalService != nil`, which silently dropped
	// inspection/golden/automation/etc. whenever the signal service was
	// unwired.
	// M100: the aiops group validates cluster_id/namespace query parameters
	// against the caller's grants (404 on denial, anti-leakage) so signals,
	// SLOs and correlation cases cannot be probed by cluster ID.
	aiopsRoutes := v1.Group("/aiops", withAuthentication(options.Auth), requireClusterQueryAccess(options.Authz))

	// M39 AIOps signal model
	if options.SignalService != nil {
		signalAPI := signalHandler{service: options.SignalService, sources: options.SignalSourceReader}
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/overview", AuthRequired: true, Handler: signalAPI.overview, AuditAction: "aiops.overview.read", AuditResource: "AIOpsOverview"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/signals", AuthRequired: true, Handler: signalAPI.listSignals, AuditAction: "aiops.signals.list", AuditResource: "Signal"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/signals/catalog", AuthRequired: true, Handler: signalAPI.listSignalCatalog})
	}

	// M40 temporal topology and change timeline
	if options.TopologyService != nil {
		topologyAPI := topologyHandler{service: options.TopologyService}
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/topology/graph", AuthRequired: true, Handler: topologyAPI.getTopologyGraph, AuditAction: "aiops.topology.graph.read", AuditResource: "TopologyGraph"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/topology/changes", AuthRequired: true, Handler: topologyAPI.listChangeEvents, AuditAction: "aiops.topology.changes.list", AuditResource: "ChangeEvent"})
	}

	// M41 SLO, error budget and impact
	if options.SLOService != nil {
		sloAPI := sloHandler{service: options.SLOService}
		// Templates catalog is a read-only public contract.
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/slos/templates", AuthRequired: true, Handler: sloAPI.listSLITemplates, AuditAction: "aiops.slo.templates.list", AuditResource: "SLITemplate"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/slos", AuthRequired: true, Handler: sloAPI.listSLODefinitions, AuditAction: "aiops.slo.definitions.list", AuditResource: "SLODefinition"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/slos", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: sloAPI.createSLODefinition, AuditAction: "aiops.slo.definitions.create", AuditResource: "SLODefinition"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/slos/:id", AuthRequired: true, Handler: sloAPI.getSLODefinition, AuditAction: "aiops.slo.definitions.read", AuditResource: "SLODefinition"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "PATCH", Path: "/slos/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: sloAPI.patchSLODefinition, AuditAction: "aiops.slo.definitions.update", AuditResource: "SLODefinition"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "DELETE", Path: "/slos/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: sloAPI.deleteSLODefinition, AuditAction: "aiops.slo.definitions.delete", AuditResource: "SLODefinition"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/slos/:id/evaluate", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: sloAPI.evaluateSLO, AuditAction: "aiops.slo.evaluate", AuditResource: "SLOEvaluation"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/slos/:id/evaluations", AuthRequired: true, Handler: sloAPI.listSLOEvaluations, AuditAction: "aiops.slo.evaluations.list", AuditResource: "SLOEvaluation"})
	}

	// M42 multi-signal correlation and deterministic RCA
	if options.CorrelationService != nil {
		correlationAPI := correlationHandler{service: options.CorrelationService}
		if options.Incidents != nil {
			correlationAPI.incidentBySource = func(ctx context.Context, sourceRef string) (*incident.Incident, error) {
				rec, err := options.Incidents.FindBySource(ctx, incident.SourceTypeCorrelation, sourceRef)
				if err != nil {
					return nil, err
				}
				return &rec, nil
			}
		}
		// Rule catalog is a read-only public contract.
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/correlation/rules", AuthRequired: true, Handler: correlationAPI.listCorrelationRules, AuditAction: "aiops.correlation.rules.list", AuditResource: "CorrelationRule"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/correlation/cases", AuthRequired: true, Handler: correlationAPI.listCorrelationCases, AuditAction: "aiops.correlation.cases.list", AuditResource: "CorrelationCase"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/correlation/cases/timeline", AuthRequired: true, Handler: correlationAPI.listCorrelationTimeline, AuditAction: "aiops.correlation.timeline.list", AuditResource: "CorrelationCase"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/correlation/cases/:id", AuthRequired: true, Handler: correlationAPI.getCorrelationCase, AuditAction: "aiops.correlation.cases.read", AuditResource: "CorrelationCase"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/correlation/cases/:id/graph", AuthRequired: true, Handler: correlationAPI.getCorrelationCaseGraph, AuditAction: "aiops.correlation.graph.read", AuditResource: "CorrelationCase"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/correlation/cases/:id/actions", AuthRequired: true, Handler: correlationAPI.listCorrelationActions, AuditAction: "aiops.correlation.actions.list", AuditResource: "ActionCandidate"})
	}

	// M43 cited and evaluated AI investigator
	if options.AIInvestigatorService != nil {
		investigatorAPI := aiInvestigatorHandler{service: options.AIInvestigatorService}
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/investigator/runbooks", AuthRequired: true, Handler: investigatorAPI.listRunbooks, AuditAction: "aiops.investigator.runbooks.list", AuditResource: "Runbook"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/investigator/cases/:case_id/investigations", AuthRequired: true, Handler: investigatorAPI.listInvestigations, AuditAction: "aiops.investigator.investigations.list", AuditResource: "Investigation"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/investigator/investigations/:id", AuthRequired: true, Handler: investigatorAPI.getInvestigation, AuditAction: "aiops.investigator.investigations.read", AuditResource: "Investigation"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/investigator/cases/:case_id/investigations", AuthRequired: true, Handler: investigatorAPI.generateInvestigation, AuditAction: "aiops.investigator.investigations.generate", AuditResource: "Investigation"})
	}

	// M44 policy-constrained automation and post-action verification
	if options.AutomationService != nil {
		automationAPI := automationHandler{service: options.AutomationService}
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/automation/runbooks", AuthRequired: true, Handler: automationAPI.listRunbooks, AuditAction: "aiops.automation.runbooks.list", AuditResource: "Runbook"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/automation/plans", AuthRequired: true, Handler: automationAPI.listPlans, AuditAction: "aiops.automation.plans.list", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/automation/plans", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: automationAPI.createPlan, AuditAction: "aiops.automation.plans.create", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/automation/plans/:plan_id", AuthRequired: true, Handler: automationAPI.getPlan, AuditAction: "aiops.automation.plans.read", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/automation/plans/:plan_id/preview", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: automationAPI.previewPlan, AuditAction: "aiops.automation.plans.preview", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/automation/plans/:plan_id/approve", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: automationAPI.approvePlan, AuditAction: "aiops.automation.plans.approve", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/automation/plans/:plan_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: automationAPI.executePlan, AuditAction: "aiops.automation.plans.execute", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/automation/plans/:plan_id/cancel", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: automationAPI.cancelPlan, AuditAction: "aiops.automation.plans.cancel", AuditResource: "ActionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/automation/plans/:plan_id/verify", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: automationAPI.verifyPlan, AuditAction: "aiops.automation.plans.verify", AuditResource: "ActionVerification"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/automation/plans/:plan_id/verification", AuthRequired: true, Handler: automationAPI.getVerification, AuditAction: "aiops.automation.verification.read", AuditResource: "ActionVerification"})
	}

	// M52 intelligent inspection (KubeEye-style compile-time rule catalog
	// + ad-hoc/scheduled runs). Catalog reads are any-auth; plan CRUD and
	// run-once trigger require operations_admin. Per-cluster effective
	// rules route is under /clusters/:cluster_id/inspection/rules
	// (cluster-scoped) so it inherits requireClusterAccess.
	if options.InspectionService != nil {
		inspAPI := inspectionHandler{service: options.InspectionService}
		// Global rule catalog (read-only contract)
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/rules/catalog", AuthRequired: true, Handler: inspAPI.listRules, AuditAction: "aiops.inspection.rules_catalog.read", AuditResource: "InspectionRuleCatalog"})
		// Inspection plans: list + create (ops_admin) + get + delete (ops_admin)
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/plans", AuthRequired: true, Handler: inspAPI.listPlans, AuditAction: "aiops.inspection.plans.list", AuditResource: "InspectionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/inspection/plans", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: inspAPI.createPlan, AuditAction: "aiops.inspection.plans.create", AuditResource: "InspectionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/plans/:id", AuthRequired: true, Handler: inspAPI.getPlan, AuditAction: "aiops.inspection.plans.read", AuditResource: "InspectionPlan"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "DELETE", Path: "/inspection/plans/:id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: inspAPI.deletePlan, AuditAction: "aiops.inspection.plans.delete", AuditResource: "InspectionPlan"})
		// Ad-hoc run-once: triggers background inspection; returns 202 Accepted with TaskView.
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/inspection/run", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: inspAPI.runOnce, AuditAction: "aiops.inspection.run_once", AuditResource: "InspectionTask"})
		// Tasks (execution records): list + detail
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/tasks", AuthRequired: true, Handler: inspAPI.listTasks, AuditAction: "aiops.inspection.tasks.list", AuditResource: "InspectionTask"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/tasks/:id", AuthRequired: true, Handler: inspAPI.getTask, AuditAction: "aiops.inspection.tasks.read", AuditResource: "InspectionTask"})
		// Results (findings normalized to M39 signal shape): list + detail
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/results", AuthRequired: true, Handler: inspAPI.listResults, AuditAction: "aiops.inspection.results.list", AuditResource: "InspectionResult"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/inspection/results/:id", AuthRequired: true, Handler: inspAPI.getResult, AuditAction: "aiops.inspection.results.read", AuditResource: "InspectionResult"})
	}

	// M56 golden quality-report. GET reads the latest report (any-auth);
	// POST triggers an async replay (SystemOpsAdmin only).
	if options.GoldenService != nil {
		goldenAPI := goldenHandler{service: options.GoldenService}
		reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/quality-report", AuthRequired: true, Handler: goldenAPI.getQualityReport, AuditAction: "aiops.quality_report.read", AuditResource: "QualityReport"})
		reg.register(aiopsRoutes, RouteDescriptor{Method: "POST", Path: "/quality-report/run", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: goldenAPI.runQualityReplay, AuditAction: "aiops.quality_report.run", AuditResource: "QualityReport"})
	}

	// M81 AIOps closed-loop runbook: maps a posture finding to its diagnosis,
	// corroborating inspection rules, AI explanation entry and dry-run
	// operation candidates. Pure read-only mapping (ADR 0004); no cluster access.
	insightAPI := insightHandler{}
	reg.register(aiopsRoutes, RouteDescriptor{Method: "GET", Path: "/insight", AuthRequired: true, Handler: insightAPI.runbook, AuditAction: "aiops.insight.runbook.read", AuditResource: "InsightRunbook"})

	// M57 Helm application catalog. Repository CRUD (SystemOpsAdmin for
	// write), chart listing/detail (any-auth, read-only index.yaml fetch),
	// and M19 controlled-operation deploy plans (SystemOpsAdmin for
	// preview/execute). Credentials are never returned in API responses.
	if options.AppCatalogService != nil {
		appCatalogAPI := appCatalogHandler{service: options.AppCatalogService}
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/app-catalog/repositories", AuthRequired: true, Handler: appCatalogAPI.listRepositories, AuditAction: "app_catalog.repositories.list", AuditResource: "HelmRepository"})
		reg.register(v1, RouteDescriptor{Method: "POST", Path: "/app-catalog/repositories", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: appCatalogAPI.createRepository, AuditAction: "app_catalog.repositories.create", AuditResource: "HelmRepository"})
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/app-catalog/repositories/:repo_id", AuthRequired: true, Handler: appCatalogAPI.getRepository, AuditAction: "app_catalog.repositories.read", AuditResource: "HelmRepository"})
		reg.register(v1, RouteDescriptor{Method: "DELETE", Path: "/app-catalog/repositories/:repo_id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: appCatalogAPI.deleteRepository, AuditAction: "app_catalog.repositories.delete", AuditResource: "HelmRepository"})
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/app-catalog/repositories/:repo_id/charts", AuthRequired: true, Handler: appCatalogAPI.listCharts, AuditAction: "app_catalog.charts.list", AuditResource: "Chart"})
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/app-catalog/repositories/:repo_id/charts/:chart_name", AuthRequired: true, Handler: appCatalogAPI.getChart, AuditAction: "app_catalog.charts.read", AuditResource: "Chart"})
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/app-catalog/plans", AuthRequired: true, Handler: appCatalogAPI.listPlans, AuditAction: "app_catalog.plans.list", AuditResource: "AppCatalogPlan"})
		reg.register(v1, RouteDescriptor{Method: "POST", Path: "/app-catalog/plans/preview", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: appCatalogAPI.previewDeploy, AuditAction: "app_catalog.plans.preview", AuditResource: "AppCatalogPlan"})
		reg.register(v1, RouteDescriptor{Method: "GET", Path: "/app-catalog/plans/:plan_id", AuthRequired: true, Handler: appCatalogAPI.getPlan, AuditAction: "app_catalog.plans.read", AuditResource: "AppCatalogPlan"})
		reg.register(v1, RouteDescriptor{Method: "POST", Path: "/app-catalog/plans/:plan_id/execute", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: appCatalogAPI.executeDeploy, AuditAction: "app_catalog.plans.execute", AuditResource: "AppCatalogPlan"})
	}

	// M46 workspace multi-tenancy. Routes are registered only when the
	// workspace service is configured. Authorization is enforced inside the
	// service (404 > 403 anti-leakage); the RequiredRoles on create/delete
	// is a defence-in-depth gate — the service re-checks SystemAdmin.
	if options.WorkspaceService != nil && options.Auth != nil {
		workspaceAPI := workspaceHandler{service: options.WorkspaceService}
		workspaceRoutes := v1.Group("/workspaces", withAuthentication(options.Auth))
		reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "", AuthRequired: true, Handler: workspaceAPI.listWorkspaces, AuditAction: "workspaces.list", AuditResource: "Workspace"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "POST", Path: "", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: workspaceAPI.createWorkspace, AuditAction: "workspaces.create", AuditResource: "Workspace"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "/:workspace_id", AuthRequired: true, Handler: workspaceAPI.getWorkspace, AuditAction: "workspaces.read", AuditResource: "Workspace"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "PATCH", Path: "/:workspace_id", AuthRequired: true, Handler: workspaceAPI.updateWorkspace, AuditAction: "workspaces.update", AuditResource: "Workspace"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "DELETE", Path: "/:workspace_id", AuthRequired: true, RequiredRoles: rolesSystemAdmin, Handler: workspaceAPI.deleteWorkspace, AuditAction: "workspaces.delete", AuditResource: "Workspace"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "/:workspace_id/memberships", AuthRequired: true, Handler: workspaceAPI.listMemberships, AuditAction: "workspaces.memberships.list", AuditResource: "WorkspaceMembership"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "POST", Path: "/:workspace_id/memberships", AuthRequired: true, Handler: workspaceAPI.addMembership, AuditAction: "workspaces.memberships.add", AuditResource: "WorkspaceMembership"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "DELETE", Path: "/:workspace_id/memberships", AuthRequired: true, Handler: workspaceAPI.removeMembership, AuditAction: "workspaces.memberships.remove", AuditResource: "WorkspaceMembership"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "/:workspace_id/quota", AuthRequired: true, Handler: workspaceAPI.getQuota, AuditAction: "workspaces.quota.read", AuditResource: "WorkspaceQuota"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "PUT", Path: "/:workspace_id/quota", AuthRequired: true, Handler: workspaceAPI.setQuota, AuditAction: "workspaces.quota.set", AuditResource: "WorkspaceQuota"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "/:workspace_id/role-bindings", AuthRequired: true, Handler: workspaceAPI.listRoleBindings, AuditAction: "workspaces.role_bindings.list", AuditResource: "UserWorkspaceGrant"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "POST", Path: "/:workspace_id/role-bindings", AuthRequired: true, Handler: workspaceAPI.grantRole, AuditAction: "workspaces.role_bindings.grant", AuditResource: "UserWorkspaceGrant"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "DELETE", Path: "/:workspace_id/role-bindings/:user_id", AuthRequired: true, Handler: workspaceAPI.revokeRole, AuditAction: "workspaces.role_bindings.revoke", AuditResource: "UserWorkspaceGrant"})
		reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "/:workspace_id/role-bindings/audit", AuthRequired: true, Handler: workspaceAPI.listRoleBindingsAudit, AuditAction: "workspaces.role_bindings.audit.list", AuditResource: "WorkspaceRoleBindingAudit"})
		// M50 workspace-level cross-cluster monitoring dashboard. The
		// monitoring service enforces workspace_viewer via ListMemberships
		// (404 > 403 anti-leakage). Registered only when both the workspace
		// and monitoring services are configured.
		if options.Monitoring != nil {
			monitoringAPI := monitoringHandler{service: options.Monitoring}
			reg.register(workspaceRoutes, RouteDescriptor{Method: "GET", Path: "/:workspace_id/monitoring/dashboard", AuthRequired: true, Handler: monitoringAPI.workspaceDashboard, AuditAction: "monitoring.dashboard.read", AuditResource: "MonitoringDashboard"})
		}
	}

	// M48 multi-cluster federation (host/member model). Routes are registered
	// only when the federation service is configured. Write operations
	// (register / deregister / promote / demote / heartbeat / status update)
	// require rolesSystemOpsAdmin; reads (overview / events / resource
	// summary / per-cluster events) are authentication-only. Anti-leakage
	// (404 > 403) is preserved by the handler.
	if options.FederationService != nil && options.Auth != nil {
		federationAPI := federationHandler{service: options.FederationService, authz: options.Authz}
		federationRoutes := v1.Group("/federation", withAuthentication(options.Auth))
		reg.register(federationRoutes, RouteDescriptor{Method: "GET", Path: "/overview", AuthRequired: true, Handler: federationAPI.overview, AuditAction: "federation.overview.read", AuditResource: "FederationOverview"})
		reg.register(federationRoutes, RouteDescriptor{Method: "GET", Path: "/events", AuthRequired: true, Handler: federationAPI.listEvents, AuditAction: "federation.events.list", AuditResource: "FederationEvent"})
		reg.register(federationRoutes, RouteDescriptor{Method: "GET", Path: "/resources/summary", AuthRequired: true, Handler: federationAPI.resourceSummary, AuditAction: "federation.resources.summary.read", AuditResource: "FederationResourceSummary"})
		reg.register(federationRoutes, RouteDescriptor{Method: "POST", Path: "/clusters/register", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: federationAPI.registerCluster, AuditAction: "federation.cluster.register", AuditResource: "FederationCluster"})
		reg.register(federationRoutes, RouteDescriptor{Method: "DELETE", Path: "/clusters/:cluster_id", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: federationAPI.deregisterCluster, AuditAction: "federation.cluster.deregister", AuditResource: "FederationCluster"})
		reg.register(federationRoutes, RouteDescriptor{Method: "POST", Path: "/clusters/:cluster_id/promote", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: federationAPI.promoteCluster, AuditAction: "federation.cluster.promote", AuditResource: "FederationCluster"})
		reg.register(federationRoutes, RouteDescriptor{Method: "POST", Path: "/clusters/:cluster_id/demote", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: federationAPI.demoteCluster, AuditAction: "federation.cluster.demote", AuditResource: "FederationCluster"})
		reg.register(federationRoutes, RouteDescriptor{Method: "POST", Path: "/clusters/:cluster_id/heartbeat", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: federationAPI.heartbeat, AuditAction: "federation.cluster.heartbeat", AuditResource: "FederationCluster"})
		reg.register(federationRoutes, RouteDescriptor{Method: "PATCH", Path: "/clusters/:cluster_id/status", AuthRequired: true, RequiredRoles: rolesSystemOpsAdmin, Handler: federationAPI.updateStatus, AuditAction: "federation.cluster.status.update", AuditResource: "FederationCluster"})
		reg.register(federationRoutes, RouteDescriptor{Method: "GET", Path: "/clusters/:cluster_id/events", AuthRequired: true, Handler: federationAPI.listClusterEvents, AuditAction: "federation.cluster.events.list", AuditResource: "FederationEvent"})
	}

	router.NoRoute(noRoute)

	return router
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		logger.Info("http request",
			zap.String("request_id", requestctx.RequestIDFrom(c.Request.Context())),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Int("response_size", c.Writer.Size()),
			zap.Duration("duration", time.Since(startedAt)),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}
