package monitoring

import "time"

// Template constants — the fixed, server-owned dashboard templates (ADR 0065
// §1). The frontend cannot submit custom PromQL or define its own panels;
// it may only select one of these templates and supply the time window +
// namespace filter.
const (
	TemplateNodeOverview      = "node_overview"
	TemplateWorkloadOverview  = "workload_overview"
	TemplatePodOverview       = "pod_overview"
	TemplateWorkspaceOverview = "workspace_overview"
)

// Metric constants — mirror metricshistory.MetricCPU/MetricMemory so the
// monitoring package does not import metricshistory in the model file.
const (
	MetricCPU    = "cpu"
	MetricMemory = "memory"

	UnitNanocores = "nanocores"
	UnitBytes     = "bytes"
)

// MaxDashboardWindow bounds the dashboard time window to the metricshistory
// query limit (24h, ADR 0065 §2).
const MaxDashboardWindow = 24 * time.Hour

// Panel describes one chart in a dashboard template. The frontend uses the
// metric/unit/resource_kind fields to drive calls to the existing
// GET /clusters/:cluster_id/metrics/history endpoint — the dashboard endpoint
// itself does not pre-fetch time series (ADR 0065 §1).
type Panel struct {
	Title        string `json:"title"`
	Metric       string `json:"metric"`
	Unit         string `json:"unit"`
	ResourceKind string `json:"resource_kind"`
	Description  string `json:"description,omitempty"`
}

// ClusterDashboardRequest is the input for a single-cluster dashboard.
type ClusterDashboardRequest struct {
	ClusterID int64     `json:"-"`
	Template  string    `json:"-"`
	Namespace string    `json:"-"` // optional, only for pod/workload templates
	From      time.Time `json:"-"`
	To        time.Time `json:"-"`
}

// ClusterDashboardResponse is the single-cluster dashboard. The template
// field carries the fixed panel definitions; the from/to echo the requested
// window so the frontend can pass them through to /metrics/history.
type ClusterDashboardResponse struct {
	Template  string    `json:"template"`
	ClusterID int64     `json:"cluster_id"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Panels    []Panel   `json:"panels"`
}

// WorkspaceDashboardRequest is the input for a workspace-level dashboard.
// ActorUserID/ActorRoles drive the workspace_viewer authorization check
// (workspace.Service.ListMemberships requires it).
type WorkspaceDashboardRequest struct {
	WorkspaceID int64     `json:"-"`
	ActorUserID int64     `json:"-"`
	ActorRoles  []string  `json:"-"`
	From        time.Time `json:"-"`
	To          time.Time `json:"-"`
}

// WorkspaceClusterEntry is one cluster's entry in the workspace dashboard.
// Namespaces lists the workspace's member namespaces on that cluster; the
// frontend uses them to filter its per-cluster /metrics/history calls.
type WorkspaceClusterEntry struct {
	ClusterID  int64    `json:"cluster_id"`
	Namespaces []string `json:"namespaces"`
}

// WorkspaceDashboardResponse is the cross-cluster dashboard. It carries the
// fixed template plus the workspace's (cluster, namespaces) topology so the
// frontend can fan out per-cluster queries. The backend does not pre-fetch
// time series for every cluster — that would be O(clusters × resources) and
// is the frontend's responsibility via the bounded /metrics/history endpoint
// (ADR 0065 §2).
type WorkspaceDashboardResponse struct {
	Template    string                  `json:"template"`
	WorkspaceID int64                   `json:"workspace_id"`
	From        time.Time               `json:"from"`
	To          time.Time               `json:"to"`
	Panels      []Panel                 `json:"panels"`
	Clusters    []WorkspaceClusterEntry `json:"clusters"`
}

// fixedTemplates is the compile-time-fixed catalogue of dashboard templates.
// Adding a template is a contract change — there is no runtime expansion
// (static-extension hard constraint, ADR 0065 §1).
var fixedTemplates = map[string][]Panel{
	TemplateNodeOverview: {
		{Title: "Node CPU Usage", Metric: MetricCPU, Unit: UnitNanocores, ResourceKind: "Node", Description: "CPU usage per node (nanocores)."},
		{Title: "Node Memory Usage", Metric: MetricMemory, Unit: UnitBytes, ResourceKind: "Node", Description: "Memory usage per node (bytes)."},
	},
	TemplateWorkloadOverview: {
		{Title: "Workload CPU Usage", Metric: MetricCPU, Unit: UnitNanocores, ResourceKind: "Pod", Description: "CPU usage per pod (nanocores), grouped by workload."},
		{Title: "Workload Memory Usage", Metric: MetricMemory, Unit: UnitBytes, ResourceKind: "Pod", Description: "Memory usage per pod (bytes), grouped by workload."},
	},
	TemplatePodOverview: {
		{Title: "Pod CPU Usage", Metric: MetricCPU, Unit: UnitNanocores, ResourceKind: "Pod", Description: "CPU usage per pod (nanocores)."},
		{Title: "Pod Memory Usage", Metric: MetricMemory, Unit: UnitBytes, ResourceKind: "Pod", Description: "Memory usage per pod (bytes)."},
	},
	TemplateWorkspaceOverview: {
		{Title: "Node CPU Usage", Metric: MetricCPU, Unit: UnitNanocores, ResourceKind: "Node", Description: "Aggregate node CPU usage across workspace clusters."},
		{Title: "Node Memory Usage", Metric: MetricMemory, Unit: UnitBytes, ResourceKind: "Node", Description: "Aggregate node memory usage across workspace clusters."},
	},
}
