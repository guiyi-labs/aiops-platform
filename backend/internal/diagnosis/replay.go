package diagnosis

import (
	"fmt"
	"sort"
	"time"
)

// ReplayStageID identifies the M81 insight-chain stage a replay step belongs
// to. Stages are emitted in a fixed product order and only included when the
// record actually contains artifacts of that stage — replay never fabricates
// history.
type ReplayStageID string

const (
	StageDiagnosisCreated ReplayStageID = "diagnosis_created"
	StageEvidence         ReplayStageID = "evidence"
	StageActivity         ReplayStageID = "activity"
	StageAIExplanation    ReplayStageID = "ai_explanation"
	StageRemediation      ReplayStageID = "remediation"
)

// stageOrder fixes the product narrative order for the replay.
var stageOrder = []ReplayStageID{
	StageDiagnosisCreated,
	StageEvidence,
	StageActivity,
	StageAIExplanation,
	StageRemediation,
}

// stageLabels are the user-facing stage names.
var stageLabels = map[ReplayStageID]string{
	StageDiagnosisCreated: "诊断创建",
	StageEvidence:         "证据采集",
	StageActivity:         "状态与协作",
	StageAIExplanation:    "AI 引用解释",
	StageRemediation:      "受控动作",
}

// ExplanationSnapshot is the minimal replay projection of an AI explanation.
// It is filled by the HTTP layer from the AIExplanation service so the pure
// replay builder stays free of cross-package dependencies.
type ExplanationSnapshot struct {
	ID        int64
	Provider  string
	Model     string
	Summary   string
	Feedback  string
	CreatedAt time.Time
}

// RemediationSnapshot is the minimal replay projection of a remediation plan.
type RemediationSnapshot struct {
	ID         string
	Action     string
	Status     string
	TargetName string
	CreatedAt  time.Time
	ExecutedAt *time.Time
}

// ReplayStep is one replayable event on the diagnosis insight chain.
type ReplayStep struct {
	Index         int              `json:"index"`
	Stage         ReplayStageID    `json:"stage"`
	Category      EvidenceCategory `json:"category,omitempty"`
	Type          string           `json:"type"`
	Summary       string           `json:"summary"`
	Integrity     string           `json:"integrity,omitempty"`
	Ref           string           `json:"ref"`
	OccurredAt    string           `json:"occurred_at,omitempty"`
	Missing       bool             `json:"missing"`
	MissingReason string           `json:"missing_reason,omitempty"`
	Detail        map[string]any   `json:"detail,omitempty"`
}

// ReplayStage summarizes how many steps a stage contributed.
type ReplayStage struct {
	Stage ReplayStageID `json:"stage"`
	Label string        `json:"label"`
	Count int           `json:"count"`
}

// ReplayView is the read-only replay projection of a diagnosis.
type ReplayView struct {
	Schema      string        `json:"schema"`
	DiagnosisID int64         `json:"diagnosis_id"`
	RuleID      string        `json:"rule_id"`
	Severity    string        `json:"severity"`
	Resource    ResourceRef   `json:"resource"`
	ObservedAt  string        `json:"observed_at"`
	Steps       []ReplayStep  `json:"steps"`
	Stages      []ReplayStage `json:"stages"`
}

// BuildReplay assembles the ordered insight chain from stored artifacts only:
// the diagnosis creation, its evidence timeline, activities (transitions,
// assignments, feedback), AI explanations and remediation plans. Nothing is
// regenerated or invented; missing optional services simply contribute no
// steps. Steps are ordered by occurred time (stable, creation order tiebreak)
// and re-indexed sequentially.
func BuildReplay(record Record, explanations []ExplanationSnapshot, remediations []RemediationSnapshot) ReplayView {
	steps := make([]ReplayStep, 0, len(record.Timeline)+len(record.Activities)+len(record.Assignments)+len(record.Feedback)+len(explanations)+len(remediations)+1)

	observed := record.ObservedAt.UTC().Format(time.RFC3339)
	created := record.CreatedAt
	if created.IsZero() {
		created = record.ObservedAt
	}
	steps = append(steps, ReplayStep{
		Stage:      StageDiagnosisCreated,
		Type:       "diagnosis_created",
		Summary:    fmt.Sprintf("诊断创建 · %s（%s）", record.RuleID, record.Severity),
		Ref:        fmt.Sprintf("diagnosis:%d", record.ID),
		OccurredAt: created.UTC().Format(time.RFC3339),
		Detail: map[string]any{
			"rule_id": record.RuleID, "severity": record.Severity,
			"summary": record.Summary, "status": record.Status,
		},
	})

	for _, entry := range record.Timeline {
		steps = append(steps, ReplayStep{
			Stage:         StageEvidence,
			Category:      entry.Category,
			Type:          entry.Type,
			Summary:       entry.Summary,
			Integrity:     entry.Integrity,
			Ref:           entry.Ref,
			OccurredAt:    entry.OccurredAt,
			Missing:       entry.Missing,
			MissingReason: entry.MissingReason,
		})
	}

	for _, activity := range record.Activities {
		steps = append(steps, ReplayStep{
			Stage:      StageActivity,
			Type:       "status_transition",
			Summary:    fmt.Sprintf("%s → %s", activity.FromStatus, activity.ToStatus),
			Ref:        fmt.Sprintf("activity:%d", activity.ID),
			OccurredAt: activity.CreatedAt.UTC().Format(time.RFC3339),
			Detail: map[string]any{
				"actor": activity.Actor.Name, "comment": activity.Comment,
			},
		})
	}
	for _, assignment := range record.Assignments {
		from := "未分配"
		if assignment.FromAssignee != nil {
			from = assignment.FromAssignee.Name
		}
		steps = append(steps, ReplayStep{
			Stage:      StageActivity,
			Type:       "assignment",
			Summary:    fmt.Sprintf("转派 %s → %s", from, assignment.ToAssignee.Name),
			Ref:        fmt.Sprintf("assignment:%d", assignment.ID),
			OccurredAt: assignment.CreatedAt.UTC().Format(time.RFC3339),
			Detail: map[string]any{
				"actor": assignment.Actor.Name, "comment": assignment.Comment,
			},
		})
	}
	for _, feedback := range record.Feedback {
		steps = append(steps, ReplayStep{
			Stage:      StageActivity,
			Type:       "feedback",
			Summary:    fmt.Sprintf("规则准确性反馈 · %s", feedback.Verdict),
			Ref:        fmt.Sprintf("feedback:%d", feedback.ID),
			OccurredAt: feedback.CreatedAt.UTC().Format(time.RFC3339),
			Detail: map[string]any{
				"actor": feedback.Actor.Name, "verdict": feedback.Verdict, "comment": feedback.Comment,
			},
		})
	}

	for _, explanation := range explanations {
		summary := explanation.Summary
		if summary == "" {
			summary = fmt.Sprintf("%s · %s", explanation.Provider, explanation.Model)
		}
		steps = append(steps, ReplayStep{
			Stage:      StageAIExplanation,
			Type:       "ai_explanation",
			Summary:    summary,
			Ref:        fmt.Sprintf("explanation:%d", explanation.ID),
			OccurredAt: explanation.CreatedAt.UTC().Format(time.RFC3339),
			Detail: map[string]any{
				"provider": explanation.Provider, "model": explanation.Model,
				"feedback": explanation.Feedback,
			},
		})
	}

	for _, plan := range remediations {
		steps = append(steps, ReplayStep{
			Stage:      StageRemediation,
			Type:       "remediation_created",
			Summary:    fmt.Sprintf("受控动作预览 · %s（%s）", plan.Action, plan.Status),
			Ref:        fmt.Sprintf("remediation:%s", plan.ID),
			OccurredAt: plan.CreatedAt.UTC().Format(time.RFC3339),
			Detail: map[string]any{
				"action": plan.Action, "status": plan.Status, "target_name": plan.TargetName,
			},
		})
		if plan.ExecutedAt != nil {
			steps = append(steps, ReplayStep{
				Stage:      StageRemediation,
				Type:       "remediation_executed",
				Summary:    fmt.Sprintf("受控动作执行 · %s → %s", plan.Action, plan.Status),
				Ref:        fmt.Sprintf("remediation:%s:executed", plan.ID),
				OccurredAt: plan.ExecutedAt.UTC().Format(time.RFC3339),
				Detail: map[string]any{
					"action": plan.Action, "status": plan.Status, "target_name": plan.TargetName,
				},
			})
		}
	}

	stableSortReplaySteps(steps)

	view := ReplayView{
		Schema:      "aiops.diagnosis-replay/v1",
		DiagnosisID: record.ID,
		RuleID:      record.RuleID,
		Severity:    record.Severity,
		Resource:    record.Resource,
		ObservedAt:  observed,
		Steps:       steps,
		Stages:      []ReplayStage{},
	}
	counts := map[ReplayStageID]int{}
	for index := range steps {
		steps[index].Index = index
		counts[steps[index].Stage]++
	}
	for _, stage := range stageOrder {
		if counts[stage] > 0 {
			view.Stages = append(view.Stages, ReplayStage{Stage: stage, Label: stageLabels[stage], Count: counts[stage]})
		}
	}
	return view
}

// stableSortReplaySteps orders steps by occurred time; steps without a time or
// with equal times keep creation order (stable sort).
func stableSortReplaySteps(steps []ReplayStep) {
	sort.SliceStable(steps, func(i, j int) bool {
		left, right := steps[i].OccurredAt, steps[j].OccurredAt
		if left != right {
			return left < right
		}
		return steps[i].Ref < steps[j].Ref
	})
}
