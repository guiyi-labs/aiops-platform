package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newHandler())
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, srv *httptest.Server, path string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return resp.StatusCode, body
}

func TestVersionAndHealth(t *testing.T) {
	srv := newTestServer(t)
	code, body := getJSON(t, srv, "/version")
	if code != http.StatusOK || body["gitVersion"] != "v1.36.0" {
		t.Fatalf("version: code=%d body=%v", code, body)
	}
	code, body = getJSON(t, srv, "/healthz")
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("healthz: code=%d body=%v", code, body)
	}
}

func TestNodes(t *testing.T) {
	srv := newTestServer(t)
	code, list := getJSON(t, srv, "/api/v1/nodes")
	if code != http.StatusOK {
		t.Fatalf("node list code=%d", code)
	}
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 node, got %d", len(items))
	}
	code, node := getJSON(t, srv, "/api/v1/nodes/demo-node")
	if code != http.StatusOK {
		t.Fatalf("node get code=%d", code)
	}
	status, _ := node["status"].(map[string]any)
	conditions, _ := status["conditions"].([]any)
	ready := ""
	for _, c := range conditions {
		cond, _ := c.(map[string]any)
		if cond["type"] == "Ready" {
			ready, _ = cond["status"].(string)
		}
	}
	if ready != "False" {
		t.Fatalf("expected Ready=False, got %q", ready)
	}
}

func TestPodHasOOMKilledEvidence(t *testing.T) {
	srv := newTestServer(t)
	code, pod := getJSON(t, srv, "/api/v1/namespaces/demo/pods/demo-pod")
	if code != http.StatusOK {
		t.Fatalf("pod get code=%d", code)
	}
	status, _ := pod["status"].(map[string]any)
	statuses, _ := status["containerStatuses"].([]any)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 container status")
	}
	cs, _ := statuses[0].(map[string]any)
	last, _ := cs["lastState"].(map[string]any)
	term, _ := last["terminated"].(map[string]any)
	if term["reason"] != "OOMKilled" {
		t.Fatalf("expected OOMKilled termination, got %v", term)
	}
}

func TestDeploymentPatchRestart(t *testing.T) {
	srv := newTestServer(t)
	code, before := getJSON(t, srv, "/apis/apps/v1/namespaces/demo/deployments/demo-app")
	if code != http.StatusOK {
		t.Fatalf("deployment get code=%d", code)
	}
	rvBefore := "40"
	if meta, ok := before["metadata"].(map[string]any); ok {
		rvBefore, _ = meta["resourceVersion"].(string)
	}

	patch := `{"metadata":{"uid":"demo-app-uid-0001","resourceVersion":"` + rvBefore + `"},"spec":{"template":{"metadata":{"annotations":{"k8s-aiops.local/restarted-at":"2026-08-12T08:00:00Z","k8s-aiops.local/remediation-id":"demo-plan-0001"}}}}}`
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/apis/apps/v1/namespaces/demo/deployments/demo-app?dryRun=All", strings.NewReader(patch))
	req.Header.Set("Content-Type", "application/strategic-merge-patch+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dry-run patch: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dry-run patch code=%d", resp.StatusCode)
	}
	resp.Body.Close()

	// dry-run must not persist: annotations unchanged and resourceVersion same
	_, afterDry := getJSON(t, srv, "/apis/apps/v1/namespaces/demo/deployments/demo-app")
	metaDry, _ := afterDry["metadata"].(map[string]any)
	if metaDry["resourceVersion"] != rvBefore {
		t.Fatalf("dry-run persisted rv: %v", metaDry["resourceVersion"])
	}

	// real patch persists and bumps resourceVersion
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/apis/apps/v1/namespaces/demo/deployments/demo-app", strings.NewReader(patch))
	req.Header.Set("Content-Type", "application/strategic-merge-patch+json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("real patch: %v", err)
	}
	var applied map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&applied); err != nil {
		t.Fatalf("decode applied: %v", err)
	}
	resp.Body.Close()

	_, after := getJSON(t, srv, "/apis/apps/v1/namespaces/demo/deployments/demo-app")
	meta, _ := after["metadata"].(map[string]any)
	if meta["resourceVersion"] == rvBefore {
		t.Fatalf("resourceVersion not bumped")
	}
	spec, _ := after["spec"].(map[string]any)
	tpl, _ := spec["template"].(map[string]any)
	tplMeta, _ := tpl["metadata"].(map[string]any)
	anns, _ := tplMeta["annotations"].(map[string]any)
	if anns["k8s-aiops.local/restarted-at"] == nil {
		t.Fatalf("restart annotation missing after real patch")
	}

	code, muts := getJSON(t, srv, "/mock/mutations")
	if code != http.StatusOK {
		t.Fatalf("mutations code=%d", code)
	}
	items, _ := muts["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 recorded mutation, got %d", len(items))
	}
}

func TestPatchConflictAndUnknownPath(t *testing.T) {
	srv := newTestServer(t)
	bad := `{"metadata":{"uid":"other-uid"},"spec":{"replicas":1}}`
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/apis/apps/v1/namespaces/demo/deployments/demo-app", strings.NewReader(bad))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("conflict patch: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	code, body := getJSON(t, srv, "/apis/not/a/real/path")
	if code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", code)
	}
	if body["reason"] != "NotFound" {
		t.Fatalf("expected NotFound reason, got %v", body["reason"])
	}
}

func TestNodeCordonPatch(t *testing.T) {
	srv := newTestServer(t)
	patch := `{"spec":{"unschedulable":true}}`
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/nodes/demo-node", strings.NewReader(patch))
	req.Header.Set("Content-Type", "application/strategic-merge-patch+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cordon patch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cordon code=%d", resp.StatusCode)
	}
	_, node := getJSON(t, srv, "/api/v1/nodes/demo-node")
	spec, _ := node["spec"].(map[string]any)
	if spec["unschedulable"] != true {
		t.Fatalf("cordon not applied: %v", spec)
	}
}
