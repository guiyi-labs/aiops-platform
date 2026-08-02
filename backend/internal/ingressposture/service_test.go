package ingressposture

import (
	"testing"
	"time"
)

func testTime() time.Time {
	return time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC)
}

func healthyIngress() IngressInfo {
	return IngressInfo{
		Namespace:        "prod",
		Name:             "web",
		UID:              "u-web",
		IngressClassName: "nginx",
		Hosts:            []string{"web.example.com"},
		HasTLS:           true,
		Backends:         []ServiceRef{{Namespace: "prod", Name: "web-svc"}},
	}
}

// TestEvaluate_HealthyIngress_NoFindings: TLS + pinned class + resolvable
// backend produces zero findings.
func TestEvaluate_HealthyIngress_NoFindings(t *testing.T) {
	in := Inputs{
		Ingresses: []IngressInfo{healthyIngress()},
		Services:  []ServiceRef{{Namespace: "prod", Name: "web-svc"}},
	}
	s := Evaluate(7, in, testTime())

	if s.Failed != 0 {
		t.Fatalf("failed = %d, want 0; findings=%+v", s.Failed, s.Findings)
	}
	if s.IngressesTotal != 1 || s.NoTLSCount != 0 || s.DeadBackendCount != 0 {
		t.Fatalf("counters = %+v, want all zero", s)
	}
	// checks: TLS + wildcard + class + 1 backend = 4
	if s.Total != 4 {
		t.Fatalf("total = %d, want 4", s.Total)
	}
}

// TestEvaluate_NoTLS_NoClass_MissingBackend: the three warning/info rules fire.
func TestEvaluate_NoTLS_NoClass_MissingBackend(t *testing.T) {
	in := Inputs{
		Ingresses: []IngressInfo{{
			Namespace: "prod",
			Name:      "api",
			UID:       "u-api",
			Hosts:     []string{"api.example.com"},
			HasTLS:    false,
			Backends:  []ServiceRef{{Namespace: "prod", Name: "ghost"}},
		}},
		Services: []ServiceRef{},
	}
	s := Evaluate(7, in, testTime())

	if s.NoTLSCount != 1 {
		t.Fatalf("no_tls_count = %d, want 1", s.NoTLSCount)
	}
	if s.DeadBackendCount != 1 {
		t.Fatalf("dead_backend_count = %d, want 1", s.DeadBackendCount)
	}
	// findings: NO_TLS, NO_INGRESS_CLASS, BACKEND_SERVICE_MISSING
	if s.Failed != 3 {
		t.Fatalf("failed = %d, want 3; findings=%+v", s.Failed, s.Findings)
	}
	codes := map[string]bool{}
	for _, f := range s.Findings {
		codes[f.Code] = true
	}
	for _, want := range []string{CodeNoTLS, CodeNoIngressClass, CodeBackendServiceMissing} {
		if !codes[want] {
			t.Errorf("missing finding %s; all=%+v", want, s.Findings)
		}
	}
	// Warnings sort before infos; among warnings, code order applies.
	if s.Findings[0].Code != CodeBackendServiceMissing {
		t.Fatalf("first finding = %s, want %s", s.Findings[0].Code, CodeBackendServiceMissing)
	}
}

// TestEvaluate_WildcardHost: a wildcard host is informational.
func TestEvaluate_WildcardHost(t *testing.T) {
	in := Inputs{
		Ingresses: []IngressInfo{{
			Namespace: "prod", Name: "wild", UID: "u-wild",
			IngressClassName: "nginx",
			Hosts:            []string{"*.example.com"},
			HasTLS:           true,
		}},
		Services: []ServiceRef{},
	}
	s := Evaluate(7, in, testTime())

	sawWildcard := false
	for _, f := range s.Findings {
		if f.Code == CodeWildcardHost {
			sawWildcard = true
		}
	}
	if !sawWildcard {
		t.Fatalf("no wildcard finding; all=%+v", s.Findings)
	}
}

// TestEvaluate_BackendDedup: duplicate backends are checked once.
func TestEvaluate_BackendDedup(t *testing.T) {
	in := Inputs{
		Ingresses: []IngressInfo{{
			Namespace: "prod", Name: "dup", UID: "u-dup",
			IngressClassName: "nginx",
			Hosts:            []string{"dup.example.com"},
			HasTLS:           true,
			Backends: []ServiceRef{
				{Namespace: "prod", Name: "ghost"},
				{Namespace: "prod", Name: "ghost"},
			},
		}},
		Services: []ServiceRef{},
	}
	s := Evaluate(7, in, testTime())

	if s.DeadBackendCount != 1 {
		t.Fatalf("dead_backend_count = %d, want 1 (deduped)", s.DeadBackendCount)
	}
}

// TestEvaluate_EmptyInputs_EmptyResult: nothing to analyze means zero checks.
func TestEvaluate_EmptyInputs_EmptyResult(t *testing.T) {
	s := Evaluate(7, Inputs{}, testTime())

	if s.Total != 0 || s.Failed != 0 || s.Passed != 0 {
		t.Fatalf("empty input counters = %+v, want all zero", s)
	}
	if s.Findings == nil {
		t.Fatal("findings must serialize as [] rather than null")
	}
	if s.BySeverity == nil || s.ByFamily == nil {
		t.Fatal("maps must be initialized")
	}
}

// TestEvaluate_SortDeterministic: same input yields the same finding order.
func TestEvaluate_SortDeterministic(t *testing.T) {
	in := Inputs{Ingresses: []IngressInfo{
		{Namespace: "b", Name: "z", UID: "1", Hosts: []string{"z.example.com"}},
		{Namespace: "a", Name: "y", UID: "2", Hosts: []string{"y.example.com"}},
	}}
	a := Evaluate(7, in, testTime())
	b := Evaluate(7, in, testTime())

	if len(a.Findings) != len(b.Findings) {
		t.Fatalf("finding counts differ: %d vs %d", len(a.Findings), len(b.Findings))
	}
	for i := range a.Findings {
		if a.Findings[i].Code != b.Findings[i].Code || a.Findings[i].Resource.Name != b.Findings[i].Resource.Name {
			t.Fatalf("order mismatch at %d: %+v vs %+v", i, a.Findings[i], b.Findings[i])
		}
	}
}
