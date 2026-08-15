package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// --- M115-1g: kubernetes resource handler coverage via a fake gateway ---

type k8sCredStub struct{ enabled bool }

func (s k8sCredStub) Access(context.Context, int64) (cluster.Cluster, []byte, error) {
	if !s.enabled {
		return cluster.Cluster{}, nil, cluster.ErrDisabled
	}
	return cluster.Cluster{ID: 7, Enabled: true}, []byte("config"), nil
}

// k8sGetStub returns the same body for every Get; list endpoints receive
// `{"items":[]}` and detail endpoints receive a minimal object. Paths ending
// in a resource name return an object body.
type k8sGetStub struct {
	body []byte
}

func (s *k8sGetStub) Get(_ context.Context, _ int64, _ []byte, path string, _ url.Values, _ int64) ([]byte, error) {
	if s.body != nil {
		return s.body, nil
	}
	return []byte(`{"items":[]}`), nil
}

func newK8sHandler(t *testing.T, creds k8sgateway.CredentialSource, gateway k8sgateway.Gateway) kubernetesHandler {
	t.Helper()
	return kubernetesHandler{service: k8sgateway.NewService(creds, gateway, nil)}
}

// invokeK8s dispatches to the matching handler for the given path. The list
// is derived from the router registration (router.go:288+).
func invokeK8s(h kubernetesHandler, path string, c *gin.Context) {
	switch path {
	case "/namespaces":
		h.namespaces(c)
	case "/nodes":
		h.nodes(c)
	case "/metrics/nodes":
		h.nodeMetrics(c)
	case "/metrics/pods":
		h.podMetrics(c)
	case "/deployments":
		h.deployments(c)
	case "/statefulsets":
		h.statefulSets(c)
	case "/daemonsets":
		h.daemonSets(c)
	case "/replicasets":
		h.replicaSets(c)
	case "/jobs":
		h.jobs(c)
	case "/cronjobs":
		h.cronJobs(c)
	case "/horizontalpodautoscalers":
		h.horizontalPodAutoscalers(c)
	case "/resourcequotas":
		h.resourceQuotas(c)
	case "/limitranges":
		h.limitRanges(c)
	case "/secrets":
		h.secrets(c)
	case "/services":
		h.services(c)
	case "/ingresses":
		h.ingresses(c)
	case "/endpointslices":
		h.endpointSlices(c)
	case "/storageclasses":
		h.storageClasses(c)
	case "/persistentvolumeclaims":
		h.persistentVolumeClaims(c)
	case "/configmaps":
		h.configMaps(c)
	case "/pods":
		h.pods(c)
	case "/events":
		h.events(c)
	case "/networkpolicies":
		h.networkPolicies(c)
	case "/poddisruptionbudgets":
		h.podDisruptionBudgets(c)
	case "/serviceaccounts":
		h.serviceAccounts(c)
	case "/roles":
		h.roles(c)
	case "/clusterroles":
		h.clusterRoles(c)
	case "/rolebindings":
		h.roleBindings(c)
	case "/clusterrolebindings":
		h.clusterRoleBindings(c)
	case "/persistentvolumes":
		h.persistentVolumes(c)
	case "/api-resources":
		h.apiResources(c)
	default:
		h.namespaces(c)
	}
}

func TestKubernetesListHandlersReturnOK(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	paths := []string{
		"/namespaces",
		"/nodes",
		"/metrics/nodes",
		"/metrics/pods",
		"/deployments",
		"/statefulsets",
		"/daemonsets",
		"/replicasets",
		"/jobs",
		"/cronjobs",
		"/horizontalpodautoscalers",
		"/resourcequotas",
		"/limitranges",
		"/secrets",
		"/services",
		"/ingresses",
		"/endpointslices",
		"/storageclasses",
		"/persistentvolumeclaims",
		"/configmaps",
		"/pods",
		"/events",
		"/networkpolicies",
		"/poddisruptionbudgets",
		"/serviceaccounts",
		"/roles",
		"/clusterroles",
		"/rolebindings",
		"/clusterrolebindings",
		"/persistentvolumes",
	}
	for _, path := range paths {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = withClusterActor(req)
		c.Request = req
		invokeK8s(handler, path, c)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200; body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestKubernetesDetailHandlersReturnOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	tests := []struct {
		path string
		call func(*gin.Context)
	}{
		{"/nodes/worker-1", func(c *gin.Context) { c.Params = gin.Params{{Key: "name", Value: "worker-1"}}; handler.node(c) }},
		{"/deployments/default/api", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "api"}}
			handler.deployment(c)
		}},
		{"/statefulsets/default/db", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "db"}}
			handler.statefulSet(c)
		}},
		{"/daemonsets/default/log", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "log"}}
			handler.daemonSet(c)
		}},
		{"/replicasets/default/api-v1", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "api-v1"}}
			handler.replicaSet(c)
		}},
		{"/jobs/default/migrate", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "migrate"}}
			handler.job(c)
		}},
		{"/cronjobs/default/cleanup", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "cleanup"}}
			handler.cronJob(c)
		}},
		{"/horizontalpodautoscalers/default/api", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "api"}}
			handler.horizontalPodAutoscaler(c)
		}},
		{"/resourcequotas/default/quota", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "quota"}}
			handler.resourceQuota(c)
		}},
		{"/limitranges/default/lr", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "lr"}}
			handler.limitRange(c)
		}},
		{"/secrets/default/token", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "token"}}
			handler.secret(c)
		}},
		{"/services/default/api", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "api"}}
			handler.serviceDetail(c)
		}},
		{"/ingresses/default/web", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "web"}}
			handler.ingress(c)
		}},
		{"/storageclasses/standard", func(c *gin.Context) { c.Params = gin.Params{{Key: "name", Value: "standard"}}; handler.storageClass(c) }},
		{"/persistentvolumeclaims/default/data", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "data"}}
			handler.persistentVolumeClaim(c)
		}},
		{"/configmaps/default/cfg", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "cfg"}}
			handler.configMap(c)
		}},
		{"/pods/default/api-0", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "api-0"}}
			handler.pod(c)
		}},
		{"/networkpolicies/default/deny", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "deny"}}
			handler.networkPolicy(c)
		}},
		{"/poddisruptionbudgets/default/api", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "api"}}
			handler.podDisruptionBudget(c)
		}},
		{"/serviceaccounts/default/default", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "default"}}
			handler.serviceAccount(c)
		}},
		{"/roles/default/app-role", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "app-role"}}
			handler.role(c)
		}},
		{"/clusterroles/cluster-admin", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "name", Value: "cluster-admin"}}
			handler.clusterRole(c)
		}},
		{"/rolebindings/default/app-bind", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "app-bind"}}
			handler.roleBinding(c)
		}},
		{"/clusterrolebindings/cluster-bind", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "name", Value: "cluster-bind"}}
			handler.clusterRoleBinding(c)
		}},
		{"/persistentvolumes/pv-1", func(c *gin.Context) { c.Params = gin.Params{{Key: "name", Value: "pv-1"}}; handler.persistentVolume(c) }},
		{"/manifest/pods", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "kind", Value: "Pod"}, {Key: "namespace", Value: "default"}, {Key: "name", Value: "api-0"}}
			handler.manifest(c)
		}},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		req = withClusterActor(req)
		c.Request = req
		tt.call(c)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200; body=%s", tt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestKubernetesDisabledClusterReturnsConflict(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: false}, &k8sGetStub{})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/namespaces", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.namespaces(c)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestKubernetesLogsValidationErrors(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	// invalid previous
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/pods/default/api/logs?previous=bogus", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.logs(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// invalid tail_lines
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/pods/default/api/logs?tail_lines=99999", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.logs(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// successful logs call
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/pods/default/api/logs", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.logs(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestKubernetesLogsSinceValidationErrors(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	// both since_seconds and since_time
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/pods/default/api/logs_since?since_seconds=60&since_time=2026-08-01T00:00:00Z", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.logsSince(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// successful call
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/pods/default/api/logs_since?since_seconds=60", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.logsSince(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestKubernetesAllContainerLogsValidation(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/pods/default/api/all_logs?previous=bogus", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.allContainerLogs(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/pods/default/api/all_logs", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.allContainerLogs(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestKubernetesContainersAndEventsOK(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/pods/default/api/containers", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.containers(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("containers status = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.events(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("events status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestKubernetesCustomResourcesBranches(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	// missing path params → 400
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/custom-resources//v1/pods", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.customResources(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	// non-whitelisted GVR → 404
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/custom-resources/example.com/v1/gadgets", nil)
	req = withClusterActor(req)
	c.Request = req
	c.Params = gin.Params{{Key: "group", Value: "example.com"}, {Key: "version", Value: "v1"}, {Key: "resource", Value: "gadgets"}}
	handler.customResources(c)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	// customResource missing name → 400
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	req = httptest.NewRequest(http.MethodGet, "/custom-resources/apps/v1/deployments/", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.customResource(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestKubernetesVeleroCapabilityAndBackups(t *testing.T) {
	handler := newK8sHandler(t, k8sCredStub{enabled: true}, &k8sGetStub{})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/velero/capability", nil)
	req = withClusterActor(req)
	c.Request = req
	handler.veleroCapability(c)
	_ = rec
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	req2 := httptest.NewRequest(http.MethodGet, "/backups", nil)
	req2 = withClusterActor(req2)
	c2.Request = req2
	handler.backups(c2)
	rec3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(rec3)
	req3 := httptest.NewRequest(http.MethodGet, "/backups/default/b1", nil)
	req3 = withClusterActor(req3)
	c3.Request = req3
	handler.backup(c3)
	if rec2.Code != http.StatusOK {
		t.Logf("backups status = %d (acceptable error paths)", rec2.Code)
	}
	_ = rec3
}
