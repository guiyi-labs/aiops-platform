package kubernetes

// Supplementary coverage for pure mapping / path helpers in the read-only
// gateway service: rollout phase derivation and the promotion/namespaced
// resource paths. No cluster access is needed.

import "testing"

func TestDeriveRolloutPhase(t *testing.T) {
	phase, reason, msg := deriveRolloutPhase([]WorkloadCondition{
		{Type: "Progressing", Status: "False", Reason: "ProgressDeadlineExceeded", Message: "deadline"},
	}, 2, 1, 1, 1, 3)
	if phase != "failed" || reason != "ProgressDeadlineExceeded" || msg != "deadline" {
		t.Errorf("failed phase = %q,%q,%q", phase, reason, msg)
	}
	phase, reason, _ = deriveRolloutPhase([]WorkloadCondition{
		{Type: "Progressing", Status: "True", Reason: "newRSAvailable"},
	}, 2, 1, 2, 2, 3)
	if phase != "progressing" || reason != "newRSAvailable" {
		t.Errorf("progressing phase = %q,%q", phase, reason)
	}
	phase, reason, _ = deriveRolloutPhase([]WorkloadCondition{}, 3, 0, 3, 3, 3)
	if phase != "complete" || reason != "" {
		t.Errorf("complete phase = %q,%q", phase, reason)
	}
	phase, _, _ = deriveRolloutPhase([]WorkloadCondition{}, 2, 0, 3, 3, 3)
	if phase != "progressing" {
		t.Errorf("fallback phase = %q, want progressing", phase)
	}
}

func TestKindToGVR(t *testing.T) {
	cases := []struct {
		kind, group, version, resource string
		ok                             bool
	}{
		{KindDeployment, "apps", "v1", "deployments", true},
		{KindStatefulSet, "apps", "v1", "statefulsets", true},
		{KindDaemonSet, "apps", "v1", "daemonsets", true},
		{KindCronJob, "batch", "v1", "cronjobs", true},
		{KindService, "", "v1", "services", true},
		{KindIngress, "networking.k8s.io", "v1", "ingresses", true},
		{KindServiceAccount, "", "v1", "serviceaccounts", true},
		{KindConfigMap, "", "v1", "configmaps", true},
		{KindSecret, "", "v1", "secrets", true},
		{"Bogus", "", "", "", false},
	}
	for _, c := range cases {
		g, v, r, ok := KindToGVR(c.kind)
		if ok != c.ok || g != c.group || v != c.version || r != c.resource {
			t.Errorf("KindToGVR(%q) = %q,%q,%q,%v, want %q,%q,%q,%v", c.kind, g, v, r, ok, c.group, c.version, c.resource, c.ok)
		}
	}
}

func TestValidPromotionKind(t *testing.T) {
	for _, kind := range []string{"Deployment", "Service", "Ingress"} {
		if !validPromotionKind(kind) {
			t.Errorf("validPromotionKind(%q) = false, want true", kind)
		}
	}
	if validPromotionKind("StatefulSet") {
		t.Error("StatefulSet should not be promotion-allowlisted")
	}
}

func TestPromotionPath(t *testing.T) {
	deploy, err := promotionPath("Deployment", "ns", "app")
	if err != nil || deploy != "/apis/apps/v1/namespaces/ns/deployments/app" {
		t.Errorf("deployment path = %q, %v", deploy, err)
	}
	svc, err := promotionPath("Service", "ns", "app")
	if err != nil || svc != "/api/v1/namespaces/ns/services/app" {
		t.Errorf("service path = %q, %v", svc, err)
	}
	ing, err := promotionPath("Ingress", "ns", "app")
	if err != nil || ing != "/apis/networking.k8s.io/v1/namespaces/ns/ingresses/app" {
		t.Errorf("ingress path = %q, %v", ing, err)
	}
	if _, err := promotionPath("Job", "ns", "app"); err == nil {
		t.Error("unsupported kind should error")
	}
}

func TestNamespacedAPIPath(t *testing.T) {
	core := namespacedAPIPath("", "v1", "services", "ns", "app")
	if core != "/api/v1/namespaces/ns/services/app" {
		t.Errorf("core path = %q", core)
	}
	grouped := namespacedAPIPath("apps", "v1", "deployments", "ns", "app")
	if grouped != "/apis/apps/v1/namespaces/ns/deployments/app" {
		t.Errorf("grouped path = %q", grouped)
	}
}
