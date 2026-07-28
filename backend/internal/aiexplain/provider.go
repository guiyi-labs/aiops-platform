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
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ProviderResult{}, fmt.Errorf("%w: decode response", ErrInvalidOutput)
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

func decodeStructuredOutput(raw string, evidenceIDs map[string]struct{}) (ProviderResult, error) {
	var output struct {
		Summary            string              `json:"summary"`
		Analysis           string              `json:"analysis"`
		RecommendedActions []RecommendedAction `json:"recommended_actions"`
		Citations          []Citation          `json:"citations"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return ProviderResult{}, fmt.Errorf("%w: malformed JSON", ErrInvalidOutput)
	}
	if strings.TrimSpace(output.Summary) == "" || strings.TrimSpace(output.Analysis) == "" || len(output.Citations) == 0 {
		return ProviderResult{}, fmt.Errorf("%w: required content missing", ErrInvalidOutput)
	}
	for _, citation := range output.Citations {
		if _, ok := evidenceIDs[citation.EvidenceID]; !ok || strings.TrimSpace(citation.Claim) == "" {
			return ProviderResult{}, fmt.Errorf("%w: unknown evidence citation", ErrInvalidOutput)
		}
	}
	for _, action := range output.RecommendedActions {
		if strings.TrimSpace(action.Action) == "" || (action.Priority != "high" && action.Priority != "medium" && action.Priority != "low") {
			return ProviderResult{}, fmt.Errorf("%w: invalid recommended action", ErrInvalidOutput)
		}
		for _, id := range action.EvidenceIDs {
			if _, ok := evidenceIDs[id]; !ok {
				return ProviderResult{}, fmt.Errorf("%w: unknown action evidence", ErrInvalidOutput)
			}
		}
	}
	return ProviderResult{Summary: strings.TrimSpace(output.Summary), Analysis: strings.TrimSpace(output.Analysis), RecommendedActions: output.RecommendedActions, Citations: output.Citations}, nil
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
