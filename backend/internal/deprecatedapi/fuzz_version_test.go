package deprecatedapi

import (
	"strings"
	"testing"
)

// FuzzParseAPIVersion exercises apiVersion splitting + the catalog lookup.
// The parser must never panic; its split must round-trip the original input.
func FuzzParseAPIVersion(f *testing.F) {
	for _, seed := range []string{
		"v1", "apps/v1", "extensions/v1beta1", "networking.k8s.io/v1",
		"/", "a/b/c", "v1beta1", "", "unknown.io/v1/v2",
	} {
		f.Add(seed, "Deployment")
		f.Add(seed, "Ingress")
		f.Add(seed, "")
	}
	f.Fuzz(func(t *testing.T, apiVersion, kind string) {
		group, version := parseAPIVersion(apiVersion)
		if strings.Contains(apiVersion, "/") {
			if group+"/"+version != apiVersion {
				t.Fatalf("split %q -> %q + %q does not round-trip", apiVersion, group, version)
			}
		} else if group != "" || version != apiVersion {
			t.Fatalf("bare %q split to %q + %q", apiVersion, group, version)
		}
		if minorVersion(version) < 0 {
			t.Fatalf("negative minor version for %q", version)
		}
		Lookup(apiVersion, kind)
	})
}

// FuzzMinorVersion exercises the minor-version extractor used when comparing
// server versions against catalog removal thresholds.
func FuzzMinorVersion(f *testing.F) {
	for _, seed := range []string{"v1.1", "1.22", "v2", "1.22.3", "v1", "22.5", "not-a-version", "", "v1_22"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		value := minorVersion(raw)
		if value < 0 {
			t.Fatalf("negative minor version %d for %q", value, raw)
		}
	})
}
