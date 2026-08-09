package netpolicy

import (
	"testing"
)

// FuzzParsePort exercises the exposed-port parser. It must never panic and
// every accepted port must fall in the valid TCP/UDP range.
func FuzzParsePort(f *testing.F) {
	for _, seed := range []string{"80", "443", "0", "-1", "65535", "65536", "abc", "", "1.5", " 8080 "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		port, ok := parsePort(raw)
		if !ok {
			return
		}
		if port <= 0 || port > 65535 {
			t.Fatalf("port %d out of range for %q", port, raw)
		}
	})
}
