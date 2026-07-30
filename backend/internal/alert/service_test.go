package alert

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/metricshistory"
)

// --- mock repository ---

type mockRepo struct {
	rules      map[int64]*Rule
	instances  map[int64]*Instance
	nextRuleID int64
	nextInstID int64
	nextDiagID int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		rules:      make(map[int64]*Rule),
		instances:  make(map[int64]*Instance),
		nextRuleID: 1,
		nextInstID: 1,
		nextDiagID: 1,
	}
}

func (m *mockRepo) CreateRule(_ context.Context, rule *Rule, _ time.Duration) error {
	activeCount := 0
	for _, r := range m.rules {
		if r.ClusterID == rule.ClusterID && !r.Deleted {
			activeCount++
		}
	}
	if activeCount >= MaxRulesPerCluster {
		return ErrClusterLimit
	}
	for _, r := range m.rules {
		if r.ClusterID == rule.ClusterID && !r.Deleted && r.DisplayName == rule.DisplayName {
			return ErrDuplicateName
		}
	}
	rule.ID = m.nextRuleID
	m.nextRuleID++
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	m.rules[rule.ID] = rule
	return nil
}

func (m *mockRepo) GetRule(_ context.Context, id int64) (Rule, error) {
	r, ok := m.rules[id]
	if !ok {
		return Rule{}, ErrRuleNotFound
	}
	return *r, nil
}

func (m *mockRepo) ListRules(_ context.Context, filter RuleListFilter) ([]Rule, error) {
	var result []Rule
	for _, r := range m.rules {
		if r.ClusterID != filter.ClusterID {
			continue
		}
		if !filter.IncludeDeleted && r.Deleted {
			continue
		}
		result = append(result, *r)
	}
	return result, nil
}

func (m *mockRepo) PatchRule(_ context.Context, id int64, input PatchRuleInput, _ ActorRef) (Rule, error) {
	r, ok := m.rules[id]
	if !ok {
		return Rule{}, ErrRuleNotFound
	}
	if r.Deleted {
		return Rule{}, ErrRuleDeleted
	}
	if input.DisplayName != nil {
		for _, other := range m.rules {
			if other.ID != id && other.ClusterID == r.ClusterID && !other.Deleted && other.DisplayName == *input.DisplayName {
				return Rule{}, ErrDuplicateName
			}
		}
		r.DisplayName = *input.DisplayName
	}
	if input.Enabled != nil {
		r.Enabled = *input.Enabled
	}
	r.UpdatedAt = time.Now()
	return *r, nil
}

func (m *mockRepo) DeleteRule(_ context.Context, id int64) error {
	r, ok := m.rules[id]
	if !ok {
		return ErrRuleNotFound
	}
	if r.Deleted {
		return ErrRuleDeleted
	}
	for _, inst := range m.instances {
		if inst.RuleID == id && inst.State == StateFiring {
			return ErrRuleUnresolvedAlert
		}
	}
	r.Deleted = true
	r.Enabled = false
	r.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) GetUnresolvedInstance(_ context.Context, ruleID int64) (*Instance, error) {
	for _, inst := range m.instances {
		if inst.RuleID == ruleID && inst.State == StateFiring {
			return inst, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) CreateInstance(_ context.Context, instance *Instance) error {
	instance.ID = m.nextInstID
	m.nextInstID++
	instance.CreatedAt = time.Now()
	instance.UpdatedAt = time.Now()
	instance.State = StateFiring
	m.instances[instance.ID] = instance
	return nil
}

func (m *mockRepo) CreateFiring(ctx context.Context, record *diagnosis.Record, instance *Instance) error {
	record.ID = m.nextDiagID
	m.nextDiagID++
	instance.DiagnosisID = record.ID
	return m.CreateInstance(ctx, instance)
}

func (m *mockRepo) TouchInstance(_ context.Context, ruleID int64, lastFiredAt time.Time, evidenceAnchor string) error {
	for _, inst := range m.instances {
		if inst.RuleID == ruleID && inst.State == StateFiring {
			inst.LastFiredAt = lastFiredAt
			inst.LatestEvidenceAnchor = evidenceAnchor
			inst.UpdatedAt = time.Now()
			return nil
		}
	}
	return ErrAlertNotFound
}

func (m *mockRepo) ResolveInstance(_ context.Context, ruleID int64, resolvedAt time.Time) error {
	for _, inst := range m.instances {
		if inst.RuleID == ruleID && inst.State == StateFiring {
			inst.State = StateResolved
			inst.ResolvedAt = &resolvedAt
			inst.UpdatedAt = time.Now()
			return nil
		}
	}
	return ErrAlertNotFound
}

func (m *mockRepo) ListInstances(_ context.Context, filter InstanceListFilter) ([]Instance, error) {
	var result []Instance
	for _, inst := range m.instances {
		r, ok := m.rules[inst.RuleID]
		if !ok || r.ClusterID != filter.ClusterID {
			continue
		}
		if filter.State != "" && inst.State != filter.State {
			continue
		}
		if filter.RuleID > 0 && inst.RuleID != filter.RuleID {
			continue
		}
		result = append(result, *inst)
	}
	return result, nil
}

func (m *mockRepo) GetInstance(_ context.Context, id int64) (Instance, error) {
	inst, ok := m.instances[id]
	if !ok {
		return Instance{}, ErrAlertNotFound
	}
	return *inst, nil
}

func (m *mockRepo) ClaimDueRules(_ context.Context, now time.Time, batchSize int, claimLease time.Duration) ([]Rule, error) {
	var due []Rule
	for _, r := range m.rules {
		if !r.Deleted && r.Enabled && !r.NextDueAt.After(now) {
			due = append(due, *r)
		}
	}
	if len(due) > batchSize {
		due = due[:batchSize]
	}
	return due, nil
}

func (m *mockRepo) ReleaseClaim(_ context.Context, ruleID int64, nextDueAt time.Time, evalState string, evalAt time.Time, errCode string) error {
	r, ok := m.rules[ruleID]
	if !ok {
		return ErrRuleNotFound
	}
	r.ClaimExpiresAt = nil
	r.NextDueAt = nextDueAt
	r.LastEvaluationState = evalState
	r.LastEvaluationAt = &evalAt
	r.LastErrorCode = errCode
	r.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) UpdateRuleHealth(_ context.Context, ruleID int64, evalState string, evalAt time.Time, errCode string) error {
	r, ok := m.rules[ruleID]
	if !ok {
		return ErrRuleNotFound
	}
	r.LastEvaluationState = evalState
	r.LastEvaluationAt = &evalAt
	r.LastErrorCode = errCode
	r.UpdatedAt = time.Now()
	return nil
}

// --- mock metric evaluator ---

type mockEvaluator struct {
	response metricshistory.EvaluationResponse
	err      error
	query    metricshistory.EvaluationQuery
}

func (m *mockEvaluator) Evaluate(_ context.Context, query metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error) {
	m.query = query
	return m.response, m.err
}

// --- mock diagnosis repository ---

type mockDiagnosisRepo struct {
	records []*diagnosis.Record
	nextID  int64
}

func newMockDiagnosisRepo() *mockDiagnosisRepo {
	return &mockDiagnosisRepo{nextID: 1}
}

func (m *mockDiagnosisRepo) Save(_ context.Context, record *diagnosis.Record) error {
	record.ID = m.nextID
	m.nextID++
	m.records = append(m.records, record)
	return nil
}

// --- helper ---

func makeTestRule() Rule {
	return Rule{
		ID:            1,
		ClusterID:     1,
		DisplayName:   "High CPU Alert",
		ResourceKind:  ResourceKindNode,
		ResourceName:  "worker-01",
		MetricName:    MetricCPU,
		Operator:      OperatorGTE,
		Threshold:     3000000000,
		ForSeconds:    300,
		MinimumPoints: 5,
		Enabled:       true,
		NextDueAt:     time.Now(),
		Creator:       ActorRef{ID: 1, Name: "admin"},
	}
}

func makeFiringEval() metricshistory.EvaluationResponse {
	return metricshistory.EvaluationResponse{
		State:           metricshistory.EvaluationStateFiring,
		PointsEvaluated: 10,
		BreachingPoints: 8,
		SustainedWindows: []metricshistory.SustainedWindow{
			{SpanSeconds: 300, BreachingPoints: 8},
		},
		Series: metricshistory.Series{
			ClusterID:    1,
			ResourceKind: "Node",
			ResourceName: "worker-01",
			MetricName:   "cpu",
		},
	}
}

func makeNormalEval() metricshistory.EvaluationResponse {
	return metricshistory.EvaluationResponse{
		State:           metricshistory.EvaluationStateNormal,
		PointsEvaluated: 10,
		BreachingPoints: 0,
		Series: metricshistory.Series{
			ClusterID:    1,
			ResourceKind: "Node",
			ResourceName: "worker-01",
			MetricName:   "cpu",
		},
	}
}

func makeInsufficientEval() metricshistory.EvaluationResponse {
	return metricshistory.EvaluationResponse{
		State:           metricshistory.EvaluationStateInsufficientData,
		PointsEvaluated: 1,
		BreachingPoints: 0,
		Series: metricshistory.Series{
			ClusterID:    1,
			ResourceKind: "Node",
			ResourceName: "worker-01",
			MetricName:   "cpu",
		},
	}
}

func makeTestService(repo *mockRepo, evaluator *mockEvaluator) *Service {
	return NewService(repo, newMockDiagnosisRepo(), evaluator, 60*time.Second)
}

// --- tests ---

func TestCreateRule(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, newMockDiagnosisRepo(), &mockEvaluator{}, 60*time.Second)

	t.Run("valid rule creation", func(t *testing.T) {
		rule, err := svc.CreateRule(context.Background(), CreateRuleInput{
			ClusterID:     1,
			DisplayName:   "Test Alert",
			ResourceKind:  ResourceKindNode,
			ResourceName:  "worker-01",
			MetricName:    MetricCPU,
			Operator:      OperatorGTE,
			Threshold:     3000000000,
			ForSeconds:    300,
			MinimumPoints: 5,
		}, ActorRef{ID: 1, Name: "admin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.ID == 0 {
			t.Error("expected rule ID to be set")
		}
		if rule.DisplayName != "Test Alert" {
			t.Errorf("expected display_name 'Test Alert', got %q", rule.DisplayName)
		}
	})

	t.Run("invalid rule rejected", func(t *testing.T) {
		_, err := svc.CreateRule(context.Background(), CreateRuleInput{
			ClusterID:    1,
			DisplayName:  "Bad",
			ResourceKind: "Pod",
			ResourceName: "worker-01",
			MetricName:   MetricCPU,
			Operator:     OperatorGTE,
			Threshold:    3000000000,
			ForSeconds:   300,
		}, ActorRef{ID: 1, Name: "admin"})
		if err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("cluster limit enforced", func(t *testing.T) {
		for i := 0; i < MaxRulesPerCluster; i++ {
			_, err := svc.CreateRule(context.Background(), CreateRuleInput{
				ClusterID:     99,
				DisplayName:   fmt.Sprintf("Rule-%d", i),
				ResourceKind:  ResourceKindNode,
				ResourceName:  "worker-01",
				MetricName:    MetricCPU,
				Operator:      OperatorGTE,
				Threshold:     3000000000,
				ForSeconds:    300,
				MinimumPoints: 5,
			}, ActorRef{ID: 1, Name: "admin"})
			if err != nil {
				t.Fatalf("unexpected error creating rule %d: %v", i, err)
			}
		}
		_, err := svc.CreateRule(context.Background(), CreateRuleInput{
			ClusterID:     99,
			DisplayName:   "Over Limit",
			ResourceKind:  ResourceKindNode,
			ResourceName:  "worker-01",
			MetricName:    MetricCPU,
			Operator:      OperatorGTE,
			Threshold:     3000000000,
			ForSeconds:    300,
			MinimumPoints: 5,
		}, ActorRef{ID: 1, Name: "admin"})
		if err != ErrClusterLimit {
			t.Errorf("expected ErrClusterLimit, got %v", err)
		}
	})

	t.Run("duplicate name rejected", func(t *testing.T) {
		input := CreateRuleInput{
			ClusterID:     88,
			DisplayName:   "Duplicate",
			ResourceKind:  ResourceKindNode,
			ResourceName:  "worker-01",
			MetricName:    MetricCPU,
			Operator:      OperatorGTE,
			Threshold:     3000000000,
			ForSeconds:    300,
			MinimumPoints: 5,
		}
		_, err := svc.CreateRule(context.Background(), input, ActorRef{ID: 1, Name: "admin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = svc.CreateRule(context.Background(), input, ActorRef{ID: 1, Name: "admin"})
		if err != ErrDuplicateName {
			t.Errorf("expected ErrDuplicateName, got %v", err)
		}
	})
}

func TestClusterScopedSingleObjectAccess(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule
	instance := &Instance{ID: 9, RuleID: rule.ID, State: StateFiring}
	repo.instances[instance.ID] = instance
	svc := makeTestService(repo, &mockEvaluator{})

	if _, err := svc.GetRule(context.Background(), 2, rule.ID); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("cross-cluster rule read must be hidden, got %v", err)
	}
	name := "changed"
	if _, err := svc.PatchRule(context.Background(), 2, rule.ID, PatchRuleInput{DisplayName: &name}, ActorRef{}); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("cross-cluster rule patch must be hidden, got %v", err)
	}
	if err := svc.DeleteRule(context.Background(), 2, rule.ID); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("cross-cluster rule delete must be hidden, got %v", err)
	}
	if _, err := svc.GetInstance(context.Background(), 2, instance.ID); !errors.Is(err, ErrAlertNotFound) {
		t.Fatalf("cross-cluster alert read must be hidden, got %v", err)
	}
}

func TestEvaluateRule_FiringCreatesInstance(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := makeTestService(repo, evaluator)

	err := svc.EvaluateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inst, err := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst == nil {
		t.Fatal("expected unresolved instance to be created, got nil")
	}
	if inst.RuleID != rule.ID {
		t.Errorf("expected rule_id %d, got %d", rule.ID, inst.RuleID)
	}
	if inst.DiagnosisID == 0 {
		t.Error("expected diagnosis_id to be set")
	}
}

func TestEvaluateRule_RepeatedFiringDeduplicates(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := makeTestService(repo, evaluator)

	// First firing — creates instance and diagnosis
	err := svc.EvaluateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}

	inst1, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst1 == nil {
		t.Fatal("expected instance after first firing")
	}
	diagID1 := inst1.DiagnosisID

	// Second firing — should touch, not create a new instance/diagnosis
	err = svc.EvaluateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}

	inst2, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst2 == nil {
		t.Fatal("expected instance after second firing")
	}
	if inst2.DiagnosisID != diagID1 {
		t.Errorf("expected same diagnosis_id %d, got %d — deduplication failed", diagID1, inst2.DiagnosisID)
	}
	if inst2.ID != inst1.ID {
		t.Errorf("expected same instance ID %d, got %d — deduplication failed", inst1.ID, inst2.ID)
	}

	// Count instances — should be exactly 1
	count := 0
	for _, inst := range repo.instances {
		if inst.RuleID == rule.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 instance for rule, got %d", count)
	}
}

func TestEvaluateRule_NormalResolvesFiring(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	// First: firing
	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := makeTestService(repo, evaluator)
	svc.EvaluateRule(context.Background(), rule)

	inst, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst == nil {
		t.Fatal("expected instance after firing")
	}
	instID := inst.ID

	// Then: normal — should resolve
	evaluator.response = makeNormalEval()
	svc.EvaluateRule(context.Background(), rule)

	// Should no longer have unresolved instance
	unresolved, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if unresolved != nil {
		t.Error("expected no unresolved instance after normal evaluation")
	}

	// The resolved instance should still exist
	resolved, ok := repo.instances[instID]
	if !ok {
		t.Fatal("resolved instance should still exist")
	}
	if resolved.State != StateResolved {
		t.Errorf("expected state %q, got %q", StateResolved, resolved.State)
	}
	if resolved.ResolvedAt == nil {
		t.Error("expected resolved_at to be set")
	}
}

func TestEvaluateRule_UsesBoundedRecentLookback(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	rule.ForSeconds = 60
	rule.MinimumPoints = 2
	repo.rules[rule.ID] = &rule
	evaluator := &mockEvaluator{response: makeNormalEval()}
	svc := NewService(repo, newMockDiagnosisRepo(), evaluator, time.Minute)
	now := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	if err := svc.EvaluateRule(context.Background(), rule); err != nil {
		t.Fatalf("evaluate rule: %v", err)
	}
	if !evaluator.query.To.Equal(now) || !evaluator.query.From.Equal(now.Add(-3*time.Minute)) {
		t.Fatalf("query window = %s..%s, want %s..%s", evaluator.query.From, evaluator.query.To, now.Add(-3*time.Minute), now)
	}
}

func TestEvaluationLookback_UsesPointRequirementAndClampsAt24Hours(t *testing.T) {
	rule := makeTestRule()
	rule.ForSeconds = 60
	rule.MinimumPoints = 5
	if got := evaluationLookback(rule, time.Minute); got != 6*time.Minute {
		t.Fatalf("point-based lookback = %s, want 6m", got)
	}
	rule.ForSeconds = 86400
	rule.MinimumPoints = 1440
	if got := evaluationLookback(rule, time.Minute); got != 24*time.Hour {
		t.Fatalf("clamped lookback = %s, want 24h", got)
	}
}

func TestEvaluateRule_LaterFiringCreatesNewInstance(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := makeTestService(repo, evaluator)

	// First firing
	svc.EvaluateRule(context.Background(), rule)
	inst1, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	inst1ID := inst1.ID

	// Normal — resolves
	evaluator.response = makeNormalEval()
	svc.EvaluateRule(context.Background(), rule)

	// Later firing — should create a NEW instance
	evaluator.response = makeFiringEval()
	svc.EvaluateRule(context.Background(), rule)

	inst2, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst2 == nil {
		t.Fatal("expected new instance after later firing")
	}
	if inst2.ID == inst1ID {
		t.Error("expected a new instance ID, got the same as the first — should create new instance after resolution")
	}

	// Should have 2 instances total
	count := 0
	for _, inst := range repo.instances {
		if inst.RuleID == rule.ID {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 instances (1 resolved + 1 firing), got %d", count)
	}
}

func TestEvaluateRule_InsufficientDataNeverFires(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeInsufficientEval()}
	svc := makeTestService(repo, evaluator)

	err := svc.EvaluateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inst, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst != nil {
		t.Error("insufficient_data should never create an alert instance")
	}

	updated, _ := repo.GetRule(context.Background(), rule.ID)
	if updated.LastEvaluationState != EvalStateInsufficient {
		t.Errorf("expected eval state %q, got %q", EvalStateInsufficient, updated.LastEvaluationState)
	}
}

func TestEvaluateRule_InsufficientDataNeverResolves(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	// First: firing
	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := makeTestService(repo, evaluator)
	svc.EvaluateRule(context.Background(), rule)

	// Then: insufficient_data — should NOT resolve
	evaluator.response = makeInsufficientEval()
	svc.EvaluateRule(context.Background(), rule)

	inst, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst == nil {
		t.Error("insufficient_data should not resolve a firing alert")
	}
	if inst.State != StateFiring {
		t.Errorf("expected state %q, got %q", StateFiring, inst.State)
	}
}

func TestEvaluateRule_EvaluatorErrorNeverFires(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{err: fmt.Errorf("upstream error")}
	svc := makeTestService(repo, evaluator)

	err := svc.EvaluateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inst, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst != nil {
		t.Error("evaluator error should never create an alert instance")
	}

	updated, _ := repo.GetRule(context.Background(), rule.ID)
	if updated.LastEvaluationState != EvalStateError {
		t.Errorf("expected eval state %q, got %q", EvalStateError, updated.LastEvaluationState)
	}
	if updated.LastErrorCode == "" {
		t.Error("expected error code to be set")
	}
}

func TestEvaluateRule_EvaluatorErrorNeverResolves(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	// First: firing
	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := makeTestService(repo, evaluator)
	svc.EvaluateRule(context.Background(), rule)

	// Then: evaluator error — should NOT resolve
	evaluator.response = metricshistory.EvaluationResponse{}
	evaluator.err = fmt.Errorf("upstream error")
	svc.EvaluateRule(context.Background(), rule)

	inst, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
	if inst == nil {
		t.Error("evaluator error should not resolve a firing alert")
	}
}

func TestEvaluateRule_DisabledRuleNotScheduled(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	rule.Enabled = false
	repo.rules[rule.ID] = &rule

	// ClaimDueRules should not return disabled rules
	due, err := repo.ClaimDueRules(context.Background(), time.Now(), 20, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range due {
		if r.ID == rule.ID {
			t.Error("disabled rule should not be claimed")
		}
	}
}

func TestEvaluateRule_DeletedRuleNotScheduled(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	rule.Deleted = true
	repo.rules[rule.ID] = &rule

	due, err := repo.ClaimDueRules(context.Background(), time.Now(), 20, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range due {
		if r.ID == rule.ID {
			t.Error("deleted rule should not be claimed")
		}
	}
}

func TestDeleteRule_WithUnresolvedAlert(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	// Create an unresolved instance
	repo.instances[1] = &Instance{
		ID:     1,
		RuleID: rule.ID,
		State:  StateFiring,
	}

	err := repo.DeleteRule(context.Background(), rule.ID)
	if err != ErrRuleUnresolvedAlert {
		t.Errorf("expected ErrRuleUnresolvedAlert, got %v", err)
	}
}

func TestPatchRule_ImmutableEvaluationFields(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	// Patch only display name and enabled — should succeed
	name := "Updated Alert"
	enabled := false
	patched, err := repo.PatchRule(context.Background(), rule.ID, PatchRuleInput{
		DisplayName: &name,
		Enabled:     &enabled,
	}, ActorRef{ID: 1, Name: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patched.DisplayName != "Updated Alert" {
		t.Errorf("expected display_name 'Updated Alert', got %q", patched.DisplayName)
	}
	if patched.Enabled != false {
		t.Error("expected enabled to be false")
	}
	// Evaluation fields should remain unchanged
	if patched.Threshold != rule.Threshold {
		t.Error("threshold should not change")
	}
	if patched.ForSeconds != rule.ForSeconds {
		t.Error("for_seconds should not change")
	}
}
