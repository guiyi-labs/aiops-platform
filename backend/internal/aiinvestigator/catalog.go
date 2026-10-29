package aiinvestigator

import (
	"fmt"
	"sort"
)

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
	RunbookID  string   `json:"runbook_id"`  // fixed, server-owned ID (e.g. "rollback_last_rollout")
	ActionCode string   `json:"action_code"` // M19 controlled-operations code (e.g. "deployment.rollback")
	Title      string   `json:"title"`
	Steps      []string `json:"steps"` // human-readable steps (for the operator, not the AI)
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

// runbookOrder 固定目录的展示顺序。
//
// 不能直接遍历 catalog map：Go 的 map 遍历顺序是随机的，同一份目录每次请求
// 返回的次序都不同——控制台上卡片刷新一次就换个位置，UI 基线截图因此永远
// 无法稳定（同一份代码连拍两张就能差 10%）。顺序在这里显式写出来，新增
// runbook 时也一并登记，审阅时能直接看出展示次序。
var runbookOrder = []string{
	"rollback_last_rollout",
	"rollout_restart_pods",
	"inspect_pvc_capacity",
	"inspect_node_maintenance",
}

// AllRunbooks returns the full runbook catalog in a stable order. Used by the
// HTTP route and the golden fixtures.
func AllRunbooks() []RunbookDescriptor {
	out := make([]RunbookDescriptor, 0, len(catalog))
	seen := make(map[string]struct{}, len(catalog))
	for _, id := range runbookOrder {
		if r, ok := catalog[id]; ok {
			out = append(out, r)
			seen[id] = struct{}{}
		}
	}
	// 兜底：登记了但未写进 runbookOrder 的条目按 ID 排序追加。
	// 宁可顺序不理想，也不能因为漏登记就静默丢失一条 runbook。
	rest := make([]string, 0, len(catalog)-len(out))
	for id := range catalog {
		if _, ok := seen[id]; !ok {
			rest = append(rest, id)
		}
	}
	if len(rest) > 0 {
		sort.Strings(rest)
		for _, id := range rest {
			out = append(out, catalog[id])
		}
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
