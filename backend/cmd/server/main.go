package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/appcatalog"
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/backup"
	"k8s-aiops.local/backend/internal/buildinfo"
	"k8s-aiops.local/backend/internal/capability"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/config"
	"k8s-aiops.local/backend/internal/copyops"
	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/eventstream"
	"k8s-aiops.local/backend/internal/federation"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/gitops"
	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/golden"
	"k8s-aiops.local/backend/internal/httpserver"
	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/inspection"
	"k8s-aiops.local/backend/internal/knowledge"
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
	"k8s-aiops.local/backend/internal/restore"
	"k8s-aiops.local/backend/internal/servicemesh"
	signalsvc "k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/slo"
	"k8s-aiops.local/backend/internal/store"
	"k8s-aiops.local/backend/internal/topology"
	"k8s-aiops.local/backend/internal/workspace"
	"k8s-aiops.local/backend/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, err := newLogger(cfg.Environment)
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	database, err := store.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("open database", zap.Error(err))
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("close database", zap.Error(err))
		}
	}()
	if err := migrations.Apply(context.Background(), database.SQL()); err != nil {
		logger.Fatal("apply database migrations", zap.Error(err))
	}

	authRepository := auth.NewGormRepository(database.GORM())
	authService := auth.NewService(
		authRepository,
		auth.NewPasswordHasher(),
		auth.NewTokenManager(cfg.JWTSigningKey, cfg.AccessTokenTTL),
		cfg.RefreshTokenTTL,
	)
	created, err := authService.BootstrapAdmin(context.Background(), cfg.BootstrapUsername, cfg.BootstrapPassword)
	if err != nil {
		logger.Fatal("bootstrap administrator", zap.Error(err))
	}
	if created {
		logger.Warn("bootstrap administrator created; change the default password before production",
			zap.String("username", cfg.BootstrapUsername))
	}
	credentialEncryptor, err := cluster.NewEncryptor(cfg.CredentialEncryptionKey, cfg.CredentialKeyVersion, cfg.CredentialDecryptionKeys)
	if err != nil {
		logger.Fatal("configure credential encryption", zap.Error(err))
	}
	clusterRegistry := cluster.NewClientProvider(cfg.ClusterProbeTimeout)
	clusterService := cluster.NewService(cluster.NewGormRepository(database.GORM()), credentialEncryptor, clusterRegistry)
	kubernetesService := k8sgateway.NewService(clusterService, clusterRegistry, clusterRegistry)
	metricsHistoryService, err := metricshistory.NewService(metricshistory.Config{Retention: cfg.MetricsHistoryRetention, DownsampleRetention: 30 * 24 * time.Hour, MaxArchiveQueryWindow: 30 * 24 * time.Hour}, metricshistory.NewGormRepository(database.GORM()))
	if err != nil {
		logger.Fatal("configure metrics history", zap.Error(err))
	}
	// M80 unified governance posture: the single read-only collector shared by
	// the optimization analyzers and the aggregated posture endpoint.
	optimizationCollector := optimization.NewCollector(
		optimization.NewKubernetesLister(clusterRegistry, clusterService),
		optimization.NewMetricsHistorySource(metricsHistoryService, 24*time.Hour),
		optimization.NewNodeUsageSource(metricsHistoryService, 24*time.Hour),
	)
	metricsCollector, err := metricshistory.NewCollector(metricshistory.CollectorConfig{
		Enabled: cfg.MetricsHistoryEnabled, CollectionInterval: cfg.MetricsCollectionInterval,
		PerClusterTimeout: cfg.MetricsCollectionTimeout, CleanupInterval: cfg.MetricsCleanupInterval,
		MaxClusters: cfg.MetricsMaxClusters, MaxConcurrentClusters: cfg.MetricsMaxConcurrency, MaxSamples: 1800,
	}, clusterService, kubernetesService, metricsHistoryService, logger,
		// M99-B: record per-Deployment readiness gauges for the SLO
		// workload_readiness source.
		metricshistory.WithWorkloadReadinessSource(kubernetesService))
	if err != nil {
		logger.Fatal("configure metrics history collector", zap.Error(err))
	}
	fleetService := fleet.NewService(fleet.Config{MaxClusters: 20, MaxConcurrentClusters: 4, PerClusterTimeout: 4 * time.Second, ResourceSampleLimit: 100}, clusterService, kubernetesService)
	globalSearchService := globalsearch.NewService(globalsearch.Config{MaxClusters: 20, MaxConcurrentClusters: 4, PerClusterTimeout: 4 * time.Second, MaxResults: 100, PerKindLimit: 100}, clusterService, kubernetesService)
	savedFilterService := globalsearch.NewSavedFilterService(globalsearch.NewSavedFilterGormRepository(database.GORM()))
	diagnosisService := diagnosis.NewService(kubernetesService, diagnosis.NewGormRepository(database.GORM())).WithMetricEvaluator(metricsHistoryService)
	remediationService := remediation.NewService(diagnosisService, kubernetesService, remediation.NewGormRepository(database.GORM()))
	// M98 incident workspace: a collaborative wrapper around a diagnosis (or a
	// client-observed finding) with a stable number, assignee, followers,
	// timeline, status machine and a read-only postmortem view.
	alertService := alert.NewService(alert.NewGormRepository(database.GORM()), diagnosis.NewGormRepository(database.GORM()), metricsHistoryService, cfg.AlertMinEvaluationInterval)
	// M52: intelligent inspection (KubeEye-style compile-time rule catalog).
	// The rule executor uses the same read-only kubernetes gateway as M49/M51;
	// the cluster lister resolves reachable clusters for ad-hoc runs. Both are
	// wired as adapters so the inspection package stays free of kubernetes.
	inspectionService, err := inspection.NewService(
		inspection.Config{
			MaxConcurrentClusters: 4,
			PerClusterTimeout:     15 * time.Second,
			MaxTaskResults:        1000,
		},
		inspection.NewGormRepository(database.GORM()),
		inspection.NewDefaultExecutor(kubernetesService),
		inspectionClusterLister{svc: clusterService},
		logger,
	)
	if err != nil {
		logger.Fatal("configure inspection service", zap.Error(err))
	}
	// M39-M42 AIOps signal, SLO and correlation services. Wired into the
	// production server so the aiops routes (signals / slos / correlation)
	// are live; the SLO burn-alert sink normalizes burn transitions into
	// signal occurrences (M99). The evaluator reads workload readiness from
	// metrics history (M99-B); request-ratio templates report honest no-data
	// until a traffic metrics provider is configured.
	signalRepository := signalsvc.NewGormRepository(database.GORM())
	signalService := signalsvc.NewService(signalsvc.ServiceOptions{Repository: signalRepository})
	sloRepository := slo.NewGormRepository(database.GORM())
	sloService := slo.NewService(sloRepository,
		slo.NewEvaluator(slo.NewMetricshistorySource(metricsHistoryService)),
		slo.WithBurnAlertSink(signalsvc.NewSLOBurnSignalSink(signalService, sloRepository)))
	// M99-C: the production correlation provider reads signal/topology/
	// diagnosis repositories; the periodic worker correlates every enabled
	// cluster (per namespace, with an all-namespace fallback when the cluster
	// is unreachable).
	topologyRepository := topology.NewGormRepository(database.GORM())
	diagnosisRepository := diagnosis.NewGormRepository(database.GORM())
	correlationProvider := correlation.NewRepositoryInputProvider(signalRepository, topologyRepository, diagnosisRepository)
	correlationService := correlation.NewService(correlation.NewGormRepository(database.GORM()), nil, correlationProvider)
	correlationWorker := correlation.NewWorker(correlation.WorkerConfig{Interval: cfg.CorrelationInterval}, clusterService, kubernetesService, correlationService, logger)
	incidentSourceResolver := NewIncidentResolver(diagnosis.NewGormRepository(database.GORM()), alertService, inspectionService, signalService, correlationService)
	incidentRepository := incident.NewGormRepository(database.GORM())
	incidentService := incident.NewService(incidentRepository).
		WithSLADurations(cfg.IncidentSLATargets).
		WithResolver(incidentSourceResolver).
		WithEvidenceResolver(incidentSourceResolver)
	promotionService := promotion.NewService(kubernetesService, promotion.NewGormRepository(database.GORM()))
	appCatalogService := appcatalog.NewService(kubernetesService, appcatalog.NewGormRepository(database.GORM()))
	// M58: GitOps read-only adapter (ArgoCD Application browse) + interactive
	// cross-cluster copy service. Both are thin wrappers over kubernetes typed
	// readers (ADR 0070).
	gitopsService := gitops.NewService(kubernetesService)
	copyOpsService := copyops.NewService(kubernetesService, copyops.NewGormRepository(database.GORM()))
	backupService := backup.NewService(kubernetesService, backup.NewGormRepository(database.GORM()))
	maintenanceService := maintenance.NewService(kubernetesService, maintenance.NewGormRepository(database.GORM()))
	namespacePostureService := namespaceposture.NewService(kubernetesService)
	restoreService := restore.NewService(kubernetesService, restore.NewGormRepository(database.GORM()))
	// M105: diagnosis→signal drain. The M39 signal layer was previously
	// populated only by the SLO burn sink; this drain normalizes new/updated
	// diagnosis records into signal occurrences (producer=diagnosis) so the
	// overview, correlation engine and incident signal source see diagnosis
	// state. Watermark starts at boot; re-ingest is idempotent by fingerprint.
	diagnosisSignalDrain := signalsvc.NewDiagnosisDrain(
		signalsvc.DrainConfig{Interval: cfg.Signal.DiagnosisDrainInterval},
		diagnosisRepository,
		signalService,
		logger,
	)
	authzRepo := authz.NewGormRepository(database.GORM())
	authzService := authz.NewService(authzRepo)
	grantManager := authz.NewGrantManager(authzRepo)
	workspaceService := workspace.NewService(workspace.NewGormRepository(database.GORM()))
	federationService := federation.NewService(federation.NewGormRepository(database.GORM()), newKubernetesClusterLister(kubernetesService))

	// M50: capability providers (Prometheus + Loki). Constructed only when
	// cfg.Capability.Enabled is true and the corresponding endpoint is set.
	// When disabled, the Options fields stay nil and the capability / logs
	// routes return 503 (ADR 0053 §6, ADR 0065 §3).
	var capabilityMetricsProvider capability.MetricsProvider
	var capabilityLogProvider capability.LogProvider
	if cfg.Capability.Enabled {
		if cfg.Capability.PrometheusEndpoint != "" {
			mp, err := capability.NewPrometheusMetricsProvider(capability.PrometheusConfig{
				Endpoint:       cfg.Capability.PrometheusEndpoint,
				RequestTimeout: cfg.Capability.PrometheusTimeout,
			})
			if err != nil {
				logger.Fatal("configure prometheus capability provider", zap.Error(err))
			}
			capabilityMetricsProvider = mp
		}
		if cfg.Capability.LokiEndpoint != "" {
			lp, err := capability.NewLokiLogProvider(capability.LokiConfig{
				Endpoint:       cfg.Capability.LokiEndpoint,
				RequestTimeout: cfg.Capability.LokiTimeout,
			})
			if err != nil {
				logger.Fatal("configure loki capability provider", zap.Error(err))
			}
			capabilityLogProvider = lp
		}
	}

	// M60: compile-time provider registry (ADR 0075). One Registry is created
	// per process; we register the stable provider catalog here and wire its
	// lifecycle (StartAll before serving, StopAll during graceful shutdown).
	// Cluster roles include the full set so every registered descriptor can
	// report its own ClusterRoles gate; the registry applies the per-process
	// role set at runtime when computing initial state.
	capabilityRegistry := capability.NewRegistry(
		[]string{
			capability.ClusterRoleStandalone,
			capability.ClusterRoleHost,
			capability.ClusterRoleMember,
		},
		5*time.Second,
	)
	metricsConfigured := capabilityMetricsProvider != nil
	logsConfigured := capabilityLogProvider != nil
	// metrics / logs providers are two individual registry entries that share
	// the prometheus / loki HealthChecker adapter (when the provider carries
	// a probe hook). Both are registered as capability kind so the GUI groups
	// them together.
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "metrics_prometheus",
		Description:  "Fixed-template SLI metrics provider backed by a Prometheus-compatible HTTP API",
		Kind:         "capability",
		Dependencies: nil,
		ClusterRoles: nil,
		Configured:   metricsConfigured,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "logs_loki",
		Description:  "Read-only historical log provider backed by a Loki-compatible HTTP API",
		Kind:         "capability",
		Dependencies: nil,
		ClusterRoles: nil,
		Configured:   logsConfigured,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "federation",
		Description:  "Host / member cluster topology aggregation (ADR 0063)",
		Kind:         "federation",
		Dependencies: nil,
		ClusterRoles: []string{capability.ClusterRoleHost, capability.ClusterRoleStandalone},
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "inspection_scheduler",
		Description:  "Compile-time KubeEye-style inspection rule catalog with periodical Cron runner",
		Kind:         "inspection",
		Dependencies: []string{"metrics_prometheus"},
		ClusterRoles: []string{capability.ClusterRoleStandalone, capability.ClusterRoleHost},
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "service_mesh_readonly",
		Description:  "Istio VirtualService/DestinationRule listing and traffic-metrics projections (read-only)",
		Kind:         "mesh",
		Dependencies: []string{"metrics_prometheus"},
		ClusterRoles: nil,
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "gitops_argocd",
		Description:  "Read-only ArgoCD Application browse projection and capability probe",
		Kind:         "gitops",
		Dependencies: nil,
		ClusterRoles: nil,
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "copyops_cross_cluster",
		Description:  "Interactive cross-cluster resource copy with M19 controlled-operation confirm + idempotency contract",
		Kind:         "copyops",
		Dependencies: []string{"federation"},
		ClusterRoles: []string{capability.ClusterRoleHost, capability.ClusterRoleStandalone},
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "app_catalog_helm",
		Description:  "Helm repository CRUD, index.yaml chart browse and M19 controlled HelmRelease deploy plans",
		Kind:         "appcatalog",
		Dependencies: nil,
		ClusterRoles: nil,
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "backup_restore_velero",
		Description:  "Velero backup/restore plan execution and browse projections",
		Kind:         "backup",
		Dependencies: nil,
		ClusterRoles: nil,
		Configured:   true,
	})
	_ = capabilityRegistry.Register(capability.ProviderDescriptor{
		Name:         "ai_investigator",
		Description:  "Cited, bounded AI investigation advisor with deterministic M42 RCA as primary input",
		Kind:         "ai",
		Dependencies: nil,
		ClusterRoles: nil,
		Configured:   cfg.AIEnabled,
	})

	// M50: monitoring service aggregates metricshistory across clusters for
	// the workspace-level dashboard. Bounded fan-out mirrors federation.
	monitoringService := monitoring.NewService(monitoring.Config{
		MaxClusters:       20,
		MaxConcurrent:     4,
		PerClusterTimeout: 4 * time.Second,
	}, metricsHistoryService, workspaceService)
	// M51: bounded SSE event stream over Kubernetes Events. The service wraps
	// the read-only gateway via an adapter (ADR 0066); nil lister would make
	// the handler return 503, so we wire the real gateway.
	eventStreamService, err := eventstream.NewService(eventstream.Config{}, kubernetesEventLister{gateway: kubernetesService})
	if err != nil {
		logger.Fatal("configure eventstream service", zap.Error(err))
	}

	// M52: service-mesh read-only access. Istio VirtualService/DestinationRule
	// listing reuses the M49 CRD gateway; traffic metrics come from the M30
	// metrics-history table (tagged source=istio). Both are nil-safe: the
	// handler returns 503 when the service is nil, and the service returns
	// ErrIstioNotInstalled / ErrMeshDataUnavailable for partial install.
	serviceMeshService := servicemesh.NewService(kubernetesService, metricsHistoryService)

	// M56: golden quality-report service. The replay runner verifies the
	// golden dataset (3 scenarios, 10-step mandatory + 2 negative companions)
	// against the current M39-M44 engine contracts and persists the quality
	// report to .artifacts/quality-report/. GET reads the latest report;
	// POST triggers an async replay (SystemOpsAdmin only).
	goldenStorage := golden.NewFileReportStorage(".artifacts/quality-report")
	goldenService := golden.NewService(goldenEngineContracts(), goldenStorage, logger)

	var oidcSessionManager *oidc.SessionManager
	var oidcPostLogoutURI string
	if cfg.OIDC.Enabled {
		provider, err := oidc.NewProvider(oidc.ProviderConfig{
			Issuer:                   cfg.OIDC.Issuer,
			ClientID:                 cfg.OIDC.ClientID,
			ClientSecret:             cfg.OIDC.ClientSecret,
			RedirectURI:              cfg.OIDC.RedirectURI,
			RequiredScopes:           cfg.OIDC.RequiredScopes,
			AllowedSigningAlgorithms: cfg.OIDC.AllowedSigningAlgorithms,
			ClaimMapping: oidc.ClaimMapping{
				Subject:     cfg.OIDC.ClaimMapping.Subject,
				Username:    cfg.OIDC.ClaimMapping.Username,
				DisplayName: cfg.OIDC.ClaimMapping.DisplayName,
				Groups:      cfg.OIDC.ClaimMapping.Groups,
			},
			GroupToRoles: cfg.OIDC.GroupToRoles,
			MFA: oidc.MFAConfig{
				Required:       cfg.OIDC.MFA.Required,
				EvidenceClaim:  cfg.OIDC.MFA.EvidenceClaim,
				AcceptedValues: cfg.OIDC.MFA.AcceptedValues,
			},
			Sessions: oidc.SessionConfig{
				MaxAge:           cfg.OIDC.Sessions.MaxAge,
				Reauthentication: cfg.OIDC.Sessions.Reauthentication,
				RevokeOnDisable:  cfg.OIDC.Sessions.RevokeOnDisable,
			},
			JWKSCacheTTL:       cfg.OIDC.JWKS.CacheTTL,
			JWKSRefreshTimeout: cfg.OIDC.JWKS.RefreshTimeout,
			SigningKey:         cfg.OIDC.AuthSessionSigningKey,
		})
		if err != nil {
			logger.Fatal("configure OIDC provider", zap.Error(err))
		}
		bootstrapCtx, cancelBootstrap := context.WithTimeout(context.Background(), 30*time.Second)
		if err := provider.Init(bootstrapCtx); err != nil {
			cancelBootstrap()
			logger.Fatal("initialize OIDC provider", zap.Error(err))
		}
		cancelBootstrap()
		resolver := oidc.NewGormIdentityResolver(database.GORM())
		issuer := oidc.NewAuthSessionIssuer(authService)
		oidcSessionManager = oidc.NewSessionManager(provider, resolver, issuer, oidc.SessionManagerConfig{
			Reauthentication: cfg.OIDC.Sessions.Reauthentication,
		})
		oidcPostLogoutURI = cfg.OIDC.RedirectURI
		logger.Info("OIDC provider initialized",
			zap.String("issuer", cfg.OIDC.Issuer),
			zap.String("client_id", cfg.OIDC.ClientID),
		)
	}
	alertScheduler := alert.NewScheduler(alert.SchedulerConfig{
		Enabled:           cfg.AlertEnabled,
		PollInterval:      cfg.AlertPollInterval,
		ClaimBatch:        cfg.AlertClaimBatch,
		WorkerConcurrency: cfg.AlertWorkerConcurrency,
		EvaluationTimeout: cfg.AlertEvaluationTimeout,
		ClaimLease:        cfg.AlertClaimLease,
		MinEvalInterval:   cfg.AlertMinEvaluationInterval,
	}, alertService, alert.NewGormRepository(database.GORM()), logger)
	var aiProvider aiexplain.Provider
	if cfg.AIEnabled {
		aiProvider = aiexplain.NewResponsesProvider(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel, cfg.AIRequestTimeout, cfg.AIMaxOutputTokens)
	}
	aiExplanationService := aiexplain.NewService(aiexplain.ServiceConfig{Enabled: cfg.AIEnabled, ProviderName: "responses-compatible", Model: cfg.AIModel, DailyTokenBudget: cfg.AIDailyTokenBudget, MaxConcurrentRequests: cfg.AIMaxConcurrentRequests, MaxOutputTokens: cfg.AIMaxOutputTokens, ReservationTTL: cfg.AIRequestTimeout + time.Minute}, diagnosisService, aiProvider, aiexplain.NewGormRepository(database.GORM()))
	// P1 RAG knowledge base: resolved diagnoses are distilled into knowledge
	// entries and re-injected as verified historical references. Wiring is
	// enabled whenever AI is enabled; the knowledge layer degrades silently
	// (empty retrieval / failed ingest never block the diagnosis chain).
	knowledgeRepository := knowledge.NewGormRepository(database.GORM())
	knowledgeRetrieverCfg := knowledge.DefaultConfig()
	knowledgeRetrieverCfg.RerankEnabled = false // re-rank is a Phase-2 API call; keep the default cost-free
	knowledgeRetriever := knowledge.NewRetriever(knowledgeRepository, knowledgeRetrieverCfg)
	diagnosisService = diagnosisService.WithKnowledgeIngester(knowledge.NewDiagnosisIngester(knowledgeRepository))
	aiExplanationService = aiExplanationService.WithKnowledgeRetriever(knowledgeRetriever)
	auditService := audit.NewService(audit.NewGormRepository(database.GORM()))
	notificationRepository := notification.NewGormRepository(database.GORM())
	if err := notificationRepository.SetEnabled(context.Background(), cfg.NotificationEnabled); err != nil {
		logger.Fatal("configure diagnosis notifications", zap.Error(err))
	}
	notificationService := notification.NewService(notification.ServiceConfig{
		Enabled: cfg.NotificationEnabled, WebhookURL: cfg.NotificationWebhookURL,
		WebhookSecret: cfg.NotificationWebhookSecret, PollInterval: cfg.NotificationPollInterval,
		RequestTimeout: cfg.NotificationRequestTimeout, RetryBase: cfg.NotificationRetryBase,
		MaxAttempts: cfg.NotificationMaxAttempts, BatchSize: cfg.NotificationBatchSize,
	}, notificationRepository, logger)
	incidentSLAMonitor := incident.NewSLAMonitor(incident.SLAMonitorConfig{
		// The SLA monitor feeds the notification webhook outbox, so it is only
		// active when notifications are enabled (mirrors the diagnosis trigger).
		Enabled:              cfg.IncidentSLAMonitorEnabled && cfg.NotificationEnabled,
		PollInterval:         cfg.IncidentSLAPollInterval,
		ApproachingWindow:    cfg.IncidentSLAApproachingWin,
		FirstEscalationAfter: cfg.IncidentSLAFirstEscalationAfter,
		FinalEscalationAfter: cfg.IncidentSLAFinalEscalationAfter,
		BatchSize:            cfg.IncidentSLABatchSize,
	}, incidentRepository, slaEnqueuer{service: notificationService}, logger)
	backgroundContext, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	var backgroundWait sync.WaitGroup
	backgroundWait.Add(6)
	go func() {
		defer backgroundWait.Done()
		notificationService.Run(backgroundContext)
	}()
	go func() {
		defer backgroundWait.Done()
		incidentSLAMonitor.Run(backgroundContext)
	}()
	go func() {
		defer backgroundWait.Done()
		if cfg.Signal.DiagnosisIngestion {
			diagnosisSignalDrain.Run(backgroundContext)
		}
	}()
	go func() {
		defer backgroundWait.Done()
		metricsCollector.Run(backgroundContext)
	}()
	go func() {
		defer backgroundWait.Done()
		alertScheduler.Run(backgroundContext)
	}()
	go func() {
		defer backgroundWait.Done()
		correlationWorker.Run(backgroundContext)
	}()

	// M60: start providers that carry background goroutines. Providers that
	// implement Lifecycle are started in dependency order (topological sort).
	// Start errors are logged but never fatal — a partial provider failure must
	// not crash the control plane.
	if startErr := capabilityRegistry.StartAll(backgroundContext); startErr != nil {
		logger.Warn("some providers failed to start", zap.Error(startErr))
	}

	server := &http.Server{
		Addr: cfg.HTTPAddress,
		Handler: httpserver.New(logger, httpserver.Options{
			Probe:                     database,
			Auth:                      authService,
			Clusters:                  clusterService,
			Kubernetes:                kubernetesService,
			Fleet:                     fleetService,
			GlobalSearch:              globalSearchService,
			SavedFilters:              savedFilterService,
			MetricsHistory:            metricsHistoryService,
			Diagnosis:                 diagnosisService,
			Incidents:                 incidentService,
			IncidentResolver:          incidentSourceResolver,
			AIExplanation:             aiExplanationService,
			Audit:                     auditService,
			Notifications:             notificationService,
			Remediation:               remediationService,
			Promotion:                 promotionService,
			Alert:                     alertService,
			Backup:                    backupService,
			Maintenance:               maintenanceService,
			NamespacePosture:          namespacePostureService,
			Restore:                   restoreService,
			Authz:                     authzService,
			GrantManager:              grantManager,
			SignalService:             signalService,
			SLOService:                sloService,
			CorrelationService:        correlationService,
			WorkspaceService:          workspaceService,
			FederationService:         federationService,
			Monitoring:                monitoringService,
			EventStreamService:        eventStreamService,
			InspectionService:         inspectionService,
			ServiceMeshService:        serviceMeshService,
			GoldenService:             goldenService,
			KnowledgeRepository:       knowledgeRepository,
			AppCatalogService:         appCatalogService,
			GitOpsService:             gitopsService,
			CopyOpsService:            copyOpsService,
			CapabilityMetricsProvider: capabilityMetricsProvider,
			CapabilityLogProvider:     capabilityLogProvider,
			CapabilityRegistry:        capabilityRegistry,
			Optimization:              optimization.NewService(finops.DefaultCostRate(), optimizationCollector),
			// M80 unified governance posture: aggregates all M61-M78 analyzers.
			// The deprecated-API target version is supplied per request via the
			// posture endpoint query parameter (none in the static config), so
			// the evaluator is built without a fixed target.
			Posture:        posture.New(optimizationCollector),
			OIDC:           oidcSessionManager,
			OIDCPostLogout: oidcPostLogoutURI,
			SecureCookies:  cfg.SecureCookies,
			RefreshTTL:     cfg.RefreshTokenTTL,
			Version:        buildinfo.Version,
		}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api server started",
			zap.String("address", cfg.HTTPAddress),
			zap.String("environment", cfg.Environment),
			zap.String("version", buildinfo.Version),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-errCh:
		logger.Error("api server stopped unexpectedly", zap.Error(err))
	}
	stopBackground()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	// M60: stop providers that implement Lifecycle, in reverse dependency order
	// (dependents stop before their dependencies). Errors are aggregated into
	// a single log line — shutdown must not hang on provider teardown.
	if stopErr := capabilityRegistry.StopAll(shutdownCtx); stopErr != nil {
		logger.Warn("some providers failed to stop cleanly", zap.Error(stopErr))
	}
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	backgroundDone := make(chan struct{})
	go func() {
		backgroundWait.Wait()
		close(backgroundDone)
	}()
	select {
	case <-backgroundDone:
	case <-shutdownCtx.Done():
		logger.Error("background services did not stop before shutdown deadline", zap.Error(shutdownCtx.Err()))
	}
}

func newLogger(environment string) (*zap.Logger, error) {
	if environment == "development" {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}
