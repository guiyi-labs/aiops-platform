package diagnosis

import (
	"testing"
	"time"
)

func TestSLADeadline(t *testing.T) {
	observed := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	tests := map[string]time.Duration{"critical": time.Hour, "high": 4 * time.Hour, "warning": 24 * time.Hour, "info": 72 * time.Hour}
	for severity, duration := range tests {
		if got := SLADeadline(severity, observed); !got.Equal(observed.Add(duration)) {
			t.Fatalf("SLADeadline(%q) = %s", severity, got)
		}
	}
}
