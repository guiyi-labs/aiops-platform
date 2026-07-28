package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/requestctx"
)

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, errorResponse{
		Code:      code,
		Message:   message,
		RequestID: requestctx.RequestIDFrom(c.Request.Context()),
	})
}

func noRoute(c *gin.Context) {
	writeError(c, http.StatusNotFound, "ROUTE_NOT_FOUND", "the requested route does not exist")
}
