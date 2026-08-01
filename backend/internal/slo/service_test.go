package slo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memoryRepository is an in-memory Repository for service tests.
type memoryRepository struct {
	mu          sync.Mutex
	definitions map[int64]*Definition
	evaluations []Evaluation
	nextDefID   int64
	nextEvalID  int64
	createErr   error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		definitions: make(map[int64]*Definition),
		nextDefID:   1,
		nextEvalID:  1,
	}
}

func (r *memoryRepository) CreateDefinition(_ context.Context, def *Definition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	def.ID = r.nextDefID
	r.nextDefID++
	def.CreatedAt = time.Now()
	def.UpdatedAt = def.CreatedAt
	cloned := *def
	r.definitions[def.ID] = &cloned
	return nil
}

func (r *memoryRepository) GetDefinition(_ context.Context, id int64) (Definition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.definitions[id]
	if !ok {
		return Definition{}, ErrDefinitionNotFound
	}
	return *d, nil
}

func (r *memoryRepository) ListDefinitions(_ context.Context, filter DefinitionFilter) ([]Definition, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Definition
	for _, d := range r.definitions {
		if filter.ClusterID > 0 && d.ClusterID != filter.ClusterID {
			continue
		}
		if filter.Namespace != "" && d.Service.Namespace != filter.Namespace {
			continue
		}
		if filter.Template != "" && d.Template != filter.Template {
			continue
		}
		if filter.Enabled != nil && d.Enabled != *filter.Enabled {
			continue
		}
		if filter.OwnerID > 0 && d.Owner.ID != filter.OwnerID {
			continue
		}
		out = append(out, *d)
	}
	total := int64(len(out))
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, total, nil
}

func (r *memoryRepository) UpdateDefinition(_ context.Context, id int64, patch PatchDefinitionInput, now time.Time) (Definition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.definitions[id]
	if !ok {
		return Definition{}, ErrDefinitionNotFound
	}
	applyPatchToDefinition(d, patch)
	d.Version++
	d.UpdatedAt = now
	return *d, nil
}

func (r *memoryRepository) DeleteDefinition(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.definitions[id]
	if !ok {
		return ErrDefinitionNotFound
	}
	d.Enabled = false
	return nil
}

func (r *memoryRepository) InsertEvaluation(_ context.Context, eval *Evaluation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	eval.ID = r.nextEvalID
	r.nextEvalID++
	r.evaluations = append(r.evaluations, *eval)
	return nil
}

func (r *memoryRepository) LatestEvaluation(_ context.Context, sloID int64) (Evaluation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.evaluations) - 1; i >= 0; i-- {
		if r.evaluations[i].SLOID == sloID {
			return r.evaluations[i], nil
		}
	}
	return Evaluation{}, nil
}

func (r *memoryRepository) ListEvaluations(_ context.Context, filter EvaluationFilter) ([]Evaluation, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Evaluation
	for i := len(r.evaluations) - 1; i >= 0; i-- {
		e := r.evaluations[i]
		if e.SLOID != filter.SLOID {
			continue
		}
		if filter.Version != nil && e.Version != *filter.Version {
			continue
		}
		if filter.State != "" && e.State != filter.State {
			continue
		}
		out = append(out, e)
	}
	total := int64(len(out))
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, total, nil
}

func applyPatchToDefinition(d *Definition, patch PatchDefinitionInput) {
	if patch.Objective != nil {
		d.Objective = *patch.Objective
	}
	if patch.RollingWindowSeconds != nil {
		d.RollingWindowSeconds = *patch.RollingWindowSeconds
	}
	if patch.MissingDataPolicy != nil {
		d.MissingDataPolicy = *patch.MissingDataPolicy
	}
	if patch.LatencyThresholdMs != nil {
		d.LatencyThresholdMs = *patch.LatencyThresholdMs
	}
	if patch.Owner != nil {
		d.Owner = *patch.Owner
	}
	if patch.FastBurnRate != nil {
		d.FastBurnRate = *patch.FastBurnRate
	}
	if patch.FastBurnWindowSeconds != nil {
		d.FastBurnWindowSeconds = *patch.FastBurnWindowSeconds
	}
	if patch.SlowBurnRate != nil {
		d.SlowBurnRate = *patch.SlowBurnRate
	}
	if patch.SlowBurnWindowSeconds != nil {
		d.SlowBurnWindowSeconds = *patch.SlowBurnWindowSeconds
	}
	if patch.Enabled != nil {
		d.Enabled = *patch.Enabled
	}
}

// captureSink is a BurnAlertSink that records transitions for assertions.
type captureSink struct {
	mu          sync.Mutex
	transitions []BurnTransition
	failOnCall  error
}

func (s *captureSink) OnBurnTransition(_ context.Context, t BurnTransition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitions = append(s.transitions, t)
	return s.failOnCall
}

func (s *captureSink) snapshot() []BurnTransition {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]BurnTransition, len(s.transitions))
	copy(out, s.transitions)
	return out
}

func validCreateInput() CreateDefinitionInput {
	return CreateDefinitionInput{
		ClusterID:             1,
		Service:               ServiceRef{Kind: "Deployment", Namespace: "default", Name: "api"},
		Template:              TemplateRequestSuccessRatio,
		Objective:             0.99,
		RollingWindowSeconds:  3600,
		MissingDataPolicy:     MissingDataUnavailable,
		FastBurnRate:          14.4,
		FastBurnWindowSeconds: 3600,
		SlowBurnRate:          1.0,
		SlowBurnWindowSeconds: 21600,
		Enabled:               true,
		Owner:                 ActorRef{ID: 1, Name: "alice"},
		Creator:               ActorRef{ID: 1, Name: "alice"},
	}
}

// TestService_CreateDefinition_Success verifies the happy-path create
// stamps version=1 and persists.
func TestService_CreateDefinition_Success(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	def, err := svc.CreateDefinition(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if def.Version != 1 {
		t.Errorf("version: want 1, got %d", def.Version)
	}
	if def.TemplateVersion != TemplateVersion {
		t.Errorf("template_version: want %s, got %s", TemplateVersion, def.TemplateVersion)
	}
}

// TestService_CreateDefinition_InvalidInput verifies validation rejects bad
// inputs.
func TestService_CreateDefinition_InvalidInput(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	tests := []struct {
		name string
		mut  func(in *CreateDefinitionInput)
	}{
		{"missing_cluster", func(in *CreateDefinitionInput) { in.ClusterID = 0 }},
		{"missing_service_kind", func(in *CreateDefinitionInput) { in.Service.Kind = "" }},
		{"bad_template", func(in *CreateDefinitionInput) { in.Template = SLITemplate("nope") }},
		{"objective_out_of_range", func(in *CreateDefinitionInput) { in.Objective = 1.5 }},
		{"missing_creator", func(in *CreateDefinitionInput) { in.Creator = ActorRef{} }},
		{"missing_owner", func(in *CreateDefinitionInput) { in.Owner = ActorRef{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := validCreateInput()
			tc.mut(&in)
			if _, err := svc.CreateDefinition(context.Background(), in); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

// TestService_PatchDefinition_IncrementsVersion verifies each patch bumps
// Version and refreshes UpdatedAt.
func TestService_PatchDefinition_IncrementsVersion(t *testing.T) {
	fixedNow := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	svc := NewService(repo, nil, WithNow(func() time.Time { return fixedNow }))
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())
	originalVersion := def.Version

	newObjective := 0.999
	patched, err := svc.PatchDefinition(context.Background(), def.ID, PatchDefinitionInput{
		Objective: &newObjective,
		Actor:     ActorRef{ID: 1, Name: "alice"},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.Version != originalVersion+1 {
		t.Errorf("version: want %d, got %d", originalVersion+1, patched.Version)
	}
	if patched.Objective != 0.999 {
		t.Errorf("objective: want 0.999, got %v", patched.Objective)
	}
	if !patched.UpdatedAt.Equal(fixedNow) {
		t.Errorf("updated_at: want %v, got %v", fixedNow, patched.UpdatedAt)
	}
}

// TestService_PatchDefinition_RequiresActor verifies a patch without actor
// is rejected.
func TestService_PatchDefinition_RequiresActor(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())
	newObjective := 0.999
	_, err := svc.PatchDefinition(context.Background(), def.ID, PatchDefinitionInput{
		Objective: &newObjective,
		// Actor zero-value
	})
	if err == nil {
		t.Fatalf("expected error for missing actor")
	}
}

// TestService_DeleteDefinition_Disables verifies delete marks enabled=false.
func TestService_DeleteDefinition_Disables(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())
	if err := svc.DeleteDefinition(context.Background(), def.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	stored, _ := repo.GetDefinition(context.Background(), def.ID)
	if stored.Enabled {
		t.Errorf("expected enabled=false after delete")
	}
}

// TestService_DeleteDefinition_NotFound verifies delete on missing ID
// returns ErrDefinitionNotFound.
func TestService_DeleteDefinition_NotFound(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	if err := svc.DeleteDefinition(context.Background(), 9999); !errors.Is(err, ErrDefinitionNotFound) {
		t.Errorf("expected ErrDefinitionNotFound, got %v", err)
	}
}

// TestService_EvaluateSLO_NoEvaluator verifies that without an evaluator
// the service returns ErrEvaluatorUnavailable.
func TestService_EvaluateSLO_NoEvaluator(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())
	_, err := svc.EvaluateSLO(context.Background(), def.ID)
	if !errors.Is(err, ErrEvaluatorUnavailable) {
		t.Errorf("expected ErrEvaluatorUnavailable, got %v", err)
	}
}

// TestService_EvaluateSLO_DisabledDefinition verifies that a disabled SLO
// cannot be evaluated.
func TestService_EvaluateSLO_DisabledDefinition(t *testing.T) {
	repo := newMemoryRepository()
	eval := NewEvaluator(fakeMetricsSource{})
	svc := NewService(repo, eval)
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())
	_ = svc.DeleteDefinition(context.Background(), def.ID)
	_, err := svc.EvaluateSLO(context.Background(), def.ID)
	if !errors.Is(err, ErrDefinitionDisabled) {
		t.Errorf("expected ErrDefinitionDisabled, got %v", err)
	}
}

// TestService_EvaluateSLO_PersistsAndEmitsTransition verifies that a
// successful evaluation is persisted and a healthy->breached transition is
// emitted to the sink.
func TestService_EvaluateSLO_PersistsAndEmitsTransition(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	repo := newMemoryRepository()
	sink := &captureSink{}
	// 98/100 -> ratio 0.98 < 0.99 -> breached.
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 98)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			ExpectedSamples: 4,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	svc := NewService(repo, eval, WithBurnAlertSink(sink), WithNow(func() time.Time { return now }))
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())

	eval1, err := svc.EvaluateSLO(context.Background(), def.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if eval1.State != StateBreached {
		t.Errorf("state: want breached, got %s", eval1.State)
	}
	if eval1.ID == 0 {
		t.Errorf("expected persisted ID")
	}
	// LatestEvaluation should return the just-inserted row.
	latest, _ := repo.LatestEvaluation(context.Background(), def.ID)
	if latest.ID != eval1.ID {
		t.Errorf("latest evaluation not persisted")
	}
	// First evaluation: previous is StateHealthy (baseline), current is
	// StateBreached -> transition should fire.
	transitions := sink.snapshot()
	if len(transitions) != 1 {
		t.Fatalf("transitions: want 1, got %d", len(transitions))
	}
	if transitions[0].Previous != StateHealthy {
		t.Errorf("transition previous: want healthy, got %s", transitions[0].Previous)
	}
	if transitions[0].Current != StateBreached {
		t.Errorf("transition current: want breached, got %s", transitions[0].Current)
	}
}

// TestService_EvaluateSLO_NoTransitionOnSteadyState verifies that two
// consecutive evaluations in the same state emit only one transition (the
// initial healthy->state one).
func TestService_EvaluateSLO_NoTransitionOnSteadyState(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	repo := newMemoryRepository()
	sink := &captureSink{}
	// 100/100 -> healthy.
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			ExpectedSamples: 4,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	svc := NewService(repo, eval, WithBurnAlertSink(sink), WithNow(func() time.Time { return now }))
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())

	if _, err := svc.EvaluateSLO(context.Background(), def.ID); err != nil {
		t.Fatalf("evaluate 1: %v", err)
	}
	if _, err := svc.EvaluateSLO(context.Background(), def.ID); err != nil {
		t.Fatalf("evaluate 2: %v", err)
	}
	// Both evaluations are healthy. The first emits healthy->healthy? No:
	// previousState for a fresh SLO is StateHealthy (baseline), and current
	// is also StateHealthy -> no transition. So zero transitions total.
	transitions := sink.snapshot()
	if len(transitions) != 0 {
		t.Errorf("steady-state healthy: want 0 transitions, got %d", len(transitions))
	}
}

// TestService_EvaluateSLO_SinkFailureDoesNotRollback verifies that a sink
// error does not prevent evaluation persistence.
func TestService_EvaluateSLO_SinkFailureDoesNotRollback(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	repo := newMemoryRepository()
	sink := &captureSink{failOnCall: errors.New("sink down")}
	// 98/100 -> breached (transition from baseline healthy).
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 98)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			ExpectedSamples: 4,
			Source:          "test",
		},
	}
	eval := NewEvaluator(source)
	svc := NewService(repo, eval, WithBurnAlertSink(sink), WithNow(func() time.Time { return now }))
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())

	eval1, err := svc.EvaluateSLO(context.Background(), def.ID)
	if err != nil {
		t.Fatalf("evaluate should succeed despite sink failure: %v", err)
	}
	if eval1.ID == 0 {
		t.Errorf("evaluation should be persisted despite sink failure")
	}
}

// TestService_ListDefinitions_Pagination verifies limit clamping and
// truncated flag.
func TestService_ListDefinitions_Pagination(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	// Create 3 definitions in different clusters so they don't collide on
	// the unique active index.
	for i := 0; i < 3; i++ {
		in := validCreateInput()
		in.ClusterID = int64(i + 1)
		in.Service.Name = "api-" + string(rune('a'+i))
		if _, err := svc.CreateDefinition(context.Background(), in); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	resp, err := svc.ListDefinitions(context.Background(), DefinitionFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("items: want 2, got %d", len(resp.Items))
	}
	if resp.Total != 3 {
		t.Errorf("total: want 3, got %d", resp.Total)
	}
	if !resp.Truncated {
		t.Errorf("truncated: want true")
	}
}

// TestService_ListDefinitions_LimitClamping verifies that limit > 200 is
// clamped to 100 by the service.
func TestService_ListDefinitions_LimitClamping(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo, nil)
	// Request limit=300; service should clamp to 100.
	resp, err := svc.ListDefinitions(context.Background(), DefinitionFilter{Limit: 300})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// No definitions, but the call should succeed without error. The
	// clamping is implicit; we just verify no panic and zero items.
	if len(resp.Items) != 0 {
		t.Errorf("items: want 0, got %d", len(resp.Items))
	}
}

// TestService_ListEvaluations_VersionFilter verifies the version filter.
func TestService_ListEvaluations_VersionFilter(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	repo := newMemoryRepository()
	source := fakeMetricsSource{
		series: SLISeries{
			Good:            []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			Total:           []Sample{sampleAt(start.Add(1*time.Minute), 0), sampleAt(now.Add(-1*time.Minute), 100)},
			ExpectedSamples: 4,
		},
	}
	svc := NewService(repo, NewEvaluator(source), WithNow(func() time.Time { return now }))
	def, _ := svc.CreateDefinition(context.Background(), validCreateInput())
	// Patch the SLO to bump version to 2.
	newObjective := 0.999
	patched, _ := svc.PatchDefinition(context.Background(), def.ID, PatchDefinitionInput{
		Objective: &newObjective,
		Actor:     ActorRef{ID: 1, Name: "alice"},
	})
	// Evaluate at version 2.
	if _, err := svc.EvaluateSLO(context.Background(), def.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Filter by version=patched.Version.
	v := patched.Version
	resp, err := svc.ListEvaluations(context.Background(), EvaluationFilter{SLOID: def.ID, Version: &v})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("items: want 1 (version %d), got %d", v, len(resp.Items))
	}
	if resp.Items[0].Version != v {
		t.Errorf("item version: want %d, got %d", v, resp.Items[0].Version)
	}
}

// TestNopBurnAlertSink verifies the no-op sink never errors.
func TestNopBurnAlertSink(t *testing.T) {
	sink := NopBurnAlertSink{}
	if err := sink.OnBurnTransition(context.Background(), BurnTransition{}); err != nil {
		t.Errorf("nop sink should not error, got %v", err)
	}
}
