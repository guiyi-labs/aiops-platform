package aiexplain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResponsesProviderGeneratesValidatedExplanation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s auth=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Store || body.Model != "test-model" || body.Text.Format.Type != "json_schema" || !body.Text.Format.Strict || body.MaxOutputTokens != 800 {
			t.Fatalf("unexpected request body %#v", body)
		}
		structured := `{"summary":"镜像拉取失败","analysis":"容器等待状态证明镜像拉取失败。","recommended_actions":[{"action":"核对镜像名称","priority":"high","evidence_ids":["E1"]}],"citations":[{"evidence_id":"E1","claim":"容器处于 ImagePullBackOff"}]}`
		_ = json.NewEncoder(response).Encode(map[string]any{
			"id": "resp_test", "model": "test-model-2026",
			"output": []any{map[string]any{
				"type":    "message",
				"content": []any{map[string]any{"type": "output_text", "text": structured}},
			}},
			"usage": map[string]any{"input_tokens": 120, "output_tokens": 45},
		})
	}))
	defer server.Close()

	provider := NewResponsesProvider(server.URL+"/v1", "test-key", "test-model", time.Second, 800)
	result, err := provider.Generate(context.Background(), Prompt{System: "system", Input: "input", EvidenceIDs: map[string]struct{}{"E1": {}}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.ProviderResponseID != "resp_test" || result.Model != "test-model-2026" || result.InputTokens != 120 || len(result.Citations) != 1 {
		t.Fatalf("Generate() = %#v", result)
	}
}

func TestDecodeStructuredOutputRejectsUnknownCitation(t *testing.T) {
	raw := `{"summary":"summary","analysis":"analysis","recommended_actions":[],"citations":[{"evidence_id":"E99","claim":"invented"}]}`
	_, err := decodeStructuredOutput(raw, map[string]struct{}{"E1": {}})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("decodeStructuredOutput() error = %v, want ErrInvalidOutput", err)
	}
}

func TestResponsesProviderMapsUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusTooManyRequests) }))
	defer server.Close()
	provider := NewResponsesProvider(server.URL, "test-key", "test-model", time.Second, 800)
	_, err := provider.Generate(context.Background(), Prompt{})
	if !errors.Is(err, ErrProviderFailure) {
		t.Fatalf("Generate() error = %v, want ErrProviderFailure", err)
	}
}
