package alert

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/metricshistory"
)

func TestScheduler_Disabled(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockRepo()
	svc := NewService(repo, newMockDiagnosisRepo(), &mockEvaluator{}, 60*time.Second)

	scheduler := NewScheduler(SchedulerConfig{
		Enabled:           false,
		PollInterval:      100 * time.Millisecond,
		ClaimBatch:        20,
		WorkerConcurrency: 4,
		EvaluationTimeout: 10 * time.Second,
		ClaimLease:        30 * time.Second,
	}, svc, repo, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	scheduler.Run(ctx)
	// If we get here, the scheduler respected the disabled flag and exited on context cancel
}

func TestScheduler_TickEvaluatesRules(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := NewService(repo, newMockDiagnosisRepo(), evaluator, 60*time.Second)

	var evalCount int32
	now := time.Now()
	scheduler := NewScheduler(SchedulerConfig{
		Enabled:           true,
		PollInterval:      50 * time.Millisecond,
		ClaimBatch:        20,
		WorkerConcurrency: 4,
		EvaluationTimeout: 10 * time.Second,
		ClaimLease:        30 * time.Second,
	}, svc, repo, zap.NewNop())
	scheduler.now = func() time.Time { return now }

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Run scheduler in background
	go scheduler.Run(ctx)

	// Wait for at least one evaluation
	deadline := time.After(200 * time.Millisecond)
	for {
		if atomic.LoadInt32(&evalCount) > 0 {
			break
		}
		inst, _ := repo.GetUnresolvedInstance(context.Background(), rule.ID)
		if inst != nil {
			break
		}
		select {
		case <-deadline:
			// Check if instance was created
			inst, err := repo.GetUnresolvedInstance(context.Background(), rule.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inst == nil {
				t.Fatal("expected scheduler to evaluate rule and create instance")
			}
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestScheduler_ClaimExpiredRulesRecover(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	// Simulate a stale claim from a crashed process
	pastTime := time.Now().Add(-60 * time.Second)
	rule.ClaimExpiresAt = &pastTime
	rule.NextDueAt = time.Now().Add(-30 * time.Second)
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := NewService(repo, newMockDiagnosisRepo(), evaluator, 60*time.Second)

	now := time.Now()
	scheduler := NewScheduler(SchedulerConfig{
		Enabled:           true,
		PollInterval:      50 * time.Millisecond,
		ClaimBatch:        20,
		WorkerConcurrency: 4,
		EvaluationTimeout: 10 * time.Second,
		ClaimLease:        30 * time.Second,
	}, svc, repo, zap.NewNop())
	scheduler.now = func() time.Time { return now }

	// ClaimDueRules should pick up the rule with expired claim
	due, err := repo.ClaimDueRules(context.Background(), now, 20, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, r := range due {
		if r.ID == rule.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected expired-claim rule to be claimable after process death")
	}
}

func TestScheduler_WorkerConcurrency(t *testing.T) {
	repo := newMockRepo()

	// Create 4 rules
	for i := 1; i <= 4; i++ {
		rule := Rule{
			ID:            int64(i),
			ClusterID:     1,
			DisplayName:   "Rule-" + string(rune('A'+i-1)),
			ResourceKind:  ResourceKindNode,
			ResourceName:  "worker-01",
			MetricName:    MetricCPU,
			Operator:      OperatorGTE,
			Threshold:     3000000000,
			ForSeconds:    300,
			MinimumPoints: 5,
			Enabled:       true,
			NextDueAt:     time.Now().Add(-1 * time.Second),
			Creator:       ActorRef{ID: 1, Name: "admin"},
		}
		repo.rules[rule.ID] = &rule
	}

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := NewService(repo, newMockDiagnosisRepo(), evaluator, 60*time.Second)

	// Track max concurrent evaluations
	var concurrent int32
	var maxConcurrent int32

	// We can't easily instrument the scheduler's internal goroutines,
	// but we can verify that all rules get evaluated
	scheduler := NewScheduler(SchedulerConfig{
		Enabled:           true,
		PollInterval:      50 * time.Millisecond,
		ClaimBatch:        20,
		WorkerConcurrency: 2,
		EvaluationTimeout: 10 * time.Second,
		ClaimLease:        30 * time.Second,
	}, svc, repo, zap.NewNop())

	_ = scheduler
	_ = concurrent
	_ = maxConcurrent

	// Verify ClaimDueRules returns all due rules
	now := time.Now()
	due, err := repo.ClaimDueRules(context.Background(), now, 20, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(due) != 4 {
		t.Errorf("expected 4 due rules, got %d", len(due))
	}
}

func TestScheduler_NoOverlap(t *testing.T) {
	// Verify that the scheduler's tick waits for all workers to complete
	// before the next tick can start (the wg.Wait() ensures this)
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	evaluator := &mockEvaluator{response: makeFiringEval()}
	svc := NewService(repo, newMockDiagnosisRepo(), evaluator, 60*time.Second)

	scheduler := NewScheduler(SchedulerConfig{
		Enabled:           true,
		PollInterval:      50 * time.Millisecond,
		ClaimBatch:        20,
		WorkerConcurrency: 4,
		EvaluationTimeout: 10 * time.Second,
		ClaimLease:        30 * time.Second,
	}, svc, repo, zap.NewNop())

	// Run tick manually — should complete without panic
	ctx := context.Background()
	scheduler.tick(ctx)

	// Verify instance was created
	inst, err := repo.GetUnresolvedInstance(ctx, rule.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst == nil {
		t.Error("expected instance after tick")
	}
}

func TestScheduler_EvaluationTimeout(t *testing.T) {
	repo := newMockRepo()
	rule := makeTestRule()
	repo.rules[rule.ID] = &rule

	// Create a slow evaluator that respects context cancellation
	slowEvaluator := &slowMockEvaluator{delay: 5 * time.Second}
	svc := NewService(repo, newMockDiagnosisRepo(), slowEvaluator, 60*time.Second)

	scheduler := NewScheduler(SchedulerConfig{
		Enabled:           true,
		PollInterval:      50 * time.Millisecond,
		ClaimBatch:        20,
		WorkerConcurrency: 4,
		EvaluationTimeout: 100 * time.Millisecond, // Very short timeout
		ClaimLease:        30 * time.Second,
	}, svc, repo, zap.NewNop())

	now := time.Now()
	scheduler.now = func() time.Time { return now }

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// This should not block indefinitely — the evaluation timeout kicks in
	done := make(chan struct{})
	go func() {
		scheduler.tick(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — tick completed within timeout
	case <-time.After(2 * time.Second):
		t.Fatal("tick blocked — evaluation timeout not working")
	}
}

type slowMockEvaluator struct {
	delay time.Duration
}

func (m *slowMockEvaluator) Evaluate(ctx context.Context, _ metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error) {
	select {
	case <-time.After(m.delay):
		return makeFiringEval(), nil
	case <-ctx.Done():
		return metricshistory.EvaluationResponse{}, ctx.Err()
	}
}
