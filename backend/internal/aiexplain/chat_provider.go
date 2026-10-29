package aiexplain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatCompletionsProvider 走传统 Chat Completions 协议（POST {base_url}/chat/completions），
// 用 response_format.json_schema 的严格模式约束输出。
//
// 为什么需要它：ResponsesProvider 打的是 /responses（OpenAI Responses API），而绝大多数
// OpenAI 兼容端点（DeepSeek、通义、Kimi、vLLM、Ollama 等）只实现 /chat/completions，
// 在 /responses 上直接 404。两种协议的结构化输出能力等价，因此把它做成同一 Provider
// 接口下的第二个实现，由配置项 protocol 选择，其余四重约束（引用校验、token 预算、
// 并发准入、确定性回退）完全复用，无需感知协议差异。
type ChatCompletionsProvider struct {
	endpoint        string
	apiKey          string
	model           string
	client          *http.Client
	maxOutputTokens int
}

func NewChatCompletionsProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) *ChatCompletionsProvider {
	return &ChatCompletionsProvider{
		endpoint:        strings.TrimRight(baseURL, "/") + "/chat/completions",
		apiKey:          apiKey,
		model:           model,
		client:          &http.Client{Timeout: timeout},
		maxOutputTokens: maxOutputTokens,
	}
}

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	ResponseFormat chatResponseFmt   `json:"response_format"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Temperature    float64           `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponseFmt struct {
	Type       string         `json:"type"`
	JSONSchema chatJSONSchema `json:"json_schema"`
}

type chatJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (p *ChatCompletionsProvider) Generate(ctx context.Context, prompt Prompt) (ProviderResult, error) {
	payload := chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.Input},
		},
		ResponseFormat: chatResponseFmt{
			Type: "json_schema",
			JSONSchema: chatJSONSchema{
				Name:   "kubernetes_diagnosis_explanation",
				Strict: true,
				Schema: explanationSchema(),
			},
		},
		MaxTokens:   p.maxOutputTokens,
		Temperature: 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProviderResult{}, fmt.Errorf("encode provider request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return ProviderResult{}, fmt.Errorf("build provider request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: transport error", ErrProviderFailure)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes))
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: read response", ErrProviderFailure)
	}

	// 把上游的状态码与错误摘要带回，便于控制台的连通性测试给出可操作的提示
	// （401 密钥错、404 路径错、429 限流）。错误摘要截断，避免把整页 HTML 塞进日志。
	if resp.StatusCode >= 400 {
		return ProviderResult{}, providerStatusError(resp.StatusCode, raw)
	}

	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ProviderResult{}, fmt.Errorf("%w: decode response", ErrInvalidOutput)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return ProviderResult{}, fmt.Errorf("%w: %s", ErrProviderFailure, truncate(decoded.Error.Message, 200))
	}
	if len(decoded.Choices) == 0 {
		return ProviderResult{}, fmt.Errorf("%w: empty choices", ErrInvalidOutput)
	}

	result, err := decodeStructuredOutput(decoded.Choices[0].Message.Content, prompt.EvidenceIDs)
	if err != nil {
		return ProviderResult{}, err
	}
	result.Provider = "chat-completions-compatible"
	result.Model = firstNonEmpty(decoded.Model, p.model)
	result.ProviderResponseID = decoded.ID
	result.InputTokens = decoded.Usage.PromptTokens
	result.OutputTokens = decoded.Usage.CompletionTokens
	return result, nil
}

// providerStatusError 把上游 HTTP 状态映射成可读错误，供连通性测试直接展示。
func providerStatusError(status int, body []byte) error {
	detail := strings.TrimSpace(string(body))
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		detail = envelope.Error.Message
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: HTTP %d（密钥无效或无权限）: %s", ErrProviderFailure, status, truncate(detail, 200))
	case http.StatusNotFound:
		return fmt.Errorf("%w: HTTP 404（接口路径不存在，检查 base_url 与协议选择）: %s", ErrProviderFailure, truncate(detail, 200))
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: HTTP 429（限流或额度不足）: %s", ErrProviderFailure, truncate(detail, 200))
	default:
		return fmt.Errorf("%w: HTTP %d: %s", ErrProviderFailure, status, truncate(detail, 200))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
