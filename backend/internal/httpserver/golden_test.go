package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/golden"
)

// --- golden test helpers ---

func newGoldenTestEngine(t *testing.T, svc *golden.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Next()
	})
	h := goldenHandler{service: svc}
	api := r.Group("/api/v1/aiops")
	{
		api.GET("/quality-report", h.getQualityReport)
		api.POST("/quality-report/run", h.runQualityReplay)
	}
	return r
}

func mustNewGoldenService(t *testing.T, storage golden.ReportStorage) *golden.Service {
	t.Helper()
	if storage == nil {
		storage = golden.NopReportStorage{}
	}
	return golden.NewService(testEngineContracts(), storage, zap.NewNop())
}

func testEngineContracts() golden.EngineContracts {
	return golden.EngineContracts{
		Versions: golden.EngineVersions{
			SignalVersion:       "1.0",
			TopologyVersion:     "1.0",
			SLOVersion:          "1.0",
			CorrelationVersion:  "1.0",
			InvestigatorVersion: "1.0",
			AutomationVersion:   "1.0",
			VerifierVersion:     "1.0",
		},
		ValidPlanStatuses: map[string]bool{
			"draft": true, "previewed": true, "approved": true,
			"executing": true, "succeeded": true, "failed": true,
			"expired": true, "cancelled": true, "verified": true,
		},
		ValidVerificationStatuses: map[string]bool{
			"pending": true, "effective": true, "ineffective": true,
			"failed": true, "unknown": true,
		},
	}
}

// --- GET /quality-report tests ---

func TestGolden_GetQualityReport_404WhenNoReport(t *testing.T) {
	r := newGoldenTestEngine(t, mustNewGoldenService(t, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/quality-report", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGolden_GetQualityReport_503WhenServiceNil(t *testing.T) {
	r := newGoldenTestEngine(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/quality-report", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestGolden_GetQualityReport_200WithReport(t *testing.T) {
	dir := t.TempDir()
	storage := golden.NewFileReportStorage(dir)
	report := golden.QualityReport{
		ReportVersion:        golden.ReportVersion,
		DatasetVersionBefore: "1.0",
		DatasetVersionAfter:  "1.0",
		ScenarioResults: []golden.ScenarioQuality{
			{ScenarioID: golden.ScenarioMandatoryEndToEnd, PassedBefore: true, PassedAfter: true, Delta: "preserved", StepsTotal: 10},
		},
		Summary:     golden.QualitySummary{TotalScenarios: 1, Preserved: 1},
		GeneratedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := storage.Save(report); err != nil {
		t.Fatalf("Save: %v", err)
	}
	svc := mustNewGoldenService(t, storage)

	r := newGoldenTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/quality-report", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["report_version"] != golden.ReportVersion {
		t.Errorf("report_version = %v, want %q", body["report_version"], golden.ReportVersion)
	}
}

// --- POST /quality-report/run tests ---

func TestGolden_RunQualityReplay_202Accepted(t *testing.T) {
	dir := t.TempDir()
	storage := golden.NewFileReportStorage(dir)
	svc := mustNewGoldenService(t, storage)

	r := newGoldenTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/quality-report/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		t.Errorf("task_id missing or empty: %v", body["task_id"])
	}

	// The replay runs in a background goroutine that writes the report to
	// the TempDir-backed storage. Wait for it to finish before returning,
	// otherwise t.TempDir() cleanup can race with the still-running
	// goroutine and fail with "directory not empty".
	if ok && taskID != "" {
		waitForReplay(t, svc, taskID)
	}
}

// waitForReplay polls the golden service until the async replay task
// reaches a terminal status, so callers that share a t.TempDir()-backed
// storage do not race with the background writer during test cleanup.
func waitForReplay(t *testing.T, svc *golden.Service, taskID string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("replay did not complete within 10s")
		default:
		}
		view, found := svc.GetTask(taskID)
		if found && (view.Status == golden.ReplayTaskSucceeded || view.Status == golden.ReplayTaskFailed) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestGolden_RunQualityReplay_503WhenServiceNil(t *testing.T) {
	r := newGoldenTestEngine(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/quality-report/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestGolden_RunReplay_ProducesReport(t *testing.T) {
	dir := t.TempDir()
	storage := golden.NewFileReportStorage(dir)
	svc := mustNewGoldenService(t, storage)

	r := newGoldenTestEngine(t, svc)

	// Trigger replay.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/quality-report/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for the async replay to complete.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("replay did not complete within 5s")
		default:
		}
		// Poll GET /quality-report until we get 200.
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/quality-report", nil)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code == http.StatusOK {
			var body map[string]interface{}
			if err := json.Unmarshal(w2.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal report: %v", err)
			}
			if body["report_version"] != golden.ReportVersion {
				t.Errorf("report_version = %v, want %q", body["report_version"], golden.ReportVersion)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
