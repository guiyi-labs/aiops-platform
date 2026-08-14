// Package alertoverview implements the M114-1 alert noise-reduction
// aggregation: read-only grouping of alert instances by rule (resource),
// dedup counts, first/last fired, and correlation-case linkage.  All inputs
// are plain data; the package performs no cluster access or writes.
package alertoverview

import (
	"sort"
	"time"
)

const (
	// MinWindowMinutes is the smallest aggregation window (1 minute).
	MinWindowMinutes = 1
	// MaxWindowMinutes is the largest aggregation window (7 days).
	MaxWindowMinutes = 7 * 24 * 60
	// MinGroups is the minimum returned group count.
	MinGroups = 1
	// MaxGroups is the maximum returned group count.
	MaxGroups = 200
)

// RuleRef is the alert rule reference passed by the handler.
type RuleRef struct {
	ID           int64
	DisplayName  string
	ResourceKind string
	ResourceName string
	MetricName   string
}

// InstanceRef is a flattened alert instance.
type InstanceRef struct {
	RuleID       int64
	State        string // "firing" | "resolved"
	FirstFiredAt time.Time
	LastFiredAt  time.Time
	ResolvedAt   *time.Time
}

// ResourceRef is a kind/name/uid resource citation.
type ResourceRef struct {
	Kind      string
	Namespace string
	Name      string
	UID       string
}

// CaseRef is a flattened correlation case with its resource set.
type CaseRef struct {
	ID         int64
	Status     string
	ReasonCode string
	Resources  []ResourceRef
}

// Group is one aggregated alert rule group in the response.
type Group struct {
	RuleID         int64     `json:"rule_id"`
	DisplayName    string    `json:"display_name"`
	ResourceKind   string    `json:"resource_kind"`
	ResourceName   string    `json:"resource_name"`
	MetricName     string    `json:"metric_name"`
	FiringCount    int       `json:"firing_count"`
	ResolvedCount  int       `json:"resolved_count"`
	FirstFiredAt   time.Time `json:"first_fired_at"`
	LastFiredAt    time.Time `json:"last_fired_at"`
	RelatedCaseIDs []int64   `json:"related_case_ids,omitempty"`
}

// Response is the bounded aggregation response.
type Response struct {
	Scope         string    `json:"scope"`
	ObservedAt    time.Time `json:"observed_at"`
	WindowMinutes int       `json:"window_minutes"`
	GroupsTotal   int       `json:"groups_total"`
	Groups        []Group   `json:"groups"`
	TotalFiring   int       `json:"total_firing"`
	TotalResolved int       `json:"total_resolved"`
	FailClosed    bool      `json:"fail_closed"`
	EmptyNote     string    `json:"empty_note,omitempty"`
}

// Aggregate computes the alert noise-reduction overview.  Instances are
// filtered by window (LastFiredAt >= now - window for resolved; firing
// instances always pass).  Results are grouped by rule, sorted by
// LastFiredAt desc then DisplayName asc, and capped to maxGroups.
// FailClosed is set when no groups remain.
func Aggregate(rules []RuleRef, instances []InstanceRef, cases []CaseRef, window time.Duration, now time.Time, maxGroups int) Response {
	if maxGroups < MinGroups {
		maxGroups = MinGroups
	}
	if maxGroups > MaxGroups {
		maxGroups = MaxGroups
	}
	if window <= 0 {
		window = time.Duration(MaxWindowMinutes) * time.Minute
	}
	windowStart := now.Add(-window)

	// Build rule lookup.
	ruleMap := make(map[int64]RuleRef, len(rules))
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	// Group instances by rule, filtering by window.
	type ruleGroup struct {
		rule          RuleRef
		firingCount   int
		resolvedCount int
		firstFiredAt  time.Time
		lastFiredAt   time.Time
		caseIDs       map[int64]struct{}
	}
	groups := make(map[int64]*ruleGroup)
	totalFiring := 0
	totalResolved := 0

	for _, inst := range instances {
		// Window filter: firing instances always count; resolved only
		// if their LastFiredAt falls within the window.
		if inst.State != "firing" && inst.LastFiredAt.Before(windowStart) {
			continue
		}
		g, ok := groups[inst.RuleID]
		if !ok {
			r := ruleMap[inst.RuleID]
			g = &ruleGroup{
				rule:         r,
				firstFiredAt: inst.FirstFiredAt,
				lastFiredAt:  inst.LastFiredAt,
				caseIDs:      make(map[int64]struct{}),
			}
			groups[inst.RuleID] = g
		}
		if inst.State == "firing" {
			g.firingCount++
			totalFiring++
		} else {
			g.resolvedCount++
			totalResolved++
		}
		if inst.FirstFiredAt.Before(g.firstFiredAt) {
			g.firstFiredAt = inst.FirstFiredAt
		}
		if inst.LastFiredAt.After(g.lastFiredAt) {
			g.lastFiredAt = inst.LastFiredAt
		}
	}

	// Match cases to groups by resource kind+name.  Rules identify resources
	// by kind+name (no UID on a rule), so a case covering the same resource
	// kind+name in its primary/linked resource set is related.
	for _, cs := range cases {
		for _, res := range cs.Resources {
			for _, g := range groups {
				if g.rule.ID == 0 {
					continue // no matching rule
				}
				if res.Kind == g.rule.ResourceKind && res.Name == g.rule.ResourceName {
					g.caseIDs[cs.ID] = struct{}{}
				}
			}
		}
	}

	// Flatten and sort.
	result := make([]Group, 0, len(groups))
	for _, g := range groups {
		// Skip rules with no in-window activity at all.
		if g.firingCount == 0 && g.resolvedCount == 0 {
			continue
		}
		caseIDs := make([]int64, 0, len(g.caseIDs))
		for cid := range g.caseIDs {
			caseIDs = append(caseIDs, cid)
		}
		sort.Slice(caseIDs, func(i, j int) bool { return caseIDs[i] < caseIDs[j] })
		result = append(result, Group{
			RuleID:         g.rule.ID,
			DisplayName:    g.rule.DisplayName,
			ResourceKind:   g.rule.ResourceKind,
			ResourceName:   g.rule.ResourceName,
			MetricName:     g.rule.MetricName,
			FiringCount:    g.firingCount,
			ResolvedCount:  g.resolvedCount,
			FirstFiredAt:   g.firstFiredAt,
			LastFiredAt:    g.lastFiredAt,
			RelatedCaseIDs: caseIDs,
		})
	}

	// Sort: groups with cases first, then by LastFiredAt desc, then DisplayName asc.
	sort.Slice(result, func(i, j int) bool {
		aHasCase := len(result[i].RelatedCaseIDs) > 0
		bHasCase := len(result[j].RelatedCaseIDs) > 0
		if aHasCase != bHasCase {
			return aHasCase
		}
		if !result[i].LastFiredAt.Equal(result[j].LastFiredAt) {
			return result[i].LastFiredAt.After(result[j].LastFiredAt)
		}
		return result[i].DisplayName < result[j].DisplayName
	})

	// Cap groups.
	truncated := len(result) > maxGroups
	if truncated {
		result = result[:maxGroups]
	}
	_ = truncated

	failClosed := len(result) == 0
	note := ""
	if failClosed {
		note = "no alerts in window"
	}

	return Response{
		Scope:         "alerts:overview",
		ObservedAt:    now,
		WindowMinutes: int(window.Minutes()),
		GroupsTotal:   len(result),
		Groups:        result,
		TotalFiring:   totalFiring,
		TotalResolved: totalResolved,
		FailClosed:    failClosed,
		EmptyNote:     note,
	}
}
