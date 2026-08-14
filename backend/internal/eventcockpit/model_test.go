package eventcockpit

import (
	"testing"
	"time"
)

func mkEvent(sev, reason, msg, ns, kind, name, uid string, count int32, seen time.Time) EventInput {
	return EventInput{
		ID: uid, Severity: sev, Reason: reason, Message: msg, Count: count,
		FirstSeen: seen, LastSeen: seen, Namespace: ns, Kind: kind, Name: name, UID: uid,
	}
}

func TestAggregate_GroupsByReasonAndResource(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	inputs := []EventInput{
		mkEvent("Warning", "CrashLoopBackOff", "restarting", "prod", "Pod", "api-0", "uid-a", 12, now.Add(-2*time.Hour)),
		mkEvent("Warning", "CrashLoopBackOff", "restarting", "prod", "Pod", "api-0", "uid-a", 3, now.Add(-time.Hour)),
		mkEvent("Normal", "Scheduled", "assigned", "prod", "Pod", "api-0", "uid-b", 1, now.Add(-time.Hour)),
		mkEvent("Warning", "OOMKilling", "killed", "staging", "Pod", "worker-1", "uid-c", 7, now.Add(-30*time.Minute)),
		// outside the window
		mkEvent("Warning", "OldEvent", "old", "prod", "Pod", "old-0", "uid-d", 9, now.Add(-2*24*time.Hour)),
	}

	got := Aggregate(inputs, 24*time.Hour, now, 20)

	if got.FailClosed {
		t.Fatalf("expected fail_closed=false, got %v (note=%s)", got.FailClosed, got.EmptyNote)
	}
	// 2 CrashLoopBackOff(folded) + 1 Scheduled + 1 OOMKilling = 3 groups.
	if got.GroupsTotal != 3 {
		t.Fatalf("groups_total=%d, want 3: %+v", got.GroupsTotal, got.Groups)
	}
	if got.TotalEvents != 4 {
		t.Fatalf("total_events=%d, want 4 (old event filtered)", got.TotalEvents)
	}
	if got.TotalRawCount != 23 {
		t.Fatalf("total_raw_count=%d, want 23", got.TotalRawCount)
	}

	// CrashLoopBackOff should be first (highest raw_count) and folded (event_count=2).
	top := got.Groups[0]
	if top.Reason != "CrashLoopBackOff" {
		t.Fatalf("top group reason=%s, want CrashLoopBackOff", top.Reason)
	}
	if top.EventCount != 2 {
		t.Fatalf("CrashLoopBackOff event_count=%d, want 2 (folded)", top.EventCount)
	}
	if top.RawCount != 15 {
		t.Fatalf("CrashLoopBackOff raw_count=%d, want 15", top.RawCount)
	}
	if top.Severity != "warning" || top.ResourceUID != "uid-a" {
		t.Fatalf("severity/uid mismatch: %+v", top)
	}
	// First vs last seen must differ (most recent LastSeen wins).
	if !top.FirstSeen.Equal(now.Add(-2 * time.Hour)) {
		t.Fatalf("first_seen=%v", top.FirstSeen)
	}
	if !top.LastSeen.Equal(now.Add(-time.Hour)) {
		t.Fatalf("last_seen=%v", top.LastSeen)
	}
}

func TestAggregate_SeverityNormalization(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	inputs := []EventInput{
		mkEvent("Normal", "Scheduled", "x", "ns", "Pod", "p1", "u1", 1, now.Add(-time.Minute)),
		mkEvent("", "Unknown", "x", "ns", "Pod", "p2", "u2", 1, now.Add(-time.Minute)),
	}
	got := Aggregate(inputs, time.Hour, now, 20)
	if got.Groups[0].Severity != "info" || got.Groups[1].Severity != "info" {
		t.Fatalf("severity normalization failed: %+v", got.Groups)
	}
}

func TestAggregate_FailClosedEmpty(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	// Events outside the window → effectively empty.
	inputs := []EventInput{
		mkEvent("Warning", "Old", "x", "ns", "Pod", "p", "u", 1, now.Add(-2*24*time.Hour)),
	}
	got := Aggregate(inputs, 24*time.Hour, now, 20)
	if !got.FailClosed {
		t.Fatalf("expected fail_closed=true for empty window, got %+v", got)
	}
	if got.EmptyNote == "" {
		t.Fatalf("expected an empty note")
	}
}

func TestAggregate_MaxGroupsCapsAndSorts(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	var inputs []EventInput
	for i := 0; i < 5; i++ {
		inputs = append(inputs, mkEvent("Warning", "R", "x", "ns", "Pod", "p"+string(rune('a'+i)), "u"+string(rune('a'+i)), int32((i+1)*2), now.Add(-time.Minute)))
	}
	got := Aggregate(inputs, time.Hour, now, 3)
	if got.GroupsTotal != 3 {
		t.Fatalf("groups_total=%d, want capped 3", got.GroupsTotal)
	}
	// Highest raw_count first.
	if got.Groups[0].RawCount != 10 {
		t.Fatalf("top raw_count=%d, want 10", got.Groups[0].RawCount)
	}
}

func TestAggregate_TrendBucketsByDay(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	inputs := []EventInput{
		mkEvent("Warning", "R", "x", "ns", "Pod", "p", "u1", 1, now.Add(-2*time.Hour)),
		mkEvent("Warning", "R", "x", "ns", "Pod", "p", "u2", 1, now.Add(-25*time.Hour)),
	}
	got := Aggregate(inputs, 7*24*time.Hour, now, 20)
	if len(got.Trend) != 2 {
		t.Fatalf("trend len=%d, want 2 distinct days", len(got.Trend))
	}
	if got.Trend[0].Day >= got.Trend[1].Day {
		t.Fatalf("trend not sorted ascending: %+v", got.Trend)
	}
}
