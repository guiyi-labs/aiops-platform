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
	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/buildinfo"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/config"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/fleet"
	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/httpserver"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/notification"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/store"
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
	clusterRegistry := cluster.NewRegistry(cfg.ClusterProbeTimeout)
	clusterService := cluster.NewService(cluster.NewGormRepository(database.GORM()), credentialEncryptor, clusterRegistry)
	kubernetesService := k8sgateway.NewService(clusterService, clusterRegistry)
	metricsHistoryService, err := metricshistory.NewService(metricshistory.Config{Retention: cfg.MetricsHistoryRetention}, metricshistory.NewGormRepository(database.GORM()))
	if err != nil {
		logger.Fatal("configure metrics history", zap.Error(err))
	}
	metricsCollector, err := metricshistory.NewCollector(metricshistory.CollectorConfig{
		Enabled: cfg.MetricsHistoryEnabled, CollectionInterval: cfg.MetricsCollectionInterval,
		PerClusterTimeout: cfg.MetricsCollectionTimeout, CleanupInterval: cfg.MetricsCleanupInterval,
		MaxClusters: cfg.MetricsMaxClusters, MaxConcurrentClusters: cfg.MetricsMaxConcurrency, MaxSamples: 1800,
	}, clusterService, kubernetesService, metricsHistoryService, logger)
	if err != nil {
		logger.Fatal("configure metrics history collector", zap.Error(err))
	}
	fleetService := fleet.NewService(fleet.Config{MaxClusters: 20, MaxConcurrentClusters: 4, PerClusterTimeout: 4 * time.Second, ResourceSampleLimit: 100}, clusterService, kubernetesService)
	globalSearchService := globalsearch.NewService(globalsearch.Config{MaxClusters: 20, MaxConcurrentClusters: 4, PerClusterTimeout: 4 * time.Second, MaxResults: 100, PerKindLimit: 100}, clusterService, kubernetesService)
	savedFilterService := globalsearch.NewSavedFilterService(globalsearch.NewSavedFilterGormRepository(database.GORM()))
	diagnosisService := diagnosis.NewService(kubernetesService, diagnosis.NewGormRepository(database.GORM()))
	remediationService := remediation.NewService(diagnosisService, kubernetesService, remediation.NewGormRepository(database.GORM()))
	var aiProvider aiexplain.Provider
	if cfg.AIEnabled {
		aiProvider = aiexplain.NewResponsesProvider(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel, cfg.AIRequestTimeout, cfg.AIMaxOutputTokens)
	}
	aiExplanationService := aiexplain.NewService(aiexplain.ServiceConfig{Enabled: cfg.AIEnabled, ProviderName: "responses-compatible", Model: cfg.AIModel, DailyTokenBudget: cfg.AIDailyTokenBudget, MaxConcurrentRequests: cfg.AIMaxConcurrentRequests, MaxOutputTokens: cfg.AIMaxOutputTokens, ReservationTTL: cfg.AIRequestTimeout + time.Minute}, diagnosisService, aiProvider, aiexplain.NewGormRepository(database.GORM()))
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
	backgroundContext, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	var backgroundWait sync.WaitGroup
	backgroundWait.Add(2)
	go func() {
		defer backgroundWait.Done()
		notificationService.Run(backgroundContext)
	}()
	go func() {
		defer backgroundWait.Done()
		metricsCollector.Run(backgroundContext)
	}()

	server := &http.Server{
		Addr: cfg.HTTPAddress,
		Handler: httpserver.New(logger, httpserver.Options{
			Probe:         database,
			Auth:          authService,
			Clusters:      clusterService,
			Kubernetes:    kubernetesService,
			Fleet:         fleetService,
			GlobalSearch:  globalSearchService,
			SavedFilters:  savedFilterService,
			Diagnosis:     diagnosisService,
			AIExplanation: aiExplanationService,
			Audit:         auditService,
			Notifications: notificationService,
			Remediation:   remediationService,
			SecureCookies: cfg.SecureCookies,
			RefreshTTL:    cfg.RefreshTokenTTL,
			Version:       buildinfo.Version,
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
