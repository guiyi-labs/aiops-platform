package deployment_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestManagedClusterRBACKeepsMutationNamespacedAndBounded(t *testing.T) {
	directory := filepath.Join(repositoryRoot(t), "deploy", "managed-cluster")
	contents, err := os.ReadFile(filepath.Join(directory, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var configuration kustomization
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		t.Fatal(err)
	}
	resources := loadResources(t, directory, configuration.Resources)
	observer := requireResource(t, resources, "ClusterRole/aiops-platform-observer")
	allowedMutations := map[string]map[string]bool{
		"/nodes":                            {"patch": true},
		"/pods/eviction":                    {"create": true},
		"/namespaces":                       {"create": true},
		"/resourcequotas":                   {"create": true},
		"networking.k8s.io/networkpolicies": {"create": true},
		"velero.io/backups":                 {"create": true},
		"velero.io/restores":                {"create": true},
	}
	for _, rule := range rules(t, observer) {
		apiGroups := stringSlice(t, rule["apiGroups"])
		if len(apiGroups) != 1 {
			t.Fatalf("observer rule must contain exactly one API group: %#v", rule)
		}
		resources := stringSlice(t, rule["resources"])
		for _, verb := range stringSlice(t, rule["verbs"]) {
			if verb == "get" || verb == "list" {
				continue
			}
			for _, resource := range resources {
				key := apiGroups[0] + "/" + resource
				if !allowedMutations[key][verb] {
					t.Fatalf("observer contains unreviewed mutation %s %s", verb, key)
				}
			}
		}
	}
	for _, permission := range []struct {
		apiGroup string
		resource string
		verb     string
	}{
		{apiGroup: "", resource: "nodes", verb: "patch"},
		{apiGroup: "", resource: "pods/eviction", verb: "create"},
		{apiGroup: "", resource: "namespaces", verb: "create"},
		{apiGroup: "", resource: "resourcequotas", verb: "create"},
		{apiGroup: "networking.k8s.io", resource: "networkpolicies", verb: "create"},
		{apiGroup: "velero.io", resource: "backups", verb: "create"},
		{apiGroup: "velero.io", resource: "restores", verb: "create"},
	} {
		if !roleAllows(observer, permission.apiGroup, permission.resource, []string{permission.verb}) {
			t.Fatalf("observer is missing reviewed mutation %s %s/%s", permission.verb, permission.apiGroup, permission.resource)
		}
	}
	for _, permission := range []struct {
		apiGroup string
		resource string
	}{
		{apiGroup: "", resource: "configmaps"},
		{apiGroup: "", resource: "persistentvolumeclaims"},
		{apiGroup: "", resource: "resourcequotas"},
		{apiGroup: "", resource: "limitranges"},
		{apiGroup: "", resource: "secrets"},
		{apiGroup: "apps", resource: "statefulsets"},
		{apiGroup: "apps", resource: "daemonsets"},
		{apiGroup: "apps", resource: "replicasets"},
		{apiGroup: "batch", resource: "jobs"},
		{apiGroup: "batch", resource: "cronjobs"},
		{apiGroup: "autoscaling", resource: "horizontalpodautoscalers"},
		{apiGroup: "networking.k8s.io", resource: "ingresses"},
		{apiGroup: "storage.k8s.io", resource: "storageclasses"},
	} {
		if !roleAllows(observer, permission.apiGroup, permission.resource, []string{"get", "list"}) {
			t.Fatalf("observer is missing get/list for %s/%s", permission.apiGroup, permission.resource)
		}
	}
	remediator := requireResource(t, resources, "Role/aiops-platform-deployment-remediator")
	if nestedMap(t, remediator, "metadata")["namespace"] != "aiops-demo" {
		t.Fatal("remediator example must be explicitly namespaced")
	}
	remediationRules := rules(t, remediator)
	if len(remediationRules) != 2 || !roleAllows(remediator, "apps", "deployments", []string{"get", "patch"}) || !roleAllows(remediator, "batch", "cronjobs", []string{"get", "patch"}) {
		t.Fatalf("unexpected remediation rules: %#v", remediationRules)
	}
	for _, forbidden := range []string{"secrets", "pods/exec", "roles", "rolebindings"} {
		for _, rule := range remediationRules {
			for _, resource := range stringSlice(t, rule["resources"]) {
				if resource == forbidden {
					t.Fatalf("remediator grants forbidden resource %q", forbidden)
				}
			}
		}
	}
}

func roleAllows(role map[string]any, apiGroup, resource string, verbs []string) bool {
	for _, rule := range role["rules"].([]any) {
		values := rule.(map[string]any)
		groups, resources, ruleVerbs := values["apiGroups"].([]any), values["resources"].([]any), values["verbs"].([]any)
		if containsString(groups, apiGroup) && containsString(resources, resource) {
			allowed := true
			for _, verb := range verbs {
				if !containsString(ruleVerbs, verb) {
					allowed = false
					break
				}
			}
			if allowed {
				return true
			}
		}
	}
	return false
}

func containsString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func rules(t *testing.T, resource map[string]any) []map[string]any {
	t.Helper()
	values, ok := resource["rules"].([]any)
	if !ok {
		t.Fatal("resource rules are missing")
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		rule, ok := value.(map[string]any)
		if !ok {
			t.Fatal("rule is not an object")
		}
		result = append(result, rule)
	}
	return result
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("value is not a list: %#v", value)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok {
			t.Fatalf("list item is not a string: %#v", value)
		}
		result = append(result, item)
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
