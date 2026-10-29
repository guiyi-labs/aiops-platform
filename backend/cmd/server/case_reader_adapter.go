package main

import (
	"context"
	"errors"
	"fmt"

	"k8s-aiops.local/backend/internal/aiinvestigator"
	"k8s-aiops.local/backend/internal/automation"
	"k8s-aiops.local/backend/internal/correlation"
)

// correlationCaseSource 是 correlation 用例的只读读取面。定义成本包接口而非直接依赖
// *correlation.Service，便于测试时替换。
type correlationCaseSource interface {
	GetCase(ctx context.Context, caseID int64) (correlation.CaseView, error)
	ListActionCandidates(ctx context.Context, caseID int64) (correlation.ActionCandidateListResponse, error)
}

// 编译期断言：两个适配器都满足各自服务的 CaseReader。
var (
	_ automation.CaseReader       = automationCaseReader{}
	_ aiinvestigator.CaseReader   = investigatorCaseReader{}
)

// automationCaseReader 把关联用例适配成受控自动化所需的 CaseContext。
//
// 为什么需要这一层：automation 与 correlation 属于同一分层里的兄弟包，互不依赖；
// 两边对「用例」的字段需求也不同（automation 只关心动作目标，investigator 需要
// 完整的因子/信号/变更上下文）。因此适配放在装配层（cmd/server），而不是让
// 两个业务包互相 import。
type automationCaseReader struct {
	cases correlationCaseSource
}

func (r automationCaseReader) GetCase(ctx context.Context, caseID int64) (automation.CaseContext, error) {
	view, err := r.cases.GetCase(ctx, caseID)
	if err != nil {
		if errors.Is(err, correlation.ErrCaseNotFound) {
			return automation.CaseContext{}, automation.ErrCaseNotFound
		}
		return automation.CaseContext{}, fmt.Errorf("load correlation case %d: %w", caseID, err)
	}
	primary := view.Case.PrimaryResource
	out := automation.CaseContext{
		CaseID:           view.Case.ID,
		ClusterID:        view.Case.ClusterID,
		PrimaryKind:      primary.Kind,
		PrimaryNamespace: primary.Namespace,
		PrimaryName:      primary.Name,
		PrimaryUID:       primary.UID,
	}
	// 关联用例提升为「已确认」时才有根因变更候选；未确认时留空，
	// 由人工在预览阶段选择目标——不要把候选当结论。
	if view.Case.RootChangeCandidateID != nil {
		out.ActionCandidateID = view.Case.RootChangeCandidateID
	}
	return out, nil
}

func (r automationCaseReader) EligibleActionCodes(ctx context.Context, caseID int64) (map[string]bool, error) {
	resp, err := r.cases.ListActionCandidates(ctx, caseID)
	if err != nil {
		if errors.Is(err, correlation.ErrCaseNotFound) {
			return nil, automation.ErrCaseNotFound
		}
		return nil, fmt.Errorf("list action candidates for case %d: %w", caseID, err)
	}
	eligible := make(map[string]bool, len(resp.Items))
	for _, candidate := range resp.Items {
		if candidate.Eligible {
			eligible[candidate.Code] = true
		}
	}
	return eligible, nil
}

// investigatorCaseReader 把关联用例适配成 AI 调查所需的完整上下文。
// 与自动化不同，这里必须带因子、信号链、资源链与变更候选——模型的引用校验
// 依赖这些 ID，缺一项就会导致「模型无法引用」而不是「答得不好」。
type investigatorCaseReader struct {
	cases correlationCaseSource
}

func (r investigatorCaseReader) GetCase(ctx context.Context, caseID int64) (aiinvestigator.CaseContext, error) {
	view, err := r.cases.GetCase(ctx, caseID)
	if err != nil {
		return aiinvestigator.CaseContext{}, fmt.Errorf("load correlation case %d: %w", caseID, err)
	}
	primary := view.Case.PrimaryResource
	out := aiinvestigator.CaseContext{
		CaseID:               view.Case.ID,
		ClusterID:            view.Case.ClusterID,
		RuleID:               view.Case.RuleID,
		PrimaryResourceKind:  primary.Kind,
		PrimaryResourceName:  primary.Name,
		PrimaryResourceUID:   primary.UID,
		Confidence:           string(view.Case.Confidence),
		EvidenceCompleteness: string(view.Case.EvidenceCompleteness),
	}
	for _, f := range view.Case.Factors {
		out.Factors = append(out.Factors, aiinvestigator.FactorContext{Kind: f.Kind, Value: f.Value, Weight: f.Weight})
	}
	for _, link := range view.SignalLinks {
		out.SignalLinks = append(out.SignalLinks, aiinvestigator.SignalLinkContext{
			SignalOccurrenceID: link.SignalOccurrenceID,
			Relation:           string(link.Relation),
			SignalID:           link.SignalID,
			Producer:           link.Producer,
			ObservedAt:         link.ObservedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	for _, link := range view.ResourceLinks {
		out.ResourceLinks = append(out.ResourceLinks, aiinvestigator.ResourceLinkContext{
			Kind:      link.Resource.Kind,
			Namespace: link.Resource.Namespace,
			Name:      link.Resource.Name,
			UID:       link.Resource.UID,
			Relation:  string(link.Relation),
		})
	}
	for _, candidate := range view.ChangeCandidates {
		out.ChangeCandidates = append(out.ChangeCandidates, aiinvestigator.ChangeCandidateContext{
			ChangeEventID: candidate.ChangeEventID,
			RuleID:        candidate.RuleID,
			Confidence:    string(candidate.Confidence),
			Rank:          candidate.Rank,
			ReasonCode:    candidate.ReasonCode,
		})
	}
	return out, nil
}

func (r investigatorCaseReader) EligibleActionCodes(ctx context.Context, caseID int64) (map[string]bool, error) {
	resp, err := r.cases.ListActionCandidates(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("list action candidates for case %d: %w", caseID, err)
	}
	eligible := make(map[string]bool, len(resp.Items))
	for _, candidate := range resp.Items {
		if candidate.Eligible {
			eligible[candidate.Code] = true
		}
	}
	return eligible, nil
}
