package alertoverview

import (
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
}

func TestAggregate_GroupsByRuleAndDedups(t *testing.T) {
	now := fixedNow()
	rules := []RuleRef{
		{ID: 1, DisplayName: "api ErrorRate", ResourceKind: "Deployment", ResourceName: "api", MetricName: "error_rate"},
		{ID: 2, DisplayName: "worker CPU", ResourceKind: "Node", ResourceName: "worker-0", MetricName: "node_cpu"},
	}
	instances := []InstanceRef{
		{RuleID: 1, State: "firing", FirstFiredAt: now.Add(-2 * time.Hour), LastFiredAt: now.Add(-5 * time.Minute)},
		{RuleID: 1, State: "firing", FirstFiredAt: now.Add(-3 * time.Hour), LastFiredAt: now.Add(-1 * time.Minute)},
		{RuleID: 1, State: "resolved", FirstFiredAt: now.Add(-5 * time.Hour), LastFiredAt: now.Add(-4 * time.Hour), ResolvedAt: ptr(now.Add(-4 * time.Hour))},
		{RuleID: 2, State: "firing", FirstFiredAt: now.Add(-30 * time.Minute), LastFiredAt: now.Add(-30 * time.Minute)},
	}

	resp := Aggregate(rules, instances, nil, 24*time.Hour, now, 50)

	if resp.FailClosed {
		t.Fatalf("expected non-fail-closed response, got %+v", resp)
	}
	if resp.GroupsTotal != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", resp.GroupsTotal, resp.Groups)
	}
	if resp.TotalFiring != 3 || resp.TotalResolved != 1 {
		t.Fatalf("expected total firing 3 resolved 1, got firing=%d resolved=%d", resp.TotalFiring, resp.TotalResolved)
	}

	// Sort: rule 1 has latest LastFiredAt (-1m) → first.
	if resp.Groups[0].RuleID != 1 || resp.Groups[0].FiringCount != 2 || resp.Groups[0].ResolvedCount != 1 {
		t.Fatalf("unexpected first group: %+v", resp.Groups[0])
	}
	if resp.Groups[0].DisplayName != "api ErrorRate" {
		t.Fatalf("expected display name from rule, got %q", resp.Groups[0].DisplayName)
	}
}

func TestAggregate_WindowFiltersResolvedInstances(t *testing.T) {
	now := fixedNow()
	rules := []RuleRef{{ID: 1, DisplayName: "api", ResourceKind: "Deployment", ResourceName: "api"}}
	instances := []InstanceRef{
		{RuleID: 1, State: "firing", FirstFiredAt: now.Add(-10 * time.Minute), LastFiredAt: now.Add(-10 * time.Minute)},
		// Resolved long ago: outside a 6h window → must be dropped.
		{RuleID: 1, State: "resolved", FirstFiredAt: now.Add(-3 * 24 * time.Hour), LastFiredAt: now.Add(-3 * 24 * time.Hour), ResolvedAt: ptr(now.Add(-3 * 24 * time.Hour))},
	}
	resp := Aggregate(rules, instances, nil, 6*time.Hour, now, 50)
	if resp.GroupsTotal != 1 || resp.TotalResolved != 0 {
		t.Fatalf("expected only the firing instance in window, got %+v", resp)
	}
}

func TestAggregate_FailClosedEmpty(t *testing.T) {
	now := fixedNow()
	resp := Aggregate(nil, nil, nil, 24*time.Hour, now, 50)
	if !resp.FailClosed || resp.EmptyNote == "" {
		t.Fatalf("expected fail-closed with note, got %+v", resp)
	}
}

func TestAggregate_LinksCorrelationCases(t *testing.T) {
	now := fixedNow()
	rules := []RuleRef{
		{ID: 1, DisplayName: "api", ResourceKind: "Deployment", ResourceName: "api"},
		{ID: 2, DisplayName: "db", ResourceKind: "StatefulSet", ResourceName: "db"},
	}
	instances := []InstanceRef{
		{RuleID: 1, State: "firing", FirstFiredAt: now.Add(-5 * time.Minute), LastFiredAt: now.Add(-5 * time.Minute)},
		{RuleID: 2, State: "firing", FirstFiredAt: now.Add(-5 * time.Minute), LastFiredAt: now.Add(-5 * time.Minute)},
	}
	cases := []CaseRef{
		{ID: 42, Status: "active", ReasonCode: "rollout_precedes_pod_failure", Resources: []ResourceRef{{Kind: "Deployment", Name: "api"}}},
	}
	resp := Aggregate(rules, instances, cases, 24*time.Hour, now, 50)
	if resp.GroupsTotal != 2 {
		t.Fatalf("expected 2 groups, got %d", resp.GroupsTotal)
	}
	// api group must be first (has case) with case link.
	apiGroup := resp.Groups[0]
	if apiGroup.RuleID != 1 || len(apiGroup.RelatedCaseIDs) != 1 || apiGroup.RelatedCaseIDs[0] != 42 {
		t.Fatalf("expected api group linked to case 42, got %+v", apiGroup)
	}
	if resp.Groups[1].RuleID != 2 || len(resp.Groups[1].RelatedCaseIDs) != 0 {
		t.Fatalf("expected db group with no case, got %+v", resp.Groups[1])
	}
}

func TestAggregate_CapsGroups(t *testing.T) {
	now := fixedNow()
	rules := make([]RuleRef, 0, 5)
	instances := make([]InstanceRef, 0, 5)
	for i := 0; i < 5; i++ {
		ruleID := int64(i + 1)
		rules = append(rules, RuleRef{ID: ruleID, DisplayName: string(rune('A' + i)), ResourceKind: "Deployment", ResourceName: string(rune('a' + i))})
		instances = append(instances, InstanceRef{RuleID: ruleID, State: "firing", FirstFiredAt: now.Add(-time.Duration(i) * time.Minute), LastFiredAt: now.Add(-time.Duration(i) * time.Minute)})
	}
	resp := Aggregate(rules, instances, nil, 24*time.Hour, now, 3)
	if resp.GroupsTotal != 3 {
		t.Fatalf("expected 3 capped groups, got %d", resp.GroupsTotal)
	}
}

func ptr(t time.Time) *time.Time { return &t }
