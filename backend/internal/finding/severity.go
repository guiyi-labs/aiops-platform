package finding

// SeverityRank ranks the canonical severities so analyzers share one ordering
// (critical first). The closed set is {critical:0, warning:1, info:2}; any
// unknown or unnormalized severity ranks as info so it never out-ranks a real
// observation (matches the posture risk-ordering fuzz contract).
func SeverityRank(sev string) int {
	switch NormalizeSeverity(sev) {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// NormalizeSeverity maps analyzer-specific severity vocabularies onto the
// canonical info / warning / critical set used across posture, optimization,
// diagnosis and inspection. Diagnosis records keep their own vocabulary
// (high/medium/low) for API compatibility; this helper converts it only when
// a consumer needs the unified scale.
func NormalizeSeverity(sev string) string {
	switch sev {
	case "high":
		return SeverityCritical
	case "medium":
		return SeverityWarning
	case "low":
		return SeverityInfo
	case SeverityCritical, SeverityWarning, SeverityInfo:
		return sev
	default:
		return SeverityInfo
	}
}

// MaxSeverity returns the more severe of two severities using the canonical
// ordering and NormalizeSeverity mapping. Returns b when both normalize equal.
func MaxSeverity(a, b string) string {
	if SeverityRank(a) < SeverityRank(b) {
		return a
	}
	return b
}
