package insight

import "testing"

func TestResolvePodFinding(t *testing.T) {
	rb := Resolve(7, "image", "Pod", "demo", "web-7d9c6", "IMG-UNTAGGED")
	if rb.ClusterID != 7 || rb.Domain != "image" || rb.Kind != "Pod" {
		t.Fatalf("unexpected runbook head: %+v", rb)
	}
	if rb.Name != "web-7d9c6" || rb.Namespace != "demo" {
		t.Fatalf("resource citation lost: %+v", rb)
	}
	if !rb.ReadOnly {
		t.Fatal("runbook must be read-only")
	}
	if len(rb.Diagnoses) != 1 || rb.Diagnoses[0].ResourceKind != "Pod" {
		t.Fatalf("expected one Pod diagnosis route, got %+v", rb.Diagnoses)
	}
	if rb.AI == nil || rb.AI.Endpoint == "" {
		t.Fatal("expected an AI explanation entry when a diagnosis applies")
	}
	if len(rb.Operations) != 0 {
		t.Fatalf("Pod has no operation candidates, got %+v", rb.Operations)
	}
	if len(rb.Inspection) == 0 {
		t.Fatal("expected inspection corroboration for the image domain")
	}
}

func TestResolveDeploymentDomainNetwork(t *testing.T) {
	rb := Resolve(3, "network", "Deployment", "prod", "api", "NET-EXPOSE")
	route := rb.Diagnoses[0]
	if len(route.RuleIDs) != 1 || route.RuleIDs[0] != "deployment.replicas_unavailable.v1" {
		t.Fatalf("unexpected diagnosis rules: %+v", route.RuleIDs)
	}
	var sawIngress bool
	for _, r := range rb.Inspection {
		if r.RuleCode == "ingress_backend_unhealthy" {
			sawIngress = true
		}
	}
	if !sawIngress {
		t.Fatalf("expected ingress_backend_unhealthy corroboration: %+v", rb.Inspection)
	}
	if len(rb.Operations) != 4 {
		t.Fatalf("expected 4 deployment operation candidates, got %+v", rb.Operations)
	}
	for _, op := range rb.Operations {
		if !op.DryRunFirst {
			t.Fatalf("operation %s must be dry-run first", op.Action)
		}
	}
}

func TestResolveNodeCordonPreview(t *testing.T) {
	rb := Resolve(3, "capacity", "Node", "", "worker-1", "CAP-NODE")
	if len(rb.Diagnoses) != 1 || rb.Diagnoses[0].ResourceKind != "Node" {
		t.Fatalf("expected Node diagnosis, got %+v", rb.Diagnoses)
	}
	if len(rb.Operations) != 1 || rb.Operations[0].Action != "cordon" {
		t.Fatalf("expected cordon candidate, got %+v", rb.Operations)
	}
}

func TestResolveUnknownKindStillCarriesInspection(t *testing.T) {
	rb := Resolve(1, "cis", "CustomResource", "ns", "x", "X")
	if len(rb.Diagnoses) != 0 || rb.AI != nil {
		t.Fatalf("unknown kind must not get diagnosis/AI: %+v", rb)
	}
	if len(rb.Inspection) == 0 {
		t.Fatal("domain inspection mapping must still apply for unknown kinds")
	}
}

func TestResolveUnknownDomainNoInspection(t *testing.T) {
	rb := Resolve(1, "unknown_domain", "Deployment", "ns", "x", "X")
	if len(rb.Inspection) != 0 {
		t.Fatalf("unknown domain must yield no corroboration: %+v", rb.Inspection)
	}
	if len(rb.Operations) == 0 {
		t.Fatal("kind-driven operation candidates must survive an unknown domain")
	}
}
