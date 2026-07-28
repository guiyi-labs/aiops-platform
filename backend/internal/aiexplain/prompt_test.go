package aiexplain

import (
	"strings"
	"testing"
	"unicode/utf8"

	"k8s-aiops.local/backend/internal/diagnosis"
)

func TestBuildPromptRedactsSensitiveEvidenceFields(t *testing.T) {
	prompt := BuildPrompt(diagnosis.Record{RuleID: "test.rule", Evidence: []diagnosis.Evidence{{Type: "event", Source: "event/test", Content: map[string]any{"message": "pull failed", "token": "secret-value", "nested": map[string]any{"password": "hidden"}}}}})
	if len(prompt.EvidenceIDs) != 1 || !strings.Contains(prompt.Input, `"id":"E1"`) {
		t.Fatalf("BuildPrompt() = %#v", prompt)
	}
	if strings.Contains(prompt.Input, "secret-value") || strings.Contains(prompt.Input, "hidden") || strings.Count(prompt.Input, "[REDACTED]") != 2 {
		t.Fatalf("sensitive evidence was not redacted: %s", prompt.Input)
	}
}

func TestBuildPromptKeepsContextBoundedAndValidUTF8(t *testing.T) {
	evidence := make([]diagnosis.Evidence, 20)
	for index := range evidence {
		evidence[index] = diagnosis.Evidence{Type: "event", Source: "event/test", Content: map[string]any{"message": strings.Repeat("故障", 3000)}}
	}
	prompt := BuildPrompt(diagnosis.Record{RuleID: "test.rule", Evidence: evidence})
	if len(prompt.Input) > maxPromptBytes || !utf8.ValidString(prompt.Input) || len(prompt.EvidenceIDs) >= len(evidence) {
		t.Fatalf("prompt bounds invalid: bytes=%d evidence=%d", len(prompt.Input), len(prompt.EvidenceIDs))
	}
}
