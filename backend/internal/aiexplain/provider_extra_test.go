package aiexplain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// sanitizeValue
// ---------------------------------------------------------------------------

func TestSanitizeValueHandlesArraysAndScalars(t *testing.T) {
	nested := map[string]any{"apiToken": "leaked", "reason": "Failed"}
	got := sanitizeValue([]any{"plain", nested, float64(3), nil, []any{map[string]any{"cookie": "session"}}})

	list, ok := got.([]any)
	if !ok || len(list) != 5 {
		t.Fatalf("sanitizeValue([]any) = %#v", got)
	}
	if list[0] != "plain" || list[2] != float64(3) || list[3] != nil {
		t.Fatalf("scalars altered: %#v", list)
	}
	sanitizedMap, ok := list[1].(map[string]any)
	if !ok {
		t.Fatalf("nested map = %#v", list[1])
	}
	if sanitizedMap["apiToken"] != "[REDACTED]" || sanitizedMap["reason"] != "Failed" {
		t.Fatalf("nested redaction = %#v", sanitizedMap)
	}
	inner, ok := list[4].([]any)
	if !ok || len(inner) != 1 {
		t.Fatalf("nested slice = %#v", list[4])
	}
	if cookie, ok := inner[0].(map[string]any); !ok || cookie["cookie"] != "[REDACTED]" {
		t.Fatalf("cookie redaction = %#v", inner[0])
	}
}

func TestSanitizeValuePassesThroughUntouchedTypes(t *testing.T) {
	if got := sanitizeValue(42); got != 42 {
		t.Fatalf("sanitizeValue(int) = %#v", got)
	}
	if got := sanitizeValue(true); got != true {
		t.Fatalf("sanitizeValue(bool) = %#v", got)
	}
	if got := sanitizeValue(nil); got != nil {
		t.Fatalf("sanitizeValue(nil) = %#v", got)
	}
	if got := sanitizeValue("short"); got != "short" {
		t.Fatalf("sanitizeValue(string) = %#v", got)
	}
}

func TestSanitizeValueTruncatesLongStrings(t *testing.T) {
	got, ok := sanitizeValue(strings.Repeat("a", 5000)).(string)
	if !ok {
		t.Fatalf("sanitizeValue() type = %T", got)
	}
	if len(got) != 4096+len("…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated length = %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// decodeStructuredOutput
// ---------------------------------------------------------------------------

func TestDecodeStructuredOutputValidation(t *testing.T) {
	evidence := map[string]struct{}{"E1": {}, "E2": {}}
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "malformed json", raw: `{not-json`, wantErr: true},
		{name: "missing summary", raw: `{"analysis":"a","citations":[{"evidence_id":"E1","claim":"c"}]}`, wantErr: true},
		{name: "blank summary", raw: `{"summary":"   ","analysis":"a","citations":[{"evidence_id":"E1","claim":"c"}]}`, wantErr: true},
		{name: "missing analysis", raw: `{"summary":"s","citations":[{"evidence_id":"E1","claim":"c"}]}`, wantErr: true},
		{name: "missing citations", raw: `{"summary":"s","analysis":"a"}`, wantErr: true},
		{name: "unknown citation", raw: `{"summary":"s","analysis":"a","citations":[{"evidence_id":"E99","claim":"c"}]}`, wantErr: true},
		{name: "blank claim", raw: `{"summary":"s","analysis":"a","citations":[{"evidence_id":"E1","claim":"  "}]}`, wantErr: true},
		{name: "blank action", raw: `{"summary":"s","analysis":"a","citations":[{"evidence_id":"E1","claim":"c"}],"recommended_actions":[{"action":" ","priority":"high","evidence_ids":["E1"]}]}`, wantErr: true},
		{name: "invalid priority", raw: `{"summary":"s","analysis":"a","citations":[{"evidence_id":"E1","claim":"c"}],"recommended_actions":[{"action":"restart","priority":"urgent","evidence_ids":["E1"]}]}`, wantErr: true},
		{name: "missing priority", raw: `{"summary":"s","analysis":"a","citations":[{"evidence_id":"E1","claim":"c"}],"recommended_actions":[{"action":"restart","evidence_ids":["E1"]}]}`, wantErr: true},
		{name: "unknown action evidence", raw: `{"summary":"s","analysis":"a","citations":[{"evidence_id":"E1","claim":"c"}],"recommended_actions":[{"action":"restart","priority":"high","evidence_ids":["E42"]}]}`, wantErr: true},
		{
			name: "valid trims whitespace",
			raw:  `{"summary":"  summary  ","analysis":"\nanalysis\n","citations":[{"evidence_id":"E2","claim":"c"}],"recommended_actions":[{"action":"restart","priority":"low","evidence_ids":["E1","E2"]}]}`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := decodeStructuredOutput(testCase.raw, evidence)
			if testCase.wantErr {
				if !errors.Is(err, ErrInvalidOutput) {
					t.Fatalf("decodeStructuredOutput() error = %v, want ErrInvalidOutput", err)
				}
				if result.Summary != "" || result.Analysis != "" || len(result.Citations) != 0 {
					t.Fatalf("result = %#v, want zero value on failure", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeStructuredOutput() error = %v", err)
			}
			if result.Summary != "summary" || result.Analysis != "analysis" {
				t.Fatalf("result = %#v", result)
			}
			if len(result.Citations) != 1 || result.Citations[0].EvidenceID != "E2" {
				t.Fatalf("citations = %#v", result.Citations)
			}
			if len(result.RecommendedActions) != 1 || result.RecommendedActions[0].Priority != "low" ||
				len(result.RecommendedActions[0].EvidenceIDs) != 2 {
				t.Fatalf("recommended actions = %#v", result.RecommendedActions)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResponsesProvider
// ---------------------------------------------------------------------------

// responsesServer spins up a stub /responses endpoint returning the raw body.
func responsesServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(status)
		_, _ = response.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func envelope(t *testing.T, outputText string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id": "resp_test", "model": "test-model-2026",
		"output": []any{map[string]any{
			"type":    "message",
			"content": []any{map[string]any{"type": "output_text", "text": outputText}},
		}},
		"usage": map[string]any{"input_tokens": 7, "output_tokens": 3},
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return raw
}

func TestResponsesProviderRejectsInvalidEndpoint(t *testing.T) {
	provider := NewResponsesProvider("http://[::1", "key", "model", time.Second, 100)
	_, err := provider.Generate(context.Background(), Prompt{})
	if err == nil || !strings.Contains(err.Error(), "create provider request") {
		t.Fatalf("Generate() error = %v, want request construction failure", err)
	}
}

func TestResponsesProviderReportsTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()

	provider := NewResponsesProvider(endpoint, "key", "model", time.Second, 100)
	_, err := provider.Generate(context.Background(), Prompt{})
	if !errors.Is(err, ErrProviderFailure) {
		t.Fatalf("Generate() error = %v, want ErrProviderFailure", err)
	}
}

func TestResponsesProviderRejectsUndecodableEnvelope(t *testing.T) {
	server := responsesServer(t, http.StatusOK, []byte(`{"id":`))
	provider := NewResponsesProvider(server.URL, "key", "model", time.Second, 100)

	_, err := provider.Generate(context.Background(), Prompt{})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Generate() error = %v, want ErrInvalidOutput", err)
	}
}

func TestResponsesProviderRejectsOversizedResponse(t *testing.T) {
	server := responsesServer(t, http.StatusOK, bytes.Repeat([]byte("x"), maxProviderResponseBytes+64))
	provider := NewResponsesProvider(server.URL, "key", "model", 10*time.Second, 100)

	_, err := provider.Generate(context.Background(), Prompt{})
	if !errors.Is(err, ErrProviderFailure) {
		t.Fatalf("Generate() error = %v, want ErrProviderFailure", err)
	}
}

func TestResponsesProviderRejectsInvalidStructuredOutput(t *testing.T) {
	server := responsesServer(t, http.StatusOK, envelope(t, `{"summary":"","analysis":"","citations":[]}`))
	provider := NewResponsesProvider(server.URL, "key", "model", time.Second, 100)

	_, err := provider.Generate(context.Background(), Prompt{EvidenceIDs: map[string]struct{}{"E1": {}}})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Generate() error = %v, want ErrInvalidOutput", err)
	}
}

func TestResponsesProviderSkipsNonTextOutputAndFallsBackToConfiguredModel(t *testing.T) {
	structured := `{"summary":"s","analysis":"a","recommended_actions":[],"citations":[{"evidence_id":"E1","claim":"c"}]}`
	body, err := json.Marshal(map[string]any{
		"output": []any{
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "reasoning_text", "text": "ignored"}}},
			map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": structured}}},
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	server := responsesServer(t, http.StatusOK, body)
	provider := NewResponsesProvider(server.URL, "", "configured-model", time.Second, 100)

	result, err := provider.Generate(context.Background(), Prompt{EvidenceIDs: map[string]struct{}{"E1": {}}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Model != "configured-model" {
		t.Fatalf("model = %q, want the configured fallback", result.Model)
	}
	if result.Provider != "responses-compatible" || result.ProviderResponseID != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Summary != "s" || result.Analysis != "a" || len(result.Citations) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Fatalf("usage = %d/%d, want 0/0", result.InputTokens, result.OutputTokens)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Fatalf("firstNonEmpty(all empty) = %q", got)
	}
	if got := firstNonEmpty("", "second", "third"); got != "second" {
		t.Fatalf("firstNonEmpty() = %q, want second", got)
	}
	if got := firstNonEmpty("first", "second"); got != "first" {
		t.Fatalf("firstNonEmpty() = %q, want first", got)
	}
}
