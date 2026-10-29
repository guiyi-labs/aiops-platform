package automation

import "sort"

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
	RunbookID  string   `json:"runbook_id"`  // fixed, server-owned ID (e.g. "rollback_last_rollout")
	ActionCode string   `json:"action_code"` // M19 controlled-operations code (e.g. "deployment.rollback")
	Title      string   `json:"title"`
	Steps      []string `json:"steps"` // human-readable steps (for the operator)
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
// runbookOrder 固定目录的展示顺序（理由同 aiinvestigator/catalog.go：
// 直接遍历 map 会让每次请求返回的次序随机，控制台卡片刷新一次就换位置）。
// automation 只收可执行 runbook，故这里只有两条。
var runbookOrder = []string{
	"rollback_last_rollout",
	"rollout_restart_pods",
}

func AllRunbooks() []RunbookDescriptor {
	out := make([]RunbookDescriptor, 0, len(catalog))
	seen := make(map[string]struct{}, len(catalog))
	for _, id := range runbookOrder {
		if r, ok := catalog[id]; ok {
			out = append(out, r)
			seen[id] = struct{}{}
		}
	}
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
