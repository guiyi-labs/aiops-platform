package incidentchat

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

// NopProvider returns a deterministic, citation-valid result for testing
// and for the AI_ENABLED=false fallback path. It never calls a real model.
type NopProvider struct{}

func (NopProvider) Generate(_ context.Context, prompt Prompt) (ProviderResult, error) {
	// Cite the incident record when present in the authorized set; fall
	// back to the first authorized evidence ID so citations always pass.
	var incidentID, firstEvidenceID string
	for id := range prompt.AuthorizedEvidence {
		if strings.HasPrefix(id, "incident:") {
			incidentID = id
		} else if strings.HasPrefix(id, "evidence:") && firstEvidenceID == "" {
			firstEvidenceID = id
		}
	}
	cited := incidentID
	if cited == "" {
		cited = firstEvidenceID
	}
	return ProviderResult{
		Provider:   "nop",
		Model:      "nop-1.0",
		Answer:     "AI 模型未启用，以上为基于事故记录的确定性摘要；由证据 " + cited + " 支撑，未做任何推断。",
		NextChecks: []string{"确认事故来源记录是否反映当前集群状态"},
		Citations:  []Citation{{EvidenceID: cited, Claim: "incident record exists"}},
	}, nil
}

// ResponsesProvider sends a chat request to the OpenAI-style responses API.
type ResponsesProvider struct {
	endpoint        string
	apiKey          string
	model           string
	client          *http.Client
	maxOutputTokens int
}

func NewResponsesProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) *ResponsesProvider {
	return &ResponsesProvider{
		endpoint:        strings.TrimRight(baseURL, "/") + "/responses",
		apiKey:          apiKey,
		model:           model,
		client:          &http.Client{Timeout: timeout},
		maxOutputTokens: maxOutputTokens,
	}
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

func chatSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"additionalProperties": false,
		"required": []any{"answer", "next_checks", "citations"},
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
			"next_checks": map[string]any{
				"type": "array",
				"maxItems": 8,
				"items":    map[string]any{"type": "string"},
			},
			"citations": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 64,
				"items": map[string]any{
					"type":     "object",
					"required": []any{"evidence_id", "claim"},
					"properties": map[string]any{
						"evidence_id": map[string]any{"type": "string"},
						"claim":       map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

const maxProviderResponseBytes = 1 << 20

func (p *ResponsesProvider) Generate(ctx context.Context, prompt Prompt) (ProviderResult, error) {
	body, err := json.Marshal(responsesRequest{
		Model: p.model,
		Store: false,
		Input: []responsesInput{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.Input},
		},
		Text: responsesText{Format: responsesFormat{
			Type: "json_schema", Name: "incident_ai_chat", Strict: true, Schema: chatSchema(),
		}},
		MaxOutputTokens: p.maxOutputTokens,
	})
	if err != nil {
		return ProviderResult{}, fmt.Errorf("encode chat request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return ProviderResult{}, fmt.Errorf("create chat request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: %v", ErrDisabled, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxProviderResponseBytes))
		return ProviderResult{}, fmt.Errorf("%w: HTTP %d", ErrDisabled, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil || len(raw) > maxProviderResponseBytes {
		return ProviderResult{}, fmt.Errorf("%w: response exceeds limit", ErrDisabled)
	}
	// Extract the text content from the responses API envelope.
	var envelope struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ProviderResult{}, fmt.Errorf("%w: parse envelope: %v", ErrDisabled, err)
	}
	rawText := ""
	for _, out := range envelope.Output {
		for _, c := range out.Content {
			rawText = c.Text
		}
	}
	if rawText == "" {
		return ProviderResult{}, fmt.Errorf("%w: empty provider output", ErrDisabled)
	}
	result, err := DecodeProviderJSON(rawText)
	if err != nil {
		return ProviderResult{}, err
	}
	result.Provider = "responses-compatible"
	result.Model = p.model
	return result, nil
}