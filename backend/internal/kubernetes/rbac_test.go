package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
)

func TestRolesListAndDetailPathsAndNormalization(t *testing.T) {
	listBody := []byte(`{"items":[{"metadata":{"name":"pod-reader","namespace":"prod"},"rules":[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]},{"metadata":{"name":"secret-reader","namespace":"prod"},"rules":null}]}`)
	detailBody := []byte(`{"metadata":{"name":"pod-reader","namespace":"prod"},"rules":[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]}`)
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/rbac.authorization.k8s.io/v1/namespaces/prod/roles":            {body: listBody},
		"/apis/rbac.authorization.k8s.io/v1/namespaces/prod/roles/pod-reader": {body: detailBody},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := service.Roles(context.Background(), 7, "prod", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/namespaces/prod/roles" {
		t.Fatalf("list path = %q", gateway.path)
	}
	if response.Total != 2 || len(response.Items) != 2 {
		t.Fatalf("response = %#v", response)
	}
	// null rules must surface as [] not null
	encoded, _ := json.Marshal(response.Items[1].Rules)
	if string(encoded) != "[]" {
		t.Fatalf("null rules normalized to %s", encoded)
	}

	item, err := service.Role(context.Background(), 7, "prod", "pod-reader")
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/namespaces/prod/roles/pod-reader" {
		t.Fatalf("detail path = %q", gateway.path)
	}
	if len(item.Rules) != 1 || item.Rules[0].Verbs[0] != "get" {
		t.Fatalf("item = %#v", item)
	}
}

func TestRolesEmptyRulesNormalizedOnDetail(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"empty","namespace":"prod"},"rules":null}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	item, err := service.Role(context.Background(), 7, "prod", "empty")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(item.Rules)
	if string(encoded) != "[]" {
		t.Fatalf("rules = %s", encoded)
	}
}

func TestClusterRolesListAndDetailPaths(t *testing.T) {
	listBody := []byte(`{"items":[{"metadata":{"name":"cluster-admin"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}]}`)
	detailBody := []byte(`{"metadata":{"name":"cluster-admin"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}`)
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/rbac.authorization.k8s.io/v1/clusterroles":               {body: listBody},
		"/apis/rbac.authorization.k8s.io/v1/clusterroles/cluster-admin": {body: detailBody},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := service.ClusterRoles(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/clusterroles" {
		t.Fatalf("list path = %q", gateway.path)
	}
	if response.Total != 1 || response.Items[0].Metadata.Name != "cluster-admin" {
		t.Fatalf("response = %#v", response)
	}

	_, err = service.ClusterRole(context.Background(), 7, "cluster-admin")
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/clusterroles/cluster-admin" {
		t.Fatalf("detail path = %q", gateway.path)
	}
}

func TestRoleBindingsListAndDetailPathsAndNormalization(t *testing.T) {
	listBody := []byte(`{"items":[{"metadata":{"name":"read-pods","namespace":"prod"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"pod-reader"},"subjects":[{"kind":"User","name":"alice","apiGroup":"rbac.authorization.k8s.io"}]},{"metadata":{"name":"no-subjects","namespace":"prod"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"empty"},"subjects":null}]}`)
	detailBody := []byte(`{"metadata":{"name":"read-pods","namespace":"prod"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"pod-reader"},"subjects":[{"kind":"User","name":"alice","apiGroup":"rbac.authorization.k8s.io"}]}`)
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/rbac.authorization.k8s.io/v1/namespaces/prod/rolebindings":           {body: listBody},
		"/apis/rbac.authorization.k8s.io/v1/namespaces/prod/rolebindings/read-pods": {body: detailBody},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := service.RoleBindings(context.Background(), 7, "prod", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/namespaces/prod/rolebindings" {
		t.Fatalf("list path = %q", gateway.path)
	}
	if response.Total != 2 {
		t.Fatalf("response = %#v", response)
	}
	encoded, _ := json.Marshal(response.Items[1].Subjects)
	if string(encoded) != "[]" {
		t.Fatalf("null subjects normalized to %s", encoded)
	}

	item, err := service.RoleBinding(context.Background(), 7, "prod", "read-pods")
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/namespaces/prod/rolebindings/read-pods" {
		t.Fatalf("detail path = %q", gateway.path)
	}
	if item.RoleRef.Kind != "Role" || len(item.Subjects) != 1 {
		t.Fatalf("item = %#v", item)
	}
}

func TestClusterRoleBindingsListAndDetailPaths(t *testing.T) {
	listBody := []byte(`{"items":[{"metadata":{"name":"cluster-admin-binding"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"cluster-admin"},"subjects":[{"kind":"Group","name":"system:masters","apiGroup":"rbac.authorization.k8s.io"}]}]}`)
	detailBody := []byte(`{"metadata":{"name":"cluster-admin-binding"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"cluster-admin"},"subjects":[{"kind":"Group","name":"system:masters","apiGroup":"rbac.authorization.k8s.io"}]}`)
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/rbac.authorization.k8s.io/v1/clusterrolebindings":                       {body: listBody},
		"/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/cluster-admin-binding": {body: detailBody},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := service.ClusterRoleBindings(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings" {
		t.Fatalf("list path = %q", gateway.path)
	}
	if response.Total != 1 || response.Items[0].RoleRef.Name != "cluster-admin" {
		t.Fatalf("response = %#v", response)
	}

	_, err = service.ClusterRoleBinding(context.Background(), 7, "cluster-admin-binding")
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/cluster-admin-binding" {
		t.Fatalf("detail path = %q", gateway.path)
	}
}

func TestRBACReadsMapNotFoundToSentinel(t *testing.T) {
	gateway := &gatewayStub{err: cluster.APIStatusError{StatusCode: 404}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := service.Role(context.Background(), 7, "prod", "missing")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("Role error = %v", err)
	}
}

func TestRBACClusterScopedNamesEscaped(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"weird name"},"rules":[]}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.ClusterRole(context.Background(), 7, "weird name")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gateway.path, "/clusterroles/weird%20name") {
		t.Fatalf("escaped path = %q", gateway.path)
	}
}

func TestManifestPathCoversAllRBACKinds(t *testing.T) {
	cases := map[string]string{
		"Role":               manifestPath("Role", "prod", "pod-reader"),
		"ClusterRole":        manifestPath("ClusterRole", "", "cluster-admin"),
		"RoleBinding":        manifestPath("RoleBinding", "prod", "read-pods"),
		"ClusterRoleBinding": manifestPath("ClusterRoleBinding", "", "cluster-admin-binding"),
	}
	expectedSubstr := map[string]string{
		"Role":               "/roles/pod-reader",
		"ClusterRole":        "/clusterroles/cluster-admin",
		"RoleBinding":        "/rolebindings/read-pods",
		"ClusterRoleBinding": "/clusterrolebindings/cluster-admin-binding",
	}
	for kind, path := range cases {
		if path == "" {
			t.Fatalf("manifestPath(%q) returned empty", kind)
		}
		if !strings.Contains(path, expectedSubstr[kind]) {
			t.Fatalf("%q path = %q, want substring %q", kind, path, expectedSubstr[kind])
		}
	}
}
