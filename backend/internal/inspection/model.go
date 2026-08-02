package inspection

import (
	"database/sql/driver"
	"errors"
	"time"

	"github.com/lib/pq"
)

// Constants for M52 inspection.
const (
	MaxRuleCodeLen    = 128
	MaxPlanNameLen    = 128
	MaxReasonLen      = 500
	MaxFingerprintLen = 64

	RuleDomainNode     = "node"
	RuleDomainWorkload = "workload"
	RuleDomainStorage  = "storage"
	RuleDomainNetwork  = "network"

	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"

	StateActive   = "active"
	StateResolved = "resolved"

	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusPartial   = "partial"
	TaskStatusFailed    = "failed"

	TriggerManual   = "manual"
	TriggerSchedule = "schedule"

	DefaultMaxTaskResults        = 1000
	DefaultPerClusterTimeout     = 15 * time.Second
	DefaultMaxConcurrentClusters = 4
)

var (
	ErrRuleNotFound        = errors.New("inspection rule not found")
	ErrPlanNotFound        = errors.New("inspection plan not found")
	ErrTaskNotFound        = errors.New("inspection task not found")
	ErrResultNotFound      = errors.New("inspection result not found")
	ErrInvalidRuleCode     = errors.New("invalid inspection rule code")
	ErrInvalidPlan         = errors.New("invalid inspection plan")
	ErrClusterUnreachable  = errors.New("cluster unreachable during inspection")
	ErrRuleExecutionFailed = errors.New("inspection rule execution failed")
)

// Int64Array is a []int64 stored as Postgres bigint[].
type Int64Array []int64

func (a Int64Array) Value() (driver.Value, error) {
	return pq.Int64Array(a).Value()
}

func (a *Int64Array) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}
	return (*pq.Int64Array)(a).Scan(value)
}

// StringArray is a []string stored as Postgres varchar[].
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	return pq.StringArray(a).Value()
}

func (a *StringArray) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}
	return (*pq.StringArray)(a).Scan(value)
}

// RuleDescriptor is the compile-time catalog entry for an inspection rule.
// Unregistered rule codes fail closed at runtime (RunInspectOnce returns
// ErrRuleNotFound); the catalog is append-only across versions.
type RuleDescriptor struct {
	Code            string        `json:"code"`
	SchemaVersion   string        `json:"schema_version"` // currently "1.0"
	Domain          string        `json:"domain"`         // node | workload | storage | network
	DefaultSeverity string        `json:"default_severity"`
	SignalCode      string        `json:"signal_code"` // M39 normalized signal code
	Description     string        `json:"description"`
	Remediation     string        `json:"remediation"`
	Timeout         time.Duration `json:"timeout"`
}

// RuleOverride represents a per-cluster runtime override for a rule. Absence
// of a row in inspection_rules means "use the compiled-in defaults".
type RuleOverride struct {
	ID               int64
	ClusterID        int64
	RuleCode         string
	Enabled          bool
	SeverityOverride string // empty = use catalog DefaultSeverity
	UpdatedBy        *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Plan is a scheduled or on-demand inspection plan.
type Plan struct {
	ID         int64
	Name       string
	CreatorID  int64
	ClusterIDs Int64Array  `gorm:"type:bigint[]"`      // empty = all reachable clusters
	RuleCodes  StringArray `gorm:"type:varchar(128)[]"` // empty = all enabled rules
	CronSpec   string   // empty = manual only
	Enabled    bool
	LastRunAt  *time.Time
	NextRunAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// PlanView is the public projection for a Plan.
type PlanView struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatorID  int64      `json:"creator_id"`
	ClusterIDs []int64    `json:"cluster_ids"`
	RuleCodes  []string   `json:"rule_codes"`
	CronSpec   string     `json:"cron_spec"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	NextRunAt  *time.Time `json:"next_run_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Task is one execution of a plan or an ad-hoc inspection run.
type Task struct {
	ID                int64
	PlanID            *int64
	PlanNameSnapshot  string
	TriggeredBy       *int64
	TriggerReason     string // manual | schedule
	ClusterIDs        Int64Array  `gorm:"type:bigint[]"`
	RuleCodes         StringArray `gorm:"type:varchar(128)[]"`
	Status            string
	StartedAt         *time.Time
	FinishedAt        *time.Time
	TotalClusters     int
	CompletedClusters int
	FindingCount      int
	ErrorSummary      string
	CreatedAt         time.Time
}

// TaskView is the public projection for a Task.
type TaskView struct {
	ID                int64      `json:"id"`
	PlanID            *int64     `json:"plan_id,omitempty"`
	PlanNameSnapshot  string     `json:"plan_name_snapshot"`
	TriggeredBy       *int64     `json:"triggered_by,omitempty"`
	TriggerReason     string     `json:"trigger_reason"`
	ClusterIDs        []int64    `json:"cluster_ids"`
	RuleCodes         []string   `json:"rule_codes"`
	Status            string     `json:"status"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	TotalClusters     int        `json:"total_clusters"`
	CompletedClusters int        `json:"completed_clusters"`
	FindingCount      int        `json:"finding_count"`
	ErrorSummary      string     `json:"error_summary"`
	CreatedAt         time.Time  `json:"created_at"`
}

// Result is one normalized inspection finding (maps 1:1 to M39 signal).
type Result struct {
	ID               int64
	TaskID           int64
	ClusterID        int64
	RuleCode         string
	SignalCode       string
	Severity         string
	State            string
	Namespace        string
	ResourceKind     string
	ResourceName     string
	ResourceUID      string
	Fingerprint      string
	EvidenceSnapshot string
	ObservedAt       time.Time
	CreatedAt        time.Time
}

// ResultView is the public projection for a Result. Includes parsed JSON
// evidence_snapshot (callers do not see the raw TEXT DB column).
type ResultView struct {
	ID           int64                  `json:"id"`
	TaskID       int64                  `json:"task_id"`
	ClusterID    int64                  `json:"cluster_id"`
	RuleCode     string                 `json:"rule_code"`
	SignalCode   string                 `json:"signal_code"`
	Severity     string                 `json:"severity"`
	State        string                 `json:"state"`
	Namespace    string                 `json:"namespace,omitempty"`
	ResourceKind string                 `json:"resource_kind,omitempty"`
	ResourceName string                 `json:"resource_name,omitempty"`
	ResourceUID  string                 `json:"resource_uid,omitempty"`
	Fingerprint  string                 `json:"fingerprint"`
	Evidence     map[string]interface{} `json:"evidence,omitempty"`
	ObservedAt   time.Time              `json:"observed_at"`
}

// ListFilter bounds inspection queries.
type ListFilter struct {
	ClusterID  *int64
	RuleCode   string
	SignalCode string
	Severity   string
	State      string
	TaskID     *int64
	Limit      int
	Offset     int
}

// ListResponse is the generic paginated list response.
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Finding is the in-memory rule output before DB persistence. Each rule
// execution produces 0..N Findings; the service de-duplicates by fingerprint
// within the same task.
type Finding struct {
	RuleCode     string
	SignalCode   string
	Severity     string
	State        string
	Namespace    string
	ResourceKind string
	ResourceName string
	ResourceUID  string
	Evidence     map[string]interface{}
	ObservedAt   time.Time
}

// TableName methods (GORM convention).
func (RuleOverride) TableName() string { return "inspection_rules" }
func (Plan) TableName() string         { return "inspection_plans" }
func (Task) TableName() string         { return "inspection_tasks" }
func (Result) TableName() string       { return "inspection_results" }
