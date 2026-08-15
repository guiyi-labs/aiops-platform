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

// SummaryResponse is the M112-3 cited incident summary. Every factual
// claim cites an authorized evidence ID; the root_cause field is always
// labelled as a candidate (the AI cannot declare a confirmed root cause).
type SummaryResponse struct {
	IncidentID         int64           `json:"incident_id"`
	ResourceContext    ResourceContext `json:"resource_context"`
	Mode               string          `json:"mode"` // "ai" or "deterministic"
	RootCauseCandidate string          `json:"root_cause_candidate"`
	Impact             string          `json:"impact"`
	EvidenceSummary    string          `json:"evidence_summary"`
	NextSteps          []string        `json:"next_steps"`
	Citations          []Citation      `json:"citations"`
	Provider           string          `json:"provider"`
	Model              string          `json:"model"`
	InputTokens        int             `json:"input_tokens"`
	OutputTokens       int             `json:"output_tokens"`
	FailClosed         bool            `json:"fail_closed"`
	StageGatePassed    bool            `json:"stage_gate_passed"`
	StageGateReason    string          `json:"stage_gate_reason,omitempty"`
}

// summaryProviderResult is the raw provider output for a summary request.
type summaryProviderResult struct {
	RootCauseCandidate string
	Impact             string
	EvidenceSummary    string
	NextSteps          []string
	Citations          []Citation
}

// SummaryStageGate decides whether the AI provider may be called. Returns
// (pass, reason). The gate is deterministic and side-effect-free.
//
// Stage gate rules:
//   - Incident must have ≥1 evidence item (otherwise nothing to summarize).
//   - AI must be enabled.
func SummaryStageGate(hasEvidence bool, aiEnabled bool) (bool, string) {
	if !hasEvidence {
		return false, "no_evidence"
	}
	if !aiEnabled {
		return false, "ai_disabled"
	}
	return true, "ok"
}

// BuildSummaryPrompt assembles the prompt for a one-shot cited incident
// summary. The output schema requires:
//   - root_cause_candidate (string, labelled as candidate only)
//   - impact (string)
//   - evidence_summary (string)
//   - next_steps ([]string, max 6)
//   - citations ([]Citation, each referencing an authorized evidence ID)
func BuildSummaryPrompt(incident IncidentSnapshot, evidence []EvidenceItem) Prompt {
	authorized := map[string]struct{}{
		AuthorizedEvidenceID("incident", fmt.Sprintf("%d", incident.ID)): {},
	}
	var evidenceLines []string
	for _, e := range evidence {
		id := AuthorizedEvidenceID("evidence", e.SourceRef)
		authorized[id] = struct{}{}
		line := "[" + id + "] " + e.SourceType + " " + e.Title
		if e.Summary != "" {
			line += " — " + e.Summary
		}
		if len(e.Fields) > 0 {
			var parts []string
			for _, f := range e.Fields {
				if f.Value != "" {
					parts = append(parts, f.Label+": "+f.Value)
				}
			}
			if len(parts) > 0 {
				line += " (" + strings.Join(parts, "; ") + ")"
			}
		}
		evidenceLines = append(evidenceLines, line)
	}

	system := "你是 Kubernetes 事故摘要助手。基于提供的证据给出结构化摘要。" +
		"输出必须是 JSON，包含 root_cause_candidate（根因候选，只能写候选，不能写已确认）、" +
		"impact（影响范围）、evidence_summary（证据摘要）、" +
		"next_steps（最多 6 条下一步，字符串数组）、" +
		"citations（数组，每项含 evidence_id 与 claim，claim 必须与摘要对应）。" +
		"所有事实断言必须引用上一条消息给出的证据 ID；不允许引用列表之外的 evidence_id；" +
		"不要编造集群状态；来源不确定时写'尚无足够证据'。"

	input := fmt.Sprintf(
		"事故：%s（%s，级别 %s，状态 %s，来源 %s）。\n"+
			"影响资源：%s/%s %s。\n"+
			"事故摘要：%s\n\n"+
			"证据列表（每行开头是 [evidence_id]，回答中的 citations.evidence_id 必须取自这些 ID 或 incident:%d）：\n%s",
		incident.Title, incident.Number, incident.Severity, incident.Status, incident.SourceType,
		incident.Namespace, incident.Kind, incident.Name,
		incident.Summary,
		incident.ID,
		strings.Join(evidenceLines, "\n"),
	)

	return Prompt{System: system, Input: input, AuthorizedEvidence: authorized}
}

// DecodeSummaryProviderJSON decodes raw provider JSON into a summary result.
func DecodeSummaryProviderJSON(raw string) (summaryProviderResult, error) {
	var output struct {
		RootCauseCandidate string     `json:"root_cause_candidate"`
		Impact             string     `json:"impact"`
		EvidenceSummary    string     `json:"evidence_summary"`
		NextSteps          []string   `json:"next_steps"`
		Citations          []Citation `json:"citations"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return summaryProviderResult{}, fmt.Errorf("malformed JSON: %w", err)
	}
	result := summaryProviderResult{
		RootCauseCandidate: strings.TrimSpace(output.RootCauseCandidate),
		Impact:             strings.TrimSpace(output.Impact),
		EvidenceSummary:    strings.TrimSpace(output.EvidenceSummary),
		NextSteps:          output.NextSteps,
		Citations:          output.Citations,
	}
	if result.NextSteps == nil {
		result.NextSteps = []string{}
	}
	if result.Citations == nil {
		result.Citations = []Citation{}
	}
	return result, nil
}

// ValidateSummaryResult checks the provider summary output against the
// authorized evidence set. Rules mirror the chat validation with summary-
// specific field checks.
func ValidateSummaryResult(result summaryProviderResult, authorized map[string]struct{}) error {
	for _, text := range []string{result.RootCauseCandidate, result.Impact, result.EvidenceSummary} {
		if containsPromptInjectionString(text) {
			return fmt.Errorf("invalid output: prompt injection content rejected")
		}
	}
	for _, step := range result.NextSteps {
		if containsPromptInjectionString(step) {
			return fmt.Errorf("invalid output: prompt injection content rejected")
		}
	}
	if strings.TrimSpace(result.RootCauseCandidate) == "" {
		return fmt.Errorf("invalid output: root_cause_candidate is empty")
	}
	if strings.TrimSpace(result.Impact) == "" {
		return fmt.Errorf("invalid output: impact is empty")
	}
	if strings.TrimSpace(result.EvidenceSummary) == "" {
		return fmt.Errorf("invalid output: evidence_summary is empty")
	}
	if len(result.Citations) == 0 {
		return fmt.Errorf("invalid output: no citations")
	}
	if len(result.Citations) > 64 {
		return fmt.Errorf("invalid output: too many citations (%d > 64)", len(result.Citations))
	}
	if len(result.NextSteps) > 6 {
		return fmt.Errorf("invalid output: too many next_steps (%d > 6)", len(result.NextSteps))
	}
	for _, c := range result.Citations {
		if strings.TrimSpace(c.Claim) == "" {
			return fmt.Errorf("invalid output: citation with empty claim")
		}
		if _, ok := authorized[strings.TrimSpace(c.EvidenceID)]; !ok {
			return ErrCitationRejected
		}
	}
	return nil
}

// SummaryProvider is the AI provider interface for one-shot incident
// summaries. Implementations send the prompt to the model and return
// structured JSON output. The interface is separate from the chat Provider
// because the output schema differs.
type SummaryProvider interface {
	GenerateSummary(ctx context.Context, prompt Prompt) (summaryProviderResult, error)
}

// NopSummaryProvider returns a deterministic, citation-valid summary for
// testing and for the AI_ENABLED=false fallback path.
type NopSummaryProvider struct{}

func (NopSummaryProvider) GenerateSummary(_ context.Context, prompt Prompt) (summaryProviderResult, error) {
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
	return summaryProviderResult{
		RootCauseCandidate: "AI 未启用，无法提供根因候选；请查看证据列表。",
		Impact:             "由证据 " + cited + " 基础推断影响范围（未做真实 AI 分析）。",
		EvidenceSummary:    "基于已记录的证据，尚无 AI 参与的综合摘要。",
		NextSteps:          []string{"确认事故来源记录是否反映当前集群状态"},
		Citations:          []Citation{{EvidenceID: cited, Claim: "incident record exists"}},
	}, nil
}

// ResponsesSummaryProvider sends a summary request to the OpenAI-style
// responses API with the summary-specific JSON schema.
type ResponsesSummaryProvider struct {
	endpoint        string
	apiKey          string
	model           string
	client          *http.Client
	maxOutputTokens int
}

func NewResponsesSummaryProvider(baseURL, apiKey, model string, timeout time.Duration, maxOutputTokens int) *ResponsesSummaryProvider {
	return &ResponsesSummaryProvider{
		endpoint:        strings.TrimRight(baseURL, "/") + "/responses",
		apiKey:          apiKey,
		model:           model,
		client:          &http.Client{Timeout: timeout},
		maxOutputTokens: maxOutputTokens,
	}
}

func summarySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"root_cause_candidate", "impact", "evidence_summary", "next_steps", "citations"},
		"properties": map[string]any{
			"root_cause_candidate": map[string]any{"type": "string"},
			"impact":               map[string]any{"type": "string"},
			"evidence_summary":     map[string]any{"type": "string"},
			"next_steps": map[string]any{
				"type":     "array",
				"maxItems": 6,
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

func (p *ResponsesSummaryProvider) GenerateSummary(ctx context.Context, prompt Prompt) (summaryProviderResult, error) {
	body, err := json.Marshal(responsesRequest{
		Model: p.model,
		Store: false,
		Input: []responsesInput{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.Input},
		},
		Text: responsesText{Format: responsesFormat{
			Type: "json_schema", Name: "incident_summary", Strict: true, Schema: summarySchema(),
		}},
		MaxOutputTokens: p.maxOutputTokens,
	})
	if err != nil {
		return summaryProviderResult{}, fmt.Errorf("encode summary request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return summaryProviderResult{}, fmt.Errorf("create summary request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return summaryProviderResult{}, fmt.Errorf("%w: %v", ErrDisabled, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxProviderResponseBytes))
		return summaryProviderResult{}, fmt.Errorf("%w: HTTP %d", ErrDisabled, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil || len(raw) > maxProviderResponseBytes {
		return summaryProviderResult{}, fmt.Errorf("%w: response exceeds limit", ErrDisabled)
	}
	var envelope struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return summaryProviderResult{}, fmt.Errorf("%w: parse envelope: %v", ErrDisabled, err)
	}
	rawText := ""
	for _, out := range envelope.Output {
		for _, c := range out.Content {
			rawText = c.Text
		}
	}
	if rawText == "" {
		return summaryProviderResult{}, fmt.Errorf("%w: empty provider output", ErrDisabled)
	}
	result, err := DecodeSummaryProviderJSON(rawText)
	if err != nil {
		return summaryProviderResult{}, err
	}
	return result, nil
}
