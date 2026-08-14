// Package eventcockpit implements the M114-2 event cockpit: a bounded,
// read-only aggregation of Kubernetes Events grouped by severity, reason,
// and resource. It follows the ADR-0004 pattern — pure input→output
// function with no cluster access, no write path, no side effects.
//
// Acceptance (from M114 roadmap Track E):
//   - All queries bounded (time window, group cap, page limit).
//   - Event aggregation preserves original evidence deep-links.
//   - Empty source is fail-closed (never treated as healthy).
package eventcockpit

import (
	"fmt"
	"sort"
	"time"
)

// GroupKey identifies one aggregation bucket.
type GroupKey struct {
	Severity  string // derived from k8s Event.Type: "warning" / "info"
	Reason    string
	Namespace string
	Kind      string // involvedObject.kind
	Name      string // involvedObject.name
}

// EventInput is the minimal projection of a Kubernetes Event that the
// aggregation layer needs. The handler populates it from k8s gateway
// Events(); callers do not import the kubernetes package.
type EventInput struct {
	ID        string
	Severity  string // warning / info (derived from k8s Type field)
	Reason    string
	Message   string
	Count     int32
	FirstSeen time.Time
	LastSeen  time.Time
	Namespace string
	Kind      string // involvedObject.kind
	Name      string // involvedObject.name
	UID       string // involvedObject.uid (for deep-link)
}

// AggregatedGroup is one aggregation bucket in the cockpit output.
type AggregatedGroup struct {
	Severity     string    `json:"severity"`
	Reason       string    `json:"reason"`
	Namespace    string    `json:"namespace"`
	Kind         string    `json:"kind"`
	ResourceName string    `json:"resource_name"`
	ResourceUID  string    `json:"resource_uid"`
	RawCount     int64     `json:"raw_count"`   // sum of event.Count across folded events
	EventCount   int       `json:"event_count"` // number of distinct k8s events in this group
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Message      string    `json:"sample_message"` // representative message (last seen)
}

// TrendPoint is one day within the cockpit trend time series.
type TrendPoint struct {
	Day    string `json:"day"` // YYYY-MM-DD (UTC)
	Events int64  `json:"events"`
	Groups int    `json:"groups"`
}

// CockpitResponse is the read-only aggregation result. It always carries
// scope + observed_at. FailClosed is true when the window contains no
// events — an empty result is never treated as healthy (M99-D convention).
type CockpitResponse struct {
	Scope         string            `json:"scope"`
	ObservedAt    string            `json:"observed_at,omitempty"`
	WindowMinutes int               `json:"window_minutes"`
	GroupsTotal   int               `json:"groups_total"`
	Groups        []AggregatedGroup `json:"groups"`
	Trend         []TrendPoint      `json:"trend"`
	TotalEvents   int64             `json:"total_events"`
	TotalRawCount int64             `json:"total_raw_count"`
	FailClosed    bool              `json:"fail_closed"`
	EmptyNote     string            `json:"empty_note,omitempty"`
}

const (
	MinWindowMinutes = 1
	MaxWindowMinutes = 7 * 24 * 60 // 7 days
	MinGroups        = 1
	MaxGroups        = 200
)

// Aggregate computes the event cockpit over a bounded slice of events.
// window is the time window (e.g., 24 hours); now is the current UTC time;
// maxGroups caps the number of output groups (sorted by raw_count desc).
//
// Rules:
//   - Events outside the window are silently dropped.
//   - Events are grouped by (severity, reason, namespace, kind, name).
//   - Severity is normalized to lowercase ("warning" / "info").
//   - Empty input → FailClosed = true (empty window is never healthy).
func Aggregate(inputs []EventInput, window time.Duration, now time.Time, maxGroups int) CockpitResponse {
	if window <= 0 {
		window = 24 * time.Hour
	}
	if maxGroups < MinGroups {
		maxGroups = 20
	}
	if maxGroups > MaxGroups {
		maxGroups = MaxGroups
	}
	since := now.UTC().Add(-window)
	summary := CockpitResponse{
		Scope:         fmt.Sprintf("events:cockpit:window:%dm", int(window.Minutes())),
		ObservedAt:    now.UTC().Format(time.RFC3339),
		WindowMinutes: int(window.Minutes()),
		Groups:        []AggregatedGroup{},
		Trend:         []TrendPoint{},
	}

	type groupAcc struct {
		AggregatedGroup
		timestamps []time.Time // all LastSeen for trend bucketing
	}

	groups := map[GroupKey]*groupAcc{}
	totalEvents := int64(0)
	totalRawCount := int64(0)

	for _, ev := range inputs {
		if ev.LastSeen.Before(since) {
			continue
		}
		totalEvents++
		totalRawCount += int64(ev.Count)
		key := GroupKey{
			Severity:  normalizeSeverity(ev.Severity),
			Reason:    ev.Reason,
			Namespace: ev.Namespace,
			Kind:      ev.Kind,
			Name:      ev.Name,
		}
		g, ok := groups[key]
		if !ok {
			g = &groupAcc{
				AggregatedGroup: AggregatedGroup{
					Severity:     normalizeSeverity(ev.Severity),
					Reason:       ev.Reason,
					Namespace:    ev.Namespace,
					Kind:         ev.Kind,
					ResourceName: ev.Name,
					ResourceUID:  ev.UID,
					FirstSeen:    ev.LastSeen,
					LastSeen:     ev.LastSeen,
				},
			}
			groups[key] = g
		}
		g.RawCount += int64(ev.Count)
		g.EventCount++
		g.Message = ev.Message
		if ev.LastSeen.After(g.LastSeen) {
			g.LastSeen = ev.LastSeen
		}
		if ev.FirstSeen.Before(g.FirstSeen) {
			g.FirstSeen = ev.FirstSeen
		}
		g.timestamps = append(g.timestamps, ev.LastSeen)
	}

	// Flatten and sort groups by raw_count descending, then alphabetically for stability.
	flat := make([]*groupAcc, 0, len(groups))
	for _, g := range groups {
		flat = append(flat, g)
	}
	sort.Slice(flat, func(i, j int) bool {
		if flat[i].RawCount != flat[j].RawCount {
			return flat[i].RawCount > flat[j].RawCount
		}
		if flat[i].Severity != flat[j].Severity {
			return flat[i].Severity > flat[j].Severity // warning > info
		}
		return flat[i].Reason < flat[j].Reason
	})

	if len(flat) > maxGroups {
		flat = flat[:maxGroups]
	}

	summary.GroupsTotal = len(flat)
	summary.TotalEvents = totalEvents
	summary.TotalRawCount = totalRawCount
	for _, g := range flat {
		summary.Groups = append(summary.Groups, g.AggregatedGroup)
	}

	// Build trend: per-day event count across all groups.
	dayBuckets := map[string]*TrendPoint{}
	for _, g := range groups {
		for _, ts := range g.timestamps {
			if ts.Before(since) {
				continue
			}
			day := ts.UTC().Format("2006-01-02")
			bp, ok := dayBuckets[day]
			if !ok {
				bp = &TrendPoint{Day: day}
				dayBuckets[day] = bp
			}
			bp.Events++
			bp.Groups++
		}
	}
	days := make([]string, 0, len(dayBuckets))
	for d := range dayBuckets {
		days = append(days, d)
	}
	sort.Strings(days)
	for _, d := range days {
		summary.Trend = append(summary.Trend, *dayBuckets[d])
	}

	// Fail-closed: no events in window → never healthy.
	if totalEvents == 0 {
		summary.FailClosed = true
		summary.EmptyNote = "no events found in the time window (fail-closed)"
	}

	return summary
}

// normalizeSeverity maps the kubernetes Event.Type field to a consistent
// lowercase severity. Unknown types are treated as "info".
func normalizeSeverity(sev string) string {
	switch sev {
	case "Warning":
		return "warning"
	case "Normal":
		return "info"
	default:
		return "info"
	}
}
