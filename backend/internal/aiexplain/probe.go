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

// ProbeConfig 是连通性测试的输入：用「用户刚填的那份值」而不是已保存的配置，
// 这样可以在保存前先验证，避免把不可用的配置写进库。
type ProbeConfig struct {
	Protocol        string
	BaseURL         string
	Model           string
	APIKey          string
	RequestTimeout  time.Duration
	MaxOutputTokens int
}

// ProbeResult 描述一次探测的结果。OK 为 true 表示至少有一条路径可用。
type ProbeResult struct {
	OK         bool   `json:"ok"`
	Method     string `json:"method"` // models | completion
	LatencyMS  int64  `json:"latency_ms"`
	Detail     string `json:"detail,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// Probe 验证 base_url + 密钥 + 模型是否可用。
//
// 顺序：先打 GET {base}/models（零 token 成本，OpenAI 兼容端点普遍实现）；
// 若该端点 404（部分网关不实现 discovery），退回一次极小 completion 请求。
// 两者都失败时返回带状态码与可操作提示的错误（密钥错 / 路径错 / 限流）。
func Probe(ctx context.Context, cfg ProbeConfig) (ProbeResult, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return ProbeResult{}, fmt.Errorf("%w: base_url is empty", ErrProviderFailure)
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	started := time.Now()
	res := probeModels(ctx, client, base, cfg.APIKey)
	res.LatencyMS = time.Since(started).Milliseconds()
	if res.OK {
		return res, nil
	}
	if res.HTTPStatus != http.StatusNotFound {
		return res, fmt.Errorf("%w: %s", ErrProviderFailure, res.Detail)
	}

	// /models 不存在（很多只做推理的网关如此）——改用极小 completion 探测。
	started = time.Now()
	res2 := probeCompletion(ctx, client, base, cfg)
	res2.LatencyMS = time.Since(started).Milliseconds()
	if res2.OK {
		return res2, nil
	}
	return res2, fmt.Errorf("%w: %s", ErrProviderFailure, res2.Detail)
}

func probeModels(ctx context.Context, client *http.Client, base, apiKey string) ProbeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return ProbeResult{Detail: "build request: " + err.Error()}
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		// 网络层错误不暴露内部地址细节，只给可操作描述。
		return ProbeResult{Detail: "无法连接到 " + hostOnly(base) + "（DNS/网络/证书问题）"}
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var listing struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &listing)
		detail := "端点可达，密钥有效"
		if len(listing.Data) > 0 {
			detail = fmt.Sprintf("端点可达，密钥有效，可见 %d 个模型", len(listing.Data))
		}
		return ProbeResult{OK: true, Method: "models", HTTPStatus: resp.StatusCode, Detail: detail}
	}
	return ProbeResult{Method: "models", HTTPStatus: resp.StatusCode, Detail: statusDetail(resp.StatusCode, body)}
}

func probeCompletion(ctx context.Context, client *http.Client, base string, cfg ProbeConfig) ProbeResult {
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = ProtocolResponsesConstant
	}
	var endpoint string
	var payload any
	if protocol == ProtocolChatCompletionsConstant {
		endpoint = base + "/chat/completions"
		payload = map[string]any{
			"model":      cfg.Model,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens": 16,
		}
	} else {
		endpoint = base + "/responses"
		payload = map[string]any{
			"model":             cfg.Model,
			"input":             []map[string]string{{"role": "user", "content": "ping"}},
			"max_output_tokens": 16,
			"store":             false,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProbeResult{Detail: "encode payload: " + err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ProbeResult{Detail: "build request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{Method: "completion", Detail: "无法连接到 " + hostOnly(base) + "（DNS/网络/证书问题）"}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ProbeResult{OK: true, Method: "completion", HTTPStatus: resp.StatusCode, Detail: "推理请求成功（协议与模型均可用）"}
	}
	return ProbeResult{Method: "completion", HTTPStatus: resp.StatusCode, Detail: statusDetail(resp.StatusCode, respBody)}
}

// 协议常量在本包的独立副本，避免把 aisettings 反引入 aiexplain（会形成循环依赖）。
const (
	ProtocolResponsesConstant       = "responses"
	ProtocolChatCompletionsConstant = "chat_completions"
)

func statusDetail(status int, body []byte) string {
	detail := strings.TrimSpace(string(body))
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		if envelope.Error.Message != "" {
			detail = envelope.Error.Message
		} else if envelope.Message != "" {
			detail = envelope.Message
		}
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("HTTP %d：密钥无效或无权限（%s）", status, truncate(detail, 180))
	case http.StatusNotFound:
		return fmt.Sprintf("HTTP 404：接口路径不存在，检查 base_url 是否需要带 /v1，以及协议是否选对（%s）", truncate(detail, 180))
	case http.StatusTooManyRequests:
		return fmt.Sprintf("HTTP 429：限流或额度不足（%s）", truncate(detail, 180))
	default:
		return fmt.Sprintf("HTTP %d：%s", status, truncate(detail, 180))
	}
}

func hostOnly(raw string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if i := strings.IndexAny(trimmed, "/?#"); i >= 0 {
		trimmed = trimmed[:i]
	}
	return trimmed
}
