package deployment_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestIsolatedDiagnosisE2EFixturesAreDeterministic(t *testing.T) {
	directory := filepath.Join(repositoryRoot(t), "deploy", "diagnosis-e2e")
	contents, err := os.ReadFile(filepath.Join(directory, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var configuration kustomization
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		t.Fatal(err)
	}
	resources := loadResources(t, directory, configuration.Resources)

	requireResource(t, resources, "Namespace/aiops-diagnosis-e2e")
	deployment := requireResource(t, resources, "Deployment/stalled-deployment")
	if nestedMap(t, deployment, "spec")["replicas"] != 2 {
		t.Fatalf("stalled Deployment must request exactly two replicas: %#v", nestedMap(t, deployment, "spec")["replicas"])
	}
	selector := nestedMap(t, deployment, "spec", "template", "spec", "nodeSelector")
	if selector["aiops.local/diagnosis-target"] != "never" {
		t.Fatalf("stalled Deployment must use the isolated impossible selector: %#v", selector)
	}

	node := requireResource(t, resources, "Node/synthetic-not-ready")
	if nestedMap(t, node, "spec")["unschedulable"] != true {
		t.Fatal("synthetic Node must remain unschedulable")
	}
	labels := nestedMap(t, node, "metadata", "labels")
	if labels["aiops.local/synthetic"] != "true" {
		t.Fatalf("synthetic Node marker is missing: %#v", labels)
	}
}
