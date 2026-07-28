package deployment_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDemoScenariosCoverDeterministicDiagnoses(t *testing.T) {
	directory := filepath.Join(repositoryRoot(t), "deploy", "demo-scenarios")
	contents, err := os.ReadFile(filepath.Join(directory, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var configuration kustomization
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		t.Fatal(err)
	}
	resources := loadResources(t, directory, configuration.Resources)

	for _, required := range []string{
		"Namespace/aiops-demo",
		"Deployment/healthy-nginx",
		"Deployment/image-pull-backoff",
		"Deployment/crash-loop-backoff",
		"Deployment/service-without-endpoints",
		"Service/service-without-endpoints",
		"Ingress/healthy-nginx",
		"PersistentVolumeClaim/demo-cache",
		"ConfigMap/demo-runtime-profile",
		"StatefulSet/demo-stateful",
		"DaemonSet/demo-node-agent",
		"ReplicaSet/demo-replica",
		"Job/demo-backup",
		"CronJob/demo-cleanup",
		"HorizontalPodAutoscaler/healthy-nginx",
		"ResourceQuota/demo-budget",
		"LimitRange/demo-storage-bounds",
		"Secret/demo-key-catalog",
		"PersistentVolumeClaim/m18-pending-pvc",
		"Ingress/m18-broken-ingress",
		"HorizontalPodAutoscaler/m18-saturated-hpa",
	} {
		requireResource(t, resources, required)
	}

	for _, name := range []string{"healthy-nginx", "service-without-endpoints"} {
		container := firstContainer(t, nestedMap(t, requireResource(t, resources, "Deployment/"+name), "spec", "template", "spec"))
		if container["image"] != "nginxinc/nginx-unprivileged:1.27-alpine" {
			t.Fatalf("Deployment/%s must use the non-root nginx image: %#v", name, container["image"])
		}
		ports, ok := container["ports"].([]any)
		if !ok || len(ports) != 1 || ports[0].(map[string]any)["containerPort"] != 8080 {
			t.Fatalf("Deployment/%s must expose the unprivileged nginx port 8080: %#v", name, ports)
		}
	}
	imagePull := firstContainer(t, nestedMap(t, requireResource(t, resources, "Deployment/image-pull-backoff"), "spec", "template", "spec"))
	if imagePull["imagePullPolicy"] != "Always" || imagePull["image"] != "nginx:aiops-demo-tag-does-not-exist" {
		t.Fatalf("image pull scenario must deterministically request a missing image: %#v", imagePull)
	}

	crashLoop := firstContainer(t, nestedMap(t, requireResource(t, resources, "Deployment/crash-loop-backoff"), "spec", "template", "spec"))
	command := stringSlice(t, crashLoop["command"])
	if !equalStrings(command, []string{"/bin/sh", "-c", "echo intentional crash for AIOps diagnosis >&2; exit 42"}) {
		t.Fatalf("crash-loop scenario must exit deterministically: %#v", command)
	}

	service := requireResource(t, resources, "Service/service-without-endpoints")
	selector := nestedMap(t, service, "spec", "selector")
	if selector["app.kubernetes.io/name"] != "intentionally-unmatched" {
		t.Fatalf("service scenario selector must remain intentionally unmatched: %#v", selector)
	}
	workload := requireResource(t, resources, "Deployment/service-without-endpoints")
	labels := nestedMap(t, workload, "spec", "template", "metadata", "labels")
	if labels["app.kubernetes.io/name"] == selector["app.kubernetes.io/name"] {
		t.Fatal("service-without-endpoints workload must not match the Service selector")
	}
}
