package main

import (
	"context"
	"errors"
	"testing"

	"k8s-aiops.local/backend/internal/automation"
	"k8s-aiops.local/backend/internal/correlation"
)

// fakeCaseSource 是 correlationCaseSource 的测试替身。
type fakeCaseSource struct {
	view correlation.CaseView
	err  error
	acts correlation.ActionCandidateListResponse
}

func (f fakeCaseSource) GetCase(context.Context, int64) (correlation.CaseView, error) {
	if f.err != nil {
		return correlation.CaseView{}, f.err
	}
	return f.view, nil
}

func (f fakeCaseSource) ListActionCandidates(context.Context, int64) (correlation.ActionCandidateListResponse, error) {
	if f.err != nil {
		return correlation.ActionCandidateListResponse{}, f.err
	}
	return f.acts, nil
}

func sampleCaseView() correlation.CaseView {
	return correlation.CaseView{
		Case: correlation.Case{
			ID:                   7,
			ClusterID:            3,
			RuleID:               "correlation.rollout_causes_pod_failure.v1",
			PrimaryResource:      correlation.ResourceCitation{Kind: "Pod", Namespace: "krm", Name: "krm-backend-abc", UID: "uid-1"},
			Confidence:           correlation.ConfidenceCandidate,
			EvidenceCompleteness: correlation.CompletenessComplete,
			Factors: []correlation.Factor{
				{Kind: "same_uid", Value: "match", Weight: 0.4},
				{Kind: "topology_distance", Value: "2", Weight: 0.2},
			},
		},
		SignalLinks: []correlation.SignalLink{
			{SignalOccurrenceID: 11, Relation: correlation.SignalRelationTrigger, SignalID: "pod.crash_loop_backoff.v1", Producer: "diagnosis"},
		},
		ResourceLinks: []correlation.ResourceLink{
			{Resource: correlation.ResourceCitation{Kind: "Deployment", Namespace: "krm", Name: "krm-backend", UID: "uid-2"}, Relation: correlation.ResourceRelationUpstream},
		},
		ChangeCandidates: []correlation.ChangeCandidate{
			{ChangeEventID: 5, RuleID: "change.rollout.v1", Confidence: correlation.ConfidenceCandidate, Rank: 1, ReasonCode: "rollout_precedes_symptom"},
		},
	}
}

// automation 只关心动作目标：主资源身份必须完整映射，否则预览会打到错的对象上。
func TestAutomationCaseReaderMapsPrimaryResource(t *testing.T) {
	reader := automationCaseReader{cases: fakeCaseSource{view: sampleCaseView()}}

	ctx, err := reader.GetCase(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetCase: %v", err)
	}
	if ctx.CaseID != 7 || ctx.ClusterID != 3 {
		t.Fatalf("case identity mismatch: %+v", ctx)
	}
	if ctx.PrimaryKind != "Pod" || ctx.PrimaryNamespace != "krm" || ctx.PrimaryName != "krm-backend-abc" || ctx.PrimaryUID != "uid-1" {
		t.Fatalf("primary resource mismatch: %+v", ctx)
	}
	// 未确认的用例不得带出「根因候选」——候选不是结论。
	if ctx.ActionCandidateID != nil {
		t.Fatalf("expected no action candidate for unconfirmed case, got %v", *ctx.ActionCandidateID)
	}
}

func TestAutomationCaseReaderMapsNotFound(t *testing.T) {
	reader := automationCaseReader{cases: fakeCaseSource{err: correlation.ErrCaseNotFound}}

	_, err := reader.GetCase(context.Background(), 99)
	if !errors.Is(err, automation.ErrCaseNotFound) {
		t.Fatalf("expected automation.ErrCaseNotFound, got %v", err)
	}

	_, err = reader.EligibleActionCodes(context.Background(), 99)
	if !errors.Is(err, automation.ErrCaseNotFound) {
		t.Fatalf("expected automation.ErrCaseNotFound from EligibleActionCodes, got %v", err)
	}
}

// 只有 eligible 的候选才算「可执行」，blocked 的不能放行。
func TestAutomationCaseReaderFiltersEligibleCodes(t *testing.T) {
	reader := automationCaseReader{cases: fakeCaseSource{acts: correlation.ActionCandidateListResponse{
		Items: []correlation.ActionCandidate{
			{Code: "deployment.rollback", Eligible: true},
			{Code: "deployment.scale", Eligible: false, BlockedReasons: []string{"pdb_min_available"}},
		},
	}}}

	codes, err := reader.EligibleActionCodes(context.Background(), 7)
	if err != nil {
		t.Fatalf("EligibleActionCodes: %v", err)
	}
	if !codes["deployment.rollback"] {
		t.Fatal("expected deployment.rollback to be eligible")
	}
	if codes["deployment.scale"] {
		t.Fatal("expected deployment.scale to be filtered out")
	}
}

// investigator 需要完整上下文：因子、信号链、资源链、变更候选缺一项都会削弱引用校验。
func TestInvestigatorCaseReaderMapsFullContext(t *testing.T) {
	reader := investigatorCaseReader{cases: fakeCaseSource{view: sampleCaseView()}}

	ctx, err := reader.GetCase(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetCase: %v", err)
	}
	if ctx.RuleID == "" || ctx.Confidence == "" || ctx.EvidenceCompleteness == "" {
		t.Fatalf("missing scalar context: %+v", ctx)
	}
	if len(ctx.Factors) != 2 {
		t.Fatalf("expected 2 factors, got %d", len(ctx.Factors))
	}
	if len(ctx.SignalLinks) != 1 || ctx.SignalLinks[0].SignalID == "" || ctx.SignalLinks[0].ObservedAt == "" {
		t.Fatalf("signal link not mapped: %+v", ctx.SignalLinks)
	}
	if len(ctx.ResourceLinks) != 1 || ctx.ResourceLinks[0].Kind != "Deployment" {
		t.Fatalf("resource link not mapped: %+v", ctx.ResourceLinks)
	}
	if len(ctx.ChangeCandidates) != 1 || ctx.ChangeCandidates[0].ChangeEventID != 5 {
		t.Fatalf("change candidate not mapped: %+v", ctx.ChangeCandidates)
	}
}
