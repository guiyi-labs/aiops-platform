package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/globalsearch"
)

type globalSearchHandler struct {
	service *globalsearch.Service
	authz   *authz.Service
}

func (h globalSearchHandler) search(c *gin.Context) {
	kinds, err := globalsearch.ParseKinds(c.Query("kinds"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "kinds may contain only pods, deployments, services and ingresses")
		return
	}
	clusterLimit, ok := boundedSearchInteger(c, "cluster_limit", 20, 20)
	if !ok {
		return
	}
	resultLimit, ok := boundedSearchInteger(c, "limit", 50, 100)
	if !ok {
		return
	}
	visible, _, err := authorizedClusterFilter(h.authz, c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "GLOBAL_SEARCH_FAILED", "unable to evaluate access scope")
		return
	}
	response, err := h.service.Search(c.Request.Context(), globalsearch.Query{
		Term: c.Query("q"), Namespace: c.Query("namespace"), Kinds: kinds,
		ClusterLimit: clusterLimit, ResultLimit: resultLimit,
	}, visible)
	if errors.Is(err, globalsearch.ErrInvalidQuery) {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "q, namespace or search limits are invalid")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "GLOBAL_SEARCH_FAILED", "unable to load the cluster directory")
		return
	}
	c.JSON(http.StatusOK, response)
}

func boundedSearchInteger(c *gin.Context, name string, fallback, maximum int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", name+" must be an integer from 1 through "+strconv.Itoa(maximum))
		return 0, false
	}
	return value, true
}
