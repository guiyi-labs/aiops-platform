package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/appcatalog"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
)

type appCatalogHandler struct{ service *appcatalog.Service }

// ---------------------------------------------------------------------------
// Helm repository CRUD
// ---------------------------------------------------------------------------

type createRepoRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	URL         string `json:"url"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
}

func (h appCatalogHandler) createRepository(c *gin.Context) {
	var request createRepoRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "request must contain only the helm repository fields")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	repo, err := h.service.CreateRepository(c.Request.Context(), appcatalog.CreateRepositoryRequest{
		Name:        strings.TrimSpace(request.Name),
		DisplayName: strings.TrimSpace(request.DisplayName),
		URL:         strings.TrimSpace(request.URL),
		Username:    request.Username,
		Password:    request.Password,
	}, appcatalog.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		setAuditTarget(c, "HelmRepository", "", repo.Name)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, appcatalog.RepositoryViewFrom(repo))
		return
	}
	h.writeError(c, err, "unable to create helm repository")
}

func (h appCatalogHandler) listRepositories(c *gin.Context) {
	repos, err := h.service.ListRepositories(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "unable to list helm repositories")
		return
	}
	views := make([]appcatalog.RepositoryView, 0, len(repos))
	for _, repo := range repos {
		views = append(views, appcatalog.RepositoryViewFrom(repo))
	}
	c.JSON(http.StatusOK, gin.H{"items": views, "total": len(views)})
}

func (h appCatalogHandler) getRepository(c *gin.Context) {
	repoID, ok := parsePathParamInt64(c, "repo_id")
	if !ok {
		return
	}
	repo, err := h.service.GetRepository(c.Request.Context(), repoID)
	if err == nil {
		c.JSON(http.StatusOK, appcatalog.RepositoryViewFrom(repo))
		return
	}
	h.writeError(c, err, "unable to read helm repository")
}

func (h appCatalogHandler) deleteRepository(c *gin.Context) {
	repoID, ok := parsePathParamInt64(c, "repo_id")
	if !ok {
		return
	}
	setAuditTarget(c, "HelmRepository", "", c.Param("repo_id"))
	if err := h.service.DeleteRepository(c.Request.Context(), repoID); err != nil {
		h.writeError(c, err, "unable to delete helm repository")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Chart listing / detail (read-only index.yaml fetch)
// ---------------------------------------------------------------------------

func (h appCatalogHandler) listCharts(c *gin.Context) {
	repoID, ok := parsePathParamInt64(c, "repo_id")
	if !ok {
		return
	}
	charts, err := h.service.ListCharts(c.Request.Context(), repoID)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"items": charts, "total": len(charts)})
		return
	}
	h.writeError(c, err, "unable to list charts")
}

func (h appCatalogHandler) getChart(c *gin.Context) {
	repoID, ok := parsePathParamInt64(c, "repo_id")
	if !ok {
		return
	}
	chartName := strings.TrimSpace(c.Param("chart_name"))
	if chartName == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "chart_name is required")
		return
	}
	detail, err := h.service.GetChart(c.Request.Context(), repoID, chartName)
	if err == nil {
		c.JSON(http.StatusOK, detail)
		return
	}
	h.writeError(c, err, "unable to read chart detail")
}

// ---------------------------------------------------------------------------
// Deploy preview + execute (M19 controlled-operation contract)
// ---------------------------------------------------------------------------

type deployPreviewRequest struct {
	RepoID          int64  `json:"repo_id"`
	ChartName       string `json:"chart_name"`
	ChartVersion    string `json:"chart_version"`
	TargetClusterID int64  `json:"target_cluster_id"`
	TargetNamespace string `json:"target_namespace"`
	ReleaseName     string `json:"release_name"`
	ValuesYAML      string `json:"values_yaml,omitempty"`
}

type executeDeployRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h appCatalogHandler) previewDeploy(c *gin.Context) {
	var request deployPreviewRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "request must contain only the deploy preview fields")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	setAuditTarget(c, "AppCatalogPlan", strings.TrimSpace(request.TargetNamespace), strings.TrimSpace(request.ReleaseName))
	plan, err := h.service.Preview(c.Request.Context(), appcatalog.DeployPreviewRequest{
		RepoID:          request.RepoID,
		ChartName:       strings.TrimSpace(request.ChartName),
		ChartVersion:    strings.TrimSpace(request.ChartVersion),
		TargetClusterID: request.TargetClusterID,
		TargetNamespace: strings.TrimSpace(request.TargetNamespace),
		ReleaseName:     strings.TrimSpace(request.ReleaseName),
		ValuesYAML:      request.ValuesYAML,
	}, appcatalog.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if plan.ID != "" {
		setAuditTarget(c, "AppCatalogPlan", plan.TargetNamespace, plan.ID)
		setAuditClusterID(c, plan.TargetClusterID)
	}
	if err == nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, plan)
		return
	}
	h.writeError(c, err, "unable to preview chart deployment")
}

func (h appCatalogHandler) executeDeploy(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	if len(planID) != 36 {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid plan identifier")
		return
	}
	var request executeDeployRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	setAuditTarget(c, "AppCatalogPlan", "", planID)
	plan, err := h.service.Execute(c.Request.Context(), planID, request.ConfirmationToken, idempotencyKey)
	if plan.TargetClusterID > 0 {
		setAuditClusterID(c, plan.TargetClusterID)
	}
	if err == nil {
		c.JSON(http.StatusOK, plan)
		return
	}
	h.writeError(c, err, "unable to execute chart deployment")
}

func (h appCatalogHandler) getPlan(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	if len(planID) != 36 {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid plan identifier")
		return
	}
	plan, err := h.service.GetPlan(c.Request.Context(), planID)
	if err == nil {
		c.JSON(http.StatusOK, plan)
		return
	}
	h.writeError(c, err, "unable to read app catalog plan")
}

func (h appCatalogHandler) listPlans(c *gin.Context) {
	clusterID, ok := parseClusterQuery(c, "cluster_id")
	if !ok {
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	plans, err := h.service.ListPlans(c.Request.Context(), clusterID, namespace)
	if err != nil {
		h.writeError(c, err, "unable to list app catalog plans")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": plans, "total": len(plans)})
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (appCatalogHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, appcatalog.ErrRepoNotFound):
		writeError(c, http.StatusNotFound, "REPO_NOT_FOUND", "helm repository does not exist")
	case errors.Is(err, appcatalog.ErrRepoNameExists):
		writeError(c, http.StatusConflict, "REPO_NAME_EXISTS", "helm repository name already exists")
	case errors.Is(err, appcatalog.ErrRepoURLInvalid):
		writeError(c, http.StatusBadRequest, "REPO_URL_INVALID", "helm repository URL is invalid")
	case errors.Is(err, appcatalog.ErrRepoUnreachable):
		writeError(c, http.StatusBadGateway, "REPO_UNREACHABLE", "helm repository index.yaml could not be fetched")
	case errors.Is(err, appcatalog.ErrChartNotFound):
		writeError(c, http.StatusNotFound, "CHART_NOT_FOUND", "chart not found in repository")
	case errors.Is(err, appcatalog.ErrPlanNotFound):
		writeError(c, http.StatusNotFound, "PLAN_NOT_FOUND", "app catalog plan does not exist")
	case errors.Is(err, appcatalog.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "app catalog request is invalid")
	case errors.Is(err, appcatalog.ErrNamespaceMissing):
		writeError(c, http.StatusConflict, "NAMESPACE_MISSING", "target namespace does not exist")
	case errors.Is(err, appcatalog.ErrClusterUnavailable):
		writeError(c, http.StatusConflict, "CLUSTER_UNAVAILABLE", "target cluster is unavailable")
	case errors.Is(err, appcatalog.ErrPreviewFailed):
		writeError(c, http.StatusBadRequest, "PREVIEW_FAILED", "server-side dry-run rejected the HelmRelease manifest")
	case errors.Is(err, appcatalog.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "CONFIRMATION_INVALID", "confirmation token is invalid")
	case errors.Is(err, appcatalog.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 8 to 128 characters")
	case errors.Is(err, appcatalog.ErrExpired):
		writeError(c, http.StatusGone, "PLAN_EXPIRED", "app catalog plan has expired")
	case errors.Is(err, appcatalog.ErrInProgress):
		writeError(c, http.StatusConflict, "PLAN_IN_PROGRESS", "app catalog plan is already executing")
	case errors.Is(err, appcatalog.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "PLAN_ALREADY_USED", "app catalog plan was already used with another idempotency key")
	case errors.Is(err, appcatalog.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "EXECUTION_FAILED", "Kubernetes API rejected or failed the HelmRelease creation")
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes resource does not exist")
	case errors.Is(err, k8sgateway.ErrResourceConflict):
		writeError(c, http.StatusConflict, "RESOURCE_CONFLICT", "Kubernetes resource already exists")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

// parsePathParamInt64 reads an int64 path parameter.
func parsePathParamInt64(c *gin.Context, key string) (int64, bool) {
	raw := strings.TrimSpace(c.Param(key))
	if raw == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", key+" path parameter is required")
		return 0, false
	}
	var value int64
	if _, err := parseInt64(raw, &value); err != nil || value <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", key+" must be a positive integer")
		return 0, false
	}
	return value, true
}

func parseInt64(raw string, target *int64) (int, error) {
	n := 0
	sign := int64(1)
	if len(raw) > 0 && raw[0] == '-' {
		sign = -1
		n = 1
	}
	var val int64
	for ; n < len(raw); n++ {
		ch := raw[n]
		if ch < '0' || ch > '9' {
			return 0, errors.New("invalid integer")
		}
		val = val*10 + int64(ch-'0')
	}
	*target = sign * val
	return n, nil
}
