package diagnosis

import "time"

const (
	RuleImagePullBackOff                 = "pod.image_pull_backoff.v1"
	RuleCrashLoopBackOff                 = "pod.crash_loop_backoff.v1"
	RulePodPending                       = "pod.pending.v1"
	RulePodOOMKilled                     = "pod.oom_killed.v1"
	RuleServiceNoEndpoints               = "service.no_ready_endpoints.v1"
	RuleNodeNotReady                     = "node.not_ready.v1"
	RuleDeploymentReplicasUnavailable    = "deployment.replicas_unavailable.v1"
	RuleNodePressure                     = "node.pressure.v1"
	RulePersistentVolumeClaimPending     = "persistentvolumeclaim.pending.v1"
	RuleHorizontalPodAutoscalerSaturated = "horizontalpodautoscaler.saturated.v1"
	RuleIngressBackendUnavailable        = "ingress.backend_unavailable.v1"
)

// RuleIDs returns the compiled-in deterministic diagnosis rule IDs in a
// stable order (used by the analyzer discovery contract).
func RuleIDs() []string {
	return []string{
		RuleImagePullBackOff,
		RuleCrashLoopBackOff,
		RulePodPending,
		RulePodOOMKilled,
		RuleServiceNoEndpoints,
		RuleNodeNotReady,
		RuleDeploymentReplicasUnavailable,
		RuleNodePressure,
		RulePersistentVolumeClaimPending,
		RuleHorizontalPodAutoscalerSaturated,
		RuleIngressBackendUnavailable,
	}
}

type ResourceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
}

type Evidence struct {
	Type    string         `json:"type"`
	Source  string         `json:"source"`
	Content map[string]any `json:"content"`
}

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Activity struct {
	ID         int64     `json:"id"`
	Actor      ActorRef  `json:"actor"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

type Feedback struct {
	ID        int64     `json:"id"`
	Actor     ActorRef  `json:"actor"`
	Verdict   string    `json:"verdict"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type Assignment struct {
	ID           int64     `json:"id"`
	Actor        ActorRef  `json:"actor"`
	FromAssignee *ActorRef `json:"from_assignee,omitempty"`
	ToAssignee   ActorRef  `json:"to_assignee"`
	Comment      string    `json:"comment"`
	CreatedAt    time.Time `json:"created_at"`
}

type Summary struct {
	Total     int64    `json:"total"`
	Open      int64    `json:"open"`
	Confirmed int64    `json:"confirmed"`
	Resolved  int64    `json:"resolved"`
	Dismissed int64    `json:"dismissed"`
	Overdue   int64    `json:"overdue"`
	Recent    []Record `json:"recent"`
}

type ListFilter struct {
	ClusterID int64
	Status    string
	Overdue   *bool
	// Since bounds the list to records observed at or after this time. The
	// correlation provider uses it to gather only recent diagnoses for one
	// correlation pass.
	Since *time.Time
	Limit int
}

type Record struct {
	ID              int64             `json:"id"`
	ClusterID       int64             `json:"cluster_id"`
	RuleID          string            `json:"rule_id"`
	Severity        string            `json:"severity"`
	Resource        ResourceRef       `json:"resource"`
	Status          string            `json:"status"`
	Summary         string            `json:"summary"`
	RootCauses      []string          `json:"root_causes"`
	Recommendations []string          `json:"recommendations"`
	Evidence        []Evidence        `json:"evidence"`
	Timeline        []TimelineEntry   `json:"timeline,omitempty"`
	RootCauseCard   *RootCauseCard    `json:"root_cause_card,omitempty"`
	Actions         []DiagnosisAction `json:"actions,omitempty"`
	Assignee        *ActorRef         `json:"assignee,omitempty"`
	Activities      []Activity        `json:"activities,omitempty"`
	Feedback        []Feedback        `json:"feedback,omitempty"`
	Assignments     []Assignment      `json:"assignments,omitempty"`
	ObservedAt      time.Time         `json:"observed_at"`
	SLADueAt        time.Time         `json:"sla_due_at"`
	ResolvedAt      *time.Time        `json:"resolved_at,omitempty"`
	Overdue         bool              `json:"overdue"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}
