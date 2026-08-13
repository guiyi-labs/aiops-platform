package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/incident"
)

// incidentRepoStub is a minimal in-memory Repository for handler tests. It
// implements the full contract but only exercises the create/get/list paths
// exercised by the handler tests below.
type incidentRepoStub struct {
	nextID  int64
	byID    map[int64]incident.Incident
	sources map[string]int64
}

func newIncidentRepoStub() *incidentRepoStub {
	return &incidentRepoStub{nextID: 1, byID: map[int64]incident.Incident{}, sources: map[string]int64{}}
}

func (r *incidentRepoStub) Create(_ context.Context, record *incident.Incident) error {
	key := record.SourceType + ":" + record.SourceRef
	if _, exists := r.sources[key]; exists {
		return incident.ErrSourceAlreadyUsed
	}
	now := time.Now().UTC()
	record.ID = r.nextID
	record.Number = "INC-000001"
	record.Version = 1
	record.Status = incident.StatusOpen
	record.CreatedAt = now
	record.UpdatedAt = now
	r.sources[key] = record.ID
	r.byID[record.ID] = *record
	r.nextID++
	return nil
}

func (r *incidentRepoStub) Get(_ context.Context, id int64) (incident.Incident, error) {
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	return record, nil
}

func (r *incidentRepoStub) List(_ context.Context, _ incident.ListFilter) ([]incident.Incident, error) {
	items := make([]incident.Incident, 0, len(r.byID))
	for _, record := range r.byID {
		items = append(items, record)
	}
	return items, nil
}

func (r *incidentRepoStub) Summary(context.Context) (incident.Summary, error) {
	return incident.Summary{}, nil
}

func (r *incidentRepoStub) Transition(context.Context, int64, int64, string, incident.ActorRef, string) (incident.Incident, error) {
	return incident.Incident{}, errors.New("not implemented")
}

func (r *incidentRepoStub) Assign(context.Context, int64, int64, int64, incident.ActorRef, string) (incident.Incident, error) {
	return incident.Incident{}, errors.New("not implemented")
}

func (r *incidentRepoStub) AddFollower(context.Context, int64, int64, incident.ActorRef) (incident.Incident, error) {
	return incident.Incident{}, errors.New("not implemented")
}

func (r *incidentRepoStub) RemoveFollower(context.Context, int64, int64, incident.ActorRef) (incident.Incident, error) {
	return incident.Incident{}, errors.New("not implemented")
}

func (r *incidentRepoStub) AddNote(context.Context, int64, int64, incident.ActorRef, string) (incident.Incident, error) {
	return incident.Incident{}, errors.New("not implemented")
}

func (r *incidentRepoStub) SetPostmortem(context.Context, int64, int64, incident.ActorRef, string) (incident.Incident, error) {
	return incident.Incident{}, errors.New("not implemented")
}

func newIncidentTestEngine(t *testing.T, repo *incidentRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := incidentHandler{service: incident.NewService(repo)}
	api := r.Group("/api/v1", withTestActor())
	api.GET("/incidents", h.list)
	api.GET("/incidents/summary", h.summary)
	api.GET("/incidents/:incident_id", h.get)
	api.GET("/incidents/:incident_id/evidence", h.evidence)
	api.POST("/incidents", h.create)
	api.PATCH("/incidents/:incident_id", h.transition)
	return r
}

func performIncidentRequest(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var payload *bytes.Reader
	if body == "" {
		payload = bytes.NewReader(nil)
	} else {
		payload = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestIncidentHandler_CreateAndList(t *testing.T) {
	engine := newIncidentTestEngine(t, newIncidentRepoStub())

	invalid := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents", `{"source_type":"bogus"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid create code = %d, want 400", invalid.Code)
	}

	created := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents", `{
		"source_type":"finding",
		"source_ref":"finding:7:pod.pending.v1:Pod:default:web-0",
		"cluster_id":7,
		"title":"Pod pending",
		"severity":"warning",
		"summary":"stuck",
		"resource":{"kind":"Pod","namespace":"default","name":"web-0"}
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body %s", created.Code, created.Body.String())
	}
	var record incident.Incident
	if err := json.Unmarshal(created.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if record.Number == "" || record.Status != incident.StatusOpen {
		t.Errorf("unexpected created record: %+v", record)
	}

	duplicate := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents", `{
		"source_type":"finding",
		"source_ref":"finding:7:pod.pending.v1:Pod:default:web-0",
		"cluster_id":7,
		"title":"Pod pending",
		"severity":"warning",
		"resource":{"kind":"Pod","namespace":"default","name":"web-0"}
	}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate create code = %d, want 409", duplicate.Code)
	}

	list := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list code = %d", list.Code)
	}
	var listed struct {
		Items []incident.Incident `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 {
		t.Errorf("list total = %d, items = %d, want 1/1", listed.Total, len(listed.Items))
	}

	get := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents/1", "")
	if get.Code != http.StatusOK {
		t.Fatalf("get code = %d", get.Code)
	}
	missing := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents/999", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing code = %d, want 404", missing.Code)
	}
}

func TestIncidentHandler_Evidence(t *testing.T) {
	engine := newIncidentTestEngine(t, newIncidentRepoStub())
	created := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents", `{
		"source_type": "finding", "source_ref": "finding:9:code:kind:default:pod-a",
		"cluster_id": 3, "title": "manual pod issue", "severity": "high",
		"summary": "manual report", "resource": {"kind":"Pod","namespace":"default","name":"pod-a"}
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body %s", created.Code, created.Body.String())
	}
	var record incident.Incident
	if err := json.Unmarshal(created.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents/"+strconv.FormatInt(record.ID, 10)+"/evidence", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("evidence code = %d, body %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Items []incident.EvidenceItem `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("evidence items = %d, want 1", len(payload.Items))
	}
	item := payload.Items[0]
	if item.SourceType != incident.SourceTypeFinding || item.Title != "manual pod issue" {
		t.Errorf("unexpected evidence item: source=%s title=%s", item.SourceType, item.Title)
	}
	if item.DeepLink != "/incidents" {
		t.Errorf("deep_link = %s, want /incidents", item.DeepLink)
	}

	missing := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents/99999/evidence", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing evidence code = %d, want 404", missing.Code)
	}
}
