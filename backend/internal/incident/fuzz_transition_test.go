package incident

import (
	"context"
	"errors"
	"testing"
)

// FuzzCanTransition pins the incident status machine: only the documented
// edges between valid statuses are allowed, and any input involving an
// invalid status is rejected.
func FuzzCanTransition(f *testing.F) {
	statuses := []string{StatusOpen, StatusConfirmed, StatusResolved, StatusDismissed, "", "bogus", " open", "open "}
	for _, from := range statuses {
		for _, to := range statuses {
			f.Add(from, to)
		}
	}

	f.Fuzz(func(t *testing.T, from, to string) {
		valid := map[string]bool{StatusOpen: true, StatusConfirmed: true, StatusResolved: true, StatusDismissed: true}
		allowed := CanTransition(from, to)
		if !valid[from] || !valid[to] {
			if allowed {
				t.Fatalf("CanTransition(%q, %q) = true with an invalid status", from, to)
			}
			return
		}
		expected := (from == StatusOpen && (to == StatusConfirmed || to == StatusDismissed)) ||
			(from == StatusConfirmed && (to == StatusResolved || to == StatusDismissed)) ||
			((from == StatusResolved || from == StatusDismissed) && to == StatusOpen)
		if allowed != expected {
			t.Fatalf("CanTransition(%q, %q) = %v, want %v", from, to, allowed, expected)
		}
	})
}

// FuzzTransitionSequence walks the incident status machine through the
// service layer against the CAS-mirroring in-memory repository. It verifies
// that every transition either succeeds with a version bump or fails with one
// of the documented sentinel errors, and never panics on arbitrary versions.
func FuzzTransitionSequence(f *testing.F) {
	f.Add(int64(1), int64(1), int64(1))
	f.Add(int64(0), int64(99), int64(0))

	f.Fuzz(func(t *testing.T, vA, vB, vC int64) {
		repo := newFakeRepository()
		svc := NewService(repo)
		inc, err := svc.Create(context.Background(), CreateInput{
			SourceType: SourceTypeFinding,
			SourceRef:  "finding:fuzz-sequence",
			ClusterID:  1,
			Title:      "fuzz incident",
			Severity:   SeverityInfo,
			Resource:   ResourceRef{Kind: "Node", Name: "demo-node"},
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		steps := []struct {
			status  string
			version int64
		}{
			{StatusConfirmed, vA},
			{StatusResolved, vB},
			{StatusDismissed, vC},
			{StatusOpen, 0},
			{StatusConfirmed, 0},
		}
		for _, step := range steps {
			expected := inc.Version
			if step.version != 0 {
				expected = step.version
			}
			next, err := svc.Transition(context.Background(), inc.ID, expected, step.status, ActorRef{Name: "fuzz"}, "step")
			if err == nil {
				if next.Version != inc.Version+1 {
					t.Fatalf("version = %d, want %d (status %s)", next.Version, inc.Version+1, step.status)
				}
				switch next.Status {
				case StatusOpen, StatusConfirmed, StatusResolved, StatusDismissed:
				default:
					t.Fatalf("invalid status after transition: %q", next.Status)
				}
				inc = next
				continue
			}
			if !errors.Is(err, ErrInvalidTransition) && !errors.Is(err, ErrVersionConflict) {
				t.Fatalf("unexpected transition error for %q@%d: %v", step.status, expected, err)
			}
		}
	})
}
