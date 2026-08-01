package deprecatedapi

import "testing"

func TestParseAPIVersion(t *testing.T) {
	cases := []struct {
		in    string
		wantG string
		wantV string
	}{
		{"apps/v1", "apps", "v1"},
		{"networking.k8s.io/v1beta1", "networking.k8s.io", "v1beta1"},
		{"v1", "", "v1"},
		{"extensions/v1beta1", "extensions", "v1beta1"},
	}
	for _, c := range cases {
		g, v := parseAPIVersion(c.in)
		if g != c.wantG || v != c.wantV {
			t.Errorf("parseAPIVersion(%q) = (%q,%q), want (%q,%q)", c.in, g, v, c.wantG, c.wantV)
		}
	}
}

func TestMinorVersion(t *testing.T) {
	cases := map[string]int{"v1.22": 22, "1.25": 25, "v1": 0, "garbage": 0, "": 0}
	for in, want := range cases {
		if got := minorVersion(in); got != want {
			t.Errorf("minorVersion(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLookup(t *testing.T) {
	e, ok := Lookup("networking.k8s.io/v1beta1", "Ingress")
	if !ok {
		t.Fatal("expected to find networking.k8s.io/v1beta1 Ingress")
	}
	if e.RemovedIn != 22 {
		t.Errorf("Ingress removedIn = %d, want 22", e.RemovedIn)
	}
	if e.Replacement != "networking.k8s.io/v1" {
		t.Errorf("Ingress replacement = %q", e.Replacement)
	}

	if _, ok := Lookup("apps/v1", "Deployment"); ok {
		t.Error("apps/v1 Deployment should NOT be in the catalog")
	}
}

func TestCatalogIndexUnique(t *testing.T) {
	if len(catalog) != len(catalogIndex) {
		t.Errorf("catalog has duplicate keys: %d entries vs %d index", len(catalog), len(catalogIndex))
	}
}
