package inspection

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository abstracts persistence for the inspection package.
type Repository interface {
	// Rule overrides (per-cluster)
	UpsertRuleOverride(ctx context.Context, override *RuleOverride) error
	ListRuleOverrides(ctx context.Context, clusterID int64) ([]RuleOverride, error)
	GetRuleOverride(ctx context.Context, clusterID int64, ruleCode string) (*RuleOverride, error)

	// Plans
	CreatePlan(ctx context.Context, plan *Plan) error
	GetPlan(ctx context.Context, id int64) (Plan, error)
	ListPlans(ctx context.Context, filter PlanListFilter) ([]Plan, error)
	UpdatePlan(ctx context.Context, id int64, patch PatchPlanInput) (Plan, error)
	DeletePlan(ctx context.Context, id, creatorID int64) error
	TouchPlanRun(ctx context.Context, id int64, lastRun, nextRun *gorm.DeletedAt) error

	// Tasks
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id int64) (Task, error)
	ListTasks(ctx context.Context, filter TaskListFilter) (ListResponse[Task], error)
	UpdateTaskStatus(ctx context.Context, id int64, patch PatchTaskInput) error

	// Results
	CreateResults(ctx context.Context, results []Result) error
	ListResults(ctx context.Context, filter ListFilter) (ListResponse[Result], error)
	GetResult(ctx context.Context, id int64) (Result, error)

	// Coverage (M113-3) aggregates plan → findings over a time window.
	Coverage(ctx context.Context, windowDays int, now time.Time) (CoverageSummary, error)
}

// PlanListFilter filters plans.
type PlanListFilter struct {
	CreatorID *int64
	Enabled   *bool
	Limit     int
	Offset    int
}

// TaskListFilter filters tasks.
type TaskListFilter struct {
	PlanID      *int64
	TriggeredBy *int64
	Status      string
	Limit       int
	Offset      int
}

// PatchPlanInput holds optional plan updates (PATCH).
type PatchPlanInput struct {
	Name       *string
	ClusterIDs *[]int64
	RuleCodes  *[]string
	CronSpec   *string
	Enabled    *bool
}

// PatchTaskInput holds task execution progress updates.
type PatchTaskInput struct {
	Status            *string
	StartedAt         *gorm.DeletedAt
	FinishedAt        *gorm.DeletedAt
	CompletedClusters *int
	FindingCount      *int
	ErrorSummary      *string
}

// GormRepository implements Repository with *gorm.DB.
type GormRepository struct{ db *gorm.DB }

// NewGormRepository returns a GORM-backed repository.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// --- Rule overrides ---

func (r *GormRepository) UpsertRuleOverride(ctx context.Context, o *RuleOverride) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "cluster_id"}, {Name: "rule_code"}},
		DoUpdates: clause.Assignments(map[string]any{
			"enabled":           o.Enabled,
			"severity_override": o.SeverityOverride,
			"updated_by":        o.UpdatedBy,
			"updated_at":        gorm.Expr("NOW()"),
		}),
	}).Create(o).Error
}

func (r *GormRepository) ListRuleOverrides(ctx context.Context, clusterID int64) ([]RuleOverride, error) {
	var rows []RuleOverride
	err := r.db.WithContext(ctx).Where("cluster_id = ?", clusterID).Order("rule_code ASC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) GetRuleOverride(ctx context.Context, clusterID int64, ruleCode string) (*RuleOverride, error) {
	var row RuleOverride
	err := r.db.WithContext(ctx).Where("cluster_id = ? AND rule_code = ?", clusterID, ruleCode).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// --- Plans ---

func (r *GormRepository) CreatePlan(ctx context.Context, plan *Plan) error {
	return r.db.WithContext(ctx).Create(plan).Error
}

func (r *GormRepository) GetPlan(ctx context.Context, id int64) (Plan, error) {
	var plan Plan
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Plan{}, ErrPlanNotFound
		}
		return Plan{}, err
	}
	return plan, nil
}

func (r *GormRepository) ListPlans(ctx context.Context, filter PlanListFilter) ([]Plan, error) {
	var plans []Plan
	q := r.db.WithContext(ctx).Model(&Plan{})
	if filter.CreatorID != nil {
		q = q.Where("creator_id = ?", *filter.CreatorID)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	err := q.Order("id DESC").Find(&plans).Error
	return plans, err
}

func (r *GormRepository) UpdatePlan(ctx context.Context, id int64, patch PatchPlanInput) (Plan, error) {
	plan, err := r.GetPlan(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	updates := map[string]interface{}{}
	if patch.Name != nil {
		updates["name"] = *patch.Name
	}
	if patch.ClusterIDs != nil {
		updates["cluster_ids"] = Int64Array(*patch.ClusterIDs)
	}
	if patch.RuleCodes != nil {
		updates["rule_codes"] = StringArray(*patch.RuleCodes)
	}
	if patch.CronSpec != nil {
		updates["cron_spec"] = *patch.CronSpec
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if len(updates) == 0 {
		return plan, nil
	}
	updates["updated_at"] = gorm.Expr("NOW()")
	if err := r.db.WithContext(ctx).Model(&Plan{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return Plan{}, err
	}
	return r.GetPlan(ctx, id)
}

func (r *GormRepository) DeletePlan(ctx context.Context, id, creatorID int64) error {
	result := r.db.WithContext(ctx).Where("id = ? AND creator_id = ?", id, creatorID).Delete(&Plan{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPlanNotFound
	}
	return nil
}

func (r *GormRepository) TouchPlanRun(ctx context.Context, id int64, lastRun, nextRun *gorm.DeletedAt) error {
	updates := map[string]interface{}{"updated_at": gorm.Expr("NOW()")}
	if lastRun != nil {
		updates["last_run_at"] = lastRun
	}
	if nextRun != nil {
		updates["next_run_at"] = nextRun
	}
	return r.db.WithContext(ctx).Model(&Plan{}).Where("id = ?", id).Updates(updates).Error
}

// --- Tasks ---

func (r *GormRepository) CreateTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *GormRepository) GetTask(ctx context.Context, id int64) (Task, error) {
	var task Task
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, err
	}
	return task, nil
}

func (r *GormRepository) ListTasks(ctx context.Context, filter TaskListFilter) (ListResponse[Task], error) {
	var tasks []Task
	var total int64
	q := r.db.WithContext(ctx).Model(&Task{})
	if filter.PlanID != nil {
		q = q.Where("plan_id = ?", *filter.PlanID)
	}
	if filter.TriggeredBy != nil {
		q = q.Where("triggered_by = ?", *filter.TriggeredBy)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if err := q.Count(&total).Error; err != nil {
		return ListResponse[Task]{}, err
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if err := q.Order("id DESC").Find(&tasks).Error; err != nil {
		return ListResponse[Task]{}, err
	}
	return ListResponse[Task]{Items: tasks, Total: int(total)}, nil
}

func (r *GormRepository) UpdateTaskStatus(ctx context.Context, id int64, patch PatchTaskInput) error {
	updates := map[string]interface{}{}
	if patch.Status != nil {
		updates["status"] = *patch.Status
	}
	if patch.StartedAt != nil {
		updates["started_at"] = patch.StartedAt
	}
	if patch.FinishedAt != nil {
		updates["finished_at"] = patch.FinishedAt
	}
	if patch.CompletedClusters != nil {
		updates["completed_clusters"] = *patch.CompletedClusters
	}
	if patch.FindingCount != nil {
		updates["finding_count"] = *patch.FindingCount
	}
	if patch.ErrorSummary != nil {
		updates["error_summary"] = *patch.ErrorSummary
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(updates).Error
}

// --- Results ---

func (r *GormRepository) CreateResults(ctx context.Context, results []Result) error {
	if len(results) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&results).Error
}

func (r *GormRepository) ListResults(ctx context.Context, filter ListFilter) (ListResponse[Result], error) {
	var results []Result
	var total int64
	q := r.db.WithContext(ctx).Model(&Result{})
	if filter.ClusterID != nil {
		q = q.Where("cluster_id = ?", *filter.ClusterID)
	}
	if filter.RuleCode != "" {
		q = q.Where("rule_code = ?", filter.RuleCode)
	}
	if filter.SignalCode != "" {
		q = q.Where("signal_code = ?", filter.SignalCode)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	if filter.State != "" {
		q = q.Where("state = ?", filter.State)
	}
	if filter.TaskID != nil {
		q = q.Where("task_id = ?", *filter.TaskID)
	}
	if err := q.Count(&total).Error; err != nil {
		return ListResponse[Result]{}, err
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if err := q.Order("id DESC").Find(&results).Error; err != nil {
		return ListResponse[Result]{}, err
	}
	return ListResponse[Result]{Items: results, Total: int(total)}, nil
}

func (r *GormRepository) GetResult(ctx context.Context, id int64) (Result, error) {
	var result Result
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Result{}, ErrResultNotFound
		}
		return Result{}, err
	}
	return result, nil
}

// Coverage aggregates plan → findings over the trailing windowDays calendar
// days (measured from now). It queries the three inspection tables directly:
// plans (total/enabled), tasks (status + trigger reason + per-day counts) and
// results (findings total, distinct rule codes, severity rollup, per-day
// counts). The result always carries scope/observed_at; FailClosed is true
// when the window has no findings data at all.
func (r *GormRepository) Coverage(ctx context.Context, windowDays int, now time.Time) (CoverageSummary, error) {
	if windowDays < 1 {
		windowDays = 1
	}
	since := now.UTC().AddDate(0, 0, -windowDays)
	summary := CoverageSummary{
		Scope:      "inspection:plans+tasks+results:" + intToStr(int64(windowDays)) + "d",
		ObservedAt: now.UTC().Format(time.RFC3339),
		WindowDays: windowDays,
		BySeverity: map[string]int{},
	}

	// Plans
	var planCount int64
	if err := r.db.WithContext(ctx).Model(&Plan{}).Count(&planCount).Error; err != nil {
		return summary, err
	}
	summary.PlanTotal = int(planCount)
	planCount = 0
	if err := r.db.WithContext(ctx).Model(&Plan{}).Where("enabled = true").Count(&planCount).Error; err != nil {
		return summary, err
	}
	summary.PlanEnabled = int(planCount)

	// Tasks: totals + trigger breakdown + per-day trend.
	q := r.db.WithContext(ctx).Model(&Task{}).Where("created_at >= ?", since)
	var taskTotal int64
	if err := q.Count(&taskTotal).Error; err != nil {
		return summary, err
	}
	summary.TaskTotal = int(taskTotal)

	var statusCounts []struct {
		Status string
		Count  int64
	}
	if err := r.db.WithContext(ctx).Model(&Task{}).
		Select("status, COUNT(*) AS count").
		Where("created_at >= ?", since).
		Group("status").Scan(&statusCounts).Error; err != nil {
		return summary, err
	}
	for _, sc := range statusCounts {
		switch sc.Status {
		case TaskStatusCompleted:
			summary.TaskCompleted = int(sc.Count)
		case TaskStatusFailed:
			summary.TaskFailed = int(sc.Count)
		}
	}

	var triggerCounts []struct {
		TriggerReason string
		Count         int64
	}
	if err := r.db.WithContext(ctx).Model(&Task{}).
		Select("trigger_reason, COUNT(*) AS count").
		Where("created_at >= ?", since).
		Group("trigger_reason").Scan(&triggerCounts).Error; err != nil {
		return summary, err
	}
	for _, tc := range triggerCounts {
		switch tc.TriggerReason {
		case "schedule":
			summary.TaskScheduled = int(tc.Count)
		case "manual":
			summary.TaskManual = int(tc.Count)
		}
	}

	// Findings: total, distinct rules, severity, per-day trend.
	type dayCount struct {
		Day      string
		Findings int64
	}
	var findingsByDay []dayCount
	if err := r.db.WithContext(ctx).Model(&Result{}).
		Select("TO_CHAR(observed_at, 'YYYY-MM-DD') AS day, COUNT(*) AS findings").
		Where("observed_at >= ?", since).
		Group("TO_CHAR(observed_at, 'YYYY-MM-DD')").
		Order("day ASC").Scan(&findingsByDay).Error; err != nil {
		return summary, err
	}
	dayFindings := make(map[string]int, len(findingsByDay))
	for _, row := range findingsByDay {
		dayFindings[row.Day] = int(row.Findings)
	}

	var taskDayCounts []dayCount
	if err := r.db.WithContext(ctx).Model(&Task{}).
		Select("TO_CHAR(created_at, 'YYYY-MM-DD') AS day, COUNT(*) AS findings").
		Where("created_at >= ?", since).
		Group("TO_CHAR(created_at, 'YYYY-MM-DD')").
		Order("day ASC").Scan(&taskDayCounts).Error; err != nil {
		return summary, err
	}
	dayTasks := make(map[string]int, len(taskDayCounts))
	for _, row := range taskDayCounts {
		dayTasks[row.Day] = int(row.Findings)
	}

	// Merge the two day maps into an ordered trend over the window.
	allDays := make(map[string]struct{})
	for d := range dayFindings {
		allDays[d] = struct{}{}
	}
	for d := range dayTasks {
		allDays[d] = struct{}{}
	}
	days := make([]string, 0, len(allDays))
	for d := range allDays {
		days = append(days, d)
	}
	sortStrings(days)
	for _, d := range days {
		summary.Trend = append(summary.Trend, CoverageTrendPoint{Day: d, Tasks: dayTasks[d], Findings: dayFindings[d]})
	}

	// Finding totals / distinct rules / severity.
	var findingTotal int64
	if err := r.db.WithContext(ctx).Model(&Result{}).Where("observed_at >= ?", since).Count(&findingTotal).Error; err != nil {
		return summary, err
	}
	summary.FindingTotal = int(findingTotal)

	var ruleTotal int64
	if err := r.db.WithContext(ctx).Model(&Result{}).
		Where("observed_at >= ?", since).
		Distinct("rule_code").Count(&ruleTotal).Error; err != nil {
		return summary, err
	}
	summary.DistinctRules = int(ruleTotal)

	var sevCounts []struct {
		Severity string
		Count    int64
	}
	if err := r.db.WithContext(ctx).Model(&Result{}).
		Select("severity, COUNT(*) AS count").
		Where("observed_at >= ?", since).
		Group("severity").Scan(&sevCounts).Error; err != nil {
		return summary, err
	}
	for _, sc := range sevCounts {
		summary.BySeverity[sc.Severity] = int(sc.Count)
	}

	// Fail-closed: no findings in the window is not a healthy state.
	if summary.FindingTotal == 0 {
		summary.FailClosed = true
		summary.EmptyNote = "window contains no inspection findings (fail-closed)"
	}
	// Rule coverage denominator: the total number of rules the platform
	// carried is resolved by the service (catalog). The repository reports
	// distinct rules with findings; the service computes the ratio.
	return summary, nil
}

func intToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

func sortStrings(values []string) {
	sort.Strings(values)
}
