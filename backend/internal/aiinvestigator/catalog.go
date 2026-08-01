package aiinvestigator

import "fmt"

// RunbookDescriptor describes one server-owned runbook. The catalog is the
// single source of truth for which runbooks exist, what action code they
// map to, and what eligibility constraints they carry. Adding a runbook is
// a contract change (InvestigatorVersion bump), not a runtime configuration.
//
// Runbook IDs are the only action recommendations the AI may emit. The
// investigator rechecks eligibility against the M42 Action Catalog at
// generation time — a runbook whose action code is not in the case's
// ActionCandidate list is rejected.
type RunbookDescriptor struct {
	RunbookID  string // fixed, server-owned ID (e.g. "rollback_last_rollout")
	ActionCode string // M19 controlled-operations code (e.g. "deployment.rollback")
	Title      string
	Steps      []string // human-readable steps (for the operator, not the AI)
}

// catalog is the compiled runbook catalog. Lookups via LookupRunbook fail
// closed for unlisted runbooks.
var catalog = map[string]RunbookDescriptor{
	"rollback_last_rollout": {
		RunbookID:  "rollback_last_rollout",
		ActionCode: "deployment.rollback",
		Title:      "Roll back the last rollout",
		Steps:      []string{"Confirm the target Deployment", "Preview the rollback", "Execute with confirmation", "Verify post-action SLI"},
	},
	"rollout_restart_pods": {
		RunbookID:  "rollout_restart_pods",
		ActionCode: "deployment.rollout_restart",
		Title:      "Restart the Deployment rollout",
		Steps:      []string{"Confirm the target Deployment", "Preview the rollout restart", "Execute with confirmation", "Verify post-action SLI"},
	},
	"inspect_pvc_capacity": {
		RunbookID:  "inspect_pvc_capacity",
		ActionCode: "", // advisory-only — no M19 action
		Title:      "Inspect PVC capacity and requests",
		Steps:      []string{"Check PVC capacity", "Check pod resource requests", "Expand PVC or reduce requests", "Verify pod scheduling"},
	},
	"inspect_node_maintenance": {
		RunbookID:  "inspect_node_maintenance",
		ActionCode: "", // advisory-only — node maintenance is operator-only
		Title:      "Inspect node maintenance window",
		Steps:      []string{"Check maintenance window", "Check node conditions", "Wait for maintenance to complete or escalate"},
	},
}

// LookupRunbook returns the runbook descriptor for the given ID. Returns
// ok=false for unlisted runbooks. The catalog fails closed: the investigator
// never accepts a runbook ID outside this catalog.
func LookupRunbook(id string) (RunbookDescriptor, bool) {
	r, ok := catalog[id]
	return r, ok
}

// AllRunbooks returns the full runbook catalog. Used by the HTTP route and
// the golden fixtures.
func AllRunbooks() []RunbookDescriptor {
	out := make([]RunbookDescriptor, 0, len(catalog))
	for _, r := range catalog {
		out = append(out, r)
	}
	return out
}

// EligibleRunbooks returns the runbooks whose ActionCode is in the given set
// of eligible action codes (from M42 ActionCandidate list). Advisory-only
// runbooks (empty ActionCode) are always eligible.
func EligibleRunbooks(eligibleActionCodes map[string]bool) []RunbookDescriptor {
	out := make([]RunbookDescriptor, 0, len(catalog))
	for _, r := range catalog {
		if r.ActionCode == "" || eligibleActionCodes[r.ActionCode] {
			out = append(out, r)
		}
	}
	return out
}

// ValidateRunbookEligibility returns true when the given runbook ID is in
// the catalog AND its action code (if any) is in the eligible set. Advisory
// runbooks are always eligible.
func ValidateRunbookEligibility(runbookID string, eligibleActionCodes map[string]bool) error {
	r, ok := LookupRunbook(runbookID)
	if !ok {
		return fmt.Errorf("runbook %q not in catalog", runbookID)
	}
	if r.ActionCode != "" && !eligibleActionCodes[r.ActionCode] {
		return fmt.Errorf("runbook %q action code %q not eligible for this case", runbookID, r.ActionCode)
	}
	return nil
}
