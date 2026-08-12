package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/requestctx"
)

// replay returns the read-only M81 insight-chain replay for a diagnosis.
// The replay is assembled strictly from stored artifacts: the diagnosis
// record (creation + evidence timeline + activities) plus AI explanations and
// remediation plans when their services are configured. Nothing is
// regenerated or fabricated; a missing optional service simply contributes no
// steps.
func (h diagnosisHandler) replay(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("diagnosis_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_DIAGNOSIS_ID", "diagnosis_id must be a positive integer")
		return
	}
	record, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, diagnosis.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
			return
		}
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to read diagnosis")
		return
	}

	explanations := replayExplanations(c, h.explanations, id)
	remediations := replayRemediations(c, h.remediations, id)
	view := diagnosis.BuildReplay(record, explanations, remediations)
	c.JSON(http.StatusOK, view)
}

func replayExplanations(c *gin.Context, service *aiexplain.Service, diagnosisID int64) []diagnosis.ExplanationSnapshot {
	if service == nil {
		return nil
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	items, err := service.List(c.Request.Context(), diagnosisID, metadata.ActorID)
	if err != nil {
		return nil // stored-artifact replay stays valid without optional extras
	}
	snapshots := make([]diagnosis.ExplanationSnapshot, 0, len(items))
	for _, item := range items {
		snapshots = append(snapshots, diagnosis.ExplanationSnapshot{
			ID:        item.ID,
			Provider:  item.Provider,
			Model:     item.Model,
			Summary:   strings.TrimSpace(item.Summary),
			Feedback:  explanationFeedbackSummary(item.FeedbackSummary),
			CreatedAt: item.CreatedAt,
		})
	}
	return snapshots
}

func explanationFeedbackSummary(summary aiexplain.FeedbackSummary) string {
	if summary.Total == 0 {
		return ""
	}
	return strconv.Itoa(summary.Total) + " 条反馈"
}

func replayRemediations(c *gin.Context, service *remediation.Service, diagnosisID int64) []diagnosis.RemediationSnapshot {
	if service == nil {
		return nil
	}
	plans, err := service.List(c.Request.Context(), diagnosisID)
	if err != nil {
		return nil
	}
	snapshots := make([]diagnosis.RemediationSnapshot, 0, len(plans))
	for _, plan := range plans {
		snapshots = append(snapshots, diagnosis.RemediationSnapshot{
			ID:         plan.ID,
			Action:     plan.Action,
			Status:     plan.Status,
			TargetName: plan.TargetName,
			CreatedAt:  plan.CreatedAt,
			ExecutedAt: plan.ExecutedAt,
		})
	}
	return snapshots
}
