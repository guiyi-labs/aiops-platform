package aiexplain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxProviderResponseBytes = 1 << 20

type Provider interface {
	Generate(context.Context, Prompt) (ProviderResult, error)
}

type ResponsesProvider struct {
	endpoint        string
	apiKey          string
	model           string
	client          *http.Client
	maxOutputTokens int
}

func NewResponsesProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) *ResponsesProvider {
	return &ResponsesProvider{endpoint: strings.TrimRight(baseURL, "/") + "/responses", apiKey: apiKey, model: model, client: &http.Client{Timeout: timeout}, maxOutputTokens: maxOutputTokens}
}

type responsesRequest struct {
	Model           string           `json:"model"`
	Store           bool             `json:"store"`
	Input           []responsesInput `json:"input"`
	Text            responsesText    `json:"text"`
	MaxOutputTokens int              `json:"max_output_tokens"`
}

type responsesInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responsesText struct {
	Format responsesFormat `json:"format"`
}

type responsesFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

func (p *ResponsesProvider) Generate(ctx context.Context, prompt Prompt) (ProviderResult, error) {
	body, err := json.Marshal(responsesRequest{Model: p.model, Store: false, Input: []responsesInput{{Role: "system", Content: prompt.System}, {Role: "user", Content: prompt.Input}}, Text: responsesText{Format: responsesFormat{Type: "json_schema", Name: "kubernetes_diagnosis_explanation", Strict: true, Schema: explanationSchema()}}, MaxOutputTokens: p.maxOutputTokens})
	if err != nil {
		return ProviderResult{}, fmt.Errorf("encode provider request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return ProviderResult{}, fmt.Errorf("create provider request: %w", err)
	}
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: request: %v", ErrProviderFailure, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxProviderResponseBytes))
		return ProviderResult{}, fmt.Errorf("%w: HTTP %d", ErrProviderFailure, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil || len(raw) > maxProviderResponseBytes {
		return ProviderResult{}, fmt.Errorf("%w: response exceeds limit", ErrProviderFailure)
	}
	var decoded struct {
		ID     string `json:"id"`
		Model  string `json:"model"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ProviderResult{}, fmt.Errorf("%w: decode response", ErrInvalidOutput)
	}
	// Responses API 用 status=incomplete + incomplete_details.reason 表达未跑完
	// （最常见是 max_output_tokens）。不先判这个，截断会伪装成 malformed JSON。
	if decoded.Status == "incomplete" {
		return ProviderResult{}, fmt.Errorf("%w: provider reported incomplete output (reason=%s); raise max_output_tokens in model settings", ErrInvalidOutput, firstNonEmpty(decoded.IncompleteDetails.Reason, "unknown"))
	}
	var outputText string
	for _, output := range decoded.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" {
				outputText += content.Text
			}
		}
	}
	result, err := decodeStructuredOutput(outputText, prompt.EvidenceIDs)
	if err != nil {
		return ProviderResult{}, err
	}
	result.Provider, result.Model, result.ProviderResponseID = "responses-compatible", firstNonEmpty(decoded.Model, p.model), decoded.ID
	result.InputTokens, result.OutputTokens = decoded.Usage.InputTokens, decoded.Usage.OutputTokens
	return result, nil
}

// decodeStructuredOutput 校验并解析模型输出。
//
// 每个失败分支都必须说明**为什么**失败：早先所有分支共用「failed citation validation」
// 一句笼统文案，导致接新模型时无从判断是该升 max_output_tokens、该修提示词、还是
// 该换协议。校验逻辑本身（哪些算通过）没变，变的是可诊断性。
func decodeStructuredOutput(raw string, evidenceIDs map[string]struct{}) (ProviderResult, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ProviderResult{}, fmt.Errorf("%w: provider returned empty content", ErrInvalidOutput)
	}
	// 容忍 ```json 围栏：json_schema 严格模式下本不该出现，但兼容端点里很常见。
	// 拆围栏的代价远低于让整次调用失败。
	candidate := stripCodeFence(trimmed)

	var output struct {
		Summary            string              `json:"summary"`
		Analysis           string              `json:"analysis"`
		RecommendedActions []RecommendedAction `json:"recommended_actions"`
		Citations          []Citation          `json:"citations"`
	}
	if err := json.Unmarshal([]byte(candidate), &output); err != nil {
		return ProviderResult{}, fmt.Errorf("%w: malformed JSON (%s); response starts with: %s", ErrInvalidOutput, err.Error(), truncateRunes(trimmed, 120))
	}
	var missing []string
	if strings.TrimSpace(output.Summary) == "" {
		missing = append(missing, "summary")
	}
	if strings.TrimSpace(output.Analysis) == "" {
		missing = append(missing, "analysis")
	}
	if len(output.Citations) == 0 {
		missing = append(missing, "citations")
	}
	if len(missing) > 0 {
		return ProviderResult{}, fmt.Errorf("%w: required fields empty (%s)", ErrInvalidOutput, strings.Join(missing, ", "))
	}

	available := sortedEvidenceIDs(evidenceIDs)
	for _, citation := range output.Citations {
		if strings.TrimSpace(citation.Claim) == "" {
			return ProviderResult{}, fmt.Errorf("%w: citation %q has an empty claim", ErrInvalidOutput, citation.EvidenceID)
		}
		if _, ok := evidenceIDs[citation.EvidenceID]; !ok {
			return ProviderResult{}, fmt.Errorf("%w: citation references unknown evidence %q (available: %s)", ErrInvalidOutput, citation.EvidenceID, available)
		}
	}
	for _, action := range output.RecommendedActions {
		if strings.TrimSpace(action.Action) == "" {
			return ProviderResult{}, fmt.Errorf("%w: recommended action with empty text", ErrInvalidOutput)
		}
		if action.Priority != "high" && action.Priority != "medium" && action.Priority != "low" {
			return ProviderResult{}, fmt.Errorf("%w: recommended action %q has invalid priority %q (must be high|medium|low)", ErrInvalidOutput, truncateRunes(action.Action, 40), action.Priority)
		}
		for _, id := range action.EvidenceIDs {
			if _, ok := evidenceIDs[id]; !ok {
				return ProviderResult{}, fmt.Errorf("%w: recommended action %q references unknown evidence %q (available: %s)", ErrInvalidOutput, truncateRunes(action.Action, 40), id, available)
			}
		}
	}
	return ProviderResult{Summary: strings.TrimSpace(output.Summary), Analysis: strings.TrimSpace(output.Analysis), RecommendedActions: output.RecommendedActions, Citations: output.Citations}, nil
}

// stripCodeFence 去掉模型常见的 ``` 围栏。只处理「整段被一个围栏包住」的情形，
// 内容里含反引号但未包围栏时原样返回。
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	firstNewline := strings.IndexByte(s, '\n')
	if firstNewline < 0 {
		return s
	}
	body := s[firstNewline+1:]
	if end := strings.LastIndex(body, "```"); end >= 0 {
		body = body[:end]
	}
	return strings.TrimSpace(body)
}

// sortedEvidenceIDs 给出稳定的可用证据 ID 列表，用于错误提示。
// 排序后再拼接，避免 map 遍历顺序让同一次失败在不同次调用里报出不同文案。
func sortedEvidenceIDs(evidenceIDs map[string]struct{}) string {
	ids := make([]string, 0, len(evidenceIDs))
	for id := range evidenceIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "none"
	}
	return strings.Join(ids, ",")
}

// truncateRunes 按字符（而非字节）截断，避免把中文切成半个字变成乱码。
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func explanationSchema() map[string]any {
	stringArray := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"summary":             map[string]any{"type": "string"},
			"analysis":            map[string]any{"type": "string"},
			"recommended_actions": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"action": map[string]any{"type": "string"}, "priority": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}}, "evidence_ids": stringArray}, "required": []string{"action", "priority", "evidence_ids"}}},
			"citations":           map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"evidence_id": map[string]any{"type": "string"}, "claim": map[string]any{"type": "string"}}, "required": []string{"evidence_id", "claim"}}},
		},
		"required": []string{"summary", "analysis", "recommended_actions", "citations"},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
