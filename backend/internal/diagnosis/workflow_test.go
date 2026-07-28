package diagnosis

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{from: "open", to: "confirmed", want: true},
		{from: "open", to: "dismissed", want: true},
		{from: "confirmed", to: "resolved", want: true},
		{from: "resolved", to: "open", want: true},
		{from: "dismissed", to: "open", want: true},
		{from: "open", to: "resolved", want: false},
		{from: "resolved", to: "confirmed", want: false},
		{from: "open", to: "open", want: false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Fatalf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestValidFeedbackVerdict(t *testing.T) {
	for _, verdict := range []string{"accurate", "inaccurate", "uncertain"} {
		if !ValidFeedbackVerdict(verdict) {
			t.Fatalf("verdict %q is invalid", verdict)
		}
	}
	if ValidFeedbackVerdict("helpful") {
		t.Fatal("unexpected verdict accepted")
	}
}
