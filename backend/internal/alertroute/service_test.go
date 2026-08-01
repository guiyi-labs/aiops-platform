package alertroute

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- mock repository ---

type mockRepository struct {
	mu         sync.Mutex
	receivers  map[int64]*Receiver
	routes     map[int64]*Route
	silences   map[int64]*Silence
	inhibits   map[int64]*Inhibit
	deliveries map[int64]*Delivery

	nextReceiverID int64
	nextRouteID    int64
	nextSilenceID  int64
	nextInhibitID  int64
	nextDeliveryID int64

	// dispatch call tracking
	deliveredID    int64
	deliveredCount int
	failedID       int64
	failedCount    int
	failedMax      int
	failedNext     time.Time
	failedMessage  string

	// inhibit source-firing override; when non-nil, HasFiringSource returns
	// this value instead of scanning deliveries.
	firingSourceOverride *bool
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		receivers:      make(map[int64]*Receiver),
		routes:         make(map[int64]*Route),
		silences:       make(map[int64]*Silence),
		inhibits:       make(map[int64]*Inhibit),
		deliveries:     make(map[int64]*Delivery),
		nextReceiverID: 1,
		nextRouteID:    1,
		nextSilenceID:  1,
		nextInhibitID:  1,
		nextDeliveryID: 1,
	}
}

func (m *mockRepository) CreateReceiver(_ context.Context, receiver *Receiver) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	receiver.ID = m.nextReceiverID
	m.nextReceiverID++
	receiver.CreatedAt = time.Now().UTC()
	receiver.UpdatedAt = receiver.CreatedAt
	r := *receiver
	m.receivers[receiver.ID] = &r
	return nil
}

func (m *mockRepository) GetReceiver(_ context.Context, id, creatorID int64) (Receiver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receivers[id]
	if !ok || r.CreatorID != creatorID {
		return Receiver{}, ErrReceiverNotFound
	}
	return *r, nil
}

func (m *mockRepository) GetReceiverByID(_ context.Context, id int64) (Receiver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receivers[id]
	if !ok {
		return Receiver{}, ErrReceiverNotFound
	}
	return *r, nil
}

func (m *mockRepository) ListReceivers(_ context.Context, creatorID int64) ([]Receiver, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Receiver
	for _, r := range m.receivers {
		if r.CreatorID == creatorID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockRepository) DeleteReceiver(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receivers[id]
	if !ok || r.CreatorID != creatorID {
		return ErrReceiverNotFound
	}
	for _, rt := range m.routes {
		if rt.ReceiverID == id && rt.CreatorID == creatorID {
			return ErrReceiverInUse
		}
	}
	delete(m.receivers, id)
	return nil
}

func (m *mockRepository) CreateRoute(_ context.Context, route *Route) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	route.ID = m.nextRouteID
	m.nextRouteID++
	route.CreatedAt = time.Now().UTC()
	route.UpdatedAt = route.CreatedAt
	r := *route
	m.routes[route.ID] = &r
	return nil
}

func (m *mockRepository) GetRoute(_ context.Context, id, creatorID int64) (Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.routes[id]
	if !ok || r.CreatorID != creatorID {
		return Route{}, ErrRouteNotFound
	}
	return *r, nil
}

func (m *mockRepository) ListRoutes(_ context.Context, creatorID int64) ([]Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Route
	for _, r := range m.routes {
		if r.CreatorID == creatorID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockRepository) UpdateRoute(_ context.Context, id, creatorID int64, input PatchRouteInput) (Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.routes[id]
	if !ok || r.CreatorID != creatorID {
		return Route{}, ErrRouteNotFound
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

func (m *mockRepository) DeleteRoute(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.routes[id]
	if !ok || r.CreatorID != creatorID {
		return ErrRouteNotFound
	}
	delete(m.routes, id)
	return nil
}

func (m *mockRepository) ListEnabledRoutes(_ context.Context) ([]Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Route
	for _, r := range m.routes {
		if r.Enabled {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockRepository) CreateSilence(_ context.Context, silence *Silence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	silence.ID = m.nextSilenceID
	m.nextSilenceID++
	silence.CreatedAt = time.Now().UTC()
	s := *silence
	m.silences[silence.ID] = &s
	return nil
}

func (m *mockRepository) ListSilences(_ context.Context, filter SilenceListFilter) ([]Silence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Silence
	for _, s := range m.silences {
		if filter.CreatorID != nil && s.CreatorID != *filter.CreatorID {
			continue
		}
		if filter.ClusterID != nil && (s.ClusterID == nil || *s.ClusterID != *filter.ClusterID) {
			continue
		}
		result = append(result, *s)
	}
	return result, nil
}

func (m *mockRepository) DeleteSilence(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.silences[id]
	if !ok || s.CreatorID != creatorID {
		return ErrSilenceNotFound
	}
	delete(m.silences, id)
	return nil
}

func (m *mockRepository) ListActiveSilences(_ context.Context, now time.Time) ([]Silence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Silence
	for _, s := range m.silences {
		if !s.StartsAt.After(now) && s.EndsAt.After(now) {
			result = append(result, *s)
		}
	}
	return result, nil
}

// --- Inhibits (M51) ---

func (m *mockRepository) CreateInhibit(_ context.Context, inhibit *Inhibit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inhibit.ID = m.nextInhibitID
	m.nextInhibitID++
	inhibit.CreatedAt = time.Now().UTC()
	inhibit.UpdatedAt = inhibit.CreatedAt
	ins := *inhibit
	m.inhibits[inhibit.ID] = &ins
	return nil
}

func (m *mockRepository) ListInhibits(_ context.Context, filter InhibitListFilter) ([]Inhibit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Inhibit
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

func (m *mockRepository) DeleteInhibit(_ context.Context, id, creatorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inh, ok := m.inhibits[id]
	if !ok || inh.CreatorID != creatorID {
		return ErrInhibitNotFound
	}
	delete(m.inhibits, id)
	return nil
}

func (m *mockRepository) ListEnabledInhibits(_ context.Context) ([]Inhibit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Inhibit
	for _, inh := range m.inhibits {
		if inh.Enabled {
			result = append(result, *inh)
		}
	}
	return result, nil
}

func (m *mockRepository) HasFiringSource(_ context.Context, source MatchAlert, _ time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.firingSourceOverride != nil {
		return *m.firingSourceOverride, nil
	}
	for _, d := range m.deliveries {
		if d.EventType != EventTypeFiring {
			continue
		}
		if d.Status != DeliveryStatusPending && d.Status != DeliveryStatusDelivering {
			continue
		}
		if source.ClusterID != 0 && d.ClusterID != source.ClusterID {
			continue
		}
		if source.RuleName != "" && d.RuleName != source.RuleName {
			continue
		}
		if source.Severity != "" && d.Severity != source.Severity {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (m *mockRepository) CreateDelivery(_ context.Context, delivery *Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delivery.ID = m.nextDeliveryID
	m.nextDeliveryID++
	delivery.CreatedAt = time.Now().UTC()
	delivery.UpdatedAt = delivery.CreatedAt
	d := *delivery
	m.deliveries[delivery.ID] = &d
	return nil
}

func (m *mockRepository) FindActiveDelivery(_ context.Context, routeID int64, dedupeKey, eventType string) (*Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.deliveries {
		if d.RouteID == routeID && d.DedupeKey == dedupeKey && d.EventType == eventType && d.Status != DeliveryStatusDead {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) ClaimDeliveries(_ context.Context, batchSize int, now time.Time) ([]Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Delivery
	for _, d := range m.deliveries {
		if (d.Status == DeliveryStatusPending && !d.NextAttemptAt.After(now)) ||
			(d.Status == DeliveryStatusDelivering && !d.NextAttemptAt.After(now)) {
			d.Status = DeliveryStatusDelivering
			result = append(result, *d)
			if len(result) >= batchSize {
				break
			}
		}
	}
	return result, nil
}

func (m *mockRepository) MarkDelivered(_ context.Context, id int64, deliveredAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deliveries[id]
	if !ok {
		return ErrDeliveryNotFound
	}
	d.Attempts++
	d.Status = DeliveryStatusDelivered
	d.DeliveredAt = &deliveredAt
	d.LastError = ""
	m.deliveredID = id
	m.deliveredCount++
	return nil
}

func (m *mockRepository) MarkFailed(_ context.Context, id int64, maxAttempts int, nextAttempt time.Time, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deliveries[id]
	if !ok {
		return ErrDeliveryNotFound
	}
	d.Attempts++
	if d.Attempts >= maxAttempts {
		d.Status = DeliveryStatusDead
	} else {
		d.Status = DeliveryStatusPending
	}
	d.NextAttemptAt = nextAttempt
	d.DeliveredAt = nil
	d.LastError = message
	m.failedID = id
	m.failedCount++
	m.failedMax = maxAttempts
	m.failedNext = nextAttempt
	m.failedMessage = message
	return nil
}

func (m *mockRepository) ListDeliveries(_ context.Context, filter DeliveryListFilter) (ListResponse[Delivery], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var items []Delivery
	for _, d := range m.deliveries {
		if filter.ReceiverID != nil && d.ReceiverID != *filter.ReceiverID {
			continue
		}
		if filter.Status != "" && d.Status != filter.Status {
			continue
		}
		items = append(items, *d)
	}
	return ListResponse[Delivery]{Items: items, Total: len(items)}, nil
}

var _ Repository = (*mockRepository)(nil)

// --- helpers ---

func newTestService(repo Repository) *Service {
	s := NewService(repo, nil)
	fixed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	return s
}

func validSecret() string   { return strings.Repeat("a", MinSecretLen) }
func validHTTPSURL() string { return "https://example.com/webhook" }

// --- validateReceiver tests ---

func TestValidateReceiver(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		secret  string
		rename  string
		wantErr error
	}{
		{"valid", "https://example.com/webhook", validSecret(), "ok", nil},
		{"empty name", "https://example.com/webhook", validSecret(), "", ErrInvalidReceiver},
		{"name too long", "https://example.com/webhook", validSecret(), strings.Repeat("n", MaxReceiverNameLen+1), ErrInvalidReceiver},
		{"non-HTTPS URL", "http://example.com/webhook", validSecret(), "ok", ErrInvalidReceiver},
		{"secret too short", "https://example.com/webhook", "short", "ok", ErrInvalidReceiver},
		{"missing host", "https://", validSecret(), "ok", ErrInvalidReceiver},
		{"invalid URL", "://not-a-url", validSecret(), "ok", ErrInvalidReceiver},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReceiver(tc.rename, tc.url, tc.secret)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validateReceiver(%q,%q) = %v, want %v", tc.rename, tc.url, err, tc.wantErr)
			}
		})
	}
}

// --- validateRoute tests ---

func TestValidateRoute(t *testing.T) {
	clusterID := int64(7)
	valid := &Route{Priority: 10, ClusterID: &clusterID, RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}"}

	tests := []struct {
		name    string
		mutate  func(*Route)
		wantErr error
	}{
		{"valid", func(*Route) {}, nil},
		{"priority too low", func(r *Route) { r.Priority = 0 }, ErrInvalidRoute},
		{"priority too high", func(r *Route) { r.Priority = 101 }, ErrInvalidRoute},
		{"empty dedupe key", func(r *Route) { r.DedupeKey = "" }, ErrInvalidRoute},
		{"blank dedupe key", func(r *Route) { r.DedupeKey = "   " }, ErrInvalidRoute},
		{"group interval too short", func(r *Route) { gi := 10 * time.Second; r.GroupInterval = &gi }, ErrInvalidRoute},
		{"group interval too long", func(r *Route) { gi := 2 * time.Hour; r.GroupInterval = &gi }, ErrInvalidRoute},
		{"repeat interval too short", func(r *Route) { ri := 30 * time.Second; r.RepeatInterval = &ri }, ErrInvalidRoute},
		{"repeat interval too long", func(r *Route) { ri := 25 * time.Hour; r.RepeatInterval = &ri }, ErrInvalidRoute},
		{"valid group interval", func(r *Route) { gi := time.Minute; r.GroupInterval = &gi }, nil},
		{"valid repeat interval", func(r *Route) { ri := time.Hour; r.RepeatInterval = &ri }, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			route := *valid
			tc.mutate(&route)
			err := validateRoute(&route)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validateRoute() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// --- validateSilence tests ---

func TestValidateSilence(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Minute)
	validEnd := now.Add(time.Hour)

	tests := []struct {
		name    string
		mutate  func(*Silence)
		wantErr error
	}{
		{"valid", func(*Silence) {}, nil},
		{"empty reason", func(s *Silence) { s.Reason = "" }, ErrInvalidSilence},
		{"reason too long", func(s *Silence) { s.Reason = strings.Repeat("x", MaxReasonLen+1) }, ErrInvalidSilence},
		{"end before start", func(s *Silence) { s.EndsAt = s.StartsAt.Add(-time.Hour) }, ErrInvalidSilence},
		{"duration too long", func(s *Silence) { s.EndsAt = s.StartsAt.Add(MaxSilenceDuration + time.Hour) }, ErrPermanentSilence},
		{"end in past", func(s *Silence) { s.StartsAt = now.Add(-2 * time.Hour); s.EndsAt = now.Add(-time.Hour) }, ErrSilenceExpired},
		{"end exactly at max", func(s *Silence) { s.EndsAt = s.StartsAt.Add(MaxSilenceDuration) }, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			silence := &Silence{Reason: "planned maintenance", StartsAt: start, EndsAt: validEnd}
			tc.mutate(silence)
			err := validateSilence(silence, now)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validateSilence() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// --- Receiver CRUD tests ---

func TestCreateReceiver(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)

	r, err := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	if err != nil {
		t.Fatalf("CreateReceiver() error = %v", err)
	}
	if r.ID == 0 || r.CreatorID != 42 {
		t.Fatalf("receiver = %#v", r)
	}
	if r.URL != validHTTPSURL() {
		t.Fatalf("url = %q", r.URL)
	}
}

func TestCreateReceiverRejectsDuplicateName(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, _ = svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	_, err := svc.CreateReceiver(context.Background(), 42, "PagerDuty", validHTTPSURL(), validSecret())
	if !errors.Is(err, ErrDuplicateReceiverName) {
		t.Fatalf("err = %v, want %v", err, ErrDuplicateReceiverName)
	}
}

func TestCreateReceiverRejectsInvalidInput(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, err := svc.CreateReceiver(context.Background(), 42, "pagerduty", "http://insecure.example.com", "short")
	if !errors.Is(err, ErrInvalidReceiver) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidReceiver)
	}
}

func TestGetReceiverNotFound(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, err := svc.GetReceiver(context.Background(), 99, 42)
	if !errors.Is(err, ErrReceiverNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrReceiverNotFound)
	}
}

func TestListReceiversMasksURL(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, _ = svc.CreateReceiver(context.Background(), 42, "pagerduty", "https://hook.example.com/webhook", validSecret())
	views, err := svc.ListReceivers(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListReceivers() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len = %d", len(views))
	}
	if views[0].URLMasked == validHTTPSURL() || !strings.Contains(views[0].URLMasked, "***") {
		t.Fatalf("URLMasked = %q", views[0].URLMasked)
	}
	if strings.Contains(views[0].URLMasked, validSecret()) {
		t.Fatalf("secret leaked into view: %q", views[0].URLMasked)
	}
}

func TestDeleteReceiverRejectsWhenInUse(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	route, _ := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 10, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	if route.ID == 0 {
		t.Fatal("route not created")
	}
	err := svc.DeleteReceiver(context.Background(), r.ID, 42)
	if !errors.Is(err, ErrReceiverInUse) {
		t.Fatalf("err = %v, want %v", err, ErrReceiverInUse)
	}
}

func TestDeleteReceiverSucceedsWhenNotInUse(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	if err := svc.DeleteReceiver(context.Background(), r.ID, 42); err != nil {
		t.Fatalf("DeleteReceiver() error = %v", err)
	}
	if _, err := svc.GetReceiver(context.Background(), r.ID, 42); !errors.Is(err, ErrReceiverNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrReceiverNotFound)
	}
}

// --- Route CRUD tests ---

func TestCreateRoute(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	route, err := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	if err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if route.ID == 0 || !route.Enabled {
		t.Fatalf("route = %#v", route)
	}
}

func TestCreateRouteRejectsInvalidReceiver(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, err := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: 999, CreatorID: 42, Priority: 5, DedupeKey: "k",
	})
	if !errors.Is(err, ErrReceiverNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrReceiverNotFound)
	}
}

func TestCreateRouteRejectsInvalidInput(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	_, err := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 200, DedupeKey: "",
	})
	if !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidRoute)
	}
}

func TestListRoutesIncludesReceiverName(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	_, _ = svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "k",
	})
	views, err := svc.ListRoutes(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListRoutes() error = %v", err)
	}
	if len(views) != 1 || views[0].ReceiverName != "pagerduty" {
		t.Fatalf("views = %#v", views)
	}
}

func TestUpdateRoute(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	route, _ := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "k",
	})
	enabled := false
	priority := 20
	updated, err := svc.UpdateRoute(context.Background(), route.ID, 42, PatchRouteInput{Priority: &priority, Enabled: &enabled})
	if err != nil {
		t.Fatalf("UpdateRoute() error = %v", err)
	}
	if updated.Priority != 20 || updated.Enabled {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestUpdateRouteRejectsInvalidPriority(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	route, _ := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "k",
	})
	priority := 200
	_, err := svc.UpdateRoute(context.Background(), route.ID, 42, PatchRouteInput{Priority: &priority})
	if !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidRoute)
	}
}

func TestDeleteRoute(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	route, _ := svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "k",
	})
	if err := svc.DeleteRoute(context.Background(), route.ID, 42); err != nil {
		t.Fatalf("DeleteRoute() error = %v", err)
	}
	if _, err := svc.GetRoute(context.Background(), route.ID, 42); !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrRouteNotFound)
	}
}

// --- Silence CRUD tests ---

func TestCreateSilence(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	silence, err := svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, RuleName: "cpu-hot", Severity: "critical",
		Reason: "planned maintenance", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSilence() error = %v", err)
	}
	if silence.ID == 0 {
		t.Fatal("silence not created")
	}
}

func TestCreateSilenceRejectsPermanent(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	_, err := svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, Reason: "too long",
		StartsAt: now, EndsAt: now.Add(MaxSilenceDuration + time.Hour),
	})
	if !errors.Is(err, ErrPermanentSilence) {
		t.Fatalf("err = %v, want %v", err, ErrPermanentSilence)
	}
}

func TestCreateSilenceRejectsExpired(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	_, err := svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, Reason: "expired",
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour),
	})
	if !errors.Is(err, ErrSilenceExpired) {
		t.Fatalf("err = %v, want %v", err, ErrSilenceExpired)
	}
}

func TestListSilences(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	_, _ = svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, Reason: "one", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	views, err := svc.ListSilences(context.Background(), SilenceListFilter{CreatorID: intPtr(42)})
	if err != nil {
		t.Fatalf("ListSilences() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len = %d", len(views))
	}
}

func TestDeleteSilence(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	s, _ := svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, Reason: "one", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err := svc.DeleteSilence(context.Background(), s.ID, 42); err != nil {
		t.Fatalf("DeleteSilence() error = %v", err)
	}
	if err := svc.DeleteSilence(context.Background(), s.ID, 42); !errors.Is(err, ErrSilenceNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrSilenceNotFound)
	}
}

// --- IsSilenced tests ---

func TestIsSilencedMatched(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	_, _ = svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, RuleName: "cpu-hot", Severity: "critical", Reason: "maintenance",
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	silenced, silence := svc.IsSilenced(context.Background(), MatchAlert{ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring})
	if !silenced || silence == nil {
		t.Fatalf("silenced = %v, silence = %v", silenced, silence)
	}
}

func TestIsSilencedNotMatched(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	_, _ = svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, RuleName: "cpu-hot", Severity: "critical", Reason: "maintenance",
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	silenced, _ := svc.IsSilenced(context.Background(), MatchAlert{ClusterID: 1, RuleName: "disk-full", Severity: "warning", EventType: EventTypeFiring})
	if silenced {
		t.Fatal("expected not silenced")
	}
}

func TestIsSilencedExpired(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	_, _ = svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, RuleName: "cpu-hot", Severity: "critical", Reason: "maintenance",
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour),
	})
	silenced, _ := svc.IsSilenced(context.Background(), MatchAlert{ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring})
	if silenced {
		t.Fatal("expired silence should not match")
	}
}

// --- MatchAndDeliver tests ---

func TestMatchAndDeliverCreatesDeliveryForMatchingRoute(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	_, _ = svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	err := svc.MatchAndDeliver(context.Background(), MatchAlert{
		AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring,
	})
	if err != nil {
		t.Fatalf("MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(resp.Items))
	}
	d := resp.Items[0]
	if d.EventType != EventTypeFiring || d.DedupeKey != "1:cpu-hot" || d.Status != DeliveryStatusPending {
		t.Fatalf("delivery = %#v", d)
	}
}

func TestMatchAndDeliverBlockedBySilence(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	now := svc.now()
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	_, _ = svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	_, _ = svc.CreateSilence(context.Background(), &Silence{
		CreatorID: 42, RuleName: "cpu-hot", Severity: "critical", Reason: "maintenance",
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err := svc.MatchAndDeliver(context.Background(), MatchAlert{
		AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring,
	}); err != nil {
		t.Fatalf("MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 0 {
		t.Fatalf("deliveries = %d, want 0 (silenced)", len(resp.Items))
	}
}

func TestMatchAndDeliverNoRouteMatches(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	if err := svc.MatchAndDeliver(context.Background(), MatchAlert{
		AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring,
	}); err != nil {
		t.Fatalf("MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 0 {
		t.Fatalf("deliveries = %d, want 0", len(resp.Items))
	}
}

func TestMatchAndDeliverDedupePreventsDuplicateDelivery(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	_, _ = svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	alert := MatchAlert{AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring}
	if err := svc.MatchAndDeliver(context.Background(), alert); err != nil {
		t.Fatalf("first MatchAndDeliver() error = %v", err)
	}
	if err := svc.MatchAndDeliver(context.Background(), alert); err != nil {
		t.Fatalf("second MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 1 {
		t.Fatalf("deliveries = %d, want 1 (dedupe)", len(resp.Items))
	}
}

// --- DispatchOnce tests ---

func newDispatchService(t *testing.T, repo *mockRepository, serverURL string) (*Service, *Receiver) {
	t.Helper()
	svc := NewService(repo, nil)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	receiver := &Receiver{
		ID: 1, Name: "disposable", URL: serverURL, Secret: validSecret(), CreatorID: 42,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.receivers[receiver.ID] = receiver
	repo.nextReceiverID = 2
	return svc, receiver
}

func TestDispatchOnceDeliversSuccessfully(t *testing.T) {
	repo := newMockRepository()
	var receivedSig, receivedType string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedSig = r.Header.Get(SignatureHeader)
		receivedType = r.Header.Get("X-AIOps-Event-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc, _ := newDispatchService(t, repo, server.URL)
	now := svc.now()
	delivery := &Delivery{
		ID: 1, RouteID: 1, ReceiverID: 1, AlertInstanceID: 100, EventType: EventTypeFiring,
		DedupeKey: "1:cpu-hot", Status: DeliveryStatusPending, NextAttemptAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.deliveries[1] = delivery

	if err := svc.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if repo.deliveredID != 1 || repo.deliveredCount != 1 {
		t.Fatalf("delivered = %d count = %d", repo.deliveredID, repo.deliveredCount)
	}
	if repo.failedCount != 0 {
		t.Fatalf("failed count = %d, want 0", repo.failedCount)
	}
	if receivedSig != sign(receivedBody, validSecret()) {
		t.Fatalf("signature = %q", receivedSig)
	}
	if receivedType != EventTypeFiring {
		t.Fatalf("event type = %q", receivedType)
	}
}

func TestDispatchOnceRetriesOn500(t *testing.T) {
	repo := newMockRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	svc, _ := newDispatchService(t, repo, server.URL)
	svc.maxAttempts = 5
	svc.retryBase = 10 * time.Second
	now := svc.now()
	delivery := &Delivery{
		ID: 1, RouteID: 1, ReceiverID: 1, AlertInstanceID: 100, EventType: EventTypeFiring,
		DedupeKey: "1:cpu-hot", Status: DeliveryStatusPending, Attempts: 0, NextAttemptAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.deliveries[1] = delivery

	if err := svc.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if repo.failedID != 1 || repo.failedMax != 5 {
		t.Fatalf("failed id = %d max = %d", repo.failedID, repo.failedMax)
	}
	// attempts + 1 = 1, which is < maxAttempts(5), so should remain pending
	if repo.deliveries[1].Status != DeliveryStatusPending {
		t.Fatalf("status = %q, want pending", repo.deliveries[1].Status)
	}
	if repo.deliveredCount != 0 {
		t.Fatalf("delivered count = %d, want 0", repo.deliveredCount)
	}
	if !strings.Contains(repo.failedMessage, "HTTP 503") {
		t.Fatalf("failed message = %q", repo.failedMessage)
	}
}

func TestDispatchOnceDeadAfterMaxAttempts(t *testing.T) {
	repo := newMockRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc, _ := newDispatchService(t, repo, server.URL)
	svc.maxAttempts = 5
	svc.retryBase = 10 * time.Second
	now := svc.now()
	delivery := &Delivery{
		ID: 1, RouteID: 1, ReceiverID: 1, AlertInstanceID: 100, EventType: EventTypeFiring,
		DedupeKey: "1:cpu-hot", Status: DeliveryStatusPending, Attempts: 4, NextAttemptAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.deliveries[1] = delivery

	if err := svc.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if repo.failedID != 1 {
		t.Fatalf("failed id = %d", repo.failedID)
	}
	if repo.deliveries[1].Status != DeliveryStatusDead {
		t.Fatalf("status = %q, want dead", repo.deliveries[1].Status)
	}
	if repo.deliveredCount != 0 {
		t.Fatalf("delivered count = %d, want 0", repo.deliveredCount)
	}
}

func TestDispatchOnceRejectsRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	repo := newMockRepository()
	svc, _ := newDispatchService(t, repo, source.URL)
	now := svc.now()
	delivery := &Delivery{
		ID: 1, RouteID: 1, ReceiverID: 1, AlertInstanceID: 100, EventType: EventTypeFiring,
		DedupeKey: "1:cpu-hot", Status: DeliveryStatusPending, NextAttemptAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	repo.deliveries[1] = delivery

	if err := svc.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if repo.deliveredCount != 0 {
		t.Fatalf("delivered count = %d, want 0 (redirect must fail)", repo.deliveredCount)
	}
	if !strings.Contains(repo.failedMessage, "HTTP 307") {
		t.Fatalf("failed message = %q", repo.failedMessage)
	}
}

func TestDispatchOnceDedupeYieldsSingleDelivery(t *testing.T) {
	repo := newMockRepository()
	var callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc, receiver := newDispatchService(t, repo, server.URL)
	clusterID := int64(1)
	route := &Route{
		ID: 1, ReceiverID: receiver.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
		Enabled: true, CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}
	repo.routes[1] = route
	repo.nextRouteID = 2

	alert := MatchAlert{AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring}
	if err := svc.MatchAndDeliver(context.Background(), alert); err != nil {
		t.Fatalf("first MatchAndDeliver() error = %v", err)
	}
	if err := svc.MatchAndDeliver(context.Background(), alert); err != nil {
		t.Fatalf("second MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 1 {
		t.Fatalf("deliveries = %d, want 1 (dedupe)", len(resp.Items))
	}
	if err := svc.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if callCount != 1 {
		t.Fatalf("webhook call count = %d, want 1", callCount)
	}
	if repo.deliveredCount != 1 {
		t.Fatalf("delivered count = %d, want 1", repo.deliveredCount)
	}
}

func TestDispatchOnceResolvedEventDeliveredSeparately(t *testing.T) {
	repo := newMockRepository()
	var eventTypes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventTypes = append(eventTypes, r.Header.Get("X-AIOps-Event-Type"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc, receiver := newDispatchService(t, repo, server.URL)
	clusterID := int64(1)
	route := &Route{
		ID: 1, ReceiverID: receiver.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "cpu-hot", Severity: "critical", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
		Enabled: true, CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}
	repo.routes[1] = route
	repo.nextRouteID = 2

	alert := MatchAlert{AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring}
	if err := svc.MatchAndDeliver(context.Background(), alert); err != nil {
		t.Fatalf("firing MatchAndDeliver() error = %v", err)
	}
	resolved := MatchAlert{AlertInstanceID: 100, ClusterID: 1, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeResolved}
	if err := svc.MatchAndDeliver(context.Background(), resolved); err != nil {
		t.Fatalf("resolved MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 2 {
		t.Fatalf("deliveries = %d, want 2 (firing + resolved)", len(resp.Items))
	}
	if err := svc.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if len(eventTypes) != 2 {
		t.Fatalf("event types = %v, want 2", eventTypes)
	}
}

// --- helper tests ---

func TestRenderDedupeKey(t *testing.T) {
	alert := MatchAlert{ClusterID: 7, RuleName: "cpu-hot", Severity: "critical", EventType: EventTypeFiring}
	tests := []struct {
		template string
		want     string
	}{
		{"literal-key", "literal-key"},
		{"{{.ClusterID}}:{{.RuleName}}", "7:cpu-hot"},
		{"{{.Severity}}-{{.EventType}}", "critical-firing"},
	}
	for _, tc := range tests {
		got, err := renderDedupeKey(tc.template, alert)
		if err != nil {
			t.Fatalf("renderDedupeKey(%q) error = %v", tc.template, err)
		}
		if got != tc.want {
			t.Fatalf("renderDedupeKey(%q) = %q, want %q", tc.template, got, tc.want)
		}
	}
}

func TestMaskURL(t *testing.T) {
	got := maskURL("https://hook.example.com/webhook")
	if !strings.Contains(got, "***") || !strings.Contains(got, "example.com") {
		t.Fatalf("maskURL = %q", got)
	}
}

func TestSign(t *testing.T) {
	sig := sign([]byte("body"), validSecret())
	if !strings.HasPrefix(sig, "sha256=") || len(sig) < len("sha256=")+64 {
		t.Fatalf("sig = %q", sig)
	}
}

func TestRetryDelay(t *testing.T) {
	if retryDelay(10*time.Second, 1) != 10*time.Second {
		t.Fatal("base delay")
	}
	if retryDelay(10*time.Second, 3) != 40*time.Second {
		t.Fatal("exponential delay")
	}
	if retryDelay(10*time.Minute, 10) != MaxRetryDelay {
		t.Fatal("capped delay")
	}
}

func TestSanitizeErrorTruncates(t *testing.T) {
	long := errors.New(strings.Repeat("x", 1000))
	got := sanitizeError(long)
	if len(got) > MaxLastErrorLen {
		t.Fatalf("len = %d", len(got))
	}
}

func intPtr(v int64) *int64 { return &v }

// --- M51 inhibit tests ---

func boolPtr(v bool) *bool { return &v }

func TestCreateInhibit(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	inh, err := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", SourceSeverity: "critical",
		TargetRuleName: "app-503", TargetSeverity: "warning", Reason: "db outage suppresses app noise",
	})
	if err != nil {
		t.Fatalf("CreateInhibit() error = %v", err)
	}
	if inh.ID == 0 || !inh.Enabled {
		t.Fatalf("inhibit = %#v", inh)
	}
}

func TestCreateInhibitRejectsEmptySource(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, err := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, TargetRuleName: "app-503", Reason: "missing source",
	})
	if !errors.Is(err, ErrInvalidInhibit) {
		t.Fatalf("err = %v, want ErrInvalidInhibit", err)
	}
}

func TestCreateInhibitRejectsEmptyTarget(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, err := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", Reason: "missing target",
	})
	if !errors.Is(err, ErrInvalidInhibit) {
		t.Fatalf("err = %v, want ErrInvalidInhibit", err)
	}
}

func TestCreateInhibitRejectsMissingReason(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, err := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", TargetRuleName: "app-503",
	})
	if !errors.Is(err, ErrInvalidInhibit) {
		t.Fatalf("err = %v, want ErrInvalidInhibit", err)
	}
}

func TestCreateInhibitLimit(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	for i := 0; i < MaxInhibitsPerUser; i++ {
		_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
			CreatorID: 42, SourceRuleName: "db-down", TargetRuleName: "app-503", Reason: "limit test",
		})
	}
	_, err := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", TargetRuleName: "app-503", Reason: "over limit",
	})
	if !errors.Is(err, ErrInhibitLimit) {
		t.Fatalf("err = %v, want ErrInhibitLimit", err)
	}
}

func TestListInhibits(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", TargetRuleName: "app-503", Reason: "r1",
	})
	views, err := svc.ListInhibits(context.Background(), InhibitListFilter{CreatorID: intPtr(42)})
	if err != nil {
		t.Fatalf("ListInhibits() error = %v", err)
	}
	if len(views) != 1 || views[0].SourceRuleName != "db-down" || views[0].TargetRuleName != "app-503" {
		t.Fatalf("views = %#v", views)
	}
}

func TestDeleteInhibit(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	inh, _ := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", TargetRuleName: "app-503", Reason: "r1",
	})
	if err := svc.DeleteInhibit(context.Background(), inh.ID, 42); err != nil {
		t.Fatalf("DeleteInhibit() error = %v", err)
	}
	if err := svc.DeleteInhibit(context.Background(), inh.ID, 42); !errors.Is(err, ErrInhibitNotFound) {
		t.Fatalf("second delete err = %v, want ErrInhibitNotFound", err)
	}
}

func TestDeleteInhibitRejectsNonCreator(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	inh, _ := svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", TargetRuleName: "app-503", Reason: "r1",
	})
	if err := svc.DeleteInhibit(context.Background(), inh.ID, 99); !errors.Is(err, ErrInhibitNotFound) {
		t.Fatalf("non-creator delete err = %v, want ErrInhibitNotFound", err)
	}
}

func TestIsInhibitedSuppressesWhenSourceFiring(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", SourceSeverity: "critical",
		TargetRuleName: "app-503", TargetSeverity: "warning", Reason: "db outage",
	})
	repo.firingSourceOverride = boolPtr(true)
	alert := MatchAlert{ClusterID: 1, RuleName: "app-503", Severity: "warning", EventType: EventTypeFiring}
	inhibited, inh := svc.IsInhibited(context.Background(), alert)
	if !inhibited || inh == nil {
		t.Fatalf("expected inhibition, got inhibited=%v inh=%v", inhibited, inh)
	}
}

func TestIsInhibitedNotSuppressedWhenSourceNotFiring(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", SourceSeverity: "critical",
		TargetRuleName: "app-503", TargetSeverity: "warning", Reason: "db outage",
	})
	repo.firingSourceOverride = boolPtr(false)
	alert := MatchAlert{ClusterID: 1, RuleName: "app-503", Severity: "warning", EventType: EventTypeFiring}
	inhibited, _ := svc.IsInhibited(context.Background(), alert)
	if inhibited {
		t.Fatal("expected no inhibition when source is not firing")
	}
}

func TestIsInhibitedTargetDoesNotMatch(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down",
		TargetRuleName: "app-503", Reason: "db outage",
	})
	repo.firingSourceOverride = boolPtr(true)
	alert := MatchAlert{ClusterID: 1, RuleName: "unrelated-alert", EventType: EventTypeFiring}
	inhibited, _ := svc.IsInhibited(context.Background(), alert)
	if inhibited {
		t.Fatal("expected no inhibition when target does not match")
	}
}

func TestMatchAndDeliverBlockedByInhibit(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	_, _ = svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "app-503", Severity: "warning", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", SourceSeverity: "critical",
		TargetRuleName: "app-503", TargetSeverity: "warning", Reason: "db outage",
	})
	repo.firingSourceOverride = boolPtr(true)
	err := svc.MatchAndDeliver(context.Background(), MatchAlert{
		AlertInstanceID: 100, ClusterID: 1, RuleName: "app-503", Severity: "warning", EventType: EventTypeFiring,
	})
	if err != nil {
		t.Fatalf("MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 0 {
		t.Fatalf("deliveries = %d, want 0 (inhibited)", len(resp.Items))
	}
}

func TestMatchAndDeliverNotBlockedWhenSourceNotFiring(t *testing.T) {
	repo := newMockRepository()
	svc := newTestService(repo)
	r, _ := svc.CreateReceiver(context.Background(), 42, "pagerduty", validHTTPSURL(), validSecret())
	clusterID := int64(1)
	_, _ = svc.CreateRoute(context.Background(), &Route{
		ReceiverID: r.ID, CreatorID: 42, Priority: 5, ClusterID: &clusterID,
		RuleName: "app-503", Severity: "warning", DedupeKey: "{{.ClusterID}}:{{.RuleName}}",
	})
	_, _ = svc.CreateInhibit(context.Background(), &Inhibit{
		CreatorID: 42, SourceRuleName: "db-down", SourceSeverity: "critical",
		TargetRuleName: "app-503", TargetSeverity: "warning", Reason: "db outage",
	})
	repo.firingSourceOverride = boolPtr(false)
	err := svc.MatchAndDeliver(context.Background(), MatchAlert{
		AlertInstanceID: 100, ClusterID: 1, RuleName: "app-503", Severity: "warning", EventType: EventTypeFiring,
	})
	if err != nil {
		t.Fatalf("MatchAndDeliver() error = %v", err)
	}
	resp, _ := repo.ListDeliveries(context.Background(), DeliveryListFilter{})
	if len(resp.Items) != 1 {
		t.Fatalf("deliveries = %d, want 1 (not inhibited)", len(resp.Items))
	}
}
