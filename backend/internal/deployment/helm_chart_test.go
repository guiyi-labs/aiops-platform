package deployment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHelmChartMetadataIsValid(t *testing.T) {
	chart := loadChartFile(t, "Chart.yaml")

	for _, key := range []string{"apiVersion", "name", "description", "type", "version", "appVersion"} {
		if _, ok := chart[key]; !ok {
			t.Errorf("Chart.yaml is missing required field %q", key)
		}
	}

	if chart["apiVersion"] != "v2" {
		t.Errorf("Chart.yaml apiVersion must be v2 for Helm 3, got %v", chart["apiVersion"])
	}
	if chart["type"] != "application" {
		t.Errorf("Chart.yaml type must be application, got %v", chart["type"])
	}
	if chart["name"] != "aiops-platform" {
		t.Errorf("Chart.yaml name must be aiops-platform, got %v", chart["name"])
	}
	if version, ok := chart["version"].(string); !ok || version == "" {
		t.Error("Chart.yaml version must be a non-empty string")
	}
	if appVersion, ok := chart["appVersion"].(string); !ok || appVersion == "" {
		t.Error("Chart.yaml appVersion must be a non-empty string")
	}
}

func TestHelmChartValuesStructureIsComplete(t *testing.T) {
	values := loadChartFile(t, "values.yaml")

	for _, key := range []string{"namespace", "backend", "frontend", "postgres", "ingress", "networkPolicies", "existingSecret"} {
		if _, ok := values[key]; !ok {
			t.Errorf("values.yaml is missing required top-level key %q", key)
		}
	}

	namespace, ok := values["namespace"].(map[string]any)
	if !ok {
		t.Fatalf("values.yaml namespace must be an object")
	}
	if namespace["create"] != true {
		t.Error("values.yaml namespace.create must default to true so the chart is self-contained")
	}
	if namespace["name"] != "aiops-system" {
		t.Errorf("values.yaml namespace.name must default to aiops-system, got %v", namespace["name"])
	}
	namespaceLabels, ok := namespace["labels"].(map[string]any)
	if !ok {
		t.Fatalf("values.yaml namespace.labels must be an object")
	}
	for _, label := range []string{
		"pod-security.kubernetes.io/enforce",
		"pod-security.kubernetes.io/audit",
		"pod-security.kubernetes.io/warn",
	} {
		if namespaceLabels[label] != "restricted" {
			t.Errorf("values.yaml namespace.labels[%q] must be restricted, got %v", label, namespaceLabels[label])
		}
	}

	for _, component := range []string{"backend", "frontend", "postgres"} {
		cfg, ok := values[component].(map[string]any)
		if !ok {
			t.Fatalf("values.yaml %s must be an object", component)
		}
		for _, key := range []string{"image", "resources"} {
			if _, ok := cfg[key]; !ok {
				t.Errorf("values.yaml %s is missing required key %q", component, key)
			}
		}
		image, ok := cfg["image"].(map[string]any)
		if !ok {
			t.Fatalf("values.yaml %s.image must be an object", component)
		}
		for _, key := range []string{"repository", "tag", "pullPolicy"} {
			if _, ok := image[key]; !ok {
				t.Errorf("values.yaml %s.image is missing required key %q", component, key)
			}
		}
		switch pullPolicy := image["pullPolicy"].(type) {
		case string:
			switch pullPolicy {
			case "Always", "IfNotPresent", "Never":
			default:
				t.Errorf("values.yaml %s.image.pullPolicy has invalid value %q", component, pullPolicy)
			}
		default:
			t.Errorf("values.yaml %s.image.pullPolicy must be a string", component)
		}
	}

	backend, _ := values["backend"].(map[string]any)
	if _, ok := backend["config"]; !ok {
		t.Error("values.yaml backend must expose a config object consumed by the ConfigMap template")
	}
	if _, ok := backend["replicas"]; !ok {
		t.Error("values.yaml backend must declare replicas")
	}

	postgres, _ := values["postgres"].(map[string]any)
	if _, ok := postgres["storage"]; !ok {
		t.Error("values.yaml postgres must declare storage for the volume claim")
	}

	ingress, _ := values["ingress"].(map[string]any)
	if ingress["enabled"] != true {
		t.Error("values.yaml ingress.enabled must default to true")
	}
	if _, ok := ingress["className"]; !ok {
		t.Error("values.yaml ingress must declare className")
	}
	if _, ok := ingress["host"]; !ok {
		t.Error("values.yaml ingress must declare host")
	}

	if existingSecret, ok := values["existingSecret"].(string); !ok || existingSecret == "" {
		t.Error("values.yaml existingSecret must be a non-empty string")
	}
}

func TestHelmChartValuesSchemaMatchesValues(t *testing.T) {
	chartDir := helmChartDir(t)
	schemaBytes, err := os.ReadFile(filepath.Join(chartDir, "values.schema.json"))
	if err != nil {
		t.Fatalf("read values.schema.json: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("parse values.schema.json: %v", err)
	}

	if schema["type"] != "object" {
		t.Errorf("values.schema.json root type must be object, got %v", schema["type"])
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("values.schema.json required must be an array")
	}
	requiredSet := make(map[string]bool, len(required))
	for _, item := range required {
		if name, ok := item.(string); ok {
			requiredSet[name] = true
		}
	}
	for _, key := range []string{"backend", "frontend", "postgres", "existingSecret"} {
		if !requiredSet[key] {
			t.Errorf("values.schema.json required must include %q", key)
		}
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("values.schema.json properties must be an object")
	}
	for _, key := range []string{"namespace", "backend", "frontend", "postgres", "ingress", "networkPolicies", "existingSecret"} {
		if _, ok := properties[key]; !ok {
			t.Errorf("values.schema.json properties is missing %q", key)
		}
	}
}

func TestHelmChartRequiredTemplatesExist(t *testing.T) {
	chartDir := helmChartDir(t)
	for _, template := range []string{
		"_helpers.tpl",
		"namespace.yaml",
		"service-accounts.yaml",
		"configmap.yaml",
		"postgres.yaml",
		"backend.yaml",
		"frontend.yaml",
		"ingress.yaml",
		"network-policies.yaml",
	} {
		if _, err := os.Stat(filepath.Join(chartDir, "templates", template)); err != nil {
			t.Errorf("required Helm template %s is missing: %v", template, err)
		}
	}
}

func TestHelmChartTemplatesRenderExpectedResources(t *testing.T) {
	chartDir := helmChartDir(t)
	expectedMarkers := map[string][]string{
		"namespace.yaml":        {"kind: Namespace", "name: {{ .Values.namespace.name }}"},
		"service-accounts.yaml": {"kind: ServiceAccount", "name: backend", "name: frontend", "name: postgres", "automountServiceAccountToken: false"},
		"configmap.yaml":        {"kind: ConfigMap", "name: backend-config"},
		"postgres.yaml":         {"kind: Service", "kind: StatefulSet", "name: postgres", "volumeClaimTemplates"},
		"backend.yaml":          {"kind: Service", "kind: Deployment", "name: backend", "prometheus.io/scrape"},
		"frontend.yaml":         {"kind: Service", "kind: Deployment", "name: frontend"},
		"ingress.yaml":          {"kind: Ingress", "name: aiops"},
		"network-policies.yaml": {"kind: NetworkPolicy", "name: default-deny", "name: postgres", "name: backend", "name: frontend"},
	}
	for template, markers := range expectedMarkers {
		path := filepath.Join(chartDir, "templates", template)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", template, err)
		}
		text := string(contents)
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				t.Errorf("template %s is missing marker %q", template, marker)
			}
		}
	}
}

func TestHelmChartBackendTemplateEnforcesSecurityBaseline(t *testing.T) {
	chartDir := helmChartDir(t)
	contents, err := os.ReadFile(filepath.Join(chartDir, "templates", "backend.yaml"))
	if err != nil {
		t.Fatalf("read backend template: %v", err)
	}
	text := string(contents)
	for _, marker := range []string{
		"runAsNonRoot: true",
		"seccompProfile:",
		"type: RuntimeDefault",
		"allowPrivilegeEscalation: false",
		"readOnlyRootFilesystem: true",
		"capabilities:",
		"drop: [ALL]",
		"automountServiceAccountToken: false",
		"readinessProbe:",
		"livenessProbe:",
		"resources:",
		"startupProbe:",
	} {
		if !strings.Contains(text, marker) {
			t.Errorf("backend template is missing required security/operations marker %q", marker)
		}
	}
}

func TestHelmChartDoesNotGenerateSecrets(t *testing.T) {
	chartDir := helmChartDir(t)
	templatesDir := filepath.Join(chartDir, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Fatalf("list Helm templates: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(templatesDir, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", entry.Name(), err)
		}
		text := string(contents)
		if strings.Contains(text, "kind: Secret") {
			t.Errorf("Helm template %s must not render a Secret; secrets are provided externally via existingSecret", entry.Name())
		}
		if strings.Contains(text, "CHANGE_ME") {
			t.Errorf("Helm template %s must not contain CHANGE_ME placeholders", entry.Name())
		}
		if strings.Contains(text, "POSTGRES_PASSWORD:") && !strings.Contains(text, "secretKeyRef") {
			t.Errorf("Helm template %s must source POSTGRES_PASSWORD from secretKeyRef, not inline", entry.Name())
		}
		if strings.Contains(text, "JWT_SIGNING_KEY:") && !strings.Contains(text, "secretRef") && !strings.Contains(text, "secretKeyRef") {
			t.Errorf("Helm template %s must source JWT_SIGNING_KEY from a secret reference, not inline", entry.Name())
		}
	}
}

func TestHelmChartValuesDoNotContainSecrets(t *testing.T) {
	values := loadChartFile(t, "values.yaml")
	if _, ok := values["secrets"]; ok {
		t.Error("values.yaml must not declare a secrets object; secrets are external")
	}
	if _, ok := values["secret"]; ok {
		t.Error("values.yaml must not declare a secret object; secrets are external")
	}
	marshaled, err := yaml.Marshal(values)
	if err != nil {
		t.Fatalf("marshal values.yaml: %v", err)
	}
	for _, forbidden := range []string{
		"JWT_SIGNING_KEY: CHANGE_ME",
		"POSTGRES_PASSWORD: CHANGE_ME",
		"BOOTSTRAP_ADMIN_PASSWORD: CHANGE_ME",
		"CREDENTIAL_ENCRYPTION_KEY: CHANGE_ME",
		"password:", "token:", "apiKey:",
	} {
		if strings.Contains(string(marshaled), forbidden) {
			t.Errorf("values.yaml must not contain secret-like field %q", forbidden)
		}
	}
}

func TestHelmChartHelpersAreConsistent(t *testing.T) {
	chartDir := helmChartDir(t)
	contents, err := os.ReadFile(filepath.Join(chartDir, "templates", "_helpers.tpl"))
	if err != nil {
		t.Fatalf("read _helpers.tpl: %v", err)
	}
	text := string(contents)
	for _, marker := range []string{
		`{{- define "aiops-platform.namespace" -}}`,
		`{{- define "aiops-platform.labels" -}}`,
		`{{- define "aiops-platform.selectorLabels" -}}`,
		"app.kubernetes.io/name:",
		"helm.sh/chart:",
	} {
		if !strings.Contains(text, marker) {
			t.Errorf("_helpers.tpl is missing required helper %q", marker)
		}
	}
}

func TestHelmChartHelmignoreExcludesSensitiveArtifacts(t *testing.T) {
	chartDir := helmChartDir(t)
	contents, err := os.ReadFile(filepath.Join(chartDir, ".helmignore"))
	if err != nil {
		t.Fatalf("read .helmignore: %v", err)
	}
	text := string(contents)
	for _, pattern := range []string{".git/", "*.log", "*.bak", "*.tmp"} {
		if !strings.Contains(text, pattern) {
			t.Errorf(".helmignore must exclude %q", pattern)
		}
	}
}

func loadChartFile(t *testing.T, name string) map[string]any {
	t.Helper()
	chartDir := helmChartDir(t)
	contents, err := os.ReadFile(filepath.Join(chartDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var value map[string]any
	if err := yaml.Unmarshal(contents, &value); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return value
}

func helmChartDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "deploy", "helm", "aiops-platform")
}
