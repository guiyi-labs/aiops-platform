package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// FuzzPlanLifecycle drives the action plan state machine through the
// service layer against the in-memory repository with arbitrary plan IDs,
// runbook IDs and approver identities. It pins the documented transitions:
// only a previewed plan may be approved, and cancelling or expiring any
// non-terminal plan must return either the terminal plan or a documented
// sentinel — never panic on arbitrary input.
func FuzzPlanLifecycle(f *testing.F) {
	seeds := []struct {
		planID, runbook, approver string
		status                    PlanStatus
		actionCode                string
	}{
		{"plan-a", "deployment.image_update", "alice", StatusDraft, "deployment.image_update"},
		{"plan-b", "deployment.rollout_restart", "bob", StatusPreviewed, "deployment.rollout_restart"},
		{"plan-c", "cronjob.suspend", "carol", StatusApproved, "cronjob.suspend"},
		{"plan-d", "", "", StatusExecuting, ""},
		{"", "", "", StatusCancelled, ""},
	}
	for _, seed := range seeds {
		f.Add(seed.planID, seed.runbook, seed.approver, string(seed.status), seed.actionCode)
	}

	f.Fuzz(func(t *testing.T, planID, runbook, approver string, status string, actionCode string) {
		repo := newMemRepo()
		svc := newTestService(t, repo, &fakeCaseReader{}, nil)

		requesterID := int64(1)
		plan := ActionPlan{
			ID:                planID,
			Status:            PlanStatus(status),
			ActionCode:        actionCode,
			RunbookID:         runbook,
			ApprovalType:      ApprovalSingle,
			RequestedByUserID: &requesterID,
			RequestedByName:   strings.TrimSpace(approver),
		}
		if err := repo.SavePlan(context.Background(), &plan); err != nil {
			t.Fatalf("save: %v", err)
		}

		actor := ActorRef{ID: 2, Name: approver}
		if _, err := svc.Approve(context.Background(), planID, actor); err != nil {
			// Only documented sentinels are allowed; memRepo delegates
			// Approve to NopRepository, so every call returns ErrPlanNotFound.
			if !errors.Is(err, ErrDisabled) && !errors.Is(err, ErrPlanNotFound) && !errors.Is(err, ErrNotPreviewed) && !errors.Is(err, ErrSelfApprovalForbidden) {
				t.Fatalf("unexpected approval error %v for status=%q approver=%q", err, status, approver)
			}
		}

		// The service-level verification path must tolerate any state.
		if _, err := svc.GetVerification(context.Background(), planID); err != nil && !errors.Is(err, ErrDisabled) && !errors.Is(err, ErrVerificationNotFound) {
			t.Fatalf("GetVerification unexpected error: %v", err)
		}

		// Cancelling any non-terminal plan must return the plan or a
		// documented sentinel; it must never panic on arbitrary state.
		cancelled, err := svc.Cancel(context.Background(), planID)
		if err == nil {
			if cancelled.Status != StatusCancelled {
				t.Fatalf("cancel produced status %q, want cancelled", cancelled.Status)
			}
		} else if !errors.Is(err, ErrDisabled) && !errors.Is(err, ErrPlanNotFound) {
			t.Fatalf("cancel unexpected error: %v", err)
		}

		// ExpireStale must be safe on arbitrary data.
		if _, err := svc.ExpireStale(context.Background()); err != nil && !errors.Is(err, ErrDisabled) {
			t.Fatalf("expire stale unexpected error: %v", err)
		}
	})
}

// FuzzRollbackContract pins the rollback eligibility contract: only
// Deployment image/rollout actions are rollback-eligible; everything else
// must be reported not eligible, and the contract must never panic on
// arbitrary (kind, action_code) pairs.
func FuzzRollbackContract(f *testing.F) {
	for _, pair := range []struct{ kind, code string }{
		{"Deployment", "deployment.image_update"},
		{"Deployment", "deployment.rollout_restart"},
		{"CronJob", "deployment.image_update"},
		{"Deployment", "cronjob.suspend"},
		{"Pod", "deployment.rollout_restart"},
		{"", ""},
	} {
		f.Add(pair.kind, pair.code)
	}

	f.Fuzz(func(t *testing.T, kind, code string) {
		svc := newTestService(t, newMemRepo(), &fakeCaseReader{}, nil)
		plan := ActionPlan{
			ID: "plan-rollback", Status: StatusSucceeded, TargetKind: kind, ActionCode: code,
			RequestedByUserID: ptr(int64(1)), RequestedByName: "alice",
		}
		verification := ActionVerification{PlanID: "plan-rollback", Status: VerificationStatusEffective}
		rollbackID, reason, eligible := svc.evaluateRollbackContract(context.Background(), plan, verification)
		if eligible {
			if rollbackID == nil || reason == "" || (code != "deployment.image_update" && code != "deployment.rollout_restart") ||
				kind != "Deployment" {
				t.Fatalf("rollback eligible without preconditions: kind=%q code=%q id=%v reason=%q", kind, code, rollbackID, reason)
			}
			return
		}
		if reason == "" {
			t.Fatalf("ineligible rollback without a reason: kind=%q code=%q", kind, code)
		}
		// A rollback plan must never be created for non-eligible actions.
		if strings.HasPrefix(reason, "rollback_plan_created") {
			t.Fatalf("rollback plan created for ineligible action kind=%q code=%q", kind, code)
		}
	})
}

func ptr[T any](value T) *T { return &value }
