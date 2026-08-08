package kubernetes

import (
	"strconv"
	"testing"
)

// FuzzParseRevision exercises the deployment rollback revision parser. It
// must never panic and round-trip every parseable decimal through strconv.
func FuzzParseRevision(f *testing.F) {
	for _, seed := range []string{"", "0", "1", "42", "-1", "abc", "1e3", "2147483647", "2147483648"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		value := parseRevision(raw)
		if raw == "" && value != 0 {
			t.Fatalf("empty raw should yield 0, got %d", value)
		}
		if raw == "" {
			return
		}
		if parsed, err := strconv.ParseInt(raw, 10, 32); err == nil && int32(parsed) != value {
			t.Fatalf("parseRevision(%q)=%d but strconv gives %d", raw, value, parsed)
		}
	})
}
