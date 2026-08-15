package incidentchat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// EvidenceItem is the minimal incident evidence view needed for the prompt.
// It mirrors incident.EvidenceItem without importing that package.
type EvidenceItem struct {
	SourceType string
	SourceRef  string
	Title      string
	Summary    string
	Severity   string
	ObservedAt string
	DeepLink   string
	Fields     []Field
}

type Field struct {
	Label string
	Value string
}

// IncidentSnapshot is the minimal incident view for the prompt.
type IncidentSnapshot struct {
	ID         int64
	Number     string
	Title      string
	Severity   string
	Status     string
	Summary    string
	SourceType string
	ClusterID  int64
	Namespace  string
	Kind       string
	Name       string
}

// AuthorizedEvidenceID builds the stable evidence ID for an evidence item.
// The incident snapshot itself is always "incident:<id>"; timeline evidence
// uses the source ref so citations are traceable to the evidence timeline.
func AuthorizedEvidenceID(kind, ref string) string {
	switch kind {
	case "incident":
		return "incident:" + ref
	default:
		return "evidence:" + ref
	}
}

// BuildPrompt assembles the system + user prompt and the authorized evidence
// set. Returns a typed prompt; the validator later rejects any citation not
// in the authorized set.
func BuildPrompt(incident IncidentSnapshot, evidence []EvidenceItem, history []ChatMessage, question string) Prompt {
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

	var historyLines []string
	for _, m := range history {
		historyLines = append(historyLines, m.Role+": "+strings.TrimSpace(m.Content))
	}

	system := "你是 Kubernetes 事故协查助手。只基于提供的证据回答，每一条事实断言必须引用上一条消息给出的证据 ID。" +
		"输出必须是 JSON，包含 answer（回答正文）、next_checks（最多3条下一步检查，字符串数组）、" +
		"citations（数组，每项含 evidence_id 与 claim，claim 必须与回答对应）。不允许引用列表之外的证据 ID；" +
		"不要声称已确认根因（只能给出候选与检查建议）；不要编造集群状态。"

	input := fmt.Sprintf(
		"事故：%s（%s，级别 %s，状态 %s，来源 %s）。\n"+
			"影响资源：%s/%s %s。\n"+
			"事故摘要：%s\n\n"+
			"证据列表（每行开头是 [evidence_id]，回答中的 citations.evidence_id 必须取自这些 ID 或 incident:%d）：\n%s\n\n"+
			"历史对话：\n%s\n\n"+
			"当前问题：%s",
		incident.Title, incident.Number, incident.Severity, incident.Status, incident.SourceType,
		incident.Namespace, incident.Kind, incident.Name,
		incident.Summary,
		incident.ID,
		strings.Join(evidenceLines, "\n"),
		strings.Join(historyLines, "\n"),
		strings.TrimSpace(question),
	)

	return Prompt{System: system, Input: input, AuthorizedEvidence: authorized}
}

// PromptHash is a stable SHA-256 over the incident identity + authorized
// evidence IDs. Used for replay-consistency acceptance and audit.
func PromptHash(incidentID int64, evidence []EvidenceItem) string {
	ids := make([]string, 0, len(evidence)+1)
	ids = append(ids, AuthorizedEvidenceID("incident", fmt.Sprintf("%d", incidentID)))
	for _, e := range evidence {
		ids = append(ids, AuthorizedEvidenceID("evidence", e.SourceRef))
	}
	sort.Strings(ids)
	h := sha256.New()
	fmt.Fprintf(h, "incident_id=%d\n", incidentID)
	fmt.Fprintf(h, "evidence_ids=%s\n", strings.Join(ids, ","))
	return hex.EncodeToString(h.Sum(nil))
}

// DecodeProviderJSON decodes raw provider JSON into a ProviderResult.
func DecodeProviderJSON(raw string) (ProviderResult, error) {
	var output struct {
		Answer     string     `json:"answer"`
		NextChecks []string   `json:"next_checks"`
		Citations  []Citation `json:"citations"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return ProviderResult{}, fmt.Errorf("malformed JSON: %w", err)
	}
	result := ProviderResult{
		Answer:     strings.TrimSpace(output.Answer),
		NextChecks: output.NextChecks,
		Citations:  output.Citations,
	}
	if result.NextChecks == nil {
		result.NextChecks = []string{}
	}
	if result.Citations == nil {
		result.Citations = []Citation{}
	}
	return result, nil
}

// ValidateResult checks the provider raw output against the authorized
// evidence set. Rules (M44-equivalent discipline, scoped to incidents):
//  1. answer non-empty
//  2. at least one citation; max 64
//  3. every citation has a non-empty claim and an authorized evidence ID
//  4. next_checks max 8
//  5. no prompt-injection or instruction-override markers
func ValidateResult(result ProviderResult, authorized map[string]struct{}) error {
	if containsPromptInjection(result) {
		return fmt.Errorf("invalid output: prompt injection content rejected")
	}
	if strings.TrimSpace(result.Answer) == "" {
		return fmt.Errorf("invalid output: answer is empty")
	}
	if len(result.Citations) == 0 {
		return fmt.Errorf("invalid output: no citations")
	}
	if len(result.Citations) > 64 {
		return fmt.Errorf("invalid output: too many citations (%d > 64)", len(result.Citations))
	}
	if len(result.NextChecks) > 8 {
		return fmt.Errorf("invalid output: too many next_checks (%d > 8)", len(result.NextChecks))
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

func containsPromptInjection(result ProviderResult) bool {
	texts := append([]string{result.Answer}, result.NextChecks...)
	for _, c := range result.Citations {
		texts = append(texts, c.Claim)
	}
	for _, value := range texts {
		if containsPromptInjectionString(value) {
			return true
		}
	}
	return false
}

// containsPromptInjectionString reports whether a single text value carries
// a prompt-injection or instruction-override marker.
func containsPromptInjectionString(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"ignore previous instructions", "ignore all previous instructions",
		"reveal the system prompt", "system prompt has been overridden",
		"developer message", "disregard previous instructions",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
