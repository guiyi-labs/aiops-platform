package deprecatedapi

import (
	"testing"
	"time"
)

func obj(apiVersion, kind, ns, name string) ResourceObject {
	return ResourceObject{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name}
}

func TestCheckRemoved(t *testing.T) {
	objects := []ResourceObject{
		obj("extensions/v1beta1", "Ingress", "web", "legacy"),
		obj("networking.k8s.io/v1beta1", "Ingress", "web", "old"),
	}
	status := Check(7, "1.22", objects, time.Now())
	if status.Removed != 2 {
		t.Fatalf("expected 2 removed, got %d (deprecated=%d clean=%d)", status.Removed, status.Deprecated, status.Clean)
	}
	if len(status.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(status.Findings))
	}
	for _, f := range status.Findings {
		if f.Code != CodeRemovedAPI {
			t.Errorf("expected CodeRemovedAPI, got %s", f.Code)
		}
		if f.Severity != SeverityCritical {
			t.Errorf("expected critical severity, got %s", f.Severity)
		}
	}
}

func TestCheckDeprecatedBeforeRemoval(t *testing.T) {
	objects := []ResourceObject{
		obj("extensions/v1beta1", "Ingress", "web", "legacy"),
	}
	// Target 1.21: removed in 1.22, deprecation window starts ~1.19 -> deprecated.
	status := Check(7, "1.21", objects, time.Now())
	if status.Deprecated != 1 {
		t.Fatalf("expected 1 deprecated, got removed=%d deprecated=%d clean=%d", status.Removed, status.Deprecated, status.Clean)
	}
	if status.Findings[0].Code != CodeDeprecatedAPI {
		t.Errorf("expected CodeDeprecatedAPI, got %s", status.Findings[0].Code)
	}
}

func TestCheckCleanForModernAPI(t *testing.T) {
	objects := []ResourceObject{
		obj("apps/v1", "Deployment", "app", "api"),
		obj("networking.k8s.io/v1", "Ingress", "web", "new"),
		obj("batch/v1", "CronJob", "jobs", "nightly"),
	}
	status := Check(7, "1.29", objects, time.Now())
	if status.Clean != 3 || status.Removed != 0 || status.Deprecated != 0 {
		t.Fatalf("expected all clean, got %+v", status)
	}
}

func TestCheckCleanWhenTargetBeforeDeprecation(t *testing.T) {
	// flowcontrol v1beta1 removed in 1.29; deprecation window ~1.26.
	// Target 1.24 should be clean.
	objects := []ResourceObject{
		obj("flowcontrol.apiserver.k8s.io/v1beta1", "FlowSchema", "kube-system", "fs"),
	}
	status := Check(7, "1.24", objects, time.Now())
	if status.Clean != 1 {
		t.Fatalf("expected clean at 1.24, got %+v", status)
	}
}

func TestCheckMixedRollup(t *testing.T) {
	objects := []ResourceObject{
		obj("apps/v1", "Deployment", "app", "ok"),          // clean
		obj("extensions/v1beta1", "Ingress", "web", "bad"),  // removed at 1.22
		obj("batch/v1beta1", "CronJob", "jobs", "old"),      // removed at 1.25 -> deprecated at 1.22
	}
	status := Check(7, "1.22", objects, time.Now())
	if status.Total != 3 || status.Clean != 1 || status.Removed != 1 || status.Deprecated != 1 {
		t.Fatalf("unexpected rollup: %+v", status)
	}
}
