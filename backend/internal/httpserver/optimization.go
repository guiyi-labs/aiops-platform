package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/capacity"
	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/gitopsdrift"
	"k8s-aiops.local/backend/internal/imagepolicy"
	"k8s-aiops.local/backend/internal/netpolicy"
	"k8s-aiops.local/backend/internal/optimization"
	"k8s-aiops.local/backend/internal/policy"
)

// optimizationHandler exposes the read-only M61-M63 analyzers over HTTP.
// Each endpoint accepts an already-collected observation bundle in the
// request body, runs the corresponding pure analyzer, and returns findings.
// The server never reaches into the cluster itself (ADR 0004).
type optimizationHandler struct {
	svc *optimization.Service
}

// cisAnalyzeRequest carries the cluster identity plus the CIS observation
// bundle. cis.Inputs is embedded so its fields (components, workloads,
// bindings, namespaces) are accepted at the top level of the JSON body.
type cisAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	cis.Inputs
}

func (h optimizationHandler) cisAnalyze(c *gin.Context) {
	var req cisAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	inputs := req.Inputs
	if len(inputs.Workloads) == 0 && len(inputs.Bindings) == 0 && len(inputs.Namespaces) == 0 {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no observation bundle supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectCIS(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		inputs = collected
	}
	status := cis.Evaluate(req.ClusterID, inputs, time.Now())
	c.JSON(http.StatusOK, status)
}

type finopsAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	// Rate overrides the configured default cost rate when supplied.
	Rate *finops.CostRate `json:"rate,omitempty"`
	// Inputs is the per-container request/limit/usage bundle.
	Inputs []finops.ContainerInput `json:"inputs"`
}

func (h optimizationHandler) finopsAnalyze(c *gin.Context) {
	var req finopsAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if len(req.Inputs) == 0 {
		if req.ClusterID == 0 || !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "at least one container input is required (or supply cluster_id with auto-collection configured)")
			return
		}
		collected, err := h.svc.CollectFinOps(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		req.Inputs = collected
	}
	rate := h.svc.DefaultCostRate()
	if req.Rate != nil {
		rate = *req.Rate
	}
	// Every ContainerInput carries its own ClusterID; the rollup is per
	// cluster, so the first input's cluster identifies the evaluation.
	clusterID := req.Inputs[0].ClusterID
	summary := finops.Recommend(clusterID, req.Inputs, rate)
	c.JSON(http.StatusOK, summary)
}

type deprecatedAPIAnalyzeRequest struct {
	ClusterID     int64                          `json:"cluster_id"`
	TargetVersion string                         `json:"target_version"`
	Objects       []deprecatedapi.ResourceObject `json:"objects"`
}

func (h optimizationHandler) deprecatedAPIAnalyze(c *gin.Context) {
	var req deprecatedAPIAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	if req.TargetVersion == "" {
		writeError(c, http.StatusBadRequest, "INVALID_TARGET", "target_version is required (e.g. \"1.29\")")
		return
	}
	objects := req.Objects
	if len(objects) == 0 {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no objects supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectDeprecatedAPI(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		objects = collected
	}
	status := deprecatedapi.Check(req.ClusterID, req.TargetVersion, objects, time.Now())
	c.JSON(http.StatusOK, status)
}

// networkAnalyzeRequest carries the cluster identity plus the network posture
// observation bundle. netpolicy.Inputs is embedded so its fields (namespaces,
// pods, policies, services) are accepted at the top level of the JSON body.
type networkAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	netpolicy.Inputs
}

func (h optimizationHandler) networkAnalyze(c *gin.Context) {
	var req networkAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	inputs := req.Inputs
	if inputs.Empty() {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no observation bundle supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectNetPolicy(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		inputs = collected
	}
	status := netpolicy.Evaluate(req.ClusterID, inputs, time.Now())
	c.JSON(http.StatusOK, status)
}

// imageAnalyzeRequest carries the cluster identity plus the image supply-chain
// observation bundle. imagepolicy.Inputs is embedded so its "usages" field is
// accepted at the top level of the JSON body.
type imageAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	imagepolicy.Inputs
}

func (h optimizationHandler) imageAnalyze(c *gin.Context) {
	var req imageAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	inputs := req.Inputs
	if inputs.Empty() {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no observation bundle supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectImagePolicy(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		inputs = collected
	}
	status := imagepolicy.Evaluate(req.ClusterID, inputs, time.Now())
	c.JSON(http.StatusOK, status)
}

// gitopsAnalyzeRequest carries the cluster identity plus the GitOps drift
// observation bundle. gitopsdrift.Inputs is embedded so its "resources" and
// "managed_namespaces" fields are accepted at the top level of the JSON body.
type gitopsAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	gitopsdrift.Inputs
}

func (h optimizationHandler) gitopsAnalyze(c *gin.Context) {
	var req gitopsAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	inputs := req.Inputs
	if inputs.Empty() {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no observation bundle supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectGitOpsDrift(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		inputs = collected
	}
	status := gitopsdrift.Evaluate(req.ClusterID, inputs, time.Now())
	c.JSON(http.StatusOK, status)
}

// capacityAnalyzeRequest carries the cluster identity plus the capacity trend
// observation bundle. capacity.Inputs is embedded so its "cpu", "memory" and
// "horizon_days" fields are accepted at the top level of the JSON body.
type capacityAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	capacity.Inputs
}

func (h optimizationHandler) capacityAnalyze(c *gin.Context) {
	var req capacityAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	inputs := req.Inputs
	if inputs.Empty() {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no observation bundle supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectCapacity(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		// The caller's horizon preference survives auto-collection.
		collected.HorizonDays = req.Inputs.HorizonDays
		inputs = collected
	}
	status := capacity.Evaluate(req.ClusterID, inputs, time.Now())
	c.JSON(http.StatusOK, status)
}

// policyAnalyzeRequest carries the cluster identity plus the policy
// observation bundle. policy.Inputs is embedded so its "workloads" field is
// accepted at the top level of the JSON body.
type policyAnalyzeRequest struct {
	ClusterID int64 `json:"cluster_id"`
	policy.Inputs
}

func (h optimizationHandler) policyAnalyze(c *gin.Context) {
	var req policyAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.ClusterID == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id is required")
		return
	}
	inputs := req.Inputs
	if inputs.Empty() {
		if !h.svc.HasCollector() {
			writeError(c, http.StatusBadRequest, "NO_INPUTS", "no observation bundle supplied and auto-collection is not configured")
			return
		}
		collected, err := h.svc.CollectPolicy(c.Request.Context(), req.ClusterID)
		if err != nil {
			writeError(c, http.StatusBadGateway, "COLLECT_FAILED", err.Error())
			return
		}
		inputs = collected
	}
	status := policy.Evaluate(req.ClusterID, inputs, time.Now())
	c.JSON(http.StatusOK, status)
}
