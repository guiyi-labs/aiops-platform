package netpolicy

import (
	"testing"
	"time"
)

var testTime = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func findingsWithCode(status Status, code string) []Finding {
	out := make([]Finding, 0, 2)
	for _, f := range status.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func mustFinding(t *testing.T, status Status, code string) Finding {
	t.Helper()
	got := findingsWithCode(status, code)
	if len(got) != 1 {
		t.Fatalf("expected exactly one %s finding, got %d (all: %v)", code, len(got), codes(status))
	}
	return got[0]
}

func mustNoFinding(t *testing.T, status Status, code string) {
	t.Helper()
	if got := findingsWithCode(status, code); len(got) != 0 {
		t.Fatalf("expected no %s finding, got %d", code, len(got))
	}
}

func codes(status Status) []string {
	out := make([]string, 0, len(status.Findings))
	for _, f := range status.Findings {
		out = append(out, f.Code)
	}
	return out
}

func pod(ns, name string, labels map[string]string, ports ...ContainerPort) PodInfo {
	return PodInfo{Namespace: ns, Name: name, UID: "uid-" + name, Labels: labels, Ports: ports}
}

func TestEvaluateEmptyInputsProducesNothing(t *testing.T) {
	status := Evaluate(7, Inputs{}, testTime)

	if status.ClusterID != 7 {
		t.Fatalf("cluster id = %d, want 7", status.ClusterID)
	}
	if status.Total != 0 || status.Failed != 0 || status.Passed != 0 {
		t.Fatalf("expected an empty rollup, got total=%d failed=%d passed=%d", status.Total, status.Failed, status.Passed)
	}
	if status.Findings == nil {
		t.Fatal("findings must be a non-nil empty slice so it serializes as [] rather than null")
	}
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings, got %v", codes(status))
	}
}

func TestNamespaceWithoutDefaultDenyIsFlagged(t *testing.T) {
	in := Inputs{
		Namespaces: []NamespaceInfo{{Name: "shop"}, {Name: "kube-system"}, {Name: "empty"}},
		Pods: []PodInfo{
			pod("shop", "web-1", map[string]string{"app": "web"}),
			pod("kube-system", "coredns-1", map[string]string{"k8s-app": "kube-dns"}),
		},
	}

	status := Evaluate(1, in, testTime)

	found := findingsWithCode(status, CodeNamespaceNoDefaultDeny)
	if len(found) != 2 {
		t.Fatalf("expected findings for shop and kube-system, got %d", len(found))
	}
	bySeverity := map[string]string{}
	for _, f := range found {
		bySeverity[f.Resource.Name] = f.Severity
	}
	if bySeverity["shop"] != SeverityWarning {
		t.Fatalf("application namespace severity = %q, want warning", bySeverity["shop"])
	}
	if bySeverity["kube-system"] != SeverityInfo {
		t.Fatalf("system namespace severity = %q, want info (demoted to reduce noise)", bySeverity["kube-system"])
	}
	// A namespace with no pods has nothing to protect and must not be counted.
	if status.Total != 2 {
		t.Fatalf("total checks = %d, want 2 (the empty namespace must be skipped)", status.Total)
	}
	if status.IsolatedNamespaces != 0 {
		t.Fatalf("isolated namespaces = %d, want 0", status.IsolatedNamespaces)
	}
}

func TestDefaultDenyBaselineSilencesNamespaceAndPodChecks(t *testing.T) {
	in := Inputs{
		Namespaces: []NamespaceInfo{{Name: "shop"}},
		Pods:       []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "default-deny",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress"},
		}},
	}

	status := Evaluate(1, in, testTime)

	mustNoFinding(t, status, CodeNamespaceNoDefaultDeny)
	mustNoFinding(t, status, CodePodIngressUnrestricted)
	mustNoFinding(t, status, CodePolicySelectsNoPods)
	if status.IsolatedNamespaces != 1 {
		t.Fatalf("isolated namespaces = %d, want 1", status.IsolatedNamespaces)
	}
	if status.IngressCoveredPods != 1 {
		t.Fatalf("ingress covered pods = %d, want 1", status.IngressCoveredPods)
	}
	if status.EgressCoveredPods != 0 {
		t.Fatalf("egress covered pods = %d, want 0 (policy declares Ingress only)", status.EgressCoveredPods)
	}
}

func TestPartiallyCoveredNamespaceFlagsTheMissedPod(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{
			pod("shop", "web-1", map[string]string{"app": "web"}),
			pod("shop", "cache-1", map[string]string{"app": "cache"}),
		},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "web-only",
			PodSelector: Selector{MatchLabels: map[string]string{"app": "web"}},
			PolicyTypes: []string{"Ingress"},
			Ingress:     []Rule{{Peers: []Peer{{PodSelector: &Selector{MatchLabels: map[string]string{"app": "gw"}}}}}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodePodIngressUnrestricted)
	if f.Resource.Name != "cache-1" {
		t.Fatalf("unprotected pod = %q, want cache-1", f.Resource.Name)
	}
	if f.Details["family"] != FamilyCoverage {
		t.Fatalf("family = %q, want %q", f.Details["family"], FamilyCoverage)
	}
	if status.IngressCoveredPods != 1 {
		t.Fatalf("ingress covered pods = %d, want 1", status.IngressCoveredPods)
	}
}

func TestMatchExpressionsNeverProduceAFalseUncoveredAlarm(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "cache-1", map[string]string{"app": "cache"})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "expression-policy",
			PodSelector: Selector{HasExpressions: true},
			PolicyTypes: []string{"Ingress"},
			Ingress:     []Rule{{Peers: []Peer{{PodSelector: &Selector{MatchLabels: map[string]string{"app": "gw"}}}}}},
		}},
	}

	status := Evaluate(1, in, testTime)

	// The selector is not evaluated, so coverage is assumed and the dead
	// policy check is skipped rather than guessed at.
	mustNoFinding(t, status, CodePodIngressUnrestricted)
	mustNoFinding(t, status, CodePolicySelectsNoPods)
}

func TestPolicySelectingNoPodsIsFlaggedAsDead(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"})},
		Policies: []Policy{
			{
				Namespace:   "shop",
				Name:        "default-deny",
				PodSelector: Selector{},
				PolicyTypes: []string{"Ingress"},
			},
			{
				Namespace:   "shop",
				Name:        "typo-policy",
				PodSelector: Selector{MatchLabels: map[string]string{"app": "wbe"}},
				PolicyTypes: []string{"Ingress"},
				Ingress:     []Rule{{Ports: []PortRule{{Port: "8080"}}}},
			},
		},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodePolicySelectsNoPods)
	if f.Resource.Name != "typo-policy" {
		t.Fatalf("dead policy = %q, want typo-policy", f.Resource.Name)
	}
	if f.Details["pod_selector"] != "app=wbe" {
		t.Fatalf("pod_selector detail = %q, want app=wbe", f.Details["pod_selector"])
	}
	if f.Details["family"] != FamilyPolicyHygiene {
		t.Fatalf("family = %q, want %q", f.Details["family"], FamilyPolicyHygiene)
	}
}

func TestAllowAllAndWideOpenIngressRulesAreFlagged(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "sloppy",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress"},
			Ingress: []Rule{
				{Ports: []PortRule{{Port: "80"}}},                 // no peers -> allow all sources
				{Peers: []Peer{{NamespaceSelector: &Selector{}}}}, // every namespace
				{Peers: []Peer{{IPBlockCIDR: "0.0.0.0/0"}}},       // the whole internet
				{Peers: []Peer{{IPBlockCIDR: "10.0.0.0/8"}}},      // scoped, fine
			},
		}},
	}

	status := Evaluate(1, in, testTime)

	allowAll := mustFinding(t, status, CodeIngressAllowAll)
	if allowAll.Details["rule_index"] != "0" {
		t.Fatalf("allow-all rule index = %q, want 0", allowAll.Details["rule_index"])
	}
	if allowAll.Details["ports"] != "TCP/80" {
		t.Fatalf("allow-all ports detail = %q, want TCP/80", allowAll.Details["ports"])
	}
	fromAll := mustFinding(t, status, CodeIngressFromAllNS)
	if fromAll.Details["rule_index"] != "1" {
		t.Fatalf("from-all-namespaces rule index = %q, want 1", fromAll.Details["rule_index"])
	}
	wide := mustFinding(t, status, CodeWideIPBlock)
	if wide.Severity != SeverityWarning || wide.Details["direction"] != "ingress" {
		t.Fatalf("wide ipBlock finding = %+v, want warning/ingress", wide.Details)
	}
}

func TestExceptedIPBlockIsNotFlagged(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "scoped",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress"},
			Ingress:     []Rule{{Peers: []Peer{{IPBlockCIDR: "0.0.0.0/0", IPBlockExcept: []string{"169.254.169.254/32"}}}}},
		}},
	}

	status := Evaluate(1, in, testTime)

	mustNoFinding(t, status, CodeWideIPBlock)
}

func TestUnrestrictedEgressIsReportedAsInformational(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "egress-open",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress", "Egress"},
			Egress:      []Rule{{Peers: []Peer{{IPBlockCIDR: "0.0.0.0/0"}}}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeWideIPBlock)
	if f.Severity != SeverityInfo {
		t.Fatalf("egress wide ipBlock severity = %q, want info", f.Severity)
	}
	if f.Details["direction"] != "egress" {
		t.Fatalf("direction = %q, want egress", f.Details["direction"])
	}
	if status.EgressCoveredPods != 1 {
		t.Fatalf("egress covered pods = %d, want 1", status.EgressCoveredPods)
	}
}

func TestServiceWithoutBackendsIsCritical(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"})},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Type:      "ClusterIP",
			Selector:  map[string]string{"app": "webb"},
			Ports:     []ServicePort{{Port: 80, TargetPort: "8080"}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeServiceNoBackends)
	if f.Severity != SeverityCritical {
		t.Fatalf("severity = %q, want critical", f.Severity)
	}
	if f.Details["selector"] != "app=webb" {
		t.Fatalf("selector detail = %q, want app=webb", f.Details["selector"])
	}
	// Port checks must not run without backends.
	mustNoFinding(t, status, CodeServicePortUnmatched)
}

func TestSelectorlessServiceIsSkipped(t *testing.T) {
	in := Inputs{
		Services: []ServiceInfo{
			{Namespace: "shop", Name: "external-db", Type: "ClusterIP", Ports: []ServicePort{{Port: 5432}}},
			{Namespace: "shop", Name: "cname", Type: "ExternalName", ExternalName: "db.example.com"},
		},
	}

	status := Evaluate(1, in, testTime)

	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings for endpoint-managed and ExternalName services, got %v", codes(status))
	}
}

func TestNamedTargetPortThatResolvesNowhereIsCritical(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{Name: "http", ContainerPort: 8080})},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Selector:  map[string]string{"app": "web"},
			Ports:     []ServicePort{{Port: 443, TargetPort: "https"}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeServicePortUnmatched)
	if f.Severity != SeverityCritical {
		t.Fatalf("named target port severity = %q, want critical", f.Severity)
	}
	if f.Details["target_port"] != "https" {
		t.Fatalf("target_port = %q, want https", f.Details["target_port"])
	}
}

func TestNumericTargetPortNotDeclaredIsInformational(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{Name: "http", ContainerPort: 8080})},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Selector:  map[string]string{"app": "web"},
			Ports:     []ServicePort{{Port: 80, TargetPort: "9090"}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeServicePortUnmatched)
	if f.Severity != SeverityInfo {
		t.Fatalf("numeric target port severity = %q, want info (kubelet still forwards if the process listens)", f.Severity)
	}
}

func TestTargetPortDefaultsToServicePort(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{ContainerPort: 80})},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Selector:  map[string]string{"app": "web"},
			Ports:     []ServicePort{{Port: 80}},
		}},
	}

	status := Evaluate(1, in, testTime)

	mustNoFinding(t, status, CodeServicePortUnmatched)
}

func TestDefaultDenyWithoutAnAllowRuleBlocksTheServicePort(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{Name: "http", ContainerPort: 8080})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "default-deny",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress"},
		}},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Selector:  map[string]string{"app": "web"},
			Ports:     []ServicePort{{Port: 80, TargetPort: "http"}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeServicePortBlocked)
	if f.Details["resolved_port"] != "8080" {
		t.Fatalf("resolved_port = %q, want 8080 (named port resolved from the backend)", f.Details["resolved_port"])
	}
	if f.Details["policies"] != "default-deny" {
		t.Fatalf("policies = %q, want default-deny", f.Details["policies"])
	}
	if f.Details["family"] != FamilyReachability {
		t.Fatalf("family = %q, want %q", f.Details["family"], FamilyReachability)
	}
}

func TestAnAllowRuleOnTheRightPortClearsTheBlockedFinding(t *testing.T) {
	base := func(allowPort string) Inputs {
		return Inputs{
			Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{Name: "http", ContainerPort: 8080})},
			Policies: []Policy{
				{Namespace: "shop", Name: "default-deny", PodSelector: Selector{}, PolicyTypes: []string{"Ingress"}},
				{
					Namespace:   "shop",
					Name:        "allow-gw",
					PodSelector: Selector{MatchLabels: map[string]string{"app": "web"}},
					PolicyTypes: []string{"Ingress"},
					Ingress: []Rule{{
						Peers: []Peer{{PodSelector: &Selector{MatchLabels: map[string]string{"app": "gw"}}}},
						Ports: []PortRule{{Port: allowPort}},
					}},
				},
			},
			Services: []ServiceInfo{{
				Namespace: "shop",
				Name:      "web",
				Selector:  map[string]string{"app": "web"},
				Ports:     []ServicePort{{Port: 80, TargetPort: "8080"}},
			}},
		}
	}

	mustNoFinding(t, Evaluate(1, base("8080"), testTime), CodeServicePortBlocked)

	blocked := Evaluate(1, base("9090"), testTime)
	if len(findingsWithCode(blocked, CodeServicePortBlocked)) != 1 {
		t.Fatalf("expected the mismatched allow port to still be reported as blocked, got %v", codes(blocked))
	}
}

func TestPortRangeAndNamedPolicyPortsAreHonoured(t *testing.T) {
	makeInputs := func(rule PortRule) Inputs {
		return Inputs{
			Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{Name: "http", ContainerPort: 8080})},
			Policies: []Policy{{
				Namespace:   "shop",
				Name:        "allow",
				PodSelector: Selector{},
				PolicyTypes: []string{"Ingress"},
				Ingress:     []Rule{{Peers: []Peer{{PodSelector: &Selector{}}}, Ports: []PortRule{rule}}},
			}},
			Services: []ServiceInfo{{
				Namespace: "shop",
				Name:      "web",
				Selector:  map[string]string{"app": "web"},
				Ports:     []ServicePort{{Port: 80, TargetPort: "http"}},
			}},
		}
	}

	for name, rule := range map[string]PortRule{
		"range":    {Port: "8000", EndPort: 8090},
		"named":    {Port: "http"},
		"wildcard": {},
	} {
		t.Run(name, func(t *testing.T) {
			mustNoFinding(t, Evaluate(1, makeInputs(rule), testTime), CodeServicePortBlocked)
		})
	}
}

func TestExposedServiceWithoutIngressPolicyIsCritical(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{ContainerPort: 8080})},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Type:      "NodePort",
			Selector:  map[string]string{"app": "web"},
			Ports:     []ServicePort{{Port: 80, TargetPort: "8080", NodePort: 31080}},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeExposedUnrestricted)
	if f.Severity != SeverityCritical {
		t.Fatalf("severity = %q, want critical", f.Severity)
	}
	if f.Details["node_ports"] != "31080" {
		t.Fatalf("node_ports = %q, want 31080", f.Details["node_ports"])
	}
	if status.ExposedServices != 1 {
		t.Fatalf("exposed services = %d, want 1", status.ExposedServices)
	}
	if f.Details["family"] != FamilyExposure {
		t.Fatalf("family = %q, want %q", f.Details["family"], FamilyExposure)
	}
}

func TestExposedServiceWithIngressPolicyIsAccepted(t *testing.T) {
	in := Inputs{
		Pods: []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{ContainerPort: 8080})},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "allow-8080",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress"},
			Ingress:     []Rule{{Peers: []Peer{{IPBlockCIDR: "10.0.0.0/8"}}, Ports: []PortRule{{Port: "8080"}}}},
		}},
		Services: []ServiceInfo{{
			Namespace: "shop",
			Name:      "web",
			Type:      "LoadBalancer",
			Selector:  map[string]string{"app": "web"},
			Ports:     []ServicePort{{Port: 80, TargetPort: "8080"}},
		}},
	}

	status := Evaluate(1, in, testTime)

	mustNoFinding(t, status, CodeExposedUnrestricted)
	mustNoFinding(t, status, CodeServicePortBlocked)
}

func TestHostNetworkPodUnderPolicyIsReported(t *testing.T) {
	hostPod := pod("shop", "agent-1", map[string]string{"app": "agent"})
	hostPod.HostNetwork = true
	in := Inputs{
		Pods: []PodInfo{hostPod},
		Policies: []Policy{{
			Namespace:   "shop",
			Name:        "default-deny",
			PodSelector: Selector{},
			PolicyTypes: []string{"Ingress"},
		}},
	}

	status := Evaluate(1, in, testTime)

	f := mustFinding(t, status, CodeHostNetworkIneffective)
	if f.Severity != SeverityInfo {
		t.Fatalf("severity = %q, want info", f.Severity)
	}
	if f.Resource.Name != "agent-1" {
		t.Fatalf("resource = %q, want agent-1", f.Resource.Name)
	}
}

func TestRollupCountersAndOrdering(t *testing.T) {
	in := Inputs{
		Namespaces: []NamespaceInfo{{Name: "shop"}},
		Pods:       []PodInfo{pod("shop", "web-1", map[string]string{"app": "web"}, ContainerPort{ContainerPort: 8080})},
		Services: []ServiceInfo{
			{Namespace: "shop", Name: "web", Type: "NodePort", Selector: map[string]string{"app": "web"}, Ports: []ServicePort{{Port: 80, TargetPort: "8080", NodePort: 31080}}},
			{Namespace: "shop", Name: "orphan", Selector: map[string]string{"app": "gone"}, Ports: []ServicePort{{Port: 80}}},
		},
	}

	status := Evaluate(42, in, testTime)

	if status.NamespacesTotal != 1 || status.PodsTotal != 1 || status.ServicesTotal != 2 || status.PoliciesTotal != 0 {
		t.Fatalf("inventory counters wrong: %+v", status)
	}
	if status.Passed != status.Total-status.Failed {
		t.Fatalf("passed=%d does not equal total-failed (%d-%d)", status.Passed, status.Total, status.Failed)
	}
	if status.Failed != len(status.Findings) {
		t.Fatalf("failed=%d but %d findings", status.Failed, len(status.Findings))
	}
	if status.BySeverity[SeverityCritical] != 2 {
		t.Fatalf("critical count = %d, want 2 (orphan service + unrestricted NodePort)", status.BySeverity[SeverityCritical])
	}
	if status.ByFamily[FamilyReachability] != 1 || status.ByFamily[FamilyExposure] != 1 || status.ByFamily[FamilyCoverage] != 1 {
		t.Fatalf("by family rollup wrong: %v", status.ByFamily)
	}
	// Most actionable first: criticals lead.
	if status.Findings[0].Severity != SeverityCritical {
		t.Fatalf("findings are not severity-ordered: %v", codes(status))
	}
	if !status.EvaluatedAt.Equal(testTime) {
		t.Fatalf("evaluated at = %v, want %v", status.EvaluatedAt, testTime)
	}
	for _, f := range status.Findings {
		if f.ObservedAt != "2026-08-01T12:00:00Z" {
			t.Fatalf("observed at = %q, want RFC3339 UTC", f.ObservedAt)
		}
	}
}

func TestPolicyTypeDefaultsFollowKubernetes(t *testing.T) {
	ingressOnly := Policy{Name: "p"}
	if !ingressOnly.AppliesToIngress() {
		t.Fatal("a policy without policyTypes always restricts ingress")
	}
	if ingressOnly.AppliesToEgress() {
		t.Fatal("a policy without policyTypes and without egress rules must not restrict egress")
	}
	withEgress := Policy{Name: "p", Egress: []Rule{{}}}
	if !withEgress.AppliesToEgress() {
		t.Fatal("egress rules imply the Egress policy type when policyTypes is omitted")
	}
}

func TestInputsEmpty(t *testing.T) {
	if !(Inputs{}).Empty() {
		t.Fatal("zero Inputs must report empty")
	}
	if (Inputs{Pods: []PodInfo{{Name: "x"}}}).Empty() {
		t.Fatal("Inputs with pods must not report empty")
	}
}
