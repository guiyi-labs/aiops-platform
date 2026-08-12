package diagnosis

import (
	"testing"
	"time"
)

func sampleReplayRecord() Record {
	observed := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	return Record{
		ID:         7,
		ClusterID:  1,
		RuleID:     "node.not_ready.v1",
		Severity:   "critical",
		Status:     "confirmed",
		Resource:   ResourceRef{Kind: "Node", Name: "demo-node"},
		Summary:    "Node 未处于 Ready 状态。",
		ObservedAt: observed,
		Timeline: []TimelineEntry{
			{Index: 0, Category: CategoryResourceState, Type: "node_condition", Summary: "Ready = False", Ref: "diagnosis:7:evidence:0", OccurredAt: "2026-08-12T07:30:00Z", Integrity: "abc"},
			{Index: 1, Category: CategoryResourceState, Type: "node_condition", Summary: "MemoryPressure = True", Ref: "diagnosis:7:evidence:1", OccurredAt: "2026-08-12T07:25:00Z", Integrity: "def"},
		},
		Activities: []Activity{
			{ID: 10, Actor: ActorRef{Name: "operator-a"}, FromStatus: "open", ToStatus: "confirmed", Comment: "确认根因", CreatedAt: time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC)},
		},
		Assignments: []Assignment{
			{ID: 3, Actor: ActorRef{Name: "admin"}, ToAssignee: ActorRef{Name: "operator-b"}, Comment: "跟进", CreatedAt: time.Date(2026, 8, 12, 8, 20, 0, 0, time.UTC)},
		},
		Feedback: []Feedback{
			{ID: 1, Actor: ActorRef{Name: "operator-a"}, Verdict: "accurate", Comment: "", CreatedAt: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)},
		},
	}
}

func TestBuildReplayOrdersStoredArtifactsByTime(t *testing.T) {
	record := sampleReplayRecord()
	explanations := []ExplanationSnapshot{
		{ID: 5, Provider: "openai", Model: "gpt-5.4-mini", Summary: "kubelet 失联导致 Ready=False", CreatedAt: time.Date(2026, 8, 12, 8, 15, 0, 0, time.UTC)},
	}
	remediations := []RemediationSnapshot{
		{ID: "plan-0001", Action: "deployment.rollout_restart", Status: "succeeded", TargetName: "demo-app",
			CreatedAt:  time.Date(2026, 8, 12, 8, 45, 0, 0, time.UTC),
			ExecutedAt: timePtr(time.Date(2026, 8, 12, 8, 46, 0, 0, time.UTC))},
	}

	view := BuildReplay(record, explanations, remediations)

	if view.Schema != "aiops.diagnosis-replay/v1" {
		t.Fatalf("schema: %s", view.Schema)
	}
	if len(view.Steps) != 9 { // created + 2 evidence + 1 activity + 1 assignment + 1 feedback + 1 explanation + 1 remediation + 1 executed
		t.Fatalf("expected 9 steps, got %d", len(view.Steps))
	}
	// Stage counts
	if len(view.Stages) != 5 {
		t.Fatalf("expected 5 stages, got %d: %+v", len(view.Stages), view.Stages)
	}
	stageCount := map[ReplayStageID]int{}
	for _, stage := range view.Stages {
		stageCount[stage.Stage] = stage.Count
	}
	if stageCount[StageDiagnosisCreated] != 1 || stageCount[StageEvidence] != 2 || stageCount[StageActivity] != 3 ||
		stageCount[StageAIExplanation] != 1 || stageCount[StageRemediation] != 2 {
		t.Fatalf("unexpected stage counts: %+v", stageCount)
	}

	// Strict chronological order
	for i := 1; i < len(view.Steps); i++ {
		if view.Steps[i-1].OccurredAt > view.Steps[i].OccurredAt {
			t.Fatalf("steps out of order at %d: %s > %s", i, view.Steps[i-1].OccurredAt, view.Steps[i].OccurredAt)
		}
	}
	// First step is the earliest stored evidence (resource state predates the
	// platform record); the diagnosis_created step is present and ordered by
	// its recorded time.
	first := view.Steps[0]
	if first.Stage != StageEvidence || first.Type != "node_condition" || first.OccurredAt != "2026-08-12T07:25:00Z" {
		t.Fatalf("first step: %+v", first)
	}
	createdSeen := false
	for _, step := range view.Steps {
		if step.Type == "diagnosis_created" {
			createdSeen = true
			if step.OccurredAt != "2026-08-12T08:00:00Z" {
				t.Fatalf("diagnosis_created occurred_at: %s", step.OccurredAt)
			}
		}
	}
	if !createdSeen {
		t.Fatalf("diagnosis_created step missing")
	}
	// The remediation executed step exists and precedes the later feedback.
	executedSeen := false
	var executedAt string
	for _, step := range view.Steps {
		if step.Type == "remediation_executed" {
			executedSeen = true
			executedAt = step.OccurredAt
		}
	}
	if !executedSeen || executedAt != "2026-08-12T08:46:00Z" {
		t.Fatalf("remediation_executed missing or wrong time: %q", executedAt)
	}
	// The latest step is the post-hoc feedback (09:00), proving replay keeps
	// full chronological history rather than stopping at the remediation.
	last := view.Steps[len(view.Steps)-1]
	if last.Type != "feedback" || last.OccurredAt != "2026-08-12T09:00:00Z" {
		t.Fatalf("last step: %+v", last)
	}
	// Sequential indices
	for i, step := range view.Steps {
		if step.Index != i {
			t.Fatalf("index mismatch at %d: %d", i, step.Index)
		}
	}
}

func TestBuildReplayDegradesToStoredEvidenceOnly(t *testing.T) {
	// No explanations/remediations services configured -> only created + evidence + activities.
	view := BuildReplay(sampleReplayRecord(), nil, nil)
	if len(view.Steps) != 6 { // created + 2 evidence + activity + assignment + feedback
		t.Fatalf("expected 6 steps, got %d", len(view.Steps))
	}
	for _, stage := range view.Stages {
		if stage.Stage == StageAIExplanation || stage.Stage == StageRemediation {
			t.Fatalf("stage %s must be absent when no artifacts exist", stage.Stage)
		}
	}
	// Missing evidence entries keep their flags
	foundMissing := false
	for _, step := range view.Steps {
		if step.Missing {
			foundMissing = true
		}
	}
	if foundMissing {
		t.Fatalf("sample record has no missing evidence; replay must not invent flags")
	}
}

func TestBuildReplayKeepsMissingEvidenceSemantics(t *testing.T) {
	record := sampleReplayRecord()
	record.Timeline = append(record.Timeline, TimelineEntry{
		Index: 2, Category: CategoryResourceState, Type: "node_condition", Summary: "Ready = Missing",
		Ref: "diagnosis:7:evidence:2", Missing: true, MissingReason: "ReadyConditionMissing",
	})
	view := BuildReplay(record, nil, nil)
	seen := false
	for _, step := range view.Steps {
		if step.Type == "node_condition" && step.Missing {
			seen = true
			if step.MissingReason != "ReadyConditionMissing" {
				t.Fatalf("missing reason lost: %+v", step)
			}
		}
	}
	if !seen {
		t.Fatalf("missing evidence step not preserved in replay")
	}
}

func timePtr(t time.Time) *time.Time { return &t }
