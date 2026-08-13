package correlation

import (
	"context"
	"sort"
	"time"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// DefaultInterval is the correlation pass interval when WorkerConfig.Interval
// is zero. Production overrides it via CORRELATION_INTERVAL.
const DefaultInterval = 5 * time.Minute

// DefaultPerClusterTimeout bounds one cluster's correlation pass (namespace
// listing + per-scope engine runs and persistence).
const DefaultPerClusterTimeout = 10 * time.Second

// WorkerConfig configures the periodic correlation worker.
type WorkerConfig struct {
	// Interval between correlation passes.
	Interval time.Duration
	// PerClusterTimeout bounds each cluster's pass. Zero uses
	// DefaultPerClusterTimeout.
	PerClusterTimeout time.Duration
}

type clusterLister interface {
	List(context.Context) ([]cluster.Cluster, error)
}

type namespaceLister interface {
	Namespaces(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error)
}

type correlator interface {
	CorrelateNamespace(context.Context, int64, string) (CorrelateResult, error)
}

// Worker periodically runs correlation passes for every enabled cluster. One
// pass iterates the cluster's namespaces; when the namespace listing is
// unavailable (e.g. the cluster is unreachable) it falls back to a single
// all-namespace pass so rows already persisted still correlate.
type Worker struct {
	config     WorkerConfig
	clusters   clusterLister
	namespaces namespaceLister
	correlate  correlator
	logger     *zap.Logger
}

// NewWorker constructs a Worker. Nil logger is replaced by a no-op logger.
func NewWorker(config WorkerConfig, clusters clusterLister, namespaces namespaceLister, correlate correlator, logger *zap.Logger) *Worker {
	if config.Interval <= 0 {
		config.Interval = DefaultInterval
	}
	if config.PerClusterTimeout <= 0 {
		config.PerClusterTimeout = DefaultPerClusterTimeout
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Worker{config: config, clusters: clusters, namespaces: namespaces, correlate: correlate, logger: logger}
}

// Run executes one pass immediately and then every Interval until the context
// is cancelled. Pass errors are logged and never crash the worker.
func (w *Worker) Run(ctx context.Context) {
	w.runPass(ctx)
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runPass(ctx)
		}
	}
}

// runPass lists clusters in stable ID order and correlates every enabled one.
func (w *Worker) runPass(ctx context.Context) {
	clusters, err := w.clusters.List(ctx)
	if err != nil {
		w.logger.Warn("correlation worker: list clusters", zap.Error(err))
		return
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].ID < clusters[j].ID })
	for _, c := range clusters {
		if !c.Enabled {
			continue
		}
		passCtx, cancel := context.WithTimeout(ctx, w.config.PerClusterTimeout)
		w.runClusterPass(passCtx, c)
		cancel()
	}
}

func (w *Worker) runClusterPass(ctx context.Context, c cluster.Cluster) {
	namespaces, err := w.namespaces.Namespaces(ctx, c.ID, apiquery.ListQuery{Page: 1, Limit: 500})
	if err != nil {
		w.logger.Debug("correlation worker: namespace listing unavailable, using all-namespace pass",
			zap.Int64("cluster_id", c.ID), zap.Error(err))
		w.runScope(ctx, c.ID, "")
		return
	}
	if len(namespaces.Items) == 0 {
		w.logger.Debug("correlation worker: cluster has no namespaces", zap.Int64("cluster_id", c.ID))
		return
	}
	for _, ns := range namespaces.Items {
		w.runScope(ctx, c.ID, ns.Metadata.Name)
	}
	// Cluster-scoped signals (e.g. Node-level diag.node.not_ready.v1) carry an
	// empty namespace and are invisible to per-namespace passes; run one extra
	// all-namespace pass so cluster-scoped rules (e.g.
	// maintenance_causes_node_failure) can correlate. Upserts merge on
	// case_key, so repeated passes are idempotent.
	w.runScope(ctx, c.ID, "")
}

func (w *Worker) runScope(ctx context.Context, clusterID int64, namespace string) {
	result, err := w.correlate.CorrelateNamespace(ctx, clusterID, namespace)
	if err != nil {
		w.logger.Warn("correlation worker: pass errored",
			zap.Int64("cluster_id", clusterID), zap.String("namespace", namespace), zap.Error(err))
		return
	}
	if result.Partial {
		w.logger.Warn("correlation worker: partial pass",
			zap.Int64("cluster_id", clusterID), zap.String("namespace", namespace),
			zap.Int("inputs_gathered", result.InputsGathered),
			zap.Int("results_produced", result.ResultsProduced),
			zap.Int("cases_upserted", result.CasesUpserted),
			zap.Error(result.Error))
		return
	}
	w.logger.Info("correlation worker: pass complete",
		zap.Int64("cluster_id", clusterID), zap.String("namespace", namespace),
		zap.Int("inputs_gathered", result.InputsGathered),
		zap.Int("results_produced", result.ResultsProduced),
		zap.Int("cases_upserted", result.CasesUpserted))
}
