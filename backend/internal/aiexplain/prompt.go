package aiexplain

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"k8s-aiops.local/backend/internal/diagnosis"
)

const maxPromptBytes = 32 * 1024

func BuildPrompt(record diagnosis.Record) Prompt {
	type promptEvidence struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Source  string `json:"source"`
		Content any    `json:"content"`
	}
	type promptInput struct {
		RuleID          string                `json:"rule_id"`
		Severity        string                `json:"severity"`
		Resource        diagnosis.ResourceRef `json:"resource"`
		Status          string                `json:"status"`
		Summary         string                `json:"rule_summary"`
		RootCauses      []string              `json:"root_cause_hypotheses"`
		Recommendations []string              `json:"rule_recommendations"`
		Evidence        []promptEvidence      `json:"evidence"`
	}

	input := promptInput{RuleID: record.RuleID, Severity: record.Severity, Resource: record.Resource, Status: record.Status, Summary: record.Summary, RootCauses: record.RootCauses, Recommendations: record.Recommendations}
	valid := make(map[string]struct{}, len(record.Evidence))
	for index, item := range record.Evidence {
		id := fmt.Sprintf("E%d", index+1)
		evidence := promptEvidence{ID: id, Type: item.Type, Source: item.Source, Content: sanitizeValue(item.Content)}
		candidate := input
		candidate.Evidence = append(append([]promptEvidence(nil), input.Evidence...), evidence)
		encoded, _ := json.Marshal(candidate)
		if len(encoded) > maxPromptBytes {
			break
		}
		input = candidate
		valid[id] = struct{}{}
	}
	encoded, _ := json.Marshal(input)
	return Prompt{
		System: "你是 Kubernetes 故障诊断解释器。只能解释输入中的确定性规则结果和证据，不得声称已执行修复，不得把根因假设描述为已证实事实。每个关键判断必须引用提供的 evidence ID；如果证据不足，应明确说明需要补充验证。输出必须符合给定 JSON Schema。",
		Input:  string(encoded), EvidenceIDs: valid,
	}
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = sanitizeValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizeValue(item)
		}
		return result
	case string:
		return truncateUTF8(typed, 4096)
	default:
		return value
	}
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + "…"
}
