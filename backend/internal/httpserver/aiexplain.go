package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/requestctx"
)

type aiExplanationHandler struct{ service *aiexplain.Service }

type aiExplanationFeedbackRequest struct {
	Verdict string `json:"verdict" binding:"required"`
	Comment string `json:"comment"`
}

func (h aiExplanationHandler) generate(c *gin.Context) {
	id, ok := diagnosisID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "DiagnosisAIExplanation", "", strconv.FormatInt(id, 10))
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	item, err := h.service.Generate(c.Request.Context(), id, aiexplain.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		c.JSON(http.StatusCreated, item)
		return
	}
	switch {
	case errors.Is(err, diagnosis.ErrRecordNotFound):
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
	case errors.Is(err, aiexplain.ErrDisabled):
		writeError(c, http.StatusServiceUnavailable, "AI_DISABLED", "AI explanation is not enabled; deterministic diagnosis remains available")
	case errors.Is(err, aiexplain.ErrNoEvidence):
		writeError(c, http.StatusUnprocessableEntity, "AI_EVIDENCE_REQUIRED", "diagnosis has no evidence that can be cited")
	case errors.Is(err, aiexplain.ErrBudgetExceeded):
		writeError(c, http.StatusTooManyRequests, "AI_BUDGET_EXCEEDED", "remaining daily AI token budget is insufficient for this explanation; deterministic diagnosis remains available")
	case errors.Is(err, aiexplain.ErrBusy):
		writeError(c, http.StatusTooManyRequests, "AI_BUSY", "AI explanation concurrency limit is reached; retry later")
	case errors.Is(err, aiexplain.ErrProviderFailure):
		writeError(c, http.StatusBadGateway, "AI_PROVIDER_ERROR", "AI provider is temporarily unavailable; deterministic diagnosis remains available")
	case errors.Is(err, aiexplain.ErrInvalidOutput):
		writeError(c, http.StatusBadGateway, "AI_INVALID_OUTPUT", "AI provider returned an explanation that failed citation validation")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to generate AI explanation")
	}
}

func (h aiExplanationHandler) status(c *gin.Context) {
	status, err := h.service.Status(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to read AI runtime status")
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h aiExplanationHandler) quality(c *gin.Context) {
	summary, err := h.service.Quality(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to summarize AI explanation quality")
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h aiExplanationHandler) feedback(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("explanation_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_EXPLANATION_ID", "explanation_id must be a positive integer")
		return
	}
	setAuditTarget(c, "AIExplanationFeedback", "", strconv.FormatInt(id, 10))
	var request aiExplanationFeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil || len([]rune(request.Comment)) > 1000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "verdict is required and comment must not exceed 1000 characters")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	result, err := h.service.AddFeedback(c.Request.Context(), id, aiexplain.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName}, strings.TrimSpace(request.Verdict), strings.TrimSpace(request.Comment))
	if err == nil {
		c.JSON(http.StatusCreated, result)
		return
	}
	switch {
	case errors.Is(err, aiexplain.ErrInvalidFeedback):
		writeError(c, http.StatusBadRequest, "INVALID_AI_FEEDBACK", "verdict must be helpful, partially_helpful or not_helpful")
	case errors.Is(err, aiexplain.ErrExplanationNotFound):
		writeError(c, http.StatusNotFound, "AI_EXPLANATION_NOT_FOUND", "AI explanation does not exist")
	case errors.Is(err, aiexplain.ErrFeedbackExists):
		writeError(c, http.StatusConflict, "AI_FEEDBACK_EXISTS", "the current user has already rated this explanation")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to save AI explanation feedback")
	}
}

func (h aiExplanationHandler) list(c *gin.Context) {
	id, ok := diagnosisID(c)
	if !ok {
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	items, err := h.service.List(c.Request.Context(), id, metadata.ActorID)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
		return
	}
	if errors.Is(err, diagnosis.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
		return
	}
	writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list AI explanations")
}
