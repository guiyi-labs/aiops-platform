package aiexplain

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/knowledge"
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
		// CitableEvidenceIDs 显式列出唯一合法的引用值。此前只靠模型自己对
		// evidence 数组里 id 字段的观察来推断，接入能力较弱的模型时常编出自造 ID
		// （EV-1、evidence_1），触发校验失败。给一份白名单比事后纠错便宜得多。
		CitableEvidenceIDs []string         `json:"citable_evidence_ids"`
		Evidence           []promptEvidence `json:"evidence"`
	}

	input := promptInput{RuleID: record.RuleID, Severity: record.Severity, Resource: record.Resource, Status: record.Status, Summary: record.Summary, RootCauses: record.RootCauses, Recommendations: record.Recommendations}
	valid := make(map[string]struct{}, len(record.Evidence))
	for index, item := range record.Evidence {
		id := fmt.Sprintf("E%d", index+1)
		evidence := promptEvidence{ID: id, Type: item.Type, Source: item.Source, Content: sanitizeValue(item.Content)}
		candidate := input
		candidate.Evidence = append(append([]promptEvidence(nil), input.Evidence...), evidence)
		candidate.CitableEvidenceIDs = append(append([]string(nil), input.CitableEvidenceIDs...), id)
		encoded, _ := json.Marshal(candidate)
		if len(encoded) > maxPromptBytes {
			break
		}
		input = candidate
		valid[id] = struct{}{}
	}
	encoded, _ := json.Marshal(input)
	return Prompt{
		System: "你是 Kubernetes 故障诊断解释器。只能解释输入中的确定性规则结果和证据，不得声称已执行修复，不得把根因假设描述为已证实事实。" +
			"每个关键判断必须引用 evidence：citations[].evidence_id 与 recommended_actions[].evidence_ids 的取值" +
			"只能来自 citable_evidence_ids 数组中的字符串，逐字照抄，不得改写、加前缀或自造 ID。" +
			"如果证据不足以支撑某个结论，就在 analysis 中明确说明需要补充验证，而不是编造引用。输出必须符合给定 JSON Schema。",
		Input: string(encoded), EvidenceIDs: valid,
	}
}

// BuildPromptWithHistory is the RAG variant of BuildPrompt: verified
// historical cases are injected as a lead-in section and registered as
// citable evidence (historical:1..N) so the model may reference them, while
// the existing citation validator keeps rejecting fabricated evidence IDs.
func BuildPromptWithHistory(record diagnosis.Record, history []knowledge.Entry) Prompt {
	base := BuildPrompt(record)
	if len(history) == 0 {
		return base
	}
	contextBlock := knowledge.BuildPromptContext(history)
	contextBlock += "\n请优先参考历史案例中的根因与处置措施，但不得直接复制；结合当前证据给出独立判断。\n"
	valid := base.EvidenceIDs
	if valid == nil {
		valid = make(map[string]struct{})
	}
	for index := range history {
		valid[fmt.Sprintf("historical:%d", index+1)] = struct{}{}
	}
	return Prompt{System: base.System, Input: contextBlock + base.Input, EvidenceIDs: valid}
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
