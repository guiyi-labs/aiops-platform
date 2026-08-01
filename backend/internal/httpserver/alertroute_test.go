package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/alertroute"
	"k8s-aiops.local/backend/internal/requestctx"
)

// alertRouteTestRepo is an in-memory alertroute.Repository for handler tests.
// It mirrors the behaviour of GormRepository: reads are creator-scoped, deletes
// fail with ErrReceiverInUse when routes reference the receiver, and IDs are
// assigned monotonically.
type alertRouteTestRepo struct {
	mu         sync.Mutex
	receivers  map[int64]*alertroute.Receiver
	routes     map[int64]*alertroute.Route
	silences   map[int64]*alertroute.Silence
	inhibits   map[int64]*alertroute.Inhibit
	deliveries map[int64]*alertroute.Delivery

	nextReceiverID int64
	nextRouteID    int64
	nextSilenceID  int64
	nextInhibitID  int64
	nextDeliveryID int64

	// firingSourceOverride, when non-nil, makes HasFiringSource return this
	// value without scanning deliveries (mirrors the service-test mock).
	firingSourceOverride *bool
}

func newAlertRouteTestRepo() *alertRouteTestRepo {
	return &alertRouteTestRepo{
		receivers:      make(map[int64]*alertroute.Receiver),
		routes:         make(map[int64]*alertroute.Route),
		silences:       make(map[int64]*alertroute.Silence),
		inhibits:       make(map[int64]*alertroute.Inhibit),
		deliveries:     make(map[int64]*alertroute.Delivery),
		nextReceiverID: 1,
		nextRouteID:    1,
		nextSilenceID:  1,
		nextInhibitID:  1,
		nextDeliveryID: 1,
	}
}

func (m *alertRouteTestRepo) CreateReceiver(_ context.Context, receiver *alertroute.Receiver) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	receiver.ID = m.nextReceiverID
	m.nextReceiverID++
	now := time.Now().UTC()
	receiver.CreatedAt = now
	receiver.UpdatedAt = now
	r := *receiver
	m.receivers[receiver.ID] = &r
	return nil
}

func (m *alertRouteTestRepo) GetReceiver(_ context.Context, id, creatorID int64) (alertroute.Receiver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receivers[id]
	if !ok || r.CreatorID != creatorID {
		return alertroute.Receiver{}, alertroute.ErrReceiverNotFound
	}
	return *r, nil
}

func (m *alertRouteTestRepo) GetReceiverByID(_ context.Context, id int64) (alertroute.Receiver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receivers[id]
	if !ok {
		return alertroute.Receiver{}, alertroute.ErrReceiverNotFound
	}
	return *r, nil
}

func (m *alertRouteTestRepo) ListReceivers(_ context.Context, creatorID int64) ([]alertroute.Receiver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Receiver
	for _, r := range m.receivers {
		if r.CreatorID == creatorID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *alertRouteTestRepo) DeleteReceiver(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receivers[id]
	if !ok || r.CreatorID != creatorID {
		return alertroute.ErrReceiverNotFound
	}
	for _, rt := range m.routes {
		if rt.ReceiverID == id && rt.CreatorID == creatorID {
			return alertroute.ErrReceiverInUse
		}
	}
	delete(m.receivers, id)
	return nil
}

func (m *alertRouteTestRepo) CreateRoute(_ context.Context, route *alertroute.Route) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	route.ID = m.nextRouteID
	m.nextRouteID++
	now := time.Now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	r := *route
	m.routes[route.ID] = &r
	return nil
}

func (m *alertRouteTestRepo) GetRoute(_ context.Context, id, creatorID int64) (alertroute.Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.routes[id]
	if !ok || r.CreatorID != creatorID {
		return alertroute.Route{}, alertroute.ErrRouteNotFound
	}
	return *r, nil
}

func (m *alertRouteTestRepo) ListRoutes(_ context.Context, creatorID int64) ([]alertroute.Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Route
	for _, r := range m.routes {
		if r.CreatorID == creatorID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *alertRouteTestRepo) UpdateRoute(_ context.Context, id, creatorID int64, input alertroute.PatchRouteInput) (alertroute.Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.routes[id]
	if !ok || r.CreatorID != creatorID {
		return alertroute.Route{}, alertroute.ErrRouteNotFound
	}
	if input.Priority != nil {
		r.Priority = *input.Priority
	}
	if input.Enabled != nil {
		r.Enabled = *input.Enabled
	}
	if input.GroupInterval != nil {
		r.GroupInterval = input.GroupInterval
	}
	if input.RepeatInterval != nil {
		r.RepeatInterval = input.RepeatInterval
	}
	r.UpdatedAt = time.Now().UTC()
	return *r, nil
}

func (m *alertRouteTestRepo) DeleteRoute(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.routes[id]
	if !ok || r.CreatorID != creatorID {
		return alertroute.ErrRouteNotFound
	}
	delete(m.routes, id)
	return nil
}

func (m *alertRouteTestRepo) ListEnabledRoutes(_ context.Context) ([]alertroute.Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Route
	for _, r := range m.routes {
		if r.Enabled {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *alertRouteTestRepo) CreateSilence(_ context.Context, silence *alertroute.Silence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	silence.ID = m.nextSilenceID
	m.nextSilenceID++
	silence.CreatedAt = time.Now().UTC()
	s := *silence
	m.silences[silence.ID] = &s
	return nil
}

func (m *alertRouteTestRepo) ListSilences(_ context.Context, filter alertroute.SilenceListFilter) ([]alertroute.Silence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var result []alertroute.Silence
	for _, s := range m.silences {
		if filter.CreatorID != nil && s.CreatorID != *filter.CreatorID {
			continue
		}
		if filter.ClusterID != nil && (s.ClusterID == nil || *s.ClusterID != *filter.ClusterID) {
			continue
		}
		if filter.Active != nil {
			isActive := !s.StartsAt.After(now) && s.EndsAt.After(now)
			if *filter.Active != isActive {
				continue
			}
		}
		result = append(result, *s)
	}
	return result, nil
}

func (m *alertRouteTestRepo) DeleteSilence(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.silences[id]
	if !ok || s.CreatorID != creatorID {
		return alertroute.ErrSilenceNotFound
	}
	delete(m.silences, id)
	return nil
}

func (m *alertRouteTestRepo) ListActiveSilences(_ context.Context, now time.Time) ([]alertroute.Silence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Silence
	for _, s := range m.silences {
		if !s.StartsAt.After(now) && s.EndsAt.After(now) {
			result = append(result, *s)
		}
	}
	return result, nil
}

// --- Inhibits (M51) ---

func (m *alertRouteTestRepo) CreateInhibit(_ context.Context, inhibit *alertroute.Inhibit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inhibit.ID = m.nextInhibitID
	m.nextInhibitID++
	now := time.Now().UTC()
	inhibit.CreatedAt = now
	inhibit.UpdatedAt = now
	ins := *inhibit
	m.inhibits[inhibit.ID] = &ins
	return nil
}

func (m *alertRouteTestRepo) ListInhibits(_ context.Context, filter alertroute.InhibitListFilter) ([]alertroute.Inhibit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Inhibit
	for _, inh := range m.inhibits {
		if filter.CreatorID != nil && inh.CreatorID != *filter.CreatorID {
			continue
		}
		if filter.Enabled != nil && inh.Enabled != *filter.Enabled {
			continue
		}
		result = append(result, *inh)
	}
	return result, nil
}

func (m *alertRouteTestRepo) DeleteInhibit(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inh, ok := m.inhibits[id]
	if !ok || inh.CreatorID != creatorID {
		return alertroute.ErrInhibitNotFound
	}
	delete(m.inhibits, id)
	return nil
}

func (m *alertRouteTestRepo) ListEnabledInhibits(_ context.Context) ([]alertroute.Inhibit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Inhibit
	for _, inh := range m.inhibits {
		if inh.Enabled {
			result = append(result, *inh)
		}
	}
	return result, nil
}

func (m *alertRouteTestRepo) HasFiringSource(_ context.Context, _ alertroute.MatchAlert, _ time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.firingSourceOverride != nil {
		return *m.firingSourceOverride, nil
	}
	for _, d := range m.deliveries {
		if d.EventType != alertroute.EventTypeFiring {
			continue
		}
		if d.Status != alertroute.DeliveryStatusPending && d.Status != alertroute.DeliveryStatusDelivering {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (m *alertRouteTestRepo) CreateDelivery(_ context.Context, delivery *alertroute.Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delivery.ID = m.nextDeliveryID
	m.nextDeliveryID++
	now := time.Now().UTC()
	delivery.CreatedAt = now
	delivery.UpdatedAt = now
	d := *delivery
	m.deliveries[delivery.ID] = &d
	return nil
}

func (m *alertRouteTestRepo) FindActiveDelivery(_ context.Context, routeID int64, dedupeKey, eventType string) (*alertroute.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.deliveries {
		if d.RouteID == routeID && d.DedupeKey == dedupeKey && d.EventType == eventType && d.Status != alertroute.DeliveryStatusDead {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *alertRouteTestRepo) ClaimDeliveries(_ context.Context, batchSize int, now time.Time) ([]alertroute.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alertroute.Delivery
	for _, d := range m.deliveries {
		if (d.Status == alertroute.DeliveryStatusPending && !d.NextAttemptAt.After(now)) ||
			(d.Status == alertroute.DeliveryStatusDelivering && !d.NextAttemptAt.After(now)) {
			d.Status = alertroute.DeliveryStatusDelivering
			result = append(result, *d)
			if len(result) >= batchSize {
				break
			}
		}
	}
	return result, nil
}

func (m *alertRouteTestRepo) MarkDelivered(_ context.Context, id int64, deliveredAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deliveries[id]
	if !ok {
		return alertroute.ErrDeliveryNotFound
	}
	d.Attempts++
	d.Status = alertroute.DeliveryStatusDelivered
	d.DeliveredAt = &deliveredAt
	d.LastError = ""
	return nil
}

func (m *alertRouteTestRepo) MarkFailed(_ context.Context, id int64, maxAttempts int, nextAttempt time.Time, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deliveries[id]
	if !ok {
		return alertroute.ErrDeliveryNotFound
	}
	d.Attempts++
	if d.Attempts >= maxAttempts {
		d.Status = alertroute.DeliveryStatusDead
	} else {
		d.Status = alertroute.DeliveryStatusPending
	}
	d.NextAttemptAt = nextAttempt
	d.DeliveredAt = nil
	d.LastError = message
	return nil
}

func (m *alertRouteTestRepo) ListDeliveries(_ context.Context, filter alertroute.DeliveryListFilter) (alertroute.ListResponse[alertroute.Delivery], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var items []alertroute.Delivery
	for _, d := range m.deliveries {
		if filter.ReceiverID != nil && d.ReceiverID != *filter.ReceiverID {
			continue
		}
		if filter.Status != "" && d.Status != filter.Status {
			continue
		}
		items = append(items, *d)
	}
	return alertroute.ListResponse[alertroute.Delivery]{Items: items, Total: len(items)}, nil
}

var _ alertroute.Repository = (*alertRouteTestRepo)(nil)

const alertRouteActorID int64 = 42

func newAlertRouteRouter(repo alertroute.Repository) (*gin.Engine, alertrouteHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{ActorID: alertRouteActorID, RequestID: "alertroute-test"}))
		c.Next()
	})
	handler := alertrouteHandler{service: alertroute.NewService(repo, nil)}
	router.GET("/api/v1/alert-routes/receivers", handler.listReceivers)
	router.POST("/api/v1/alert-routes/receivers", handler.createReceiver)
	router.DELETE("/api/v1/alert-routes/receivers/:id", handler.deleteReceiver)
	router.GET("/api/v1/alert-routes", handler.listRoutes)
	router.POST("/api/v1/alert-routes", handler.createRoute)
	router.PATCH("/api/v1/alert-routes/:id", handler.updateRoute)
	router.DELETE("/api/v1/alert-routes/:id", handler.deleteRoute)
	router.GET("/api/v1/alert-routes/silences", handler.listSilences)
	router.POST("/api/v1/alert-routes/silences", handler.createSilence)
	router.DELETE("/api/v1/alert-routes/silences/:id", handler.deleteSilence)
	router.GET("/api/v1/alert-routes/inhibits", handler.listInhibits)
	router.POST("/api/v1/alert-routes/inhibits", handler.createInhibit)
	router.DELETE("/api/v1/alert-routes/inhibits/:id", handler.deleteInhibit)
	router.GET("/api/v1/alert-routes/deliveries", handler.listDeliveries)
	return router, handler
}

// validReceiverSecret returns a secret that satisfies MinSecretLen.
func validReceiverSecret() string { return strings.Repeat("s", alertroute.MinSecretLen) }
func validReceiverURL() string    { return "https://example.com/webhook" }

func mustCreateReceiver(t *testing.T, repo alertroute.Repository, name string) alertroute.Receiver {
	t.Helper()
	svc := alertroute.NewService(repo, nil)
	receiver, err := svc.CreateReceiver(context.Background(), alertRouteActorID, name, validReceiverURL(), validReceiverSecret())
	if err != nil {
		t.Fatalf("seed CreateReceiver(%q) error = %v", name, err)
	}
	return receiver
}

// --- Receivers ---

func TestAlertRouteListReceiversEmpty(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/receivers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.ReceiverView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteListReceiversWithItems(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustCreateReceiver(t, repo, "pagerduty")
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/receivers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.ReceiverView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Name != "pagerduty" {
		t.Fatalf("items = %#v", response.Items)
	}
	if response.Items[0].URLMasked == validReceiverURL() || !strings.Contains(response.Items[0].URLMasked, "***") {
		t.Fatalf("url_masked = %q, must be masked", response.Items[0].URLMasked)
	}
}

func TestAlertRouteCreateReceiverValid(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	body, _ := json.Marshal(map[string]string{"name": "pagerduty", "url": validReceiverURL(), "secret": validReceiverSecret()})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/receivers", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var view alertroute.ReceiverView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ID == 0 || view.Name != "pagerduty" {
		t.Fatalf("view = %#v", view)
	}
	if strings.Contains(recorder.Body.String(), validReceiverSecret()) {
		t.Fatalf("secret leaked into response: %s", recorder.Body.String())
	}
}

func TestAlertRouteCreateReceiverInvalidInput(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	body, _ := json.Marshal(map[string]string{"name": "pagerduty", "url": "http://insecure.example.com", "secret": "short"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/receivers", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAlertRouteCreateReceiverDuplicateName(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustCreateReceiver(t, repo, "pagerduty")
	router, _ := newAlertRouteRouter(repo)
	body, _ := json.Marshal(map[string]string{"name": "PagerDuty", "url": validReceiverURL(), "secret": validReceiverSecret()})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/receivers", bytes.NewReader(body)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !containsCode(recorder.Body.String(), "RECEIVER_NAME_EXISTS") {
		t.Fatalf("body = %q, want code RECEIVER_NAME_EXISTS", recorder.Body.String())
	}
}

func TestAlertRouteDeleteReceiverSuccess(t *testing.T) {
	repo := newAlertRouteTestRepo()
	receiver := mustCreateReceiver(t, repo, "pagerduty")
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/receivers/"+strconvI64(receiver.ID), nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
}

func TestAlertRouteDeleteReceiverNotFound(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/receivers/999", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

// --- Routes ---

func TestAlertRouteListRoutesEmpty(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.RouteView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteListRoutesWithItems(t *testing.T) {
	repo := newAlertRouteTestRepo()
	receiver := mustCreateReceiver(t, repo, "pagerduty")
	mustCreateRoute(t, repo, receiver.ID)
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.RouteView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].ReceiverName != "pagerduty" {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteCreateRouteValid(t *testing.T) {
	repo := newAlertRouteTestRepo()
	receiver := mustCreateReceiver(t, repo, "pagerduty")
	router, _ := newAlertRouteRouter(repo)
	body, _ := json.Marshal(map[string]any{"receiver_id": receiver.ID, "priority": 5, "rule_name": "cpu-hot", "severity": "critical", "dedupe_key": "{{.ClusterID}}:{{.RuleName}}"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var view alertroute.RouteView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ID == 0 || view.Priority != 5 || !view.Enabled {
		t.Fatalf("view = %#v", view)
	}
}

func TestAlertRouteCreateRouteInvalidPriority(t *testing.T) {
	repo := newAlertRouteTestRepo()
	receiver := mustCreateReceiver(t, repo, "pagerduty")
	router, _ := newAlertRouteRouter(repo)
	body, _ := json.Marshal(map[string]any{"receiver_id": receiver.ID, "priority": 200, "dedupe_key": "k"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAlertRouteUpdateRouteValid(t *testing.T) {
	repo := newAlertRouteTestRepo()
	receiver := mustCreateReceiver(t, repo, "pagerduty")
	route := mustCreateRoute(t, repo, receiver.ID)
	router, _ := newAlertRouteRouter(repo)
	body, _ := json.Marshal(map[string]any{"priority": 20, "enabled": false})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/v1/alert-routes/"+strconvI64(route.ID), bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var view alertroute.RouteView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Priority != 20 || view.Enabled {
		t.Fatalf("view = %#v", view)
	}
}

func TestAlertRouteUpdateRouteNotFound(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	body, _ := json.Marshal(map[string]any{"priority": 20})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/v1/alert-routes/999", bytes.NewReader(body)))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAlertRouteDeleteRouteSuccess(t *testing.T) {
	repo := newAlertRouteTestRepo()
	receiver := mustCreateReceiver(t, repo, "pagerduty")
	route := mustCreateRoute(t, repo, receiver.ID)
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/"+strconvI64(route.ID), nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
}

func TestAlertRouteDeleteRouteNotFound(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/999", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

// --- Silences ---

func TestAlertRouteListSilencesEmpty(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/silences", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.SilenceView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteListSilencesWithItems(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustCreateSilence(t, repo)
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/silences", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.SilenceView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteListSilencesActiveFilter(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustCreateSilence(t, repo)
	router, _ := newAlertRouteRouter(repo)
	// active=true must return the active silence.
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/silences?active=true", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.SilenceView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("active items = %#v, want 1", response.Items)
	}
	// active=false must return none (the seeded silence is active).
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/silences?active=false", nil))
	var response2 struct {
		Items []alertroute.SilenceView `json:"items"`
	}
	if err := json.Unmarshal(recorder2.Body.Bytes(), &response2); err != nil {
		t.Fatal(err)
	}
	if len(response2.Items) != 0 {
		t.Fatalf("inactive items = %#v, want 0", response2.Items)
	}
}

func TestAlertRouteCreateSilenceValid(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"reason": "planned maintenance", "starts_at": now.Add(-time.Minute).Format(time.RFC3339Nano), "ends_at": now.Add(time.Hour).Format(time.RFC3339Nano),
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/silences", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var view alertroute.SilenceView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ID == 0 || view.Reason != "planned maintenance" {
		t.Fatalf("view = %#v", view)
	}
}

func TestAlertRouteCreateSilenceInvalidDuration(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"reason": "too long", "starts_at": now.Format(time.RFC3339Nano), "ends_at": now.Add(alertroute.MaxSilenceDuration + time.Hour).Format(time.RFC3339Nano),
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/silences", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAlertRouteCreateSilenceMissingReason(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"starts_at": now.Add(-time.Minute).Format(time.RFC3339Nano), "ends_at": now.Add(time.Hour).Format(time.RFC3339Nano),
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/silences", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAlertRouteDeleteSilenceSuccess(t *testing.T) {
	repo := newAlertRouteTestRepo()
	silence := mustCreateSilence(t, repo)
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/silences/"+strconvI64(silence.ID), nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
}

func TestAlertRouteDeleteSilenceNotFound(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/silences/999", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

// --- Deliveries ---

func TestAlertRouteListDeliveriesEmpty(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/deliveries", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response alertroute.ListResponse[alertroute.DeliveryView]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 || response.Total != 0 {
		t.Fatalf("response = %#v", response)
	}
}

func TestAlertRouteListDeliveriesWithItems(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustSeedDelivery(t, repo, alertroute.DeliveryStatusDelivered)
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/deliveries", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response alertroute.ListResponse[alertroute.DeliveryView]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("items = %#v", response.Items)
	}
}

// --- Inhibits (M51) ---

func TestAlertRouteListInhibitsEmpty(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/inhibits", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.InhibitView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteCreateInhibitValid(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	body, _ := json.Marshal(map[string]any{
		"source_rule_name": "db-down", "source_severity": "critical",
		"target_rule_name": "app-503", "target_severity": "warning",
		"reason": "db outage suppresses app noise",
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/inhibits", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var view alertroute.InhibitView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ID == 0 || !view.Enabled {
		t.Fatalf("view = %#v", view)
	}
	if view.SourceRuleName != "db-down" || view.TargetRuleName != "app-503" {
		t.Fatalf("view matchers = %#v", view)
	}
	if view.CreatorID != alertRouteActorID {
		t.Fatalf("creator_id = %d, want %d", view.CreatorID, alertRouteActorID)
	}
}

func TestAlertRouteCreateInhibitMissingReason(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	body, _ := json.Marshal(map[string]any{
		"source_rule_name": "db-down", "target_rule_name": "app-503",
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/inhibits", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAlertRouteCreateInhibitInvalidWildcards(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	body, _ := json.Marshal(map[string]any{
		"source_rule_name": "db-down",
		"target_rule_name": "",
		"reason":           "both sides must have at least one matcher",
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/alert-routes/inhibits", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAlertRouteListInhibitsWithItems(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustCreateInhibit(t, repo, "db-down", "app-503")
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/inhibits", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.InhibitView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].SourceRuleName != "db-down" {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteListInhibitsEnabledFilter(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustCreateInhibit(t, repo, "db-down", "app-503")
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/inhibits?enabled=true", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []alertroute.InhibitView `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("items = %#v, want 1 enabled", response.Items)
	}
}

func TestAlertRouteListInhibitsInvalidEnabledQuery(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/inhibits?enabled=maybe", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestAlertRouteDeleteInhibitValid(t *testing.T) {
	repo := newAlertRouteTestRepo()
	inh := mustCreateInhibit(t, repo, "db-down", "app-503")
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/inhibits/"+strconv.FormatInt(inh.ID, 10), nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	// Subsequent delete returns 404.
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/inhibits/"+strconv.FormatInt(inh.ID, 10), nil))
	if recorder2.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want %d", recorder2.Code, http.StatusNotFound)
	}
}

func TestAlertRouteDeleteInhibitNotFound(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/inhibits/999", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAlertRouteDeleteInhibitInvalidID(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/inhibits/abc", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

// mustCreateInhibit seeds a valid inhibit via the service so handler tests
// start with persisted state.
func mustCreateInhibit(t *testing.T, repo alertroute.Repository, sourceRule, targetRule string) alertroute.Inhibit {
	t.Helper()
	svc := alertroute.NewService(repo, nil)
	inh, err := svc.CreateInhibit(context.Background(), &alertroute.Inhibit{
		CreatorID:      alertRouteActorID,
		SourceRuleName: sourceRule,
		SourceSeverity: "critical",
		TargetRuleName: targetRule,
		TargetSeverity: "warning",
		Reason:         "test inhibit",
	})
	if err != nil {
		t.Fatalf("seed CreateInhibit error = %v", err)
	}
	return inh
}

func TestAlertRouteListDeliveriesStatusFilter(t *testing.T) {
	repo := newAlertRouteTestRepo()
	mustSeedDelivery(t, repo, alertroute.DeliveryStatusDelivered)
	mustSeedDelivery(t, repo, alertroute.DeliveryStatusDead)
	router, _ := newAlertRouteRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/alert-routes/deliveries?status=delivered", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response alertroute.ListResponse[alertroute.DeliveryView]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Status != alertroute.DeliveryStatusDelivered {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestAlertRouteDeleteReceiverInvalidID(t *testing.T) {
	router, _ := newAlertRouteRouter(newAlertRouteTestRepo())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/alert-routes/receivers/not-a-number", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

// --- seed helpers ---

func mustCreateRoute(t *testing.T, repo alertroute.Repository, receiverID int64) alertroute.Route {
	t.Helper()
	svc := alertroute.NewService(repo, nil)
	route, err := svc.CreateRoute(context.Background(), &alertroute.Route{
		ReceiverID: receiverID, CreatorID: alertRouteActorID, Priority: 5,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	if err != nil {
		t.Fatalf("seed CreateRoute error = %v", err)
	}
	return route
}

func mustCreateSilence(t *testing.T, repo alertroute.Repository) alertroute.Silence {
	t.Helper()
	svc := alertroute.NewService(repo, nil)
	now := time.Now().UTC()
	silence, err := svc.CreateSilence(context.Background(), &alertroute.Silence{
		CreatorID: alertRouteActorID, Reason: "planned maintenance",
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed CreateSilence error = %v", err)
	}
	return silence
}

func mustSeedDelivery(t *testing.T, repo alertroute.Repository, status string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateDelivery(ctx, &alertroute.Delivery{
		RouteID: 1, ReceiverID: 1, AlertInstanceID: 100, EventType: alertroute.EventTypeFiring,
		DedupeKey: "1:cpu-hot", Status: status, NextAttemptAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed CreateDelivery error = %v", err)
	}
}

func strconvI64(n int64) string { return strconv.FormatInt(n, 10) }
