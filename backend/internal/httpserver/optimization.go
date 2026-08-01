package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/optimization"
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
	status := cis.Evaluate(req.ClusterID, req.Inputs, time.Now())
	c.JSON(http.StatusOK, status)
}

type finopsAnalyzeRequest struct {
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
		writeError(c, http.StatusBadRequest, "NO_INPUTS", "at least one container input is required")
		return
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
	status := deprecatedapi.Check(req.ClusterID, req.TargetVersion, req.Objects, time.Now())
	c.JSON(http.StatusOK, status)
}
