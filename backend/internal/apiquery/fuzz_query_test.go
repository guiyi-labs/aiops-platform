package apiquery

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// FuzzParseListQuery exercises the read-only list-query contract parser used
// by every paginated endpoint. Valid results must keep page/limit bounds and
// derive offset consistently; malformed input must return an error, never
// panic or return a zero-valued request that leaks into handlers.
func FuzzParseListQuery(f *testing.F) {
	seeds := []string{
		"", "page=1&limit=20", "page=0", "limit=0", "limit=101",
		"page=-3", "limit=abc", "ascending=true", "ascending=maybe",
		"sort_by=name", "sort_by=unknown", "page=2&limit=10&ascending=false",
		"name=web&label_selector=app.kubernetes.io/name%3Dnginx",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		req, err := http.NewRequest(http.MethodGet, "http://example.test/api/v1/resources?"+raw, nil)
		if err != nil {
			t.Skip("unbuildable request URL")
		}
		query, err := Parse(req, "name")
		if err != nil {
			return
		}
		if query.Page < 1 {
			t.Fatalf("page below 1: %d (raw=%q)", query.Page, raw)
		}
		if query.Limit < 1 || query.Limit > maxLimit {
			t.Fatalf("limit out of bounds: %d (raw=%q)", query.Limit, raw)
		}
		if query.Offset != (query.Page-1)*query.Limit {
			t.Fatalf("offset %d != (%d-1)*%d", query.Offset, query.Page, query.Limit)
		}
		if (query.Page-1)*query.Limit < 0 {
			t.Fatalf("overflow offset: %d", query.Offset)
		}
	})
}

// FuzzPositiveInt exercises the shared positive-integer decoder. It must
// never panic and must map every parseable value exactly once.
func FuzzPositiveInt(f *testing.F) {
	for _, seed := range []string{"", "1", "0", "-3", "42", "abc", "999999999999999999999999"} {
		f.Add(seed, 20, 100)
	}
	f.Fuzz(func(t *testing.T, raw string, fallback, max int) {
		if fallback <= 0 || max <= 0 {
			t.Skip("degenerate bounds")
		}
		value, err := positiveInt(raw, fallback)
		if err != nil {
			return
		}
		if raw == "" {
			if value != fallback {
				t.Fatalf("empty string should yield fallback %d, got %d", fallback, value)
			}
			return
		}
		if fmt.Sprintf("%d", value) != strings.TrimSpace(raw) && !strings.HasPrefix(raw, "-") {
			var parsed int
			if _, scanErr := fmt.Sscanf(raw, "%d", &parsed); scanErr == nil && parsed >= 1 {
				t.Fatalf("parsed %d for %q", value, raw)
			}
		}
	})
}
