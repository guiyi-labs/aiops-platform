package diagnosis

// ActionKind classifies a diagnosis action in the M94 action area.
type ActionKind string

const (
	// ActionKindAdvisory is a read-only recommendation; no confirmation is needed.
	ActionKindAdvisory ActionKind = "advisory"
	// ActionKindControlled is an operation that requires a Kubernetes dry-run
	// and explicit confirmation before execution.
	ActionKindControlled ActionKind = "controlled_action"
)

// DiagnosisAction is one typed entry in the diagnosis action area. The area
// distinguishes read-only advice from controlled operations; authorization
// (roles) and dependency availability are runtime concerns rendered by the
// frontend, so the projection only describes capability and gating.
type DiagnosisAction struct {
	Kind                 ActionKind `json:"kind"`
	Title                string     `json:"title"`
	Detail               string     `json:"detail,omitempty"`
	Action               string     `json:"action,omitempty"`
	RequiresDryRun       bool       `json:"requires_dry_run"`
	RequiresConfirmation bool       `json:"requires_confirmation"`
}

// controlledActionCapabilities maps a diagnosed resource kind to the
// controlled operations currently supported by the remediation pipeline.
// Every capability requires a dry-run preview and explicit confirmation.
var controlledActionCapabilities = map[string]DiagnosisAction{
	"Pod": {
		Kind:                 ActionKindControlled,
		Title:                "Rollout restart 目标 Deployment",
		Detail:               "仅允许对匹配当前 Pod 的 Deployment 执行 rollout restart；先由 Kubernetes dry-run 验证，确认后才会创建新的 Pod。",
		Action:               "deployment.rollout_restart",
		RequiresDryRun:       true,
		RequiresConfirmation: true,
	},
}

// buildActionArea derives the typed action area from the persisted record.
// It is a pure projection: it only reads the record and never triggers work,
// reaches a cluster or depends on the caller's session.
func buildActionArea(record Record) []DiagnosisAction {
	actions := make([]DiagnosisAction, 0, len(record.Recommendations)+1)
	for _, recommendation := range record.Recommendations {
		actions = append(actions, DiagnosisAction{
			Kind:  ActionKindAdvisory,
			Title: recommendation,
		})
	}
	if capability, ok := controlledActionCapabilities[record.Resource.Kind]; ok {
		actions = append(actions, capability)
	}
	return actions
}
