package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/globalsearch"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/requestctx"
)

type Options struct {
	Probe          readinessProbe
	Auth           *auth.Service
	Clusters       *cluster.Service
	Kubernetes     *k8sgateway.Service
	Diagnosis      *diagnosis.Service
	Audit          *audit.Service
	AIExplanation  *aiexplain.Service
	SecureCookies  bool
	RefreshTTL     time.Duration
	Version        string
	Metrics        *Metrics
	Notifications  *notification.Service
	Remediation    *remediation.Service
	Fleet          *fleet.Service
	GlobalSearch   *globalsearch.Service
	SavedFilters   *globalsearch.SavedFilterService
	MetricsHistory *metricshistory.Service
}

func New(logger *zap.Logger, options Options) http.Handler {
	gin.SetMode(gin.ReleaseMode)
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

	v1 := router.Group("/api/v1")
	{
		v1.GET("/health/live", health.live)
		v1.GET("/health/ready", health.ready)
		if options.Auth != nil {
			if options.MetricsHistory != nil {
				historyAPI := metricsHistoryHandler{service: options.MetricsHistory}
				v1.GET("/clusters/:cluster_id/metrics/history", withAuthentication(options.Auth), withClusterContext(), historyAPI.series)
			}
			if options.Fleet != nil {
				fleetAPI := fleetHandler{service: options.Fleet}
				v1.GET("/fleet/health", withAuthentication(options.Auth), fleetAPI.health)
			}
			if options.GlobalSearch != nil {
				searchAPI := globalSearchHandler{service: options.GlobalSearch}
				v1.GET("/fleet/resources/search", withAuthentication(options.Auth), searchAPI.search)
			}
			if options.SavedFilters != nil {
				filtersAPI := savedGlobalSearchFilterHandler{service: options.SavedFilters}
				filterRoutes := v1.Group("/fleet/resources/search/filters", withAuthentication(options.Auth))
				filterRoutes.GET("", filtersAPI.list)
				filterRoutes.POST("", filtersAPI.create)
				filterRoutes.PATCH("/:filter_id", filtersAPI.update)
				filterRoutes.DELETE("/:filter_id", filtersAPI.delete)
			}
			authRoutes := v1.Group("/auth")
			authRoutes.POST("/login", authAPI.login)
			authRoutes.POST("/refresh", authAPI.refresh)
			authRoutes.POST("/logout", authAPI.logout)
			authRoutes.GET("/me", withAuthentication(options.Auth), authAPI.me)
			authRoutes.POST("/password-change", withAuthentication(options.Auth), authAPI.changePassword)
			authRoutes.GET("/sessions", withAuthentication(options.Auth), authAPI.sessions)
			authRoutes.DELETE("/sessions/:session_id", withAuthentication(options.Auth), authAPI.revokeSession)
			authRoutes.POST("/sessions/revoke-others", withAuthentication(options.Auth), authAPI.revokeOtherSessions)
			usersAPI := userHandler{service: options.Auth}
			v1.GET("/users/assignable", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), usersAPI.assignable)
			v1.GET("/users", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin), usersAPI.list)
			v1.POST("/users", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin), usersAPI.create)
			v1.PATCH("/users/:user_id", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin), usersAPI.update)
			v1.POST("/users/:user_id/password-reset", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin), usersAPI.resetPassword)
			if options.Audit != nil {
				auditAPI := auditHandler{service: options.Audit}
				v1.GET("/audit-logs", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.SecurityAuditor), auditAPI.list)
				v1.GET("/audit-logs/export", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.SecurityAuditor), auditAPI.export)
			}
			if options.Notifications != nil {
				notificationsAPI := notificationHandler{service: options.Notifications}
				v1.GET("/notification-deliveries", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.SecurityAuditor), notificationsAPI.list)
				v1.POST("/notification-deliveries/:delivery_id/retry", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin), notificationsAPI.retry)
			}
			if options.Clusters != nil {
				clustersAPI := clusterHandler{service: options.Clusters}
				clusterRoutes := v1.Group("/clusters", withAuthentication(options.Auth))
				clusterRoutes.GET("", clustersAPI.list)
				clusterRoutes.GET("/:cluster_id", clustersAPI.get)
				clusterRoutes.POST("", requireRoles(auth.SystemAdmin), clustersAPI.create)
				clusterRoutes.PATCH("/:cluster_id", requireRoles(auth.SystemAdmin), clustersAPI.setEnabled)
				clusterRoutes.PUT("/:cluster_id/credentials", requireRoles(auth.SystemAdmin), clustersAPI.updateCredential)
				clusterRoutes.POST("/:cluster_id/probe", requireRoles(auth.SystemAdmin, auth.OperationsAdmin), clustersAPI.probe)
				clusterRoutes.DELETE("/:cluster_id", requireRoles(auth.SystemAdmin), clustersAPI.delete)
			}
			if options.Kubernetes != nil {
				resourcesAPI := kubernetesHandler{service: options.Kubernetes}
				resourceRoutes := v1.Group("/clusters/:cluster_id", withAuthentication(options.Auth), withClusterContext())
				resourceRoutes.GET("/namespaces", resourcesAPI.namespaces)
				resourceRoutes.GET("/nodes", resourcesAPI.nodes)
				resourceRoutes.GET("/metrics/nodes", resourcesAPI.nodeMetrics)
				resourceRoutes.GET("/metrics/pods", resourcesAPI.podMetrics)
				resourceRoutes.GET("/nodes/:name", resourcesAPI.node)
				resourceRoutes.GET("/pods", resourcesAPI.pods)
				resourceRoutes.GET("/pods/:namespace/:name", resourcesAPI.pod)
				resourceRoutes.GET("/pods/:namespace/:name/logs", resourcesAPI.logs)
				resourceRoutes.GET("/events", resourcesAPI.events)
				resourceRoutes.GET("/deployments", resourcesAPI.deployments)
				resourceRoutes.GET("/deployments/:namespace/:name", resourcesAPI.deployment)
				resourceRoutes.GET("/statefulsets", resourcesAPI.statefulSets)
				resourceRoutes.GET("/statefulsets/:namespace/:name", resourcesAPI.statefulSet)
				resourceRoutes.GET("/daemonsets", resourcesAPI.daemonSets)
				resourceRoutes.GET("/daemonsets/:namespace/:name", resourcesAPI.daemonSet)
				resourceRoutes.GET("/replicasets", resourcesAPI.replicaSets)
				resourceRoutes.GET("/replicasets/:namespace/:name", resourcesAPI.replicaSet)
				resourceRoutes.GET("/jobs", resourcesAPI.jobs)
				resourceRoutes.GET("/jobs/:namespace/:name", resourcesAPI.job)
				resourceRoutes.GET("/cronjobs", resourcesAPI.cronJobs)
				resourceRoutes.GET("/cronjobs/:namespace/:name", resourcesAPI.cronJob)
				resourceRoutes.GET("/horizontalpodautoscalers", resourcesAPI.horizontalPodAutoscalers)
				resourceRoutes.GET("/horizontalpodautoscalers/:namespace/:name", resourcesAPI.horizontalPodAutoscaler)
				resourceRoutes.GET("/resourcequotas", resourcesAPI.resourceQuotas)
				resourceRoutes.GET("/resourcequotas/:namespace/:name", resourcesAPI.resourceQuota)
				resourceRoutes.GET("/limitranges", resourcesAPI.limitRanges)
				resourceRoutes.GET("/limitranges/:namespace/:name", resourcesAPI.limitRange)
				resourceRoutes.GET("/secrets", resourcesAPI.secrets)
				resourceRoutes.GET("/secrets/:namespace/:name", resourcesAPI.secret)
				resourceRoutes.GET("/services", resourcesAPI.services)
				resourceRoutes.GET("/services/:namespace/:name", resourcesAPI.serviceDetail)
				resourceRoutes.GET("/ingresses", resourcesAPI.ingresses)
				resourceRoutes.GET("/ingresses/:namespace/:name", resourcesAPI.ingress)
				resourceRoutes.GET("/endpointslices", resourcesAPI.endpointSlices)
				resourceRoutes.GET("/persistentvolumeclaims", resourcesAPI.persistentVolumeClaims)
				resourceRoutes.GET("/persistentvolumeclaims/:namespace/:name", resourcesAPI.persistentVolumeClaim)
				resourceRoutes.GET("/storageclasses", resourcesAPI.storageClasses)
				resourceRoutes.GET("/storageclasses/:name", resourcesAPI.storageClass)
				resourceRoutes.GET("/configmaps", resourcesAPI.configMaps)
				resourceRoutes.GET("/configmaps/:namespace/:name", resourcesAPI.configMap)
				if options.Diagnosis != nil {
					diagnosisAPI := diagnosisHandler{service: options.Diagnosis, users: options.Auth}
					resourceRoutes.POST("/diagnoses", diagnosisAPI.create)
					v1.GET("/diagnoses", withAuthentication(options.Auth), diagnosisAPI.list)
					v1.GET("/diagnoses/summary", withAuthentication(options.Auth), diagnosisAPI.summary)
					v1.GET("/diagnoses/:diagnosis_id", withAuthentication(options.Auth), diagnosisAPI.get)
					v1.PATCH("/diagnoses/:diagnosis_id", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), diagnosisAPI.transition)
					v1.POST("/diagnoses/:diagnosis_id/feedback", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), diagnosisAPI.feedback)
					v1.PATCH("/diagnoses/:diagnosis_id/assignment", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), diagnosisAPI.assign)
					if options.Remediation != nil {
						remediationAPI := remediationHandler{service: options.Remediation}
						resourceRoutes.GET("/operations", remediationAPI.listOperations)
						resourceRoutes.POST("/operations/preview", requireRoles(auth.SystemAdmin, auth.OperationsAdmin), remediationAPI.previewOperation)
						v1.GET("/diagnoses/:diagnosis_id/remediations", withAuthentication(options.Auth), remediationAPI.list)
						v1.POST("/diagnoses/:diagnosis_id/remediations/preview", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), remediationAPI.preview)
						v1.POST("/remediations/:remediation_id/execute", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), remediationAPI.execute)
					}
					if options.AIExplanation != nil {
						aiAPI := aiExplanationHandler{service: options.AIExplanation}
						v1.GET("/ai/status", withAuthentication(options.Auth), aiAPI.status)
						v1.GET("/ai/quality", withAuthentication(options.Auth), aiAPI.quality)
						v1.POST("/ai/explanations/:explanation_id/feedback", withAuthentication(options.Auth), aiAPI.feedback)
						v1.GET("/diagnoses/:diagnosis_id/explanations", withAuthentication(options.Auth), aiAPI.list)
						v1.POST("/diagnoses/:diagnosis_id/explanations", withAuthentication(options.Auth), requireRoles(auth.SystemAdmin, auth.OperationsAdmin), aiAPI.generate)
					}
				}
			}
		}
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
