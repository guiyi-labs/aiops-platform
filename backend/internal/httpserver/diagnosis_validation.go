package httpserver

import (
	"github.com/gin-gonic/gin"
	"k8s-aiops.local/backend/internal/diagnosisbench"
	"net/http"
)

// validateDiagnosis replays only the embedded fixed corpus through production
// rule functions. It cannot read or mutate cluster resources.
func validateDiagnosis(c *gin.Context) {
	report, err := diagnosisbench.Run()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "VALIDATION_FAILED", "unable to evaluate the fixed diagnosis corpus")
		return
	}
	c.JSON(http.StatusOK, report)
}
