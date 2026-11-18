package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestToolCatalogueAndValidatorUseRetainedReadOnlySurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/tools", listIntegrationTools)
	router.POST("/validate", validateIntegrationTool)
	catalog := httptest.NewRecorder()
	router.ServeHTTP(catalog, httptest.NewRequest("GET", "/tools", nil))
	var body struct {
		Items []struct{ Name, Method, Path string }
	}
	if err := json.Unmarshal(catalog.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 6 {
		t.Fatalf("unexpected catalogue: %s", catalog.Body.String())
	}
	for _, item := range body.Items {
		if item.Method != "GET" || strings.Contains(item.Path, "incidents") {
			t.Fatalf("unsupported tool: %+v", item)
		}
	}
	for _, tc := range []struct {
		name, payload string
		status        int
		allowed       bool
	}{
		{"valid", `{"tool":"get_diagnosis","arguments":{"diagnosis_id":"42"}}`, 200, true},
		{"query", `{"tool":"search_knowledge","arguments":{"rule_id":"pod.oom_killed.v1","limit":"3"}}`, 200, true},
		{"unknown arg", `{"tool":"get_diagnosis","arguments":{"diagnosis_id":"42","execute":"true"}}`, 200, false},
		{"path escape", `{"tool":"get_diagnosis","arguments":{"diagnosis_id":"../../auth"}}`, 200, false},
		{"write tool", `{"tool":"delete_pod","arguments":{}}`, 200, false},
		{"removed", `{"tool":"list_incidents","arguments":{}}`, 200, false},
		{"missing", `{"tool":"get_diagnosis","arguments":{}}`, 200, false},
		{"unexpected field", `{"tool":"get_diagnosis","arguments":{"diagnosis_id":"42"},"url":"/admin"}`, 400, false},
		{"extra json", `{"tool":"knowledge_stats"} {}`, 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("POST", "/validate", strings.NewReader(tc.payload)))
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusOK {
				return
			}
			var result struct {
				Allowed, Executed bool
				Path              string
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Allowed != tc.allowed || result.Executed {
				t.Fatalf("invalid result: %+v", result)
			}
			if tc.name == "valid" && result.Path != "/api/v1/diagnoses/42" {
				t.Fatalf("wrong path: %+v", result)
			}
		})
	}
}
