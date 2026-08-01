package cis

import (
	"testing"
	"time"
)

func ptrBool(b bool) *bool    { return &b }
func ptrInt64(v int64) *int64 { return &v }

func TestEvaluateComponentCriticalFail(t *testing.T) {
	in := Inputs{
		Components: []ComponentConfig{
			{
				Component: "kube-apiserver",
				Flags: map[string]string{
					"anonymous-auth":     "true", // fails 1.2.1 (critical)
					"authorization-mode": "AlwaysAllow", // fails 1.2.6/1.2.7
					"profiling":          "false", // passes 1.2.12
				},
			},
		},
	}
	status := Evaluate(1, in, time.Now())
	if status.Failed < 3 {
		t.Fatalf("expected at least 3 component failures, got %d (findings=%d)", status.Failed, len(status.Findings))
	}
	if status.BySeverity[SeverityCritical] == 0 {
		t.Errorf("expected at least one critical finding, got bySeverity=%v", status.BySeverity)
	}

	var sawAnon, sawRBAC bool
	for _, f := range status.Findings {
		if f.Code == "CIS-1.2.1" {
			sawAnon = true
			if f.Severity != SeverityCritical {
				t.Errorf("CIS-1.2.1 should be critical, got %s", f.Severity)
			}
		}
		if f.Code == "CIS-1.2.7" {
			sawRBAC = true
		}
	}
	if !sawAnon || !sawRBAC {
		t.Errorf("missing expected findings: anon=%v rbac=%v", sawAnon, sawRBAC)
	}
}

func TestEvaluateComponentPassWhenUnset(t *testing.T) {
	// --anonymous-auth unset should PASS FlagShouldBeFalse (treated as false).
	in := Inputs{Components: []ComponentConfig{{Component: "kube-apiserver", Flags: map[string]string{}}}}
	status := Evaluate(1, in, time.Now())
	for _, f := range status.Findings {
		if f.Code == "CIS-1.2.1" {
			t.Errorf("CIS-1.2.1 should pass when unset, got finding %+v", f)
		}
	}
}

func TestEvaluateRBACClusterAdminNonSystem(t *testing.T) {
	in := Inputs{
		Bindings: []RBACBinding{
			{
				Kind: "ClusterRoleBinding", Name: "grant-dev", Namespace: "",
				RoleKind: "ClusterRole", RoleName: "cluster-admin",
				Subjects: []RBACSubject{{Kind: "User", Name: "alice"}},
			},
			{
				Kind: "ClusterRoleBinding", Name: "sys", Namespace: "",
				RoleKind: "ClusterRole", RoleName: "cluster-admin",
				Subjects: []RBACSubject{{Kind: "ServiceAccount", Name: "metrics-server", Namespace: "kube-system"}},
			},
		},
	}
	status := Evaluate(1, in, time.Now())
	var sawUser, sawSA bool
	for _, f := range status.Findings {
		if f.Code == "CIS-RBAC-CLUSTER-ADMIN" {
			if f.Resource.Name == "grant-dev" {
				sawUser = true
			}
			if f.Resource.Name == "sys" {
				sawSA = true
			}
		}
	}
	if !sawUser {
		t.Errorf("expected cluster-admin finding for non-system user binding")
	}
	if sawSA {
		t.Errorf("system:kube-system SA binding should NOT be flagged")
	}
}

func TestEvaluateRBACWildcard(t *testing.T) {
	in := Inputs{
		Bindings: []RBACBinding{
			{
				Kind: "RoleBinding", Name: "wide", Namespace: "default",
				RoleKind: "Role", RoleName: "wide-role",
				RoleRules: []PolicyRule{{Verbs: []string{"*"}, Resources: []string{"*"}}},
			},
		},
	}
	status := Evaluate(1, in, time.Now())
	var saw bool
	for _, f := range status.Findings {
		if f.Code == "CIS-RBAC-WILDCARD" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected wildcard RBAC finding")
	}
}

func TestEvaluateWorkloadPrivileged(t *testing.T) {
	in := Inputs{
		Workloads: []WorkloadSecurity{
			{
				Kind: "Pod", Namespace: "default", Name: "web",
				Containers: []ContainerSecurity{
					{Name: "app", Privileged: ptrBool(true)},
					{Name: "sidecar", Privileged: ptrBool(false), RunAsNonRoot: ptrBool(true)},
				},
			},
		},
	}
	status := Evaluate(1, in, time.Now())
	var sawPriv, sawSidecar bool
	for _, f := range status.Findings {
		if f.Code == "CIS-WL-PRIV" && f.Resource.Name == "web" {
			if f.Details["container"] == "app" {
				sawPriv = true
			}
			if f.Details["container"] == "sidecar" {
				sawSidecar = true
			}
		}
	}
	if !sawPriv {
		t.Errorf("expected privileged finding for app container")
	}
	if sawSidecar {
		t.Errorf("sidecar is not privileged; should not be flagged")
	}
}

func TestEvaluateWorkloadRunAsRoot(t *testing.T) {
	in := Inputs{
		Workloads: []WorkloadSecurity{
			{
				Kind: "Pod", Namespace: "default", Name: "r",
				Containers: []ContainerSecurity{
					{Name: "c", RunAsUser: ptrInt64(0)},
				},
			},
		},
	}
	status := Evaluate(1, in, time.Now())
	var saw bool
	for _, f := range status.Findings {
		if f.Code == "CIS-WL-RUN-AS-NON-ROOT" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected run-as-root finding for uid 0")
	}
}

func TestEvaluateNamespacePSA(t *testing.T) {
	in := Inputs{
		Namespaces: []NamespacePodSecurity{
			{Name: "prod", Enforce: "restricted"},
			{Name: "legacy", Enforce: "privileged"},
			{Name: "unset"},
		},
	}
	status := Evaluate(1, in, time.Now())
	var sawLegacy, sawUnset, sawProd bool
	for _, f := range status.Findings {
		if f.Code == "CIS-PSA-ENFORCE" {
			switch f.Resource.Name {
			case "legacy":
				sawLegacy = true
			case "unset":
				sawUnset = true
			case "prod":
				sawProd = true
			}
		}
	}
	if !sawLegacy || !sawUnset {
		t.Errorf("expected PSA findings for legacy/unset namespaces")
	}
	if sawProd {
		t.Errorf("prod enforce=restricted should not be flagged")
	}
}

func TestEvaluateEmptyInput(t *testing.T) {
	status := Evaluate(1, Inputs{}, time.Now())
	if status.Total != 0 || status.Failed != 0 {
		t.Errorf("empty input should yield zero totals, got total=%d failed=%d", status.Total, status.Failed)
	}
}

func TestFlagFailsCombinations(t *testing.T) {
	cases := []struct {
		name   string
		ctrl   ComponentControl
		present bool
		val    string
		want   bool
	}{
		{"should_be_false-unset", ComponentControl{Kind: FlagShouldBeFalse}, false, "", false},
		{"should_be_false-true", ComponentControl{Kind: FlagShouldBeFalse}, true, "true", true},
		{"should_be_false-false", ComponentControl{Kind: FlagShouldBeFalse}, true, "false", false},
		{"must_be_set-set", ComponentControl{Kind: FlagMustBeSet}, true, "x", false},
		{"must_be_set-unset", ComponentControl{Kind: FlagMustBeSet}, false, "", true},
		{"must_be_absent-present", ComponentControl{Kind: FlagMustBeAbsent}, true, "x", true},
		{"mode_include-ok", ComponentControl{Kind: FlagModeMustInclude, Params: FlagParams{Contains: []string{"Node", "RBAC"}}}, true, "Node,RBAC", false},
		{"mode_include-missing", ComponentControl{Kind: FlagModeMustInclude, Params: FlagParams{Contains: []string{"RBAC"}}}, true, "Node", true},
		{"not_equal-ok", ComponentControl{Kind: FlagMustNotEqual, Params: FlagParams{Disallow: []string{"AlwaysAllow"}}}, true, "Webhook", false},
		{"not_equal-bad", ComponentControl{Kind: FlagMustNotEqual, Params: FlagParams{Disallow: []string{"AlwaysAllow"}}}, true, "AlwaysAllow", true},
		{"equals-ok", ComponentControl{Kind: FlagEquals, Params: FlagParams{Allow: []string{"127.0.0.1"}}}, true, "127.0.0.1", false},
		{"equals-bad", ComponentControl{Kind: FlagEquals, Params: FlagParams{Allow: []string{"127.0.0.1"}}}, true, "0.0.0.0", true},
	}
	for _, c := range cases {
		if got := flagFails(c.ctrl, c.present, c.val); got != c.want {
			t.Errorf("%s: flagFails=%v want %v", c.name, got, c.want)
		}
	}
}
