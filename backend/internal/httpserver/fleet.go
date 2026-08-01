package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/fleet"
)

type fleetHandler struct {
	service *fleet.Service
	authz   *authz.Service
}

func (h fleetHandler) health(c *gin.Context) {
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be an integer from 1 through 20")
			return
		}
		limit = value
	}
	visible, _, err := authorizedClusterFilter(h.authz, c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FLEET_QUERY_FAILED", "unable to evaluate access scope")
		return
	}
	response, err := h.service.Compare(c.Request.Context(), limit, visible)
	if errors.Is(err, fleet.ErrInvalidLimit) {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be an integer from 1 through 20")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FLEET_QUERY_FAILED", "unable to load the cluster directory")
		return
	}
	c.JSON(http.StatusOK, response)
}
