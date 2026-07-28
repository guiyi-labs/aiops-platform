package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type readinessProbe interface {
	Ping(context.Context) error
}

type healthHandler struct {
	probe   readinessProbe
	version string
	now     func() time.Time
}

type healthResponse struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
}

func (h healthHandler) live(c *gin.Context) {
	c.JSON(http.StatusOK, h.response("ok"))
}

func (h healthHandler) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.probe.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, h.response("unavailable"))
		return
	}

	c.JSON(http.StatusOK, h.response("ready"))
}

func (h healthHandler) response(status string) healthResponse {
	return healthResponse{
		Status:    status,
		Service:   "k8s-aiops-api",
		Version:   h.version,
		CheckedAt: h.now().UTC(),
	}
}
