package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// isolateHome ensures the CLI cannot find a real ~/.kube/config while the
// tests run (demo-mode assertions must not depend on the runner's HOME).
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBECONFIG", "")
}

func TestDemoDiagnoseDiscoversAllFixtures(t *testing.T) {
	result, err := diagnoseDemo()
	if err != nil {
		t.Fatalf("diagnoseDemo: %v", err)
	}
	if result.Mode != "demo" {
		t.Fatalf("mode = %q, want demo", result.Mode)
	}
	if result.Scanned != 5 {
		t.Fatalf("scanned = %d, want 5 demo pods", result.Scanned)
	}
	byRule := map[string]bool{}
	for _, item := range result.Findings {
		byRule[item.RuleID] = true
	}
	for _, rule := range []string{
		"pod.crash_loop_backoff.v1", "pod.image_pull_backoff.v1",
		"pod.oom_killed.v1", "pod.pending.v1",
	} {
		if !byRule[rule] {
			t.Errorf("demo findings missing rule %q; got %v", rule, result.Findings)
		}
	}
	if byRule["pod.pending.v1"] && len(result.Findings) != 4 {
		// Sanity: exactly the four failure fixtures, healthy pod must not match.
		t.Fatalf("unexpected extra findings: %+v", result.Findings)
	}
	// Findings must be English-rendered and ranked severity first (critical
	// OOM before high).
	if result.Findings[0].Severity != "critical" || !strings.Contains(result.Findings[0].Summary, "OOMKilled") {
		t.Errorf("top finding = %+v, want critical OOMKilled first with English summary", result.Findings[0])
	}
}

func TestEvaluatePodChainHealthyPod(t *testing.T) {
	pods, events := demoCluster()
	var healthy k8sgateway.Pod
	for _, pod := range pods {
		if pod.Metadata.Name == "healthy-1" {
			healthy = pod
			break
		}
	}
	if healthy.Metadata.Name == "" {
		t.Fatal("demo cluster has no healthy pod")
	}
	record, matched := evaluatePodChain(0, healthy, events[healthy.Metadata.UID], time.Now().UTC())
	if matched {
		t.Fatalf("healthy pod matched rule %q, want no match", record.RuleID)
	}
}

func TestRunDiagnoseDemoMode(t *testing.T) {
	isolateHome(t)
	var stdout, stderr bytes.Buffer
	code := runDiagnose(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("demo run code = %d, want 1 (findings present); stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Mode: demo", "4 finding(s)", "CrashLoopBackOff", "ImagePullBackOff"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr should be empty in demo mode, got %q", stderr.String())
	}
}

func TestRunDiagnoseJSON(t *testing.T) {
	isolateHome(t)
	var stdout, stderr bytes.Buffer
	code := runDiagnose([]string{"-o", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("json run code = %d, want 1; stderr: %s", code, stderr.String())
	}
	var result diagnoseResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if result.Mode != "demo" || result.Scanned != 5 || len(result.Findings) != 4 {
		t.Fatalf("json result = %+v", result)
	}
	if result.Findings[0].Summary == "" || result.Findings[0].RuleID == "" {
		t.Fatalf("finding fields empty: %+v", result.Findings[0])
	}
}

func TestRunDiagnoseExplicitMissingKubeconfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDiagnose([]string{"--kubeconfig", "/nonexistent/kubeconfig"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d, want 2 (explicit missing file is an error); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "read kubeconfig") {
		t.Fatalf("stderr missing read error: %q", stderr.String())
	}
}

func TestRunDiagnoseInvalidFormat(t *testing.T) {
	isolateHome(t)
	var stdout, stderr bytes.Buffer
	if code := runDiagnose([]string{"-o", "yaml"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code = %d, want 2 for invalid format", code)
	}
}

// TestDiagnoseClusterViaFakeAPIServer runs the real-cluster code path against
// an httptest server serving a crash-loop pod plus its warning events; it
// exercises apiClient.listPods / podEvents and the REST projection.
func TestDiagnoseClusterViaFakeAPIServer(t *testing.T) {
	pods, events := demoCluster()
	var crashPod k8sgateway.Pod
	for _, pod := range pods {
		if pod.Metadata.UID == "uid-crashloop" {
			crashPod = pod
			break
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pods") && !strings.Contains(r.URL.Path, "/events"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []k8sgateway.Pod{crashPod}})
		case strings.Contains(r.URL.Path, "/events"):
			list := make([]k8sgateway.Event, 0, len(events["uid-crashloop"]))
			list = append(list, events["uid-crashloop"]...)
			_ = json.NewEncoder(w).Encode(map[string]any{"items": list})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := diagnoseCluster(context.Background(), server.Client(), server.URL, "", "default", "")
	if err != nil {
		t.Fatalf("diagnoseCluster: %v", err)
	}
	if result.Mode != "cluster" || result.Scanned != 1 || len(result.Findings) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Findings[0].RuleID != "pod.crash_loop_backoff.v1" {
		t.Fatalf("finding rule = %q, want crash loop", result.Findings[0].RuleID)
	}
}

// TestDiagnoseClusterSinglePodNotFound verifies the explicit --pod path errors
// with exit 2 when the pod does not exist in the namespace.
func TestDiagnoseClusterSinglePodNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []k8sgateway.Pod{}})
	}))
	defer server.Close()
	_, err := diagnoseCluster(context.Background(), server.Client(), server.URL, "", "default", "missing-pod")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want pod not found", err)
	}
}

// TestApiClientNon2xx propagates upstream errors as exit-2 diagnostics.
func TestApiClientNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()
	api := &apiClient{client: server.Client(), server: server.URL}
	_, err := api.listPods(context.Background(), "default")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want 403 propagated", err)
	}
}
