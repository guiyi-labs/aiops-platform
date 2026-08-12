package incident

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRepository is an in-memory Repository used to exercise the service
// layer. It mirrors the CAS versioning, status machine and timeline semantics
// of the GormRepository without requiring a database.
type fakeRepository struct {
	mu        sync.Mutex
	nextID    int64
	nextEvent int64
	byID      map[int64]*Incident
	sources   map[string]int64
	users     map[int64]string
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		nextID:  1,
		byID:    map[int64]*Incident{},
		sources: map[string]int64{},
		users: map[int64]string{
			1: "admin",
			2: "ops-user",
		},
	}
}

func sourceKey(sourceType, sourceRef string) string { return sourceType + ":" + sourceRef }

func cloneIncident(incident Incident) Incident {
	incident.Followers = append([]Follower(nil), incident.Followers...)
	incident.Timeline = append([]TimelineEvent(nil), incident.Timeline...)
	return incident
}

func (f *fakeRepository) pushEvent(record *Incident, event TimelineEvent) {
	f.nextEvent++
	event.ID = f.nextEvent
	event.CreatedAt = event.CreatedAt.UTC()
	record.Timeline = append(record.Timeline, event)
	record.UpdatedAt = event.CreatedAt
}

func (f *fakeRepository) Create(_ context.Context, record *Incident) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := sourceKey(record.SourceType, record.SourceRef)
	if _, exists := f.sources[key]; exists {
		return ErrSourceAlreadyUsed
	}
	now := time.Now().UTC()
	record.ID = f.nextID
	f.nextID++
	record.Number = "INC-" + strings.Repeat("0", 6-len(intToText(record.ID))) + intToText(record.ID)
	record.Version = 1
	record.Status = StatusOpen
	record.CreatedAt = now
	record.UpdatedAt = now
	stored := cloneIncident(*record)
	f.byID[record.ID] = &stored
	f.sources[key] = record.ID
	f.pushEvent(f.byID[record.ID], TimelineEvent{
		EventType: EventTypeSystem,
		Actor:     ActorRef{Name: "system"},
		Content:   "incident created from " + record.SourceType + " source " + record.SourceRef,
		CreatedAt: now,
	})
	*record = cloneIncident(*f.byID[record.ID])
	return nil
}

func (f *fakeRepository) Get(_ context.Context, id int64) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	return cloneIncident(*record), nil
}

func (f *fakeRepository) List(_ context.Context, filter ListFilter) ([]Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Incident, 0)
	for _, record := range f.byID {
		if filter.ClusterID > 0 && record.ClusterID != filter.ClusterID {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		if filter.AssigneeID > 0 && (record.Assignee == nil || record.Assignee.ID != filter.AssigneeID) {
			continue
		}
		if filter.FollowerID > 0 && !hasFollower(*record, filter.FollowerID) {
			continue
		}
		result = append(result, cloneIncident(*record))
	}
	return result, nil
}

func hasFollower(incident Incident, userID int64) bool {
	for _, follower := range incident.Followers {
		if follower.UserID == userID {
			return true
		}
	}
	return false
}

func (f *fakeRepository) Summary(_ context.Context) (Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var summary Summary
	for _, record := range f.byID {
		summary.Total++
		switch record.Status {
		case StatusOpen:
			summary.Open++
		case StatusConfirmed:
			summary.Confirmed++
		case StatusResolved:
			summary.Resolved++
		case StatusDismissed:
			summary.Dismissed++
		}
		if record.Overdue {
			summary.Overdue++
		}
	}
	return summary, nil
}

func (f *fakeRepository) Transition(_ context.Context, id, expectedVersion int64, toStatus string, actor ActorRef, comment string) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	if record.Version != expectedVersion {
		return Incident{}, ErrVersionConflict
	}
	if !CanTransition(record.Status, toStatus) {
		return Incident{}, ErrInvalidTransition
	}
	content := "status changed from " + record.Status + " to " + toStatus
	if strings.TrimSpace(comment) != "" {
		content += ": " + strings.TrimSpace(comment)
	}
	record.Status = toStatus
	record.Version++
	if toStatus == StatusResolved {
		now := time.Now().UTC()
		record.ResolvedAt = &now
	}
	if toStatus == StatusOpen {
		record.ResolvedAt = nil
	}
	f.pushEvent(record, TimelineEvent{EventType: EventTypeSystem, Actor: actor, Content: content, CreatedAt: time.Now().UTC()})
	return cloneIncident(*record), nil
}

func (f *fakeRepository) Assign(_ context.Context, id, expectedVersion, assigneeUserID int64, actor ActorRef, comment string) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	if record.Version != expectedVersion {
		return Incident{}, ErrVersionConflict
	}
	name, userExists := f.users[assigneeUserID]
	if !userExists {
		return Incident{}, ErrAssigneeNotFound
	}
	fromName := "unassigned"
	if record.Assignee != nil {
		fromName = record.Assignee.Name
	}
	content := "handoff from " + fromName + " to " + name
	if strings.TrimSpace(comment) != "" {
		content += ": " + strings.TrimSpace(comment)
	}
	record.Assignee = &ActorRef{ID: assigneeUserID, Name: name}
	record.Version++
	f.pushEvent(record, TimelineEvent{EventType: EventTypeSystem, Actor: actor, Content: content, CreatedAt: time.Now().UTC()})
	return cloneIncident(*record), nil
}

func (f *fakeRepository) AddFollower(_ context.Context, id, userID int64, actor ActorRef) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	name, userExists := f.users[userID]
	if !userExists {
		return Incident{}, ErrAssigneeNotFound
	}
	if hasFollower(*record, userID) {
		return Incident{}, ErrFollowerDuplicate
	}
	record.Followers = append(record.Followers, Follower{UserID: userID, Name: name, AddedAt: time.Now().UTC()})
	f.pushEvent(record, TimelineEvent{EventType: EventTypeSystem, Actor: actor, Content: name + " is now following this incident", CreatedAt: time.Now().UTC()})
	return cloneIncident(*record), nil
}

func (f *fakeRepository) RemoveFollower(_ context.Context, id, userID int64, actor ActorRef) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	name := f.users[userID]
	removed := false
	followers := record.Followers[:0]
	for _, follower := range record.Followers {
		if follower.UserID == userID {
			removed = true
			continue
		}
		followers = append(followers, follower)
	}
	record.Followers = followers
	if !removed {
		return Incident{}, ErrFollowerNotFound
	}
	f.pushEvent(record, TimelineEvent{EventType: EventTypeSystem, Actor: actor, Content: name + " stopped following this incident", CreatedAt: time.Now().UTC()})
	return cloneIncident(*record), nil
}

func (f *fakeRepository) AddNote(_ context.Context, id, expectedVersion int64, actor ActorRef, content string) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	if record.Version != expectedVersion {
		return Incident{}, ErrVersionConflict
	}
	record.Version++
	f.pushEvent(record, TimelineEvent{EventType: EventTypeNote, Actor: actor, Content: strings.TrimSpace(content), CreatedAt: time.Now().UTC()})
	return cloneIncident(*record), nil
}

func (f *fakeRepository) SetPostmortem(_ context.Context, id, expectedVersion int64, actor ActorRef, content string) (Incident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.byID[id]
	if !ok {
		return Incident{}, ErrNotFound
	}
	if record.Status != StatusResolved {
		return Incident{}, ErrPostmortemLocked
	}
	if record.Version != expectedVersion {
		return Incident{}, ErrVersionConflict
	}
	record.Postmortem = strings.TrimSpace(content)
	record.Version++
	f.pushEvent(record, TimelineEvent{EventType: EventTypeSystem, Actor: actor, Content: "postmortem updated", CreatedAt: time.Now().UTC()})
	return cloneIncident(*record), nil
}

func intToText(value int64) string {
	if value == 0 {
		return "0"
	}
	digits := "0123456789"
	result := ""
	for value > 0 {
		result = string(digits[value%10]) + result
		value /= 10
	}
	return result
}

type fakeResolver struct {
	info SourceInfo
	err  error
}

func (f *fakeResolver) Resolve(_ context.Context, sourceType, _ string, _ int64) (SourceInfo, error) {
	if sourceType != SourceTypeDiagnosis && sourceType != SourceTypeAlert {
		return SourceInfo{}, ErrInvalidSource
	}
	if f.err != nil {
		return SourceInfo{}, f.err
	}
	return f.info, nil
}

func newServiceWithFake(t *testing.T) (*Service, *fakeRepository) {
	t.Helper()
	repo := newFakeRepository()
	return NewService(repo), repo
}

func TestCreateFromDiagnosis(t *testing.T) {
	service, repo := newServiceWithFake(t)
	service.WithResolver(&fakeResolver{info: SourceInfo{
		Title:    "CrashLoopBackOff web-0",
		Summary:  "container restarted 12 times",
		Severity: SeverityHigh,
		Resource: ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-0", UID: "uid-1"},
	}})
	incident, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeDiagnosis,
		SourceRef:  SourceRefForDiagnosis(42),
		ClusterID:  7,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if incident.Status != StatusOpen || incident.Version != 1 {
		t.Errorf("unexpected initial state: status=%s version=%d", incident.Status, incident.Version)
	}
	if incident.Title != "CrashLoopBackOff web-0" || incident.Severity != SeverityHigh {
		t.Errorf("resolver enrichment failed: %+v", incident)
	}
	if incident.SLADueAt.Before(incident.ObservedAt.Add(4*time.Hour)) || incident.SLADueAt.After(incident.ObservedAt.Add(5*time.Hour)) {
		t.Errorf("SLA deadline not 4h for high: %v", incident.SLADueAt)
	}
	if len(incident.Timeline) != 1 || incident.Timeline[0].EventType != EventTypeSystem {
		t.Errorf("expected creation system event, got %+v", incident.Timeline)
	}

	duplicate, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeDiagnosis,
		SourceRef:  SourceRefForDiagnosis(42),
		ClusterID:  7,
	})
	if !errors.Is(err, ErrSourceAlreadyUsed) {
		t.Errorf("duplicate create err = %v, want ErrSourceAlreadyUsed", err)
	}
	if duplicate.ID != 0 {
		t.Errorf("duplicate create returned record %+v", duplicate)
	}
	_ = repo
}

func TestCreateValidation(t *testing.T) {
	service, _ := newServiceWithFake(t)
	cases := []CreateInput{
		{SourceType: "bogus", SourceRef: "x", ClusterID: 1, Title: "t", Severity: SeverityHigh},
		{SourceType: SourceTypeFinding, SourceRef: "", ClusterID: 1, Title: "t", Severity: SeverityHigh},
		{SourceType: SourceTypeFinding, SourceRef: "x", ClusterID: 0, Title: "t", Severity: SeverityHigh},
		{SourceType: SourceTypeFinding, SourceRef: "x", ClusterID: 1, Title: "", Severity: SeverityHigh},
		{SourceType: SourceTypeFinding, SourceRef: "x", ClusterID: 1, Title: "t", Severity: "severe"},
		{SourceType: SourceTypeFinding, SourceRef: "x", ClusterID: 1, Title: "t", Severity: SeverityHigh, Resource: ResourceRef{Kind: "Pod"}},
	}
	for i, input := range cases {
		if _, err := service.Create(context.Background(), input); err == nil {
			t.Errorf("case %d: expected validation error for %+v", i, input)
		}
	}
}

func TestCreateFinding(t *testing.T) {
	service, _ := newServiceWithFake(t)
	incident, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeFinding,
		SourceRef:  SourceRefForFinding(7, "pod.pending.v1", "Pod", "default", "web-0", ""),
		ClusterID:  7,
		Title:      "Pod pending",
		Severity:   SeverityWarning,
		Summary:    "pod stuck in pending",
		ObservedAt: time.Now().UTC(),
		Resource:   ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-0"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if incident.Severity != SeverityWarning || incident.Summary != "pod stuck in pending" {
		t.Errorf("finding fields not preserved: %+v", incident)
	}
}

func TestCreateFromAlert(t *testing.T) {
	service, _ := newServiceWithFake(t)
	service.WithResolver(&fakeResolver{info: SourceInfo{
		Title:    "Alert demo-node NodeNotReady",
		Summary:  "node memory pressure sustained",
		Severity: SeverityCritical,
		Resource: ResourceRef{Kind: "Node", Namespace: "", Name: "demo-node", UID: "uid-node"},
	}})
	incident, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeAlert,
		SourceRef:  SourceRefForAlert(9),
		ClusterID:  7,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if incident.SourceType != SourceTypeAlert || incident.SourceRef != "alert:9" {
		t.Errorf("alert source not preserved: %+v", incident)
	}
	if incident.Title != "Alert demo-node NodeNotReady" || incident.Severity != SeverityCritical {
		t.Errorf("alert resolver enrichment failed: %+v", incident)
	}

	// Dedup must hold for the same alert instance.
	_, err = service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeAlert,
		SourceRef:  SourceRefForAlert(9),
		ClusterID:  7,
	})
	if !errors.Is(err, ErrSourceAlreadyUsed) {
		t.Errorf("duplicate alert create err = %v, want ErrSourceAlreadyUsed", err)
	}
}

func TestLifecycle(t *testing.T) {
	service, _ := newServiceWithFake(t)
	incident, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeFinding,
		SourceRef:  "finding:7:pod.pending.v1:Pod:default:web-0",
		ClusterID:  7,
		Title:      "Pod pending",
		Severity:   SeverityWarning,
		Resource:   ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-0"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctx := context.Background()
	actor := ActorRef{ID: 1, Name: "admin"}

	if _, err := service.Transition(ctx, incident.ID, incident.Version, "resolved", actor, "skip"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("open->resolved err = %v, want ErrInvalidTransition", err)
	}
	if _, err := service.Transition(ctx, incident.ID, incident.Version+99, StatusConfirmed, actor, ""); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale version err = %v, want ErrVersionConflict", err)
	}

	confirmed, err := service.Transition(ctx, incident.ID, incident.Version, StatusConfirmed, actor, "reproduced")
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if confirmed.Status != StatusConfirmed || confirmed.Version != 2 {
		t.Errorf("confirmed state wrong: %+v", confirmed)
	}

	assigned, err := service.Assign(ctx, incident.ID, confirmed.Version, 2, actor, "hand over")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if assigned.Assignee == nil || assigned.Assignee.ID != 2 {
		t.Errorf("assignee wrong: %+v", assigned.Assignee)
	}

	if _, err := service.Assign(ctx, incident.ID, assigned.Version, 999, actor, ""); !errors.Is(err, ErrAssigneeNotFound) {
		t.Fatalf("bad assignee err = %v, want ErrAssigneeNotFound", err)
	}

	followed, err := service.AddFollower(ctx, incident.ID, 2, actor)
	if err != nil {
		t.Fatalf("AddFollower: %v", err)
	}
	if len(followed.Followers) != 1 {
		t.Errorf("followers = %d, want 1", len(followed.Followers))
	}
	if _, err := service.AddFollower(ctx, incident.ID, 2, actor); !errors.Is(err, ErrFollowerDuplicate) {
		t.Fatalf("duplicate follower err = %v", err)
	}
	if _, err := service.RemoveFollower(ctx, incident.ID, 2, actor); err != nil {
		t.Fatalf("RemoveFollower: %v", err)
	}
	if _, err := service.RemoveFollower(ctx, incident.ID, 2, actor); !errors.Is(err, ErrFollowerNotFound) {
		t.Fatalf("missing follower err = %v", err)
	}

	if _, err := service.AddNote(ctx, incident.ID, followed.Version, actor, "  "); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("blank note err = %v, want ErrInvalidNote", err)
	}
	noted, err := service.AddNote(ctx, incident.ID, followed.Version, actor, "digging into scheduler logs")
	if err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if last := noted.Timeline[len(noted.Timeline)-1]; last.EventType != EventTypeNote || last.Content != "digging into scheduler logs" {
		t.Errorf("note event wrong: %+v", last)
	}

	if _, err := service.SetPostmortem(ctx, incident.ID, noted.Version, actor, "postmortem"); !errors.Is(err, ErrPostmortemLocked) {
		t.Fatalf("postmortem before resolve err = %v, want ErrPostmortemLocked", err)
	}

	resolved, err := service.Transition(ctx, incident.ID, noted.Version, StatusResolved, actor, "fixed by rollout")
	if err != nil {
		t.Fatalf("Transition to resolved: %v", err)
	}
	if resolved.ResolvedAt == nil {
		t.Errorf("resolved_at not set")
	}
	postmortem, err := service.SetPostmortem(ctx, incident.ID, resolved.Version, actor, "root cause: image tag drift")
	if err != nil {
		t.Fatalf("SetPostmortem: %v", err)
	}
	if postmortem.Postmortem != "root cause: image tag drift" {
		t.Errorf("postmortem = %q", postmortem.Postmortem)
	}

	reopened, err := service.Transition(ctx, incident.ID, postmortem.Version, StatusOpen, actor, "regression observed")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Status != StatusOpen || reopened.ResolvedAt != nil {
		t.Errorf("reopen state wrong: %+v", reopened)
	}
}

func TestExportCSV(t *testing.T) {
	service, _ := newServiceWithFake(t)
	created, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeFinding,
		SourceRef:  "finding:7:pod.pending.v1:Pod:default:web-0",
		ClusterID:  7,
		Title:      "=HYPERLINK(evil)",
		Severity:   SeverityHigh,
		Summary:    "pod stuck",
		Resource:   ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-0"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var buffer bytes.Buffer
	result, err := service.ExportCSV(context.Background(), ListFilter{Limit: 50}, &buffer)
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	if result.Rows != 1 {
		t.Errorf("export rows = %d, want 1", result.Rows)
	}
	output := buffer.String()
	if !strings.HasPrefix(output, "\uFEFFnumber,title") {
		t.Errorf("CSV missing BOM/header: %q", output[:min(40, len(output))])
	}
	if !strings.Contains(output, "'=HYPERLINK(evil)") {
		t.Errorf("formula cell not redacted: %q", output)
	}
	if strings.Contains(output, "pod stuck,") == false && !strings.Contains(output, "pod stuck") {
		t.Errorf("summary missing from export: %q", output)
	}

	var single bytes.Buffer
	if result, err := service.ExportOne(context.Background(), created.ID, &single); err != nil || result.Rows != 1 {
		t.Fatalf("ExportOne err=%v rows=%d", err, result.Rows)
	}
	if !strings.Contains(single.String(), created.Number) {
		t.Errorf("ExportOne did not include the requested incident: %q", single.String())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
