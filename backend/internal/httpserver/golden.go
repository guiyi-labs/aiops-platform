package httpserver

import (
	"context"
	"errors"
	"net/http"

	"k8s-aiops.local/backend/internal/golden"

	"github.com/gin-gonic/gin"
)

// goldenHandler exposes the M56 quality-report endpoints: a read-only
// GET for the latest report and a POST to trigger an async golden dataset
// replay (SystemOpsAdmin only).
type goldenHandler struct {
	service *golden.Service
}

// getQualityReport handles GET /api/v1/aiops/quality-report.
//
// Returns the most recently saved quality report JSON. If no report has
// been generated yet, returns 404 QUALITY_REPORT_NOT_FOUND.
func (h goldenHandler) getQualityReport(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "GOLDEN_UNAVAILABLE", "quality report service is not configured")
		return
	}
	report, err := h.service.GetLatestReport()
	if err != nil {
		if errors.Is(err, golden.ErrNoReport) {
			writeError(c, http.StatusNotFound, "QUALITY_REPORT_NOT_FOUND", "no quality report has been generated yet")
			return
		}
		writeError(c, http.StatusInternalServerError, "QUALITY_REPORT_READ_FAILED", "failed to read quality report")
		return
	}
	c.JSON(http.StatusOK, report)
}

// runQualityReplay handles POST /api/v1/aiops/quality-report/run.
//
// Triggers an async golden dataset replay. Returns 202 Accepted with the
// task ID; the caller can poll GET /api/v1/aiops/quality-report to see
// the latest report once the replay completes.
func (h goldenHandler) runQualityReplay(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "GOLDEN_UNAVAILABLE", "quality report service is not configured")
		return
	}
	taskID, err := h.service.RunReplay(context.Background())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "REPLAY_TRIGGER_FAILED", "failed to trigger golden dataset replay")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"task_id": taskID,
		"status":  string(golden.ReplayTaskRunning),
		"message": "golden dataset replay started; poll GET /api/v1/aiops/quality-report for the latest report",
	})
}
