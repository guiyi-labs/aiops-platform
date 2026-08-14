package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/insight"
)

// incidentRepoStub is a minimal in-memory Repository for handler tests. It
// implements the full contract but only exercises the create/get/list paths
// exercised by the handler tests below.
type incidentRepoStub struct {
	nextID            int64
	byID              map[int64]incident.Incident
	sources           map[string]int64
	transitionErr     error
	addFollowerErr    error
	removeFollowerErr error
	addNoteErr        error
	setPostmortemErr  error
	summaryErr        error
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

func (r *incidentRepoStub) FindBySource(_ context.Context, sourceType, sourceRef string) (incident.Incident, error) {
	for _, record := range r.byID {
		if record.SourceType == sourceType && record.SourceRef == sourceRef {
			return record, nil
		}
	}
	return incident.Incident{}, incident.ErrNotFound
}

func (r *incidentRepoStub) List(_ context.Context, _ incident.ListFilter) ([]incident.Incident, error) {
	items := make([]incident.Incident, 0, len(r.byID))
	for _, record := range r.byID {
		items = append(items, record)
	}
	return items, nil
}

func (r *incidentRepoStub) Summary(_ context.Context) (incident.Summary, error) {
	if r.summaryErr != nil {
		return incident.Summary{}, r.summaryErr
	}
	return incident.Summary{}, nil
}

func (r *incidentRepoStub) Transition(_ context.Context, id, expectedVersion int64, toStatus string, _ incident.ActorRef, _ string) (incident.Incident, error) {
	if r.transitionErr != nil {
		return incident.Incident{}, r.transitionErr
	}
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	if record.Version != expectedVersion {
		return incident.Incident{}, incident.ErrVersionConflict
	}
	record.Status = toStatus
	record.Version++
	r.byID[id] = record
	return record, nil
}

func (r *incidentRepoStub) Assign(_ context.Context, id, expectedVersion, assigneeUserID int64, _ incident.ActorRef, _ string) (incident.Incident, error) {
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	if record.Version != expectedVersion {
		return incident.Incident{}, incident.ErrVersionConflict
	}
	record.Assignee = &incident.ActorRef{ID: assigneeUserID, Name: "ops-user"}
	record.Version++
	r.byID[id] = record
	return record, nil
}

func (r *incidentRepoStub) AddFollower(_ context.Context, id, userID int64, _ incident.ActorRef) (incident.Incident, error) {
	if r.addFollowerErr != nil {
		return incident.Incident{}, r.addFollowerErr
	}
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	record.Version++
	r.byID[id] = record
	return record, nil
}

func (r *incidentRepoStub) RemoveFollower(_ context.Context, id, userID int64, _ incident.ActorRef) (incident.Incident, error) {
	if r.removeFollowerErr != nil {
		return incident.Incident{}, r.removeFollowerErr
	}
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	record.Version++
	r.byID[id] = record
	return record, nil
}

func (r *incidentRepoStub) AddNote(_ context.Context, id, expectedVersion int64, _ incident.ActorRef, content string) (incident.Incident, error) {
	if r.addNoteErr != nil {
		return incident.Incident{}, r.addNoteErr
	}
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	record.Version++
	r.byID[id] = record
	return record, nil
}

func (r *incidentRepoStub) SetPostmortem(_ context.Context, id, expectedVersion int64, _ incident.ActorRef, content string) (incident.Incident, error) {
	if r.setPostmortemErr != nil {
		return incident.Incident{}, r.setPostmortemErr
	}
	record, ok := r.byID[id]
	if !ok {
		return incident.Incident{}, incident.ErrNotFound
	}
	record.Version++
	r.byID[id] = record
	return record, nil
}

func newIncidentTestEngine(t *testing.T, repo *incidentRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := incidentHandler{service: incident.NewService(repo)}
	api := r.Group("/api/v1", withTestActor())
	api.GET("/incidents", h.list)
	api.GET("/incidents/summary", h.summary)
	api.GET("/incidents/metrics", h.metrics)
	api.GET("/incidents/:incident_id", h.get)
	api.GET("/incidents/:incident_id/evidence", h.evidence)
	api.GET("/incidents/:incident_id/runbook", h.runbook)
	api.POST("/incidents", h.create)
	api.POST("/incidents/batch-assign", h.batchAssign)
	api.PATCH("/incidents/:incident_id", h.transition)
	return r
}

type incidentSourceResolverStub struct {
	info incident.SourceInfo
	err  error
}

func (r incidentSourceResolverStub) Resolve(context.Context, string, string, int64) (incident.SourceInfo, error) {
	if r.err != nil {
		return incident.SourceInfo{}, r.err
	}
	return r.info, nil
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

func TestIncidentHandler_MetricsValidatesWindowAndReturnsEmptySamples(t *testing.T) {
	engine := newIncidentTestEngine(t, newIncidentRepoStub())
	empty := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents/metrics?days=14", "")
	if empty.Code != http.StatusOK {
		t.Fatalf("metrics code = %d, body %s", empty.Code, empty.Body.String())
	}
	var metrics map[string]any
	if err := json.Unmarshal(empty.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if metrics["window_days"] != float64(14) || metrics["sample_limit"] != float64(incident.MetricsSampleLimit) {
		t.Fatalf("unexpected metrics metadata: %+v", metrics)
	}
	if metrics["mtta_seconds"] != nil || metrics["sla_compliance_rate"] != nil {
		t.Fatalf("empty lifecycle metrics must be null: %+v", metrics)
	}

	invalid := performIncidentRequest(engine, http.MethodGet, "/api/v1/incidents/metrics?days=91", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid metrics window code = %d, want 400", invalid.Code)
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

func TestIncidentHandler_RunbookIsReadOnlyAndFailClosed(t *testing.T) {
	repo := newIncidentRepoStub()
	created := performIncidentRequest(newIncidentTestEngine(t, repo), http.MethodPost, "/api/v1/incidents", `{
		"source_type": "finding", "source_ref": "finding:9:code:kind:default:pod-a",
		"cluster_id": 3, "title": "manual pod issue", "severity": "high",
		"summary": "manual report", "resource": {"kind":"Deployment","namespace":"default","name":"api"}
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body %s", created.Code, created.Body.String())
	}
	var record incident.Incident
	if err := json.Unmarshal(created.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := incidentHandler{
		service:        incident.NewService(repo),
		sourceResolver: incidentSourceResolverStub{info: incident.SourceInfo{Domain: "network", FindingCode: "NET-EXPOSE"}},
	}
	r.GET("/api/v1/incidents/:incident_id/runbook", h.runbook)
	path := "/api/v1/incidents/" + strconv.FormatInt(record.ID, 10) + "/runbook"
	resp := performIncidentRequest(r, http.MethodGet, path, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("runbook code = %d, body %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		IncidentID int64            `json:"incident_id"`
		Available  bool             `json:"available"`
		Runbook    *insight.Runbook `json:"runbook"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runbook: %v", err)
	}
	if payload.IncidentID != record.ID || !payload.Available || payload.Runbook == nil || !payload.Runbook.ReadOnly {
		t.Fatalf("unexpected runbook response: %+v", payload)
	}

	noResolver := incidentHandler{service: incident.NewService(repo)}
	noResolverRouter := gin.New()
	noResolverRouter.GET("/api/v1/incidents/:incident_id/runbook", noResolver.runbook)
	closed := performIncidentRequest(noResolverRouter, http.MethodGet, path, "")
	if closed.Code != http.StatusOK || !strings.Contains(closed.Body.String(), "source_resolver_unavailable") {
		t.Fatalf("runbook must fail closed without resolver: %d %s", closed.Code, closed.Body.String())
	}
}

func TestIncidentHandler_BatchAssign(t *testing.T) {
	repo := newIncidentRepoStub()
	engine := newIncidentTestEngine(t, repo)
	for index := 0; index < 2; index++ {
		record := &incident.Incident{
			Title:      "Pod pending",
			SourceType: incident.SourceTypeFinding,
			SourceRef:  "finding:7:code" + strconv.Itoa(index) + ":Pod:default:web-" + strconv.Itoa(index),
			ClusterID:  7,
			Severity:   incident.SeverityWarning,
			Summary:    "pending",
			Status:     incident.StatusOpen,
		}
		if err := repo.Create(context.Background(), record); err != nil {
			t.Fatalf("seed incident: %v", err)
		}
	}
	ids := make([]int64, 0, 2)
	for _, record := range repo.byID {
		ids = append(ids, record.ID)
	}
	body, _ := json.Marshal(map[string]any{"incident_ids": ids, "assignee_user_id": 2, "comment": "night shift"})
	recorder := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents/batch-assign", string(body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result incident.BatchAssignResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 2 || result.Assigned != 2 || len(result.Failed) != 0 {
		t.Fatalf("result = %+v", result)
	}
	for _, id := range ids {
		item, err := repo.Get(context.Background(), id)
		if err != nil || item.Assignee == nil || item.Assignee.ID != 2 {
			t.Fatalf("incident %d assignee = %+v err=%v", id, item.Assignee, err)
		}
	}
}

func TestIncidentHandler_BatchAssignValidation(t *testing.T) {
	repo := newIncidentRepoStub()
	engine := newIncidentTestEngine(t, repo)
	for _, payload := range []string{
		`{"incident_ids":[],"assignee_user_id":2}`,
		`{"incident_ids":[1],"assignee_user_id":0}`,
		`not-json`,
	} {
		recorder := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents/batch-assign", payload)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("payload=%s status=%d body=%s", payload, recorder.Code, recorder.Body.String())
		}
	}
	tooMany := make([]int64, incident.MaxBatchAssignSize+1)
	for index := range tooMany {
		tooMany[index] = int64(index + 1)
	}
	body, _ := json.Marshal(map[string]any{"incident_ids": tooMany, "assignee_user_id": 2})
	recorder := performIncidentRequest(engine, http.MethodPost, "/api/v1/incidents/batch-assign", string(body))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("too many status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

// --- M109: incidents handler error branches (transition/assign/follower/note/postmortem/summary) ---

func newIncidentHandlerTestRouter(stub *incidentRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := incidentHandler{service: incident.NewService(stub)}
	api := r.Group("/api/v1", withTestActor())
	api.GET("/incidents/summary", h.summary)
	api.POST("/incidents", h.create)
	api.GET("/incidents/:incident_id", h.get)
	api.PATCH("/incidents/:incident_id", h.transition)
	api.PUT("/incidents/:incident_id/assign", h.assign)
	api.POST("/incidents/:incident_id/followers", h.addFollower)
	api.DELETE("/incidents/:incident_id/followers/:user_id", h.removeFollower)
	api.POST("/incidents/:incident_id/notes", h.addNote)
	api.PUT("/incidents/:incident_id/postmortem", h.setPostmortem)
	api.GET("/incidents/:incident_id/export", h.export)
	return r
}

func incCreate(t *testing.T, r *gin.Engine) incident.Incident {
	t.Helper()
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents", `{
		"source_type":"finding","source_ref":"finding:unique:1","cluster_id":1,
		"title":"test","severity":"warning","resource":{"kind":"Pod","namespace":"default","name":"web"}
	}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed %d: %s", w.Code, w.Body.String())
	}
	var rec incident.Incident
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return rec
}

func TestIncidentSummarySuccess(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	w := performIncidentRequest(r, http.MethodGet, "/api/v1/incidents/summary", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentSummaryError(t *testing.T) {
	stub := newIncidentRepoStub()
	stub.summaryErr = errors.New("db down")
	r := newIncidentHandlerTestRouter(stub)
	w := performIncidentRequest(r, http.MethodGet, "/api/v1/incidents/summary", "")
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentTransitionSuccess(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	body := fmt.Sprintf(`{"expected_version":%d,"status":"confirmed"}`, rec.Version)
	w := performIncidentRequest(r, http.MethodPatch, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10), body)
	if w.Code != http.StatusOK || !contains(w.Body.String(), "confirmed") {
		t.Fatalf("expected 200 confirmed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentTransitionInvalidStatus(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPatch, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10),
		`{"expected_version":1,"status":"bogus"}`)
	if w.Code != http.StatusConflict || !contains(w.Body.String(), "INVALID_STATUS_TRANSITION") {
		t.Fatalf("expected 409 INVALID_STATUS_TRANSITION, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentTransitionNotFound(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	w := performIncidentRequest(r, http.MethodPatch, "/api/v1/incidents/999", `{"expected_version":1,"status":"resolved"}`)
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "INCIDENT_NOT_FOUND") {
		t.Fatalf("expected 404 INCIDENT_NOT_FOUND, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentTransitionVersionConflict(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	// version is 1, pass stale expected_version=2
	w := performIncidentRequest(r, http.MethodPatch, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10),
		`{"expected_version":2,"status":"confirmed"}`)
	if w.Code != http.StatusConflict || !contains(w.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("expected 409 VERSION_CONFLICT, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentTransitionRepoError(t *testing.T) {
	stub := newIncidentRepoStub()
	stub.transitionErr = errors.New("db down")
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPatch, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10),
		`{"expected_version":1,"status":"resolved"}`)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddFollowerSuccess(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/followers",
		`{"user_id":2}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddFollowerNotFound(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/999/followers", `{"user_id":2}`)
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "INCIDENT_NOT_FOUND") {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddFollowerRepoError(t *testing.T) {
	stub := newIncidentRepoStub()
	stub.addFollowerErr = errors.New("db down")
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/followers",
		`{"user_id":2}`)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentRemoveFollowerSuccess(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodDelete, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/followers/2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentRemoveFollowerBadUserID(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodDelete, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/followers/abc", "")
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "INVALID_USER_ID") {
		t.Fatalf("expected 400 INVALID_USER_ID, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentRemoveFollowerNotFound(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	// hit non-existent incident id → ErrNotFound
	w := performIncidentRequest(r, http.MethodDelete, "/api/v1/incidents/999/followers/2", "")
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "INCIDENT_NOT_FOUND") {
		t.Fatalf("expected 404 INCIDENT_NOT_FOUND, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentRemoveFollowerRepoError(t *testing.T) {
	stub := newIncidentRepoStub()
	stub.removeFollowerErr = errors.New("db down")
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodDelete, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/followers/2", "")
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddNoteSuccess(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	body := fmt.Sprintf(`{"expected_version":%d,"content":"looks bad"}`, rec.Version)
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/notes", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddNoteNotFound(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/999/notes", `{"expected_version":1,"content":"nope"}`)
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "INCIDENT_NOT_FOUND") {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddNoteEmptyContent(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	// service returns ErrInvalidNote for blank content
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/notes",
		fmt.Sprintf(`{"expected_version":%d,"content":"   "}`, rec.Version))
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "INVALID_NOTE") {
		t.Fatalf("expected 400 INVALID_NOTE, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentAddNoteRepoError(t *testing.T) {
	stub := newIncidentRepoStub()
	stub.addNoteErr = errors.New("db down")
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPost, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/notes",
		fmt.Sprintf(`{"expected_version":%d,"content":"note"}`, rec.Version))
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentSetPostmortemSuccess(t *testing.T) {
	stub := newIncidentRepoStub()
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPut, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/postmortem",
		fmt.Sprintf(`{"expected_version":%d,"content":"root cause found"}`, rec.Version))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentSetPostmortemNotFound(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	w := performIncidentRequest(r, http.MethodPut, "/api/v1/incidents/999/postmortem", `{"expected_version":1,"content":"x"}`)
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "INCIDENT_NOT_FOUND") {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentSetPostmortemRepoError(t *testing.T) {
	stub := newIncidentRepoStub()
	stub.setPostmortemErr = errors.New("db down")
	r := newIncidentHandlerTestRouter(stub)
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodPut, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/postmortem",
		fmt.Sprintf(`{"expected_version":%d,"content":"x"}`, rec.Version))
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentExportNotFound(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	w := performIncidentRequest(r, http.MethodGet, "/api/v1/incidents/999/export", "")
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "INCIDENT_NOT_FOUND") {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIncidentExportSuccess(t *testing.T) {
	r := newIncidentHandlerTestRouter(newIncidentRepoStub())
	rec := incCreate(t, r)
	w := performIncidentRequest(r, http.MethodGet, "/api/v1/incidents/"+strconv.FormatInt(rec.ID, 10)+"/export", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
