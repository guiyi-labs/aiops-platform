package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/aisettings"
)

// aiSettingsHandler 暴露「模型接入」的读写与连通性测试。
//
// 密钥安全约定：
//   - 读取时只回 api_key_set 与 api_key_hint（末四位），**永不返回明文**；
//   - 写入时 api_key 为空表示「保持原密钥」；非空则重新加密覆盖；
//   - 测试接口用请求体里的值（未保存也可测），同样不落库、不回显。
type aiSettingsHandler struct {
	settings *aisettings.Service
	runtime  *aiexplain.Service
}

type aiSettingsView struct {
	Enabled               bool   `json:"enabled"`
	Protocol              string `json:"protocol"`
	BaseURL               string `json:"base_url"`
	Model                 string `json:"model"`
	APIKeySet             bool   `json:"api_key_set"`
	APIKeyHint            string `json:"api_key_hint"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	MaxOutputTokens       int    `json:"max_output_tokens"`
	DailyTokenBudget      int    `json:"daily_token_budget"`
	MaxConcurrentRequests int    `json:"max_concurrent_requests"`
	UpdatedAt             string `json:"updated_at,omitempty"`
	// Runtime 是解释服务的实时状态（启用与否、今日用量、可用性）。
	Runtime *aiexplain.RuntimeStatus `json:"runtime,omitempty"`
}

type aiSettingsUpdateRequest struct {
	Enabled               bool   `json:"enabled"`
	Protocol              string `json:"protocol"`
	BaseURL               string `json:"base_url"`
	Model                 string `json:"model"`
	APIKey                string `json:"api_key"`
	ClearAPIKey           bool   `json:"clear_api_key"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	MaxOutputTokens       int    `json:"max_output_tokens"`
	DailyTokenBudget      int    `json:"daily_token_budget"`
	MaxConcurrentRequests int    `json:"max_concurrent_requests"`
}

type aiProbeRequest struct {
	Protocol              string `json:"protocol"`
	BaseURL               string `json:"base_url"`
	Model                 string `json:"model"`
	APIKey                string `json:"api_key"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

func (h aiSettingsHandler) get(c *gin.Context) {
	current, err := h.settings.Load(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to load AI settings")
		return
	}
	view := aiSettingsView{
		Enabled:               current.Enabled,
		Protocol:              current.Protocol,
		BaseURL:               current.BaseURL,
		Model:                 current.Model,
		APIKeySet:             current.APIKey != "",
		APIKeyHint:            current.APIKeyHint,
		RequestTimeoutSeconds: int(current.RequestTimeout / time.Second),
		MaxOutputTokens:       current.MaxOutputTokens,
		DailyTokenBudget:      current.DailyTokenBudget,
		MaxConcurrentRequests: current.MaxConcurrentRequests,
	}
	if !current.UpdatedAt.IsZero() {
		view.UpdatedAt = current.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if h.runtime != nil {
		// 运行时状态读失败不应让配置页报错——配置本身仍然可编辑。
		if status, serr := h.runtime.Status(c.Request.Context()); serr == nil {
			view.Runtime = &status
		}
	}
	c.JSON(http.StatusOK, view)
}

func (h aiSettingsHandler) update(c *gin.Context) {
	var body aiSettingsUpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "request body must be valid JSON")
		return
	}
	actorID := currentActorID(c)
	saved, err := h.settings.Update(c.Request.Context(), aisettings.Settings{
		Enabled:               body.Enabled,
		Protocol:              body.Protocol,
		BaseURL:               body.BaseURL,
		Model:                 body.Model,
		APIKey:                body.APIKey,
		ClearAPIKey:           body.ClearAPIKey,
		RequestTimeout:        time.Duration(body.RequestTimeoutSeconds) * time.Second,
		MaxOutputTokens:       body.MaxOutputTokens,
		DailyTokenBudget:      body.DailyTokenBudget,
		MaxConcurrentRequests: body.MaxConcurrentRequests,
	}, actorID)
	if err != nil {
		writeAISettingsError(c, err)
		return
	}
	c.JSON(http.StatusOK, aiSettingsView{
		Enabled:               saved.Enabled,
		Protocol:              saved.Protocol,
		BaseURL:               saved.BaseURL,
		Model:                 saved.Model,
		APIKeySet:             saved.APIKey != "",
		APIKeyHint:            saved.APIKeyHint,
		RequestTimeoutSeconds: int(saved.RequestTimeout / time.Second),
		MaxOutputTokens:       saved.MaxOutputTokens,
		DailyTokenBudget:      saved.DailyTokenBudget,
		MaxConcurrentRequests: saved.MaxConcurrentRequests,
	})
}

// test 用「请求体里的值」探测连通性：允许在保存前先验证。
// api_key 留空时回落到已保存的密钥，避免用户为了一次测试重复粘贴。
func (h aiSettingsHandler) test(c *gin.Context) {
	var body aiProbeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "request body must be valid JSON")
		return
	}
	if body.BaseURL == "" || body.Model == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "base_url and model are required")
		return
	}
	apiKey := body.APIKey
	if apiKey == "" {
		if current, err := h.settings.Load(c.Request.Context()); err == nil {
			apiKey = current.APIKey
		}
	}
	result, err := aiexplain.Probe(c.Request.Context(), aiexplain.ProbeConfig{
		Protocol:        body.Protocol,
		BaseURL:         body.BaseURL,
		Model:           body.Model,
		APIKey:          apiKey,
		RequestTimeout:  time.Duration(body.RequestTimeoutSeconds) * time.Second,
		MaxOutputTokens: 16,
	})
	if err != nil {
		// 探测失败不是 500——是对端不可用，用 502 表达「上游失败」。
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "detail": err.Error(), "probe": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "detail": result.Detail, "probe": result})
}

func writeAISettingsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, aisettings.ErrInvalidBaseURL):
		writeError(c, http.StatusBadRequest, "INVALID_BASE_URL", err.Error())
	case errors.Is(err, aisettings.ErrInvalidProtocol):
		writeError(c, http.StatusBadRequest, "INVALID_PROTOCOL", err.Error())
	case errors.Is(err, aisettings.ErrMissingModel):
		writeError(c, http.StatusBadRequest, "MISSING_MODEL", err.Error())
	case errors.Is(err, aisettings.ErrMissingAPIKey):
		writeError(c, http.StatusBadRequest, "MISSING_API_KEY", err.Error())
	case errors.Is(err, aisettings.ErrBudgetBelowOutputLimit):
		writeError(c, http.StatusBadRequest, "BUDGET_BELOW_OUTPUT_LIMIT", err.Error())
	case errors.Is(err, aisettings.ErrOutputLimitTooLarge):
		writeError(c, http.StatusBadRequest, "OUTPUT_LIMIT_TOO_LARGE", err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to save AI settings")
	}
}
