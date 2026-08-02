package capacity

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// saturation window (days) within which a projected saturation is treated as
// critical rather than warning.
const criticalSaturationWindowDays = 7

// Evaluate projects CPU and memory utilization forward from the observed
// bundle and reports any resource predicted to saturate (or crowd) within the
// horizon as a CAPACITY_SATURATION_RISK finding. It is a pure function: the
// same Inputs always yield the same Status, with no clock, I/O or randomness
// beyond the supplied observedAt.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	horizon := in.HorizonDays
	if horizon <= 0 {
		horizon = DefaultHorizonDays
	}

	status := Status{
		ClusterID:   clusterID,
		EvaluatedAt: observedAt,
		Total:       2,
		BySeverity:  map[string]int{},
		ByFamily:    map[string]int{},
		Findings:    []Finding{},
	}

	cpu := projectResource(in.CPU, observedAt, horizon)
	mem := projectResource(in.Memory, observedAt, horizon)

	status.CPUCapacityNanocores = in.CPU.Capacity
	status.MemCapacityBytes = in.Memory.Capacity
	status.CPUCurrentPct = cpu.currentPct
	status.MemCurrentPct = mem.currentPct
	status.CPUSaturationInDays = cpu.daysToSaturation
	status.MemSaturationInDays = mem.daysToSaturation

	addCapacityFinding(&status, "cpu", cpu, horizon)
	addCapacityFinding(&status, "memory", mem, horizon)

	status.Failed = len(status.Findings)
	status.Passed = status.Total - status.Failed
	if status.Passed < 0 {
		status.Passed = 0
	}
	sortFindings(status.Findings)
	return status
}

// projection holds the fitted trend and extrapolation for one resource.
type projection struct {
	hasTrend         bool
	currentPct       float64 // utilization fraction at observedAt
	slopePerDay      float64 // fraction growth per day
	projectedPct     float64 // utilization fraction at observedAt + horizon
	daysToSaturation float64 // days until 100%; -1 if not growing to saturation
}

// projectResource fits a linear trend to the usage fractions over time and
// extrapolates to the horizon. It returns hasTrend=false (no finding) when
// there are too few samples to fit.
func projectResource(trend ResourceTrend, observedAt time.Time, horizon int) projection {
	p := projection{hasTrend: false, daysToSaturation: -1}
	if trend.Capacity <= 0 || len(trend.Samples) < 2 {
		return p
	}
	xs := make([]float64, 0, len(trend.Samples))
	ys := make([]float64, 0, len(trend.Samples))
	for _, s := range trend.Samples {
		days := s.Timestamp.Sub(observedAt).Hours() / 24.0
		xs = append(xs, days)
		ys = append(ys, s.Value/float64(trend.Capacity))
	}
	slope, intercept := linearFit(xs, ys)
	p.hasTrend = true
	p.currentPct = intercept
	p.slopePerDay = slope
	p.projectedPct = intercept + slope*float64(horizon)
	if slope > 0 {
		p.daysToSaturation = (1.0 - intercept) / slope
	} else {
		p.daysToSaturation = -1
	}
	return p
}

// addCapacityFinding appends a CAPACITY_SATURATION_RISK finding for the
// resource when its projection crosses a risk threshold within the horizon.
func addCapacityFinding(status *Status, resource string, p projection, horizon int) {
	if !p.hasTrend {
		return
	}
	var severity string
	switch {
	case p.currentPct >= 1.0:
		severity = SeverityCritical
	case p.daysToSaturation >= 0 && p.daysToSaturation <= criticalSaturationWindowDays:
		severity = SeverityCritical
	case p.projectedPct >= 0.8:
		severity = SeverityWarning
	default:
		return // no risk within the horizon
	}

	details := map[string]string{
		"resource":          resource,
		"current_pct":       pctStr(p.currentPct),
		"projected_pct":     pctStr(p.projectedPct),
		"slope_pct_per_day": pctStr(p.slopePerDay),
		"horizon_days":      strconv.Itoa(horizon),
	}
	if p.daysToSaturation >= 0 {
		details["days_to_saturation"] = fmt.Sprintf("%.0f", math.Round(p.daysToSaturation))
	} else {
		details["days_to_saturation"] = "inf"
	}

	status.Findings = append(status.Findings, Finding{
		Code:       CodeSaturationRisk,
		Severity:   severity,
		Summary:    saturationSummary(resource, p, horizon),
		Resource:   k8sfinding.ResourceCitation{Kind: "Cluster", Name: resource},
		Details:    details,
		ObservedAt: k8sfinding.RFC3339(status.EvaluatedAt),
	})
	status.BySeverity[severity]++
	status.ByFamily[FamilyCapacity]++
}

// saturationSummary renders a human-readable Chinese summary of the projection.
func saturationSummary(resource string, p projection, horizon int) string {
	label := "CPU"
	if resource == "memory" {
		label = "内存"
	}
	switch {
	case p.currentPct >= 1.0:
		return fmt.Sprintf("%s容量当前利用率已超过 100%%，容量已耗尽，需立即扩容", label)
	case p.daysToSaturation >= 0 && p.daysToSaturation <= criticalSaturationWindowDays:
		return fmt.Sprintf("%s容量按当前增速约 %.0f 天后耗尽（%d 天窗口内）", label, math.Round(p.daysToSaturation), horizon)
	default:
		return fmt.Sprintf("%s容量预计 %d 天后达到 %.0f%% 利用率（当前 %.0f%%）", label, horizon, clampPct(p.projectedPct)*100, clampPct(p.currentPct)*100)
	}
}

// linearFit returns the least-squares slope and intercept of (xs, ys). When the
// x values are all equal (no spread) it returns a zero slope and the mean y.
func linearFit(xs, ys []float64) (slope, intercept float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	var sx, sy, sxx, sxy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		sxy += xs[i] * ys[i]
	}
	denom := n*sxx - sx*sx
	if denom == 0 {
		return 0, sy / n
	}
	slope = (n*sxy - sx*sy) / denom
	intercept = (sy - slope*sx) / n
	return slope, intercept
}

// pctStr renders a fraction as a rounded percentage string (e.g. 0.834 → "83").
func pctStr(frac float64) string {
	return fmt.Sprintf("%.0f", clampPct(frac)*100)
}

// clampPct keeps a fraction in [0, 9.99] for stable display.
func clampPct(frac float64) float64 {
	if frac < 0 {
		return 0
	}
	if frac > 9.99 {
		return 9.99
	}
	return frac
}

// severityRank orders findings critical → warning → info for stable display.
func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// sortFindings orders findings critical → warning → info, then by resource
// name, so the most urgent surface first.
func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if severityRank(findings[i].Severity) != severityRank(findings[j].Severity) {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		}
		return findings[i].Resource.Name < findings[j].Resource.Name
	})
}
