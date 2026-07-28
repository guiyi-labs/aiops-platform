package deployment_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type kustomization struct {
	Resources []string `yaml:"resources"`
}

func TestKubernetesBaselineSecurityAndOperations(t *testing.T) {
	directory := filepath.Join(repositoryRoot(t), "deploy", "kubernetes")
	contents, err := os.ReadFile(filepath.Join(directory, "kustomization.yaml"))
	if err != nil {
		t.Fatalf("read kustomization: %v", err)
	}
	var configuration kustomization
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		t.Fatalf("parse kustomization: %v", err)
	}
	for _, resource := range configuration.Resources {
		if strings.Contains(resource, "secret") {
			t.Fatalf("secret template must not be included by default: %s", resource)
		}
	}

	resources := loadResources(t, directory, configuration.Resources)
	for _, required := range []string{
		"Namespace/aiops-system", "ConfigMap/backend-config",
		"StatefulSet/postgres", "Deployment/backend", "Deployment/frontend",
		"Service/postgres", "Service/backend", "Service/frontend",
		"Ingress/aiops", "NetworkPolicy/default-deny",
	} {
		if _, ok := resources[required]; !ok {
			t.Errorf("required resource %s is missing", required)
		}
	}

	for _, name := range []string{"backend", "frontend"} {
		deployment := requireResource(t, resources, "Deployment/"+name)
		podSpec := nestedMap(t, deployment, "spec", "template", "spec")
		if nestedMap(t, podSpec, "securityContext")["runAsNonRoot"] != true {
			t.Errorf("Deployment/%s must run as non-root", name)
		}
		container := firstContainer(t, podSpec)
		for _, field := range []string{"readinessProbe", "livenessProbe", "resources"} {
			if _, ok := container[field]; !ok {
				t.Errorf("Deployment/%s container is missing %s", name, field)
			}
		}
		security := nestedMap(t, container, "securityContext")
		if security["allowPrivilegeEscalation"] != false || security["readOnlyRootFilesystem"] != true {
			t.Errorf("Deployment/%s container security context is not restricted", name)
		}
		resourcesBlock := nestedMap(t, container, "resources")
		if _, ok := resourcesBlock["requests"]; !ok {
			t.Errorf("Deployment/%s container is missing resource requests", name)
		}
		if _, ok := resourcesBlock["limits"]; !ok {
			t.Errorf("Deployment/%s container is missing resource limits", name)
		}
	}

	backendService := requireResource(t, resources, "Service/backend")
	if nestedMap(t, backendService, "spec")["type"] != "ClusterIP" {
		t.Error("backend Service must remain ClusterIP")
	}
	annotations := nestedMap(t, backendService, "metadata", "annotations")
	if annotations["prometheus.io/path"] != "/metrics" || annotations["prometheus.io/scrape"] != "true" {
		t.Error("backend Service must retain internal Prometheus scrape annotations")
	}

	ingress := requireResource(t, resources, "Ingress/aiops")
	ingressText, err := yaml.Marshal(ingress)
	if err != nil {
		t.Fatalf("marshal ingress: %v", err)
	}
	if strings.Contains(string(ingressText), "name: backend") || !strings.Contains(string(ingressText), "name: frontend") {
		t.Error("Ingress must expose only the frontend Service")
	}

	defaultDeny := requireResource(t, resources, "NetworkPolicy/default-deny")
	policySpec := nestedMap(t, defaultDeny, "spec")
	selector := nestedMap(t, policySpec, "podSelector")
	if len(selector) != 0 {
		t.Error("default-deny NetworkPolicy must select every pod")
	}
	policyTypes, ok := policySpec["policyTypes"].([]any)
	if !ok || len(policyTypes) != 2 {
		t.Error("default-deny NetworkPolicy must cover ingress and egress")
	}
}

func loadResources(t *testing.T, directory string, files []string) map[string]map[string]any {
	t.Helper()
	resources := make(map[string]map[string]any)
	for _, file := range files {
		contents, err := os.ReadFile(filepath.Join(directory, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(contents), "CHANGE_ME") {
			t.Errorf("included resource %s contains a secret placeholder", file)
		}
		decoder := yaml.NewDecoder(strings.NewReader(string(contents)))
		for {
			var resource map[string]any
			err := decoder.Decode(&resource)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			if len(resource) == 0 {
				continue
			}
			kind, _ := resource["kind"].(string)
			metadata := nestedMap(t, resource, "metadata")
			name, _ := metadata["name"].(string)
			if kind == "" || name == "" {
				t.Fatalf("resource in %s has no kind/name", file)
			}
			key := kind + "/" + name
			if _, exists := resources[key]; exists {
				t.Fatalf("duplicate resource %s", key)
			}
			resources[key] = resource
		}
	}
	return resources
}

func requireResource(t *testing.T, resources map[string]map[string]any, key string) map[string]any {
	t.Helper()
	resource, ok := resources[key]
	if !ok {
		t.Fatalf("required resource %s is missing", key)
	}
	return resource
}

func nestedMap(t *testing.T, value map[string]any, path ...string) map[string]any {
	t.Helper()
	current := value
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object", strings.Join(path, "."))
		}
		current = next
	}
	return current
}

func firstContainer(t *testing.T, podSpec map[string]any) map[string]any {
	t.Helper()
	containers, ok := podSpec["containers"].([]any)
	if !ok || len(containers) == 0 {
		t.Fatal("pod spec has no containers")
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatal("container is not an object")
	}
	return container
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate deployment test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func TestSecretTemplateIsExplicitlyUnsafeToApply(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "deploy", "kubernetes", "secret.example.yaml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secret template: %v", err)
	}
	for _, marker := range []string{"CHANGE_ME_DATABASE_PASSWORD", "CHANGE_ME_AT_LEAST_32_RANDOM_CHARACTERS", "CHANGE_ME_BASE64_ENCODED_32_BYTE_KEY"} {
		if !strings.Contains(string(contents), marker) {
			t.Errorf("secret template is missing required marker %s", marker)
		}
	}
	if strings.Contains(string(contents), fmt.Sprintf("%s: %s", "POSTGRES_PASSWORD", "change_me")) {
		t.Error("secret template must not contain a usable development password")
	}
}
