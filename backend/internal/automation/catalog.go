package automation

// Package-level catalog of executable runbooks. This mirrors the M43
// aiinvestigator runbook catalog but only includes runbooks that have
// a non-empty ActionCode (advisory-only runbooks cannot be materialized
// into action plans).
//
// Adding a runbook is a contract change (AutomationVersion bump). The
// M43 and M44 catalogs must agree on (runbook_id → action_code): the
// AI investigator must not recommend a runbook that the automation
// service cannot execute, and vice versa. The M44 ADR records this
// invariant.

// RunbookDescriptor describes one executable runbook. The catalog is the
// single source of truth for which runbooks the automation service may
// materialize into action plans.
type RunbookDescriptor struct {
	RunbookID  string // fixed, server-owned ID (e.g. "rollback_last_rollout")
	ActionCode string // M19 controlled-operations code (e.g. "deployment.rollback")
	Title      string
	Steps      []string // human-readable steps (for the operator)
}

// catalog is the compiled executable-runbook catalog. Lookups via
// LookupRunbook fail closed for unlisted runbooks.
var catalog = map[string]RunbookDescriptor{
	"rollback_last_rollout": {
		RunbookID:  "rollback_last_rollout",
		ActionCode: "deployment.rollback",
		Title:      "Roll back the last rollout",
		Steps:      []string{"Confirm the target Deployment", "Preview the rollback", "Approve (four-eyes)", "Execute with confirmation", "Verify post-action SLI"},
	},
	"rollout_restart_pods": {
		RunbookID:  "rollout_restart_pods",
		ActionCode: "deployment.rollout_restart",
		Title:      "Restart the Deployment rollout",
		Steps:      []string{"Confirm the target Deployment", "Preview the rollout restart", "Approve", "Execute with confirmation", "Verify post-action SLI"},
	},
}

// LookupRunbook returns the runbook descriptor for the given ID. Returns
// ok=false for unlisted runbooks. The catalog fails closed: the service
// never accepts a runbook ID outside this catalog.
func LookupRunbook(id string) (RunbookDescriptor, bool) {
	r, ok := catalog[id]
	return r, ok
}

// AllRunbooks returns the full executable-runbook catalog.
func AllRunbooks() []RunbookDescriptor {
	out := make([]RunbookDescriptor, 0, len(catalog))
	for _, r := range catalog {
		out = append(out, r)
	}
	return out
}
