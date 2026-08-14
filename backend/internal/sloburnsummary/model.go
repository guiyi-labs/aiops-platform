// Package sloburnsummary implements the M114-1 SLO burn posture summary:
// read-only aggregation over existing SLO definitions and their latest
// evaluations, computing per-SLO burn status for a dashboard view.
package sloburnsummary

import "time"

// BurnStatus describes the posture of one SLO relative to its burn rate.
type BurnStatus string

const (
	// StatusBurning means the SLO is actively consuming error budget.
	StatusBurning BurnStatus = "burning"
	// StatusHealthy means the SLO is meeting its objective.
	StatusHealthy BurnStatus = "healthy"
	// StatusUnavailable means evaluation data or coverage is missing.
	StatusUnavailable BurnStatus = "unavailable"
	// StatusNoData means no evaluation exists yet for the definition.
	StatusNoData BurnStatus = "no_data"
)

// ServiceRef mirrors slo.ServiceRef for the summary.
type ServiceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
}

// EvalRef is a minimal flattened evaluation for the summary.
type EvalRef struct {
	State           string
	BurnRate        float64
	Ratio           float64
	Coverage        string // complete / partial / unavailable
	ErrorBudget     float64
	RemainingBudget float64
	EvaluatedAt     time.Time
}

// Item is the burn posture for one SLO definition.
type Item struct {
	SLOID                int64      `json:"slo_id"`
	ClusterID            int64      `json:"cluster_id"`
	Service              ServiceRef `json:"service"`
	Template             string     `json:"template"`
	Objective            float64    `json:"objective"`
	Status               BurnStatus `json:"status"`
	BurnRate             float64    `json:"burn_rate,omitempty"`
	Ratio                float64    `json:"ratio,omitempty"`
	Coverage             string     `json:"coverage,omitempty"`
	ErrorBudgetRemaining float64    `json:"error_budget_remaining,omitempty"`
	EvaluatedAt          time.Time  `json:"evaluated_at,omitempty"`
}

// Response is the bounded paginated burn-summary response.
type Response struct {
	Items      []Item    `json:"items"`
	Total      int       `json:"total"`
	Truncated  bool      `json:"truncated"`
	ObservedAt time.Time `json:"observed_at"`
}

// Summarize takes definitions (enabled only) and a map of their latest
// evaluations (keyed by definition ID) and returns a bounded posture
// response sorted by status: burning first, then healthy, unavailable, no_data.
func Summarize(defs []DefRef, latest map[int64]EvalRef, limit int) Response {
	if limit <= 0 {
		limit = 50
	}
	items := make([]Item, 0, len(defs))
	for _, d := range defs {
		eval, ok := latest[d.ID]
		status := StatusNoData
		if ok {
			if eval.State == "unavailable" || eval.Coverage == "unavailable" {
				status = StatusUnavailable
			} else if eval.State == "healthy" {
				status = StatusHealthy
			} else {
				// burning_fast, burning_slow, breached → burning
				status = StatusBurning
			}
		}
		item := Item{
			SLOID:       d.ID,
			ClusterID:   d.ClusterID,
			Service:     d.Service,
			Template:    d.Template,
			Objective:   d.Objective,
			Status:      status,
			EvaluatedAt: eval.EvaluatedAt,
		}
		if ok {
			item.BurnRate = eval.BurnRate
			item.Ratio = eval.Ratio
			item.Coverage = eval.Coverage
			item.ErrorBudgetRemaining = eval.RemainingBudget
		}
		items = append(items, item)
	}

	// Sort by status priority: burning > unavailable > no_data > healthy
	statusOrder := map[BurnStatus]int{
		StatusBurning:     0,
		StatusUnavailable: 1,
		StatusNoData:      2,
		StatusHealthy:     3,
	}
	// Use stable sort so within same status, highest burn rate first.
	// Since stdlib sort is not stable, implement a two-key comparison.
	// Sort by (statusOrder, -burnRate, sloID)
	sortItems(items, statusOrder)

	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	return Response{
		Items:      items,
		Total:      len(items),
		Truncated:  truncated,
		ObservedAt: time.Now().UTC(),
	}
}

func sortItems(items []Item, order map[BurnStatus]int) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0; j-- {
			a, b := items[j-1], items[j]
			aOrd, bOrd := order[a.Status], order[b.Status]
			if aOrd > bOrd || (aOrd == bOrd && a.BurnRate < b.BurnRate) || (aOrd == bOrd && a.BurnRate == b.BurnRate && a.SLOID > b.SLOID) {
				items[j-1], items[j] = items[j], items[j-1]
			} else {
				break
			}
		}
	}
}

// DefRef is a flattened SLO definition for the summary.
type DefRef struct {
	ID        int64
	ClusterID int64
	Service   ServiceRef
	Template  string
	Objective float64
}
