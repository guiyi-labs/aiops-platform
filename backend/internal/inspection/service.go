package inspection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ClusterLister returns reachable clusters. Implemented by cluster.Service.
type ClusterLister interface {
	List(ctx context.Context) ([]struct {
		ID   int64
		Name string
	}, error)
}

// RuleExecutor runs a single inspection rule against a cluster. The M52 default
// implementation uses the read-only Kubernetes gateway (service.go defaultExecutor);
// tests may inject a fake executor.
type RuleExecutor interface {
	Execute(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error)
}

// Service orchestrates inspection rule execution, plan management and task lifecycle.
type Service struct {
	repo                  Repository
	catalog               map[string]RuleDescriptor
	catalogList           []RuleDescriptor
	executor              RuleExecutor
	clusters              ClusterLister
	logger                *zap.Logger
	now                   func() time.Time
	maxConcurrentClusters int
	perClusterTimeout     time.Duration
	maxTaskResults        int

	mu     sync.Mutex
	active map[int64]context.CancelFunc // task_id -> cancel, for running tasks
}

// Config holds validated Service construction parameters.
type Config struct {
	MaxConcurrentClusters int
	PerClusterTimeout     time.Duration
	MaxTaskResults        int
}

// NewService constructs a Service. executor and clusters may be nil only in tests
// that never call RunInspectOnce; production wiring MUST set both.
func NewService(
	cfg Config,
	repo Repository,
	executor RuleExecutor,
	clusters ClusterLister,
	logger *zap.Logger,
) (*Service, error) {
	if cfg.MaxConcurrentClusters < 1 {
		cfg.MaxConcurrentClusters = DefaultMaxConcurrentClusters
	}
	if cfg.PerClusterTimeout < 5*time.Second {
		cfg.PerClusterTimeout = DefaultPerClusterTimeout
	}
	if cfg.MaxTaskResults < 100 {
		cfg.MaxTaskResults = DefaultMaxTaskResults
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	catalogList := DefaultCatalog()
	return &Service{
		repo:                  repo,
		catalog:               CatalogByCode(catalogList),
		catalogList:           catalogList,
		executor:              executor,
		clusters:              clusters,
		logger:                logger,
		now:                   time.Now,
		maxConcurrentClusters: cfg.MaxConcurrentClusters,
		perClusterTimeout:     cfg.PerClusterTimeout,
		maxTaskResults:        cfg.MaxTaskResults,
		active:                make(map[int64]context.CancelFunc),
	}, nil
}

// --- Catalog (read-only) ---

// Catalog returns the compile-time catalog. Callers MUST NOT mutate the returned slice.
func (s *Service) Catalog() []RuleDescriptor { return s.catalogList }

// EffectiveRules returns the effective rule set for a cluster: compiled-in defaults
// merged with any per-cluster overrides from inspection_rules.
func (s *Service) EffectiveRules(ctx context.Context, clusterID int64) ([]RuleDescriptor, error) {
	overrides, err := s.repo.ListRuleOverrides(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]RuleOverride, len(overrides))
	for _, o := range overrides {
		byCode[o.RuleCode] = o
	}
	out := make([]RuleDescriptor, 0, len(s.catalogList))
	for _, d := range s.catalogList {
		o, ok := byCode[d.Code]
		if ok && !o.Enabled {
			continue
		}
		eff := d
		if ok && o.SeverityOverride != "" {
			eff.DefaultSeverity = o.SeverityOverride
		}
		out = append(out, eff)
	}
	return out, nil
}

// --- Plan CRUD ---

func (s *Service) CreatePlan(ctx context.Context, plan *Plan) (Plan, error) {
	if plan == nil {
		return Plan{}, ErrInvalidPlan
	}
	if strings.TrimSpace(plan.Name) == "" || len(plan.Name) > MaxPlanNameLen {
		return Plan{}, ErrInvalidPlan
	}
	if plan.CreatorID == 0 {
		return Plan{}, ErrInvalidPlan
	}
	for _, c := range plan.RuleCodes {
		if !IsValidRuleCode(s.catalog, c) {
			return Plan{}, fmt.Errorf("%w: %s", ErrInvalidRuleCode, c)
		}
	}
	now := s.now().UTC()
	plan.Name = strings.TrimSpace(plan.Name)
	plan.CreatedAt = now
	plan.UpdatedAt = now
	if plan.Enabled {
		plan.NextRunAt = nextCronRun(plan.CronSpec, now)
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		return Plan{}, err
	}
	return *plan, nil
}

func (s *Service) ListPlans(ctx context.Context, filter PlanListFilter) ([]PlanView, error) {
	plans, err := s.repo.ListPlans(ctx, filter)
	if err != nil {
		return nil, err
	}
	views := make([]PlanView, 0, len(plans))
	for _, p := range plans {
		views = append(views, planViewFrom(p))
	}
	return views, nil
}

func (s *Service) GetPlan(ctx context.Context, id int64) (PlanView, error) {
	p, err := s.repo.GetPlan(ctx, id)
	if err != nil {
		return PlanView{}, err
	}
	return planViewFrom(p), nil
}

func (s *Service) UpdatePlan(ctx context.Context, id int64, patch PatchPlanInput) (PlanView, error) {
	if patch.RuleCodes != nil {
		for _, c := range *patch.RuleCodes {
			if !IsValidRuleCode(s.catalog, c) {
				return PlanView{}, fmt.Errorf("%w: %s", ErrInvalidRuleCode, c)
			}
		}
	}
	p, err := s.repo.UpdatePlan(ctx, id, patch)
	if err != nil {
		return PlanView{}, err
	}
	return planViewFrom(p), nil
}

func (s *Service) DeletePlan(ctx context.Context, id, creatorID int64) error {
	return s.repo.DeletePlan(ctx, id, creatorID)
}

// --- Task execution ---

// RunInspectOnce triggers an ad-hoc inspection and returns immediately with the task
// record. Execution happens in the background (goroutine per cluster, bounded by
// MaxConcurrentClusters). Callers poll via GetTask / ListResults.
func (s *Service) RunInspectOnce(ctx context.Context, triggeredBy int64, clusterIDs []int64, ruleCodes []string) (TaskView, error) {
	if s.executor == nil || s.clusters == nil {
		return TaskView{}, ErrClusterUnreachable
	}
	for _, c := range ruleCodes {
		if c != "" && !IsValidRuleCode(s.catalog, c) {
			return TaskView{}, fmt.Errorf("%w: %s", ErrInvalidRuleCode, c)
		}
	}
	// Resolve target clusters: explicit list OR all reachable.
	effectiveClusters, err := s.resolveClusters(ctx, clusterIDs)
	if err != nil {
		return TaskView{}, err
	}
	if len(effectiveClusters) == 0 {
		return TaskView{}, ErrClusterUnreachable
	}
	now := s.now().UTC()
	task := Task{
		TriggeredBy:       &triggeredBy,
		TriggerReason:     TriggerManual,
		ClusterIDs:        effectiveClusterIDs(effectiveClusters),
		RuleCodes:         append([]string(nil), ruleCodes...),
		Status:            TaskStatusPending,
		TotalClusters:     len(effectiveClusters),
		CompletedClusters: 0,
		FindingCount:      0,
		CreatedAt:         now,
	}
	if err := s.repo.CreateTask(ctx, &task); err != nil {
		return TaskView{}, err
	}
	// Launch background execution.
	execCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.active[task.ID] = cancel
	s.mu.Unlock()
	go s.runTask(execCtx, task.ID, effectiveClusters, ruleCodes)
	return taskViewFrom(task), nil
}

func (s *Service) CancelTask(_ context.Context, taskID int64) {
	s.mu.Lock()
	cancel, ok := s.active[taskID]
	delete(s.active, taskID)
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

func (s *Service) ListTasks(ctx context.Context, filter TaskListFilter) (ListResponse[TaskView], error) {
	resp, err := s.repo.ListTasks(ctx, filter)
	if err != nil {
		return ListResponse[TaskView]{}, err
	}
	views := make([]TaskView, 0, len(resp.Items))
	for _, t := range resp.Items {
		views = append(views, taskViewFrom(t))
	}
	return ListResponse[TaskView]{Items: views, Total: resp.Total}, nil
}

func (s *Service) GetTask(ctx context.Context, id int64) (TaskView, error) {
	t, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return TaskView{}, err
	}
	return taskViewFrom(t), nil
}

// --- Results ---

func (s *Service) ListResults(ctx context.Context, filter ListFilter) (ListResponse[ResultView], error) {
	resp, err := s.repo.ListResults(ctx, filter)
	if err != nil {
		return ListResponse[ResultView]{}, err
	}
	views := make([]ResultView, 0, len(resp.Items))
	for _, r := range resp.Items {
		views = append(views, resultViewFrom(r))
	}
	return ListResponse[ResultView]{Items: views, Total: resp.Total}, nil
}

func (s *Service) GetResult(ctx context.Context, id int64) (ResultView, error) {
	r, err := s.repo.GetResult(ctx, id)
	if err != nil {
		return ResultView{}, err
	}
	return resultViewFrom(r), nil
}

// --- Internal: task execution ---

func (s *Service) runTask(ctx context.Context, taskID int64, clusters []struct {
	ID   int64
	Name string
}, ruleCodes []string) {
	defer func() {
		s.mu.Lock()
		delete(s.active, taskID)
		s.mu.Unlock()
	}()
	now := s.now()
	start := gorm.DeletedAt{Time: now, Valid: true}
	if err := s.repo.UpdateTaskStatus(ctx, taskID, PatchTaskInput{Status: pstring(TaskStatusRunning), StartedAt: &start}); err != nil {
		s.logger.Warn("inspection: mark running failed", zap.Int64("task_id", taskID), zap.Error(err))
		return
	}

	sem := make(chan struct{}, s.maxConcurrentClusters)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0
	findings := 0
	errs := []string{}
	allResults := make([]Result, 0, s.maxTaskResults)

	for _, cl := range clusters {
		cl := cl
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			clusterCtx, cancel := context.WithTimeout(ctx, s.perClusterTimeout)
			defer cancel()
			res, ferr := s.runOnCluster(clusterCtx, taskID, cl.ID, ruleCodes)
			mu.Lock()
			defer mu.Unlock()
			completed++
			if ferr != nil {
				errs = append(errs, fmt.Sprintf("cluster %d: %v", cl.ID, ferr))
			}
			if len(res) > 0 {
				findings += len(res)
				if len(allResults) < s.maxTaskResults {
					space := s.maxTaskResults - len(allResults)
					if len(res) > space {
						res = res[:space]
					}
					allResults = append(allResults, res...)
				}
			}
			_ = s.repo.UpdateTaskStatus(ctx, taskID, PatchTaskInput{
				CompletedClusters: &completed,
				FindingCount:      &findings,
			})
		}()
	}
	wg.Wait()

	// Persist all collected results in one batch.
	if len(allResults) > 0 {
		if err := s.repo.CreateResults(ctx, allResults); err != nil {
			errs = append(errs, fmt.Sprintf("persist: %v", err))
		}
	}

	status := TaskStatusCompleted
	var summary string
	if len(errs) > 0 && completed < len(clusters) {
		status = TaskStatusFailed
		summary = joinSummary(errs)
	} else if len(errs) > 0 {
		status = TaskStatusPartial
		summary = joinSummary(errs)
	}
	fin := gorm.DeletedAt{Time: s.now(), Valid: true}
	_ = s.repo.UpdateTaskStatus(ctx, taskID, PatchTaskInput{
		Status:       &status,
		FinishedAt:   &fin,
		FindingCount: &findings,
		ErrorSummary: &summary,
	})
}

func (s *Service) runOnCluster(ctx context.Context, taskID, clusterID int64, requestedRules []string) ([]Result, error) {
	rules, err := s.EffectiveRules(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	// Filter to requested rule codes (if any).
	if len(requestedRules) > 0 {
		wanted := make(map[string]bool, len(requestedRules))
		for _, r := range requestedRules {
			wanted[r] = true
		}
		filtered := rules[:0]
		for _, r := range rules {
			if wanted[r.Code] {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}
	if len(rules) == 0 {
		return nil, nil
	}
	var out []Result
	seen := make(map[string]struct{})
	for _, rule := range rules {
		findings, ferr := s.executor.Execute(ctx, clusterID, rule)
		if ferr != nil {
			if errors.Is(ferr, context.Canceled) || errors.Is(ferr, context.DeadlineExceeded) {
				return out, ferr
			}
			s.logger.Debug("inspection rule failed", zap.Int64("cluster", clusterID), zap.String("rule", rule.Code), zap.Error(ferr))
			continue
		}
		for _, f := range findings {
			fp := fingerprint(clusterID, f)
			if _, ok := seen[fp]; ok {
				continue
			}
			seen[fp] = struct{}{}
			sev := f.Severity
			if sev == "" {
				sev = rule.DefaultSeverity
			}
			state := f.State
			if state == "" {
				state = StateActive
			}
			evRaw := map[string]interface{}{}
			if f.Evidence != nil {
				evRaw = f.Evidence
			}
			evJSON, _ := json.Marshal(evRaw)
			out = append(out, Result{
				TaskID:           taskID,
				ClusterID:        clusterID,
				RuleCode:         rule.Code,
				SignalCode:       rule.SignalCode,
				Severity:         sev,
				State:            state,
				Namespace:        f.Namespace,
				ResourceKind:     f.ResourceKind,
				ResourceName:     f.ResourceName,
				ResourceUID:      f.ResourceUID,
				Fingerprint:      fp,
				EvidenceSnapshot: string(evJSON),
				ObservedAt:       f.ObservedAt,
				CreatedAt:        s.now(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out, nil
}

// --- Helpers ---

func (s *Service) resolveClusters(ctx context.Context, requested []int64) ([]struct {
	ID   int64
	Name string
}, error) {
	all, err := s.clusters.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		return all, nil
	}
	wanted := make(map[int64]bool, len(requested))
	for _, id := range requested {
		wanted[id] = true
	}
	out := make([]struct {
		ID   int64
		Name string
	}, 0, len(requested))
	for _, c := range all {
		if wanted[c.ID] {
			out = append(out, c)
		}
	}
	return out, nil
}

func effectiveClusterIDs(clusters []struct {
	ID   int64
	Name string
}) []int64 {
	out := make([]int64, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, c.ID)
	}
	return out
}

func fingerprint(clusterID int64, f Finding) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%s|%s", clusterID, f.RuleCode, f.Namespace, f.ResourceUID, f.ResourceKind+"."+f.ResourceName)
	return hex.EncodeToString(h.Sum(nil))[:MaxFingerprintLen]
}

func nextCronRun(_ string, _ time.Time) *time.Time { return nil } // placeholder: M52 exposes manual only

func joinSummary(errs []string) string {
	joined := strings.Join(errs, "; ")
	if len(joined) > MaxReasonLen {
		joined = joined[:MaxReasonLen]
	}
	return joined
}

func pstring(s string) *string { return &s }

// --- View mappers ---

func planViewFrom(p Plan) PlanView {
	return PlanView{
		ID: p.ID, Name: p.Name, CreatorID: p.CreatorID,
		ClusterIDs: append([]int64(nil), p.ClusterIDs...),
		RuleCodes:  append([]string(nil), p.RuleCodes...),
		CronSpec:   p.CronSpec, Enabled: p.Enabled,
		LastRunAt: p.LastRunAt, NextRunAt: p.NextRunAt,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func taskViewFrom(t Task) TaskView {
	return TaskView{
		ID: t.ID, PlanID: t.PlanID, PlanNameSnapshot: t.PlanNameSnapshot,
		TriggeredBy: t.TriggeredBy, TriggerReason: t.TriggerReason,
		ClusterIDs: append([]int64(nil), t.ClusterIDs...),
		RuleCodes:  append([]string(nil), t.RuleCodes...),
		Status:     t.Status, StartedAt: t.StartedAt, FinishedAt: t.FinishedAt,
		TotalClusters: t.TotalClusters, CompletedClusters: t.CompletedClusters,
		FindingCount: t.FindingCount, ErrorSummary: t.ErrorSummary, CreatedAt: t.CreatedAt,
	}
}

func resultViewFrom(r Result) ResultView {
	v := ResultView{
		ID: r.ID, TaskID: r.TaskID, ClusterID: r.ClusterID,
		RuleCode: r.RuleCode, SignalCode: r.SignalCode, Severity: r.Severity,
		State: r.State, Namespace: r.Namespace, ResourceKind: r.ResourceKind,
		ResourceName: r.ResourceName, ResourceUID: r.ResourceUID,
		Fingerprint: r.Fingerprint, ObservedAt: r.ObservedAt,
	}
	if r.EvidenceSnapshot != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(r.EvidenceSnapshot), &parsed); err == nil {
			v.Evidence = parsed
		}
	}
	return v
}
