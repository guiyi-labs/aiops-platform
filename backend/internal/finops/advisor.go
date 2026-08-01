package finops

import "time"

// Tuning knobs for the right-sizing heuristic. They are deliberately
// conservative: requests get headroom above observed p95, and limits are only
// echoed or gently suggested — never tightened — because this advisor is
// read-only and must not push risky suggestions.
const (
	cpuRequestHeadroom = 1.15 // request = p95 * 1.15
	memRequestHeadroom = 1.20 // memory is not compressible, needs more buffer
	cpuLimitFactor     = 1.50 // suggested limit when none is set
	memLimitFactor     = 1.30

	warningRatio  = 2.0 // request >= 2x p95 -> warning
	criticalRatio = 4.0 // request >= 4x p95 -> critical

	// rounding steps for human-friendly recommendations
	cpuRoundStep = 50_000_000       // 50 millicores
	memRoundStep = 64 * 1024 * 1024 // 64 MiB
)

// Recommend evaluates every container and returns a WasteSummary with one
// Recommendation per container that is over-provisioned or missing a request.
// It is pure and read-only.
func Recommend(clusterID int64, inputs []ContainerInput, rate CostRate) WasteSummary {
	summary := WasteSummary{
		ClusterID:   clusterID,
		EvaluatedAt: time.Now().UTC(),
	}

	for _, in := range inputs {
		summary.ContainersEvaluated++

		hasUsage := in.CPUUsageP95 > 0 || in.MemUsageP95 > 0
		if !hasUsage {
			// No usage signal: nothing to right-size against. Skip.
			continue
		}

		rec := Recommendation{
			ClusterID:     in.ClusterID,
			Namespace:     in.Namespace,
			WorkloadKind:  in.WorkloadKind,
			WorkloadName:  in.WorkloadName,
			ContainerName: in.ContainerName,
			Replicas:      in.Replicas,
		}
		severity := SeverityInfo
		notes := make([]string, 0, 2)

		// --- CPU ---
		if in.CPUUsageP95 > 0 {
			sugCPU := roundUp(scaleUp(in.CPUUsageP95, cpuRequestHeadroom), cpuRoundStep)
			if in.Limits.CPULimit > 0 && sugCPU > in.Limits.CPULimit {
				sugCPU = in.Limits.CPULimit
			}
			rec.SuggestedRequests.CPURequest = sugCPU
			if in.Limits.CPULimit > 0 {
				rec.SuggestedLimits.CPULimit = in.Limits.CPULimit
			} else {
				rec.SuggestedLimits.CPULimit = roundUp(scaleUp(in.CPUUsageP95, cpuLimitFactor), cpuRoundStep)
			}
			if in.Requests.CPURequest > 0 {
				idle := in.Requests.CPURequest - in.CPUUsageP95
				if idle < 0 {
					idle = 0
				}
				idleCores := float64(idle) / 1e9
				waste := idleCores * float64(in.Replicas) * rate.PerCoreMonth
				rec.MonthlyWasteUSD += waste
				summary.CPUIdleCores += idleCores * float64(in.Replicas)
				severity = worse(severity, ratioSeverity(float64(in.Requests.CPURequest), float64(in.CPUUsageP95)))
				notes = append(notes, "CPU request is larger than observed p95 usage")
			} else {
				rec.Code = "MISSING_CPU_REQUEST"
				notes = append(notes, "container has no CPU request; bursts can starve co-located pods")
			}
		}

		// --- Memory ---
		if in.MemUsageP95 > 0 {
			sugMem := roundUp(scaleUp(in.MemUsageP95, memRequestHeadroom), memRoundStep)
			if in.Limits.MemLimit > 0 && sugMem > in.Limits.MemLimit {
				sugMem = in.Limits.MemLimit
			}
			rec.SuggestedRequests.MemRequest = sugMem
			if in.Limits.MemLimit > 0 {
				rec.SuggestedLimits.MemLimit = in.Limits.MemLimit
			} else {
				rec.SuggestedLimits.MemLimit = roundUp(scaleUp(in.MemUsageP95, memLimitFactor), memRoundStep)
			}
			if in.Requests.MemRequest > 0 {
				idle := in.Requests.MemRequest - in.MemUsageP95
				if idle < 0 {
					idle = 0
				}
				idleGB := float64(idle) / 1e9
				waste := idleGB * float64(in.Replicas) * rate.PerGBMonth
				rec.MonthlyWasteUSD += waste
				summary.MemIdleGB += idleGB * float64(in.Replicas)
				severity = worse(severity, ratioSeverity(float64(in.Requests.MemRequest), float64(in.MemUsageP95)))
				notes = append(notes, "memory request is larger than observed p95 usage")
			} else {
				if rec.Code == "" {
					rec.Code = "MISSING_MEM_REQUEST"
				}
				notes = append(notes, "container has no memory request; a single OOM can evict the pod")
			}
		}

		if rec.Code == "" {
			rec.Code = "RIGHTSIZING_OVER_PROVISIONED"
		}
		rec.Severity = severity
		rec.Rationale = buildRationale(&rec, &in, notes)

		if severity == SeverityCritical || severity == SeverityWarning || rec.Code == "MISSING_CPU_REQUEST" || rec.Code == "MISSING_MEM_REQUEST" {
			if severity == SeverityCritical || severity == SeverityWarning {
				summary.ContainersOverProvisioned++
			}
			summary.MonthlyWasteUSD += rec.MonthlyWasteUSD
			summary.Recommendations = append(summary.Recommendations, rec)
		}
	}

	return summary
}

func scaleUp(v int64, factor float64) int64 {
	return int64(float64(v) * factor)
}

func roundUp(v, step int64) int64 {
	if step <= 0 || v <= 0 {
		return v
	}
	return ((v + step - 1) / step) * step
}

func ratioSeverity(request, p95 float64) string {
	if p95 <= 0 {
		return SeverityInfo
	}
	ratio := request / p95
	switch {
	case ratio >= criticalRatio:
		return SeverityCritical
	case ratio >= warningRatio:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

func worse(a, b string) string {
	rank := map[string]int{SeverityInfo: 0, SeverityWarning: 1, SeverityCritical: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func buildRationale(rec *Recommendation, in *ContainerInput, notes []string) string {
	r := "Suggested requests are derived from observed p95 usage with a safety headroom; no configuration is changed. "
	if rec.Severity == SeverityCritical || rec.Severity == SeverityWarning {
		r += "Current request is significantly above observed usage, indicating reclaimable capacity and cost. "
	}
	for _, n := range notes {
		r += n + ". "
	}
	return r
}

func formatUSD(v float64) string {
	// two decimals
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	ints := int64(v)
	frac := int64((v - float64(ints)) * 100)
	var buf [24]byte
	i := len(buf)
	if frac == 0 {
		i--
		buf[i] = '0'
		i--
		buf[i] = '.'
		i--
		buf[i] = '0'
	} else {
		i--
		buf[i] = byte('0' + frac%10)
		frac /= 10
		i--
		buf[i] = byte('0' + frac%10)
		i--
		buf[i] = '.'
	}
	if ints == 0 {
		i--
		buf[i] = '0'
	} else {
		for ints > 0 {
			i--
			buf[i] = byte('0' + ints%10)
			ints /= 10
		}
	}
	return sign + string(buf[i:])
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
