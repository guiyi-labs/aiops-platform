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
	"k8s-aiops.local/backend/internal/aisettings"
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/buildinfo"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/config"
	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/httpserver"
	"k8s-aiops.local/backend/internal/knowledge"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
	signalsvc "k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/store"
	"k8s-aiops.local/backend/internal/topology"
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
	diagnosisService := diagnosis.NewService(kubernetesService, diagnosis.NewGormRepository(database.GORM())).WithMetricEvaluator(metricsHistoryService)
	signalRepository := signalsvc.NewGormRepository(database.GORM())
	signalService := signalsvc.NewService(signalsvc.ServiceOptions{Repository: signalRepository})
	topologyRepository := topology.NewGormRepository(database.GORM())
	diagnosisRepository := diagnosis.NewGormRepository(database.GORM())
	correlationProvider := correlation.NewRepositoryInputProvider(signalRepository, topologyRepository, diagnosisRepository)
	correlationService := correlation.NewService(correlation.NewGormRepository(database.GORM()), nil, correlationProvider)
	correlationWorker := correlation.NewWorker(correlation.WorkerConfig{Interval: cfg.CorrelationInterval}, clusterService, kubernetesService, correlationService, logger)
	diagnosisSignalDrain := signalsvc.NewDiagnosisDrain(
		signalsvc.DrainConfig{Interval: cfg.Signal.DiagnosisDrainInterval},
		diagnosisRepository,
		signalService,
		logger,
	)
	authzRepo := authz.NewGormRepository(database.GORM())
	authzService := authz.NewService(authzRepo)
	// 模型接入：环境变量只做「首次播种」，此后以控制台保存的库内配置为准。
	// 修改配置会通过 onChange 回调热更新解释服务，无需重启进程。
	aiSettingsService := aisettings.NewService(
		aisettings.NewGormRepository(database.GORM()),
		credentialEncryptor,
		aisettings.Settings{
			Enabled:               cfg.AIEnabled,
			Protocol:              aisettings.ProtocolResponses,
			BaseURL:               cfg.AIBaseURL,
			Model:                 cfg.AIModel,
			APIKey:                cfg.AIAPIKey,
			RequestTimeout:        cfg.AIRequestTimeout,
			MaxOutputTokens:       cfg.AIMaxOutputTokens,
			DailyTokenBudget:      cfg.AIDailyTokenBudget,
			MaxConcurrentRequests: cfg.AIMaxConcurrentRequests,
		},
	)
	activeAISettings, err := aiSettingsService.Load(context.Background())
	if err != nil {
		logger.Warn("load AI model settings, falling back to environment values", zap.Error(err))
		activeAISettings = aisettings.Settings{
			Enabled:               cfg.AIEnabled,
			Protocol:              aisettings.ProtocolResponses,
			BaseURL:               cfg.AIBaseURL,
			Model:                 cfg.AIModel,
			APIKey:                cfg.AIAPIKey,
			RequestTimeout:        cfg.AIRequestTimeout,
			MaxOutputTokens:       cfg.AIMaxOutputTokens,
			DailyTokenBudget:      cfg.AIDailyTokenBudget,
			MaxConcurrentRequests: cfg.AIMaxConcurrentRequests,
		}
	}
	aiProvider, aiProviderName := buildAIProvider(activeAISettings)
	aiExplanationService := aiexplain.NewService(aiServiceConfig(activeAISettings, aiProviderName), diagnosisService, aiProvider, aiexplain.NewGormRepository(database.GORM()))
	aiSettingsService.WithOnChange(func(updated aisettings.Settings) {
		provider, name := buildAIProvider(updated)
		aiExplanationService.ApplyConfig(aiServiceConfig(updated, name), provider)
		logger.Info("AI model settings applied",
			zap.Bool("enabled", updated.Enabled),
			zap.String("protocol", updated.Protocol),
			zap.String("model", updated.Model),
			zap.String("api_key", aisettings.Mask(updated.APIKey)),
		)
	})
	// P1 RAG knowledge base: resolved diagnoses are distilled into knowledge
	// entries and re-injected as verified historical references. Wiring is
	// enabled whenever AI is enabled; the knowledge layer degrades silently
	// (empty retrieval / failed ingest never block the diagnosis chain).
	knowledgeRepository := knowledge.NewGormRepository(database.GORM())
	knowledgeRetrieverCfg := knowledge.DefaultConfig()
	knowledgeRetrieverCfg.RerankEnabled = false // the LLM re-rank is opt-in; the default retrieval path stays cost-free
	// The vector stage runs the offline lexical embedder: no API key, no model
	// download, and identical vectors across restarts — so the hybrid pipeline
	// behaves the same in a demo cluster as it does in the benchmark.
	knowledgeRetriever := knowledge.NewRetriever(knowledgeRepository, knowledgeRetrieverCfg).
		WithEmbeddingProvider(knowledge.NewLexicalEmbeddingProvider(0))
	diagnosisService = diagnosisService.WithKnowledgeIngester(knowledge.NewDiagnosisIngester(knowledgeRepository))
	aiExplanationService = aiExplanationService.WithKnowledgeRetriever(knowledgeRetriever)
	auditService := audit.NewService(audit.NewGormRepository(database.GORM()))
	// Only diagnosis support workers run in the focused console.
	backgroundContext, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	var backgroundWait sync.WaitGroup
	for _, run := range []func(context.Context){metricsCollector.Run, diagnosisSignalDrain.Run, correlationWorker.Run} {
		backgroundWait.Add(1)
		go func(run func(context.Context)) {
			defer backgroundWait.Done()
			run(backgroundContext)
		}(run)
	}
	topologyService := topology.NewService(
		topology.NewCollector(kubernetesService, 100),
		topology.NewGormRepository(database.GORM()),
		topologyNamespaceLister{reader: kubernetesService},
	)

	// 拓扑采集：启动预热 + 周期刷新。
	//
	// 边表存的是「当前活跃关系」：进程重启后旧边已被关闭（valid_to 非空）就不再出现，
	// 于是重启之后图会变空，看起来像采集坏了——所以启动后先补采一次。之后按
	// TOPOLOGY_COLLECT_INTERVAL 周期刷新，让图跟得上部署/扩缩容；该值为 0 时只预热、
	// 不跑循环（此时靠界面上的「采集拓扑」按需触发）。
	//
	// 全程异步且失败只记日志：采集要读 8 类资源 × 每个命名空间，绝不能让一次
	// api_server 抖动影响进程可用性，也不能阻塞启动或优雅退出。
	backgroundWait.Add(1)
	go func() {
		defer backgroundWait.Done()
		select {
		case <-backgroundContext.Done():
			return
		case <-time.After(20 * time.Second):
			// 等 api_server 连接池与集群探活就绪，避免启动瞬间把所有读取都打失败。
		}
		collect := func(phase string) bool {
			clusters, err := clusterService.List(backgroundContext)
			if err != nil {
				logger.Warn("topology collect: list clusters", zap.String("phase", phase), zap.Error(err))
				return false
			}
			for _, item := range clusters {
				if !item.Enabled {
					continue
				}
				if backgroundContext.Err() != nil {
					return false
				}
				result, collectErr := topologyService.CollectCluster(backgroundContext, item.ID, time.Now().UTC())
				if collectErr != nil {
					logger.Warn("topology collect",
						zap.String("phase", phase), zap.Int64("cluster_id", item.ID), zap.Error(collectErr))
					continue
				}
				logger.Info("topology collected",
					zap.String("phase", phase),
					zap.Int64("cluster_id", item.ID),
					zap.Int("namespaces", result.Namespaces),
					zap.Int("edges_upserted", result.TotalUpserted),
					zap.Int("edges_closed", result.TotalClosed),
					zap.Bool("partial", result.Partial),
				)
			}
			return true
		}

		collect("warm-up")
		if cfg.TopologyCollectInterval <= 0 {
			logger.Info("topology periodic collection disabled (TOPOLOGY_COLLECT_INTERVAL<=0)")
			return
		}
		ticker := time.NewTicker(cfg.TopologyCollectInterval)
		defer ticker.Stop()
		for {
			select {
			case <-backgroundContext.Done():
				return
			case <-ticker.C:
				collect("periodic")
			}
		}
	}()

	server := &http.Server{
		Addr: cfg.HTTPAddress,
		Handler: httpserver.New(logger, httpserver.Options{
			Probe: database, Auth: authService, Clusters: clusterService,
			Kubernetes: kubernetesService, Diagnosis: diagnosisService,
			MetricsHistory: metricsHistoryService, AIExplanation: aiExplanationService,
			AIModelSettings: aiSettingsService, Audit: auditService, Authz: authzService,
			GrantManager:  authz.NewGrantManager(authzRepo),
			SignalService: signalService, CorrelationService: correlationService,
			TopologyService: topologyService, KnowledgeRepository: knowledgeRepository,
			KnowledgeRetriever: knowledgeRetriever,
			SecureCookies:      cfg.SecureCookies, RefreshTTL: cfg.RefreshTokenTTL,
			Version: buildinfo.Version,
		}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout,
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
