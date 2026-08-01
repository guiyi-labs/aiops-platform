package aiinvestigator

import (
	"strings"
	"testing"
)

func TestLookupRunbook(t *testing.T) {
	t.Run("known runbook", func(t *testing.T) {
		r, ok := LookupRunbook("rollback_last_rollout")
		if !ok {
			t.Fatalf("expected rollback_last_rollout to be in catalog")
		}
		if r.RunbookID != "rollback_last_rollout" {
			t.Errorf("RunbookID = %q, want rollback_last_rollout", r.RunbookID)
		}
		if r.ActionCode != "deployment.rollback" {
			t.Errorf("ActionCode = %q, want deployment.rollback", r.ActionCode)
		}
		if r.Title == "" {
			t.Errorf("Title should not be empty")
		}
		if len(r.Steps) == 0 {
			t.Errorf("Steps should not be empty")
		}
	})
	t.Run("unknown runbook fails closed", func(t *testing.T) {
		_, ok := LookupRunbook("definitely_not_a_runbook")
		if ok {
			t.Fatalf("unknown runbook should not be found (fail closed)")
		}
	})
	t.Run("empty id fails closed", func(t *testing.T) {
		_, ok := LookupRunbook("")
		if ok {
			t.Fatalf("empty runbook id should not be found")
		}
	})
}

func TestAllRunbooks(t *testing.T) {
	all := AllRunbooks()
	if len(all) == 0 {
		t.Fatalf("catalog should not be empty")
	}
	// Every runbook in the catalog must be self-consistent.
	seen := make(map[string]bool, len(all))
	for _, r := range all {
		if r.RunbookID == "" {
			t.Errorf("runbook with empty RunbookID: %+v", r)
		}
		if seen[r.RunbookID] {
			t.Errorf("duplicate runbook id %q", r.RunbookID)
		}
		seen[r.RunbookID] = true
		// The descriptor's RunbookID must match its catalog key.
		if _, ok := LookupRunbook(r.RunbookID); !ok {
			t.Errorf("runbook %q not findable via LookupRunbook", r.RunbookID)
		}
	}
}

func TestEligibleRunbooks(t *testing.T) {
	t.Run("advisory runbooks always eligible", func(t *testing.T) {
		eligible := EligibleRunbooks(map[string]bool{})
		// Advisory-only runbooks (empty ActionCode) must appear even when no
		// action codes are eligible.
		var inspectPVC, inspectNode bool
		for _, r := range eligible {
			if r.RunbookID == "inspect_pvc_capacity" {
				inspectPVC = true
			}
			if r.RunbookID == "inspect_node_maintenance" {
				inspectNode = true
			}
		}
		if !inspectPVC {
			t.Errorf("advisory runbook inspect_pvc_capacity should be eligible with no action codes")
		}
		if !inspectNode {
			t.Errorf("advisory runbook inspect_node_maintenance should be eligible with no action codes")
		}
	})
	t.Run("action runbook gated by eligible codes", func(t *testing.T) {
		eligible := EligibleRunbooks(map[string]bool{"deployment.rollback": true})
		var rollback, restart bool
		for _, r := range eligible {
			if r.RunbookID == "rollback_last_rollout" {
				rollback = true
			}
			if r.RunbookID == "rollout_restart_pods" {
				restart = true
			}
		}
		if !rollback {
			t.Errorf("rollback_last_rollout should be eligible when deployment.rollback is in the set")
		}
		if restart {
			t.Errorf("rollout_restart_pods should NOT be eligible when only deployment.rollback is in the set")
		}
	})
	t.Run("all action runbooks eligible when both codes present", func(t *testing.T) {
		eligible := EligibleRunbooks(map[string]bool{
			"deployment.rollback":        true,
			"deployment.rollout_restart": true,
		})
		var rollback, restart bool
		for _, r := range eligible {
			if r.RunbookID == "rollback_last_rollout" {
				rollback = true
			}
			if r.RunbookID == "rollout_restart_pods" {
				restart = true
			}
		}
		if !rollback || !restart {
			t.Errorf("both action runbooks should be eligible when both codes present")
		}
	})
}

func TestValidateRunbookEligibility(t *testing.T) {
	t.Run("advisory runbook always eligible", func(t *testing.T) {
		if err := ValidateRunbookEligibility("inspect_pvc_capacity", map[string]bool{}); err != nil {
			t.Errorf("advisory runbook should be eligible: %v", err)
		}
	})
	t.Run("action runbook eligible when code present", func(t *testing.T) {
		if err := ValidateRunbookEligibility("rollback_last_rollout", map[string]bool{"deployment.rollback": true}); err != nil {
			t.Errorf("expected eligible: %v", err)
		}
	})
	t.Run("action runbook rejected when code absent", func(t *testing.T) {
		err := ValidateRunbookEligibility("rollback_last_rollout", map[string]bool{})
		if err == nil {
			t.Fatalf("expected eligibility error")
		}
		if !strings.Contains(err.Error(), "not eligible") {
			t.Errorf("error should mention not eligible, got: %v", err)
		}
	})
	t.Run("unknown runbook rejected", func(t *testing.T) {
		err := ValidateRunbookEligibility("nope", map[string]bool{"deployment.rollback": true})
		if err == nil {
			t.Fatalf("expected not-in-catalog error")
		}
		if !strings.Contains(err.Error(), "not in catalog") {
			t.Errorf("error should mention not in catalog, got: %v", err)
		}
	})
	t.Run("empty runbook id rejected", func(t *testing.T) {
		if err := ValidateRunbookEligibility("", map[string]bool{}); err == nil {
			t.Fatalf("empty runbook id should be rejected")
		}
	})
}
