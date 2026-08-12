package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/diagnosis"
)

func replayTestRouter(repo diagnosis.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := diagnosis.NewService(nil, repo)
	router := gin.New()
	router.GET("/:diagnosis_id/replay", diagnosisHandler{service: svc}.replay)
	return router
}

func replaySampleRecord() diagnosis.Record {
	observed := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	return diagnosis.Record{
		ID:         7,
		ClusterID:  1,
		RuleID:     "node.not_ready.v1",
		Severity:   "critical",
		Status:     "confirmed",
		Resource:   diagnosis.ResourceRef{Kind: "Node", Name: "demo-node"},
		Summary:    "Node 未处于 Ready 状态。",
		ObservedAt: observed,
		Evidence: []diagnosis.Evidence{
			{Type: "node_condition", Source: "kubelet", Content: map[string]any{"type": "Ready", "status": "False", "last_transition_time": "2026-08-12T07:30:00Z"}},
			{Type: "node_condition", Source: "kubelet", Content: map[string]any{"type": "MemoryPressure", "status": "True", "last_transition_time": "2026-08-12T07:25:00Z"}},
		},
		Activities: []diagnosis.Activity{
			{ID: 10, Actor: diagnosis.ActorRef{Name: "operator-a"}, FromStatus: "open", ToStatus: "confirmed", Comment: "确认根因", CreatedAt: time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC)},
		},
		Assignments: []diagnosis.Assignment{
			{ID: 3, Actor: diagnosis.ActorRef{Name: "admin"}, ToAssignee: diagnosis.ActorRef{Name: "operator-b"}, Comment: "跟进", CreatedAt: time.Date(2026, 8, 12, 8, 20, 0, 0, time.UTC)},
		},
		Feedback: []diagnosis.Feedback{
			{ID: 1, Actor: diagnosis.ActorRef{Name: "operator-a"}, Verdict: "accurate", Comment: "", CreatedAt: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)},
		},
	}
}

func TestReplayHandlerReturnsStoredInsightChain(t *testing.T) {
	repo := &diagnosisRepositoryStub{getRecord: replaySampleRecord()}
	router := replayTestRouter(repo)

	request := httptest.NewRequest(http.MethodGet, "/7/replay", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var view diagnosis.ReplayView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if view.Schema != "aiops.diagnosis-replay/v1" {
		t.Errorf("schema = %s", view.Schema)
	}
	if view.DiagnosisID != 7 {
		t.Errorf("diagnosis_id = %d", view.DiagnosisID)
	}
	// created + 2 evidence + 1 activity + 1 assignment + 1 feedback
	if len(view.Steps) != 6 {
		t.Fatalf("steps = %d, want 6", len(view.Steps))
	}
	for i, step := range view.Steps {
		if step.Index != i {
			t.Errorf("step %d index = %d", i, step.Index)
		}
	}
	// Steps must be time-ordered: evidence 07:25, evidence 07:30, created 08:00,
	// assignment 08:20, transition 08:30, feedback 09:00.
	expectedStages := []diagnosis.ReplayStageID{
		diagnosis.StageEvidence,
		diagnosis.StageEvidence,
		diagnosis.StageDiagnosisCreated,
		diagnosis.StageActivity,
		diagnosis.StageActivity,
		diagnosis.StageActivity,
	}
	for i, want := range expectedStages {
		if view.Steps[i].Stage != want {
			t.Errorf("step %d stage = %s, want %s", i, view.Steps[i].Stage, want)
		}
	}
	if len(view.Stages) != 3 {
		t.Fatalf("stages = %+v, want 3", view.Stages)
	}
	if view.Stages[0].Stage != diagnosis.StageDiagnosisCreated || view.Stages[0].Count != 1 {
		t.Errorf("stage[0] = %+v", view.Stages[0])
	}
	if view.Stages[1].Stage != diagnosis.StageEvidence || view.Stages[1].Count != 2 {
		t.Errorf("stage[1] = %+v", view.Stages[1])
	}
	if view.Stages[2].Stage != diagnosis.StageActivity || view.Stages[2].Count != 3 {
		t.Errorf("stage[2] = %+v", view.Stages[2])
	}
}

func TestReplayHandlerRejectsInvalidID(t *testing.T) {
	router := replayTestRouter(&diagnosisRepositoryStub{})
	for _, target := range []string{"/0/replay", "/-1/replay", "/abc/replay"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", target, recorder.Code)
		}
	}
}

func TestReplayHandlerNotFound(t *testing.T) {
	repo := &diagnosisRepositoryStub{getErr: diagnosis.ErrRecordNotFound}
	router := replayTestRouter(repo)

	request := httptest.NewRequest(http.MethodGet, "/42/replay", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReplayHandlerSurvivesOptionalServiceErrors(t *testing.T) {
	// Explanations/remediations are nil here (unconfigured); the replay must
	// still render the stored record steps rather than fail.
	repo := &diagnosisRepositoryStub{getRecord: replaySampleRecord()}
	gin.SetMode(gin.TestMode)
	svc := diagnosis.NewService(nil, repo)
	router := gin.New()
	router.GET("/:diagnosis_id/replay", diagnosisHandler{service: svc}.replay)

	request := httptest.NewRequest(http.MethodGet, "/7/replay", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var view diagnosis.ReplayView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(view.Steps) == 0 {
		t.Fatal("expected stored steps to render")
	}
	if view.Stages[0].Stage != diagnosis.StageDiagnosisCreated {
		t.Errorf("stage[0] = %+v", view.Stages[0])
	}
}

var _ = errors.Is
