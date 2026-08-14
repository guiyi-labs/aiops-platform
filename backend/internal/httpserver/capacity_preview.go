package httpserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/capacitypreview"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// capacityPreviewHandler exposes the M113-2 capacity-aware preview. It reads
// live node allocatable capacity and node usage metrics through the read-only
// kubernetes gateway, evaluates a candidate workload request against every
// node, and returns the nodes ranked best-fit first with per-constraint
// "why fits / why not" explanations and data freshness. The endpoint never
// writes anything; its output is a remediation preview only.
type capacityPreviewHandler struct {
	kubernetes *k8sgateway.Service
}

// capacityPreviewRequest carries the cluster id plus the candidate workload's
// resource demands (same units as the rest of the platform).
type capacityPreviewRequest struct {
	ClusterID           int64 `json:"cluster_id"`
	CPURequestNanocores int64 `json:"cpu_request_nanocores"`
	MemRequestBytes     int64 `json:"mem_request_bytes"`
	GPURequest          int64 `json:"gpu_request"`
	StorageRequestBytes int64 `json:"storage_request_bytes"`
}

func (h capacityPreviewHandler) preview(c *gin.Context) {
	var request capacityPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if request.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	if request.CPURequestNanocores < 0 || request.MemRequestBytes < 0 || request.GPURequest < 0 || request.StorageRequestBytes < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "resource requests must be non-negative")
		return
	}
	if request.CPURequestNanocores == 0 && request.MemRequestBytes == 0 && request.GPURequest == 0 && request.StorageRequestBytes == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "at least one resource request must be positive")
		return
	}
	if h.kubernetes == nil {
		writeError(c, http.StatusServiceUnavailable, "COLLECTOR_UNAVAILABLE", "kubernetes gateway is not configured")
		return
	}

	ctx := c.Request.Context()
	clusterID := request.ClusterID
	observedAt := time.Now().UTC()

	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	nodesResponse, err := h.kubernetes.Nodes(ctx, clusterID, query)
	if err != nil {
		writeError(c, http.StatusBadGateway, "NODES_FETCH_FAILED", err.Error())
		return
	}
	metricsQuery, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	metricsResponse, err := h.kubernetes.NodeMetrics(ctx, clusterID, metricsQuery)
	if err != nil {
		writeError(c, http.StatusBadGateway, "NODE_METRICS_FETCH_FAILED", err.Error())
		return
	}

	metricsByName := make(map[string]k8sgateway.NodeMetric, len(metricsResponse.Items))
	for _, m := range metricsResponse.Items {
		metricsByName[m.Metadata.Name] = m
	}

	bundle := capacitypreview.Bundle{ClusterID: clusterID, ObservedAt: observedAt}
	for _, node := range nodesResponse.Items {
		obs := capacitypreview.NodeObservation{
			Name:                  node.Metadata.Name,
			Allocatable:           node.Status.Allocatable,
			Schedulable:           !node.Spec.Unschedulable,
			StatusReady:           nodeReady(node),
			AllocatableObservedAt: node.Metadata.CreationTimestamp,
		}
		if metric, ok := metricsByName[node.Metadata.Name]; ok {
			obs.UsageCPU = metric.Usage.CPU
			obs.UsageMemory = metric.Usage.Memory
			obs.UsageObservedAt = metric.Timestamp
		}
		bundle.Nodes = append(bundle.Nodes, obs)
	}

	preview, err := capacitypreview.Evaluate(clusterID, capacitypreview.WorkloadRequest{
		CPURequestNanocores: request.CPURequestNanocores,
		MemRequestBytes:     request.MemRequestBytes,
		GPURequest:          request.GPURequest,
		StorageRequestBytes: request.StorageRequestBytes,
	}, bundle, observedAt)
	if err != nil {
		writeError(c, http.StatusBadRequest, "NO_NODES", err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func nodeReady(node k8sgateway.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" {
			return strings.EqualFold(condition.Status, "True")
		}
	}
	return false
}
