package httpserver

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"k8s-aiops.local/backend/internal/diagnosisbench"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidationReplaysWithoutAnyConfiguredServices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/validation", validateDiagnosis)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/validation", nil))
	if response.Code != 200 {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var report diagnosisbench.Report
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.TotalScenarios != 38 || len(report.Outcomes) != 38 || len(report.PerRule) != 12 {
		t.Fatalf("incomplete report: %+v", report)
	}
	if report.Outcomes[0].Record == nil || len(report.Outcomes[0].Record.Evidence) == 0 {
		t.Fatal("missing rule evidence")
	}
}
