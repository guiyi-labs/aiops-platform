package diagnosis

import "time"

func SLADeadline(severity string, observedAt time.Time) time.Time {
	var duration time.Duration
	switch severity {
	case "critical":
		duration = time.Hour
	case "high":
		duration = 4 * time.Hour
	case "warning":
		duration = 24 * time.Hour
	default:
		duration = 72 * time.Hour
	}
	return observedAt.UTC().Add(duration)
}
