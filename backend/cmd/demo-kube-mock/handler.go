package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// handler serves the minimal Kubernetes API surface the M102 demo journey
// touches. Objects are deterministic fixtures so every drill run produces the
// same evidence; PATCH mutations are merged into an in-memory store so the
// drill can verify that a controlled action actually landed on the cluster.
type handler struct {
	mu     sync.Mutex
	rv     int64
	nodes  map[string]map[string]any // name -> object
	pods   map[string]map[string]any // ns/name -> object
	deploy map[string]map[string]any // ns/name -> object
	events []map[string]any
	muts   []map[string]any
}

func newHandler() *handler {
	h := &handler{
		rv:     100,
		nodes:  map[string]map[string]any{},
		pods:   map[string]map[string]any{},
		deploy: map[string]map[string]any{},
	}
	h.nodes["demo-node"] = fixtureNode()
	h.pods["demo/demo-pod"] = fixturePod()
	h.deploy["demo/demo-app"] = fixtureDeployment()
	h.events = fixtureEvents()
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method

	switch {
	case method == http.MethodGet && path == "/healthz":
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case method == http.MethodGet && path == "/version":
		writeJSON(w, http.StatusOK, map[string]any{
			"major": "1", "minor": "36", "gitVersion": "v1.36.0",
			"gitCommit": "demo-mock", "gitTreeState": "clean",
			"buildDate": "2026-08-12T00:00:00Z", "goVersion": "go1.26",
			"compiler": "gc", "platform": "linux/arm64",
		})
	case method == http.MethodGet && path == "/mock/mutations":
		h.mu.Lock()
		muts := cloneList(h.muts)
		h.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"items": muts, "total": len(muts)})
	case method == http.MethodGet && path == "/api":
		writeJSON(w, http.StatusOK, map[string]any{
			"kind": "APIVersions", "apiVersion": "v1",
			"versions":                   []string{"v1"},
			"serverAddressByClientCIDRs": []any{},
		})
	case method == http.MethodGet && path == "/apis":
		writeJSON(w, http.StatusOK, map[string]any{
			"kind": "APIGroupList", "apiVersion": "v1",
			"groups": []map[string]any{
				apiGroup("apps", "apps/v1"),
				apiGroup("batch", "batch/v1"),
				apiGroup("networking.k8s.io", "networking.k8s.io/v1"),
				apiGroup("metrics.k8s.io", "metrics.k8s.io/v1beta1"),
				apiGroup("rbac.authorization.k8s.io", "rbac.authorization.k8s.io/v1"),
			},
		})
	case method == http.MethodGet && path == "/api/v1/nodes":
		h.mu.Lock()
		items := []any{}
		for _, n := range h.nodes {
			items = append(items, cloneMap(n))
		}
		h.mu.Unlock()
		writeList(w, "NodeList", "v1", items)
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/nodes/"):
		h.getObject(w, path, "Node", "/api/v1/nodes/", h.nodes)
	case method == http.MethodPatch && strings.HasPrefix(path, "/api/v1/nodes/"):
		h.patchObject(w, r, "Node", "/api/v1/nodes/", h.nodes)
	case method == http.MethodGet && path == "/api/v1/namespaces":
		writeList(w, "NamespaceList", "v1", []any{
			simpleMeta("Namespace", "v1", "default"),
			simpleMeta("Namespace", "v1", "kube-system"),
			simpleMeta("Namespace", "v1", "kube-public"),
			simpleMeta("Namespace", "v1", "demo"),
			simpleMeta("Namespace", "v1", "ops"),
		})
	case method == http.MethodGet && path == "/api/v1/pods":
		h.listPods(w, "")
	case method == http.MethodGet && path == "/api/v1/namespaces/demo/pods":
		h.listPods(w, "demo")
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/namespaces/demo/pods/"):
		h.getObject(w, path, "Pod", "/api/v1/namespaces/demo/pods/", h.pods)
	case method == http.MethodGet && path == "/api/v1/namespaces/demo/events":
		h.mu.Lock()
		items := cloneList(h.events)
		h.mu.Unlock()
		writeList(w, "EventList", "v1", items)
	case method == http.MethodGet && path == "/api/v1/events":
		writeList(w, "EventList", "v1", []any{})
	case method == http.MethodGet && path == "/apis/apps/v1/namespaces/demo/deployments":
		h.listDeployments(w, "demo")
	case method == http.MethodGet && strings.HasPrefix(path, "/apis/apps/v1/namespaces/demo/deployments/"):
		h.getObject(w, path, "Deployment", "/apis/apps/v1/namespaces/demo/deployments/", h.deploy)
	case method == http.MethodPatch && strings.HasPrefix(path, "/apis/apps/v1/namespaces/demo/deployments/"):
		h.patchObject(w, r, "Deployment", "/apis/apps/v1/namespaces/demo/deployments/", h.deploy)
	case method == http.MethodGet && path == "/apis/apps/v1/namespaces/demo/replicasets":
		writeList(w, "ReplicaSetList", "apps/v1", []any{fixtureReplicaSet()})
	case method == http.MethodGet && path == "/apis/apps/v1":
		writeJSON(w, http.StatusOK, apiGroup("apps", "apps/v1"))
	case method == http.MethodGet && path == "/apis/metrics.k8s.io/v1beta1/nodes":
		writeList(w, "NodeMetricsList", "metrics.k8s.io/v1beta1", []any{fixtureNodeMetric()})
	case method == http.MethodGet && path == "/apis/metrics.k8s.io/v1beta1/namespaces/demo/pods":
		writeList(w, "PodMetricsList", "metrics.k8s.io/v1beta1", []any{fixturePodMetric()})
	default:
		notFound(w, r)
	}
}

// ---------- object helpers ----------

func (h *handler) getObject(w http.ResponseWriter, path, kind, prefix string, table map[string]map[string]any) {
	name := strings.TrimPrefix(path, prefix)
	if name == "" || strings.Contains(name, "/") {
		notFoundWithMessage(w, kind, name)
		return
	}
	h.mu.Lock()
	obj, ok := table[keyFor(kind, "demo", name)]
	h.mu.Unlock()
	if !ok {
		notFoundWithMessage(w, kind, name)
		return
	}
	h.mu.Lock()
	body := cloneMap(obj)
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, body)
}

func (h *handler) listPods(w http.ResponseWriter, ns string) {
	h.mu.Lock()
	items := []any{}
	for k, p := range h.pods {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && (ns == "" || parts[0] == ns) {
			items = append(items, cloneMap(p))
		}
	}
	h.mu.Unlock()
	writeList(w, "PodList", "v1", items)
}

func (h *handler) listDeployments(w http.ResponseWriter, ns string) {
	h.mu.Lock()
	items := []any{}
	for k, d := range h.deploy {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && (ns == "" || parts[0] == ns) {
			items = append(items, cloneMap(d))
		}
	}
	h.mu.Unlock()
	writeList(w, "DeploymentList", "apps/v1", items)
}

func (h *handler) patchObject(w http.ResponseWriter, r *http.Request, kind, prefix string, table map[string]map[string]any) {
	name := strings.TrimPrefix(r.URL.Path, prefix)
	if name == "" || strings.Contains(name, "/") {
		notFoundWithMessage(w, kind, name)
		return
	}
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"kind": "Status", "apiVersion": "v1", "metadata": map[string]any{},
			"status": "Failure", "message": "invalid patch body", "reason": "BadRequest", "code": 400,
		})
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == "All"

	h.mu.Lock()
	defer h.mu.Unlock()
	key := keyFor(kind, "demo", name)
	obj, ok := table[key]
	if !ok {
		notFoundWithMessage(w, kind, name)
		return
	}
	curMeta, _ := obj["metadata"].(map[string]any)
	if pm, ok := patch["metadata"].(map[string]any); ok {
		if uid, ok := pm["uid"].(string); ok && uid != "" && curMeta["uid"] != uid {
			writeJSON(w, http.StatusConflict, conflictStatus("uid mismatch"))
			return
		}
		if rv, ok := pm["resourceVersion"].(string); ok && rv != "" && curMeta["resourceVersion"] != rv {
			writeJSON(w, http.StatusConflict, conflictStatus("resourceVersion mismatch"))
			return
		}
	}

	updated := deepClone(obj)
	applyPatch(updated, patch)
	h.rv++
	meta, _ := updated["metadata"].(map[string]any)
	meta["resourceVersion"] = strconv.FormatInt(h.rv, 10)
	updated["metadata"] = meta

	body := cloneMap(updated)
	if !dryRun {
		table[key] = updated
		h.muts = append(h.muts, map[string]any{
			"kind": kind, "name": name, "dry_run": dryRun,
			"patch": patch, "applied_at": r.URL.RawQuery,
		})
	}
	writeJSON(w, http.StatusOK, body)
}

// keyFor returns the store key for an object route. Pods and Deployments live
// under "ns/name" keys; Nodes are cluster-scoped and keyed by name.
func keyFor(kind, ns, name string) string {
	if kind == "Pod" || kind == "Deployment" {
		return ns + "/" + name
	}
	return name
}

// applyPatch deep-merges a strategic-merge-style patch into current. Maps are
// merged recursively; scalars and arrays are replaced (sufficient for the
// restart / scale / image / cordon patches the platform demo uses).
func applyPatch(current map[string]any, patch map[string]any) {
	for k, v := range patch {
		pm, ok := v.(map[string]any)
		if !ok {
			current[k] = v
			continue
		}
		cm, ok := current[k].(map[string]any)
		if !ok || cm == nil {
			current[k] = cloneMap(pm)
			continue
		}
		applyPatch(cm, pm)
	}
}

func conflictStatus(message string) map[string]any {
	return map[string]any{
		"kind": "Status", "apiVersion": "v1", "metadata": map[string]any{},
		"status": "Failure", "message": message, "reason": "Conflict", "code": 409,
	}
}

func notFoundWithMessage(w http.ResponseWriter, kind, name string) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"kind": "Status", "apiVersion": "v1", "metadata": map[string]any{},
		"status":  "Failure",
		"message": fmt.Sprintf("%s %q not found", kind, name),
		"reason":  "NotFound", "code": 404,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeList(w http.ResponseWriter, kind, apiVersion string, items []any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"kind": kind, "apiVersion": apiVersion,
		"metadata": map[string]any{"resourceVersion": "1"},
		"items":    items,
	})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"kind": "Status", "apiVersion": "v1",
		"metadata": map[string]any{},
		"status":   "Failure",
		"message":  fmt.Sprintf("the server could not find the requested resource: %s %s", r.Method, r.URL.Path),
		"reason":   "NotFound", "code": 404,
	})
}

func apiGroup(name, version string) map[string]any {
	return map[string]any{
		"name": name,
		"versions": []map[string]any{{
			"groupVersion": version, "version": strings.TrimPrefix(version, name+"/"),
		}},
		"preferredVersion":           map[string]any{"groupVersion": version, "version": strings.TrimPrefix(version, name+"/")},
		"serverAddressByClientCIDRs": []any{},
	}
}

func simpleMeta(kind, apiVersion, name string) map[string]any {
	return map[string]any{
		"kind": kind, "apiVersion": apiVersion,
		"metadata": map[string]any{"name": name, "resourceVersion": "1"},
	}
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// deepClone returns a JSON-round-trip copy so mutations on the copy never
// touch the stored object (dry-run must not persist).
func deepClone(m map[string]any) map[string]any {
	raw, err := json.Marshal(m)
	if err != nil {
		return cloneMap(m)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return cloneMap(m)
	}
	return out
}

func cloneList(in []map[string]any) []any {
	out := make([]any, 0, len(in))
	for _, item := range in {
		out = append(out, cloneMap(item))
	}
	return out
}
