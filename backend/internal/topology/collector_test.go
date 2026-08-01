package topology

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// stubReader is a test ResourceReader that returns canned resources.
type stubReader struct {
	deployments    []k8sgateway.Deployment
	replicaSets    []k8sgateway.ReplicaSet
	pods           []k8sgateway.Pod
	services       []k8sgateway.ServiceResource
	ingresses      []k8sgateway.Ingress
	endpointSlices []k8sgateway.EndpointSlice
	hpas           []k8sgateway.HorizontalPodAutoscaler
	pdbs           []k8sgateway.PodDisruptionBudget
	err            error
}

func (s *stubReader) Deployments(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	return apiquery.ListResponse[k8sgateway.Deployment]{Items: s.deployments}, s.err
}
func (s *stubReader) ReplicaSets(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ReplicaSet], error) {
	return apiquery.ListResponse[k8sgateway.ReplicaSet]{Items: s.replicaSets}, s.err
}
func (s *stubReader) Pods(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	return apiquery.ListResponse[k8sgateway.Pod]{Items: s.pods}, s.err
}
func (s *stubReader) Services(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceResource], error) {
	return apiquery.ListResponse[k8sgateway.ServiceResource]{Items: s.services}, s.err
}
func (s *stubReader) Ingresses(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Ingress], error) {
	return apiquery.ListResponse[k8sgateway.Ingress]{Items: s.ingresses}, s.err
}
func (s *stubReader) EndpointSlices(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.EndpointSlice], error) {
	return apiquery.ListResponse[k8sgateway.EndpointSlice]{Items: s.endpointSlices}, s.err
}
func (s *stubReader) HorizontalPodAutoscalers(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.HorizontalPodAutoscaler], error) {
	return apiquery.ListResponse[k8sgateway.HorizontalPodAutoscaler]{Items: s.hpas}, s.err
}
func (s *stubReader) PodDisruptionBudgets(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
	return apiquery.ListResponse[k8sgateway.PodDisruptionBudget]{Items: s.pdbs}, s.err
}

// mustDecodeJSON unmarshals JSON into a value, panicking on error. Used to
// build test fixtures without anonymous-struct tag mismatch issues.
func mustDecodeJSON(t *testing.T, data string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), v); err != nil {
		t.Fatalf("failed to decode JSON fixture: %v", err)
	}
}

func TestDeriveOwnsEdges(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	var rs k8sgateway.ReplicaSet
	mustDecodeJSON(t, `{"metadata":{"name":"web-abc","uid":"rs-uid-1","namespace":"default","ownerReferences":[{"kind":"Deployment","name":"web","uid":"dep-uid-1","controller":true}]}}`, &rs)
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"web-abc-xyz","uid":"pod-uid-1","namespace":"default","ownerReferences":[{"kind":"ReplicaSet","name":"web-abc","uid":"rs-uid-1","controller":true}]}}`, &pod)
	snap := CollectorSnapshot{
		ClusterID:   1,
		Namespace:   "default",
		ReplicaSets: []k8sgateway.ReplicaSet{rs},
		Pods:        []k8sgateway.Pod{pod},
	}

	edges := deriveOwnsEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 Owns edge (RS→Pod), got %d", len(edges))
	}
	edge := edges[0]
	if edge.Kind != EdgeOwns {
		t.Errorf("expected kind Owns, got %s", edge.Kind)
	}
	if edge.Source.UID != "rs-uid-1" {
		t.Errorf("expected source UID rs-uid-1, got %s", edge.Source.UID)
	}
	if edge.Target.UID != "pod-uid-1" {
		t.Errorf("expected target UID pod-uid-1, got %s", edge.Target.UID)
	}
	if edge.Derivation != DerivationOwnerReference {
		t.Errorf("expected derivation owner_reference, got %s", edge.Derivation)
	}
}

func TestDeriveOwnsEdges_SkipsUnknownOwner(t *testing.T) {
	now := time.Now().UTC()
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"orphan","uid":"pod-uid-2","namespace":"default","ownerReferences":[{"kind":"ReplicaSet","name":"ghost","uid":"nonexistent-uid"}]}}`, &pod)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Pods:      []k8sgateway.Pod{pod},
	}
	edges := deriveOwnsEdges(snap, now)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for unknown owner, got %d", len(edges))
	}
}

func TestDeriveSelectsEdges(t *testing.T) {
	now := time.Now().UTC()
	var svc k8sgateway.ServiceResource
	mustDecodeJSON(t, `{"metadata":{"name":"web-svc","uid":"svc-uid-1","namespace":"default"},"spec":{"type":"ClusterIP","selector":{"app":"web"}}}`, &svc)
	var pod1, pod2 k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"web-1","uid":"pod-1","namespace":"default","labels":{"app":"web"}}}`, &pod1)
	mustDecodeJSON(t, `{"metadata":{"name":"other-1","uid":"pod-2","namespace":"default","labels":{"app":"other"}}}`, &pod2)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Services:  []k8sgateway.ServiceResource{svc},
		Pods:      []k8sgateway.Pod{pod1, pod2},
	}
	edges := deriveSelectsEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 Selects edge, got %d", len(edges))
	}
	if edges[0].Source.UID != "svc-uid-1" {
		t.Errorf("expected source svc-uid-1, got %s", edges[0].Source.UID)
	}
	if edges[0].Target.UID != "pod-1" {
		t.Errorf("expected target pod-1, got %s", edges[0].Target.UID)
	}
	if edges[0].Derivation != DerivationLabelSelector {
		t.Errorf("expected derivation label_selector, got %s", edges[0].Derivation)
	}
}

func TestDeriveSelectsEdges_EmptySelectorMatchesNothing(t *testing.T) {
	now := time.Now().UTC()
	var svc k8sgateway.ServiceResource
	mustDecodeJSON(t, `{"metadata":{"name":"external-svc","uid":"svc-uid-2","namespace":"default"},"spec":{"type":"ExternalName"}}`, &svc)
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"p1","uid":"p1","namespace":"default","labels":{"app":"x"}}}`, &pod)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Services:  []k8sgateway.ServiceResource{svc},
		Pods:      []k8sgateway.Pod{pod},
	}
	edges := deriveSelectsEdges(snap, now)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for empty selector, got %d", len(edges))
	}
}

func TestDeriveRoutesToEdges(t *testing.T) {
	now := time.Now().UTC()
	var ing k8sgateway.Ingress
	mustDecodeJSON(t, `{"metadata":{"name":"web-ing","uid":"ing-uid-1","namespace":"default"},"spec":{"defaultBackend":{"service":{"name":"web-svc","port":{"number":80}}}}}`, &ing)
	var svc k8sgateway.ServiceResource
	mustDecodeJSON(t, `{"metadata":{"name":"web-svc","uid":"svc-uid-1","namespace":"default"},"spec":{"type":"ClusterIP"}}`, &svc)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Ingresses: []k8sgateway.Ingress{ing},
		Services:  []k8sgateway.ServiceResource{svc},
	}
	edges := deriveRoutesToEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 RoutesTo edge, got %d", len(edges))
	}
	if edges[0].Source.UID != "ing-uid-1" {
		t.Errorf("expected source ing-uid-1, got %s", edges[0].Source.UID)
	}
	if edges[0].Target.Name != "web-svc" {
		t.Errorf("expected target web-svc, got %s", edges[0].Target.Name)
	}
	if edges[0].Derivation != DerivationBackendConfig {
		t.Errorf("expected derivation backend_config, got %s", edges[0].Derivation)
	}
}

func TestDeriveRoutesToEdges_IncompleteTarget(t *testing.T) {
	now := time.Now().UTC()
	var ing k8sgateway.Ingress
	mustDecodeJSON(t, `{"metadata":{"name":"web-ing","uid":"ing-uid-1","namespace":"default"},"spec":{"defaultBackend":{"service":{"name":"missing-svc","port":{"number":80}}}}}`, &ing)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Ingresses: []k8sgateway.Ingress{ing},
	}
	edges := deriveRoutesToEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 incomplete edge, got %d", len(edges))
	}
	if !edges[0].Target.Incomplete {
		t.Error("expected target to be marked incomplete")
	}
}

func TestDeriveBackedByEdges(t *testing.T) {
	now := time.Now().UTC()
	var svc k8sgateway.ServiceResource
	mustDecodeJSON(t, `{"metadata":{"name":"web-svc","uid":"svc-uid-1","namespace":"default"},"spec":{"type":"ClusterIP"}}`, &svc)
	var es k8sgateway.EndpointSlice
	mustDecodeJSON(t, `{"metadata":{"name":"web-svc-abc","uid":"es-uid-1","namespace":"default","labels":{"kubernetes.io/service-name":"web-svc"}},"addressType":"IPv4","serviceName":"web-svc","endpoints":[{"addresses":["10.0.0.1"],"targetRef":{"kind":"Pod","namespace":"default","name":"web-1","uid":"pod-uid-1"}}]}`, &es)
	snap := CollectorSnapshot{
		ClusterID:      1,
		Namespace:      "default",
		Services:       []k8sgateway.ServiceResource{svc},
		EndpointSlices: []k8sgateway.EndpointSlice{es},
	}
	edges := deriveBackedByEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 BackedBy edge, got %d", len(edges))
	}
	if edges[0].Source.UID != "svc-uid-1" {
		t.Errorf("expected source svc-uid-1, got %s", edges[0].Source.UID)
	}
	if edges[0].Target.UID != "pod-uid-1" {
		t.Errorf("expected target pod-uid-1, got %s", edges[0].Target.UID)
	}
	if edges[0].Derivation != DerivationEndpointSlice {
		t.Errorf("expected derivation endpointslice, got %s", edges[0].Derivation)
	}
}

func TestDeriveRunsOnEdges(t *testing.T) {
	now := time.Now().UTC()
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"web-1","uid":"pod-uid-1","namespace":"default"},"spec":{"nodeName":"node-1","containers":[{"name":"web","image":"nginx"}]}}`, &pod)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Pods:      []k8sgateway.Pod{pod},
	}
	edges := deriveRunsOnEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 RunsOn edge, got %d", len(edges))
	}
	if edges[0].Target.Name != "node-1" {
		t.Errorf("expected target node-1, got %s", edges[0].Target.Name)
	}
	if !edges[0].Target.Incomplete {
		t.Error("expected node target to be incomplete (no node UID in snapshot)")
	}
	if edges[0].Derivation != DerivationNodeName {
		t.Errorf("expected derivation node_name, got %s", edges[0].Derivation)
	}
}

func TestDeriveMountsEdges(t *testing.T) {
	now := time.Now().UTC()
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"db-1","uid":"pod-uid-1","namespace":"default"},"spec":{"containers":[{"name":"db","image":"postgres"}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"data-pvc"}},{"name":"data","persistentVolumeClaim":{"claimName":"data-pvc"}},{"name":"cache","emptyDir":{}}]}}`, &pod)
	snap := CollectorSnapshot{
		ClusterID: 1,
		Namespace: "default",
		Pods:      []k8sgateway.Pod{pod},
	}
	edges := deriveMountsEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 Mounts edge (deduplicated), got %d", len(edges))
	}
	if edges[0].Target.Name != "data-pvc" {
		t.Errorf("expected target data-pvc, got %s", edges[0].Target.Name)
	}
	if !edges[0].Target.Incomplete {
		t.Error("expected PVC target to be incomplete")
	}
	if edges[0].Derivation != DerivationVolumeMount {
		t.Errorf("expected derivation volume_mount, got %s", edges[0].Derivation)
	}
}

func TestDeriveScalesEdges(t *testing.T) {
	now := time.Now().UTC()
	var hpa k8sgateway.HorizontalPodAutoscaler
	mustDecodeJSON(t, `{"metadata":{"name":"web-hpa","uid":"hpa-uid-1","namespace":"default"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"web"},"maxReplicas":5}}`, &hpa)
	var dep k8sgateway.Deployment
	mustDecodeJSON(t, `{"metadata":{"name":"web","uid":"dep-uid-1","namespace":"default"},"spec":{"replicas":2}}`, &dep)
	snap := CollectorSnapshot{
		ClusterID:   1,
		Namespace:   "default",
		HPAs:        []k8sgateway.HorizontalPodAutoscaler{hpa},
		Deployments: []k8sgateway.Deployment{dep},
	}
	edges := deriveScalesEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 Scales edge, got %d", len(edges))
	}
	if edges[0].Source.UID != "hpa-uid-1" {
		t.Errorf("expected source hpa-uid-1, got %s", edges[0].Source.UID)
	}
	if edges[0].Target.UID != "dep-uid-1" {
		t.Errorf("expected target dep-uid-1, got %s", edges[0].Target.UID)
	}
	if edges[0].Derivation != DerivationScaleTarget {
		t.Errorf("expected derivation scale_target_ref, got %s", edges[0].Derivation)
	}
}

func TestDeriveProtectedByEdges(t *testing.T) {
	now := time.Now().UTC()
	var pdb k8sgateway.PodDisruptionBudget
	mustDecodeJSON(t, `{"metadata":{"name":"web-pdb","uid":"pdb-uid-1","namespace":"default"},"spec":{"selector":{"matchLabels":{"app":"web"}}}}`, &pdb)
	var dep k8sgateway.Deployment
	mustDecodeJSON(t, `{"metadata":{"name":"web","uid":"dep-uid-1","namespace":"default","labels":{"app":"web"}},"spec":{"replicas":3}}`, &dep)
	snap := CollectorSnapshot{
		ClusterID:   1,
		Namespace:   "default",
		PDBs:        []k8sgateway.PodDisruptionBudget{pdb},
		Deployments: []k8sgateway.Deployment{dep},
	}
	edges := deriveProtectedByEdges(snap, now)
	if len(edges) != 1 {
		t.Fatalf("expected 1 ProtectedBy edge, got %d", len(edges))
	}
	if edges[0].Source.UID != "dep-uid-1" {
		t.Errorf("expected source dep-uid-1, got %s", edges[0].Source.UID)
	}
	if edges[0].Target.UID != "pdb-uid-1" {
		t.Errorf("expected target pdb-uid-1, got %s", edges[0].Target.UID)
	}
	if edges[0].Derivation != DerivationPDBSelector {
		t.Errorf("expected derivation pdb_selector, got %s", edges[0].Derivation)
	}
}

func TestCollectorDeriveEdges_AllKinds(t *testing.T) {
	var dep k8sgateway.Deployment
	mustDecodeJSON(t, `{"metadata":{"name":"web","uid":"dep-uid-1","namespace":"default","labels":{"app":"web"}},"spec":{"replicas":2,"selector":{"matchLabels":{"app":"web"}}}}`, &dep)
	var rs k8sgateway.ReplicaSet
	mustDecodeJSON(t, `{"metadata":{"name":"web-abc","uid":"rs-uid-1","namespace":"default","ownerReferences":[{"kind":"Deployment","name":"web","uid":"dep-uid-1","controller":true}]},"spec":{"replicas":2}}`, &rs)
	var pod k8sgateway.Pod
	mustDecodeJSON(t, `{"metadata":{"name":"web-abc-xyz","uid":"pod-uid-1","namespace":"default","labels":{"app":"web"},"ownerReferences":[{"kind":"ReplicaSet","name":"web-abc","uid":"rs-uid-1","controller":true}]},"spec":{"nodeName":"node-1","containers":[{"name":"web","image":"nginx"}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"data-pvc"}}]}}`, &pod)
	var svc k8sgateway.ServiceResource
	mustDecodeJSON(t, `{"metadata":{"name":"web-svc","uid":"svc-uid-1","namespace":"default"},"spec":{"type":"ClusterIP","selector":{"app":"web"}}}`, &svc)
	var ing k8sgateway.Ingress
	mustDecodeJSON(t, `{"metadata":{"name":"web-ing","uid":"ing-uid-1","namespace":"default"},"spec":{"defaultBackend":{"service":{"name":"web-svc","port":{"number":80}}}}}`, &ing)
	var es k8sgateway.EndpointSlice
	mustDecodeJSON(t, `{"metadata":{"name":"es-1","uid":"es-uid-1","namespace":"default","labels":{"kubernetes.io/service-name":"web-svc"}},"addressType":"IPv4","serviceName":"web-svc","endpoints":[{"addresses":["10.0.0.1"],"targetRef":{"kind":"Pod","namespace":"default","name":"web-abc-xyz","uid":"pod-uid-1"}}]}`, &es)
	var hpa k8sgateway.HorizontalPodAutoscaler
	mustDecodeJSON(t, `{"metadata":{"name":"web-hpa","uid":"hpa-uid-1","namespace":"default"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"web"},"maxReplicas":5}}`, &hpa)
	var pdb k8sgateway.PodDisruptionBudget
	mustDecodeJSON(t, `{"metadata":{"name":"web-pdb","uid":"pdb-uid-1","namespace":"default"},"spec":{"selector":{"matchLabels":{"app":"web"}}}}`, &pdb)

	reader := &stubReader{
		deployments:    []k8sgateway.Deployment{dep},
		replicaSets:    []k8sgateway.ReplicaSet{rs},
		pods:           []k8sgateway.Pod{pod},
		services:       []k8sgateway.ServiceResource{svc},
		ingresses:      []k8sgateway.Ingress{ing},
		endpointSlices: []k8sgateway.EndpointSlice{es},
		hpas:           []k8sgateway.HorizontalPodAutoscaler{hpa},
		pdbs:           []k8sgateway.PodDisruptionBudget{pdb},
	}

	collector := NewCollector(reader, 100)
	snap, err := collector.Snapshot(context.Background(), 1, "default")
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	now := time.Now().UTC()
	edges := collector.DeriveEdges(snap, now)

	kinds := make(map[EdgeKind]int)
	for _, e := range edges {
		kinds[e.Kind]++
		if e.SourceHash == "" {
			t.Errorf("edge %s→%s has empty source hash", e.Source.Name, e.Target.Name)
		}
	}

	expectedKinds := []EdgeKind{EdgeOwns, EdgeSelects, EdgeRoutesTo, EdgeBackedBy, EdgeRunsOn, EdgeMounts, EdgeScales, EdgeProtectedBy}
	for _, k := range expectedKinds {
		if kinds[k] == 0 {
			t.Errorf("expected at least 1 edge of kind %s, got 0", k)
		}
	}
}

func TestComputeEdgeHash_Deterministic(t *testing.T) {
	edge := Edge{
		Kind:       EdgeOwns,
		Source:     ResourceCitation{Kind: "ReplicaSet", Name: "rs", UID: "rs-uid"},
		Target:     ResourceCitation{Kind: "Pod", Name: "pod", UID: "pod-uid"},
		Derivation: DerivationOwnerReference,
	}
	h1 := computeEdgeHash(&edge)
	h2 := computeEdgeHash(&edge)
	if h1 != h2 {
		t.Error("hash should be deterministic")
	}

	edge2 := edge
	edge2.Target.UID = "different"
	h3 := computeEdgeHash(&edge2)
	if h1 == h3 {
		t.Error("different edges should have different hashes")
	}
}

func TestMapSelectorMatches(t *testing.T) {
	tests := []struct {
		name     string
		selector map[string]string
		labels   map[string]string
		expected bool
	}{
		{"exact match", map[string]string{"a": "1"}, map[string]string{"a": "1"}, true},
		{"superset labels", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}, true},
		{"missing label", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1"}, false},
		{"wrong value", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
		{"empty selector", map[string]string{}, map[string]string{"a": "1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapSelectorMatches(tt.selector, tt.labels); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

// Ensure metav1 import is used (PDB tests rely on LabelSelector via JSON).
var _ = metav1.LabelSelector{}
