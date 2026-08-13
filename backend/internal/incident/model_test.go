package incident

import (
	"testing"
	"time"
)

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusOpen, StatusConfirmed, true},
		{StatusOpen, StatusDismissed, true},
		{StatusOpen, StatusResolved, false},
		{StatusOpen, StatusOpen, false},
		{StatusConfirmed, StatusResolved, true},
		{StatusConfirmed, StatusDismissed, true},
		{StatusConfirmed, StatusOpen, false},
		{StatusResolved, StatusOpen, true},
		{StatusResolved, StatusConfirmed, false},
		{StatusDismissed, StatusOpen, true},
		{StatusDismissed, StatusConfirmed, false},
		{"unknown", StatusOpen, false},
	}
	for _, tc := range cases {
		if got := CanTransition(tc.from, tc.to); got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestValidSeverity(t *testing.T) {
	for _, severity := range []string{SeverityInfo, SeverityWarning, SeverityHigh, SeverityCritical} {
		if !ValidSeverity(severity) {
			t.Errorf("ValidSeverity(%q) = false, want true", severity)
		}
	}
	for _, severity := range []string{"", "severe", "CRITICAL", "low"} {
		if ValidSeverity(severity) {
			t.Errorf("ValidSeverity(%q) = true, want false", severity)
		}
	}
}

func TestSLADeadline(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		severity string
		hours    int
	}{
		{SeverityCritical, 1},
		{SeverityHigh, 4},
		{SeverityWarning, 24},
		{SeverityInfo, 72},
		{"unknown", 72},
	}
	for _, tc := range cases {
		deadline := SLADeadline(tc.severity, now)
		want := now.Add(time.Duration(tc.hours) * time.Hour)
		if !deadline.Equal(want) {
			t.Errorf("SLADeadline(%q) = %v, want %v", tc.severity, deadline, want)
		}
	}
}

func TestSourceRefs(t *testing.T) {
	if got := SourceRefForDiagnosis(42); got != "diagnosis:42" {
		t.Errorf("SourceRefForDiagnosis(42) = %q, want %q", got, "diagnosis:42")
	}
	if got := SourceRefForAlert(9); got != "alert:9" {
		t.Errorf("SourceRefForAlert(9) = %q, want %q", got, "alert:9")
	}
	if got := SourceRefForInspection(11); got != "inspection:11" {
		t.Errorf("SourceRefForInspection(11) = %q, want %q", got, "inspection:11")
	}
	got := SourceRefForFinding(7, "pod.crash_loop_backoff.v1", "Pod", "default", "web-0", "uid-1")
	want := "finding:7:pod.crash_loop_backoff.v1:Pod:default:web-0:uid-1"
	if got != want {
		t.Errorf("SourceRefForFinding() = %q, want %q", got, want)
	}
	if got := SourceRefForFinding(7, "pod.pending.v1", "Pod", "", "web-0", ""); got != "finding:7:pod.pending.v1:Pod::web-0" {
		t.Errorf("SourceRefForFinding() without uid = %q", got)
	}
}
