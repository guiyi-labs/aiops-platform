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
		writeError(c, http.StatusBadGateway, "AI_PROVIDER_ERROR", withCause("AI provider is temporarily unavailable; deterministic diagnosis remains available", err))
	case errors.Is(err, aiexplain.ErrInvalidOutput):
		// 前半句保持稳定文案（调用方可能按它做匹配），冒号后附上具体校验失败原因：
		// 未知证据 / 字段为空 / 输出被截断 / JSON 不合法，处置手段完全不同。
		writeError(c, http.StatusBadGateway, "AI_INVALID_OUTPUT", withCause("AI provider returned an explanation that failed validation", err))
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

// coverage handles GET /api/v1/ai/coverage — the M112-4 explanation
// coverage dashboard. Read-only aggregation over ai_explanations plus the
// quality feedback baseline.
func (h aiExplanationHandler) coverage(c *gin.Context) {
	summary, err := h.service.Coverage(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to aggregate AI explanation coverage")
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

// withCause 把被 errors.Is 判定过的错误细节拼到稳定文案后面。
//
// 存在的理由：ErrInvalidOutput / ErrProviderFailure 名下各有多条完全不同的真实原因
// （未知证据 ID、输出被 max_output_tokens 截断、密钥失效、限流……），处置手段互不相同。
// 只回一句笼统文案，运营者就只能靠猜；而写入 message 又必须保持前缀稳定，
// 以免破坏按文案匹配的调用方。因此前面不动、冒号后缀原因。
//
// 不泄漏模型全文：aiexplain 侧的错误只带 120 字符摘要，已在生成处截断。
func withCause(message string, err error) string {
	detail := strings.TrimSpace(err.Error())
	for _, prefix := range []string{
		aiexplain.ErrInvalidOutput.Error(),
		aiexplain.ErrProviderFailure.Error(),
	} {
		detail = strings.TrimSpace(strings.TrimPrefix(detail, prefix))
	}
	detail = strings.TrimPrefix(detail, ":")
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return message
	}
	return message + ": " + detail
}
