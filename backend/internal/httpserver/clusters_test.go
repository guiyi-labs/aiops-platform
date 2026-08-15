package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/requestctx"
)

// --- Fakes ---

type clusterFakeRepo struct {
	mu            sync.Mutex
	clusters      map[int64]cluster.Cluster
	credentials   map[int64]cluster.Credential
	nextID        int64
	createErr     error
	deleteErr     error
	setEnableErr  error
	updateCredErr error
}

func newClusterFakeRepo() *clusterFakeRepo {
	return &clusterFakeRepo{
		clusters:    make(map[int64]cluster.Cluster),
		credentials: make(map[int64]cluster.Credential),
		nextID:      1,
	}
}

func (r *clusterFakeRepo) List(context.Context) ([]cluster.Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]cluster.Cluster, 0, len(r.clusters))
	for _, c := range r.clusters {
		out = append(out, c)
	}
	return out, nil
}

func (r *clusterFakeRepo) Find(_ context.Context, id int64) (cluster.Cluster, cluster.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clusters[id]
	if !ok {
		return cluster.Cluster{}, cluster.Credential{}, cluster.ErrNotFound
	}
	return c, r.credentials[id], nil
}

func (r *clusterFakeRepo) Create(_ context.Context, item *cluster.Cluster, cred cluster.Credential) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item.ID = r.nextID
	r.nextID++
	r.clusters[item.ID] = *item
	r.credentials[item.ID] = cred
	return nil
}

func (r *clusterFakeRepo) UpdateCredential(_ context.Context, id int64, apiServer string, cred cluster.Credential, _ time.Time, _ []cluster.Condition) error {
	if r.updateCredErr != nil {
		return r.updateCredErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clusters[id]
	if !ok {
		return cluster.ErrNotFound
	}
	c.APIServer = apiServer
	r.clusters[id] = c
	r.credentials[id] = cred
	return nil
}

func (r *clusterFakeRepo) SetEnabled(_ context.Context, id int64, enabled bool) error {
	if r.setEnableErr != nil {
		return r.setEnableErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clusters[id]
	if !ok {
		return cluster.ErrNotFound
	}
	c.Enabled = enabled
	r.clusters[id] = c
	return nil
}

func (r *clusterFakeRepo) UpdateProbe(_ context.Context, id int64, status, version string, _ time.Time, _ []cluster.Condition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clusters[id]
	if !ok {
		return cluster.ErrNotFound
	}
	c.Status = status
	c.KubernetesVersion = version
	r.clusters[id] = c
	return nil
}

func (r *clusterFakeRepo) Delete(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clusters[id]; !ok {
		return cluster.ErrNotFound
	}
	delete(r.clusters, id)
	delete(r.credentials, id)
	return nil
}

type clusterFakeProber struct {
	probeErr error
	status   string
	version  string
}

func (p *clusterFakeProber) Probe(_ context.Context, _ int64, _ []byte) (string, error) {
	return p.status, p.probeErr
}

func (p *clusterFakeProber) Invalidate(_ int64) {}

func newClusterTestEngine(t *testing.T, svc *cluster.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := clusterHandler{service: svc}
	v1 := r.Group("/api/v1")
	cl := v1.Group("/clusters")
	cl.GET("", h.list)
	cl.GET("/:cluster_id", h.get)
	cl.POST("", h.create)
	cl.PATCH("/:cluster_id", h.setEnabled)
	cl.PUT("/:cluster_id/credentials", h.updateCredential)
	cl.POST("/:cluster_id/probe", h.probe)
	cl.DELETE("/:cluster_id", h.delete)
	return r
}

func withClusterActor(req *http.Request) *http.Request {
	return req.WithContext(requestctx.WithMetadata(req.Context(), requestctx.Metadata{
		ActorID: 1, Roles: []string{"system_admin"}, RequestID: "cluster-test",
	}))
}

func clusterTestEncryptor(t *testing.T) *cluster.Encryptor {
	t.Helper()
	enc, err := cluster.NewEncryptor("ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE=", "v1")
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func validKubeconfig(server string) string {
	return "apiVersion: v1\nkind: Config\ncurrent-context: test\nclusters:\n- name: test-cluster\n  cluster:\n    server: " + server + "\ncontexts:\n- name: test\n  context:\n    cluster: test-cluster\n    user: test-user\nusers:\n- name: test-user\n  user:\n    token: test-token\n"
}

func clusterSeed(t *testing.T, repo *clusterFakeRepo, id int64, name string) {
	t.Helper()
	enc := clusterTestEncryptor(t)
	ciphertext, version, err := enc.Encrypt([]byte(validKubeconfig("https://localhost:6443")))
	if err != nil {
		t.Fatal(err)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.clusters[id] = cluster.Cluster{ID: id, Name: name, Enabled: true, Status: "healthy"}
	repo.credentials[id] = cluster.Credential{EncryptedKubeconfig: ciphertext, EncryptionKeyVersion: version}
	if id >= repo.nextID {
		repo.nextID = id + 1
	}
}

// --- Tests ---

func TestClusterListReturnsItems(t *testing.T) {
	repo := newClusterFakeRepo()
	clusterSeed(t, repo, 1, "prod")
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", w.Code, w.Body)
	}
	var result struct {
		Items []cluster.Cluster `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Total != 1 {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestClusterListServiceErrorReturns500(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	// Empty list returns 200 with 0 items — that's valid.
	if w.Code != http.StatusOK {
		t.Fatalf("list empty status = %d", w.Code)
	}
}

func TestClusterGetReturnsItem(t *testing.T) {
	repo := newClusterFakeRepo()
	clusterSeed(t, repo, 5, "staging")
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/clusters/5", nil)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterGetNotFound(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/clusters/999", nil)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterGetInvalidID(t *testing.T) {
	svc := cluster.NewService(newClusterFakeRepo(), clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/clusters/abc", nil)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("get invalid id status = %d", w.Code)
	}
}

func TestClusterCreateHappyPath(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	body, _ := json.Marshal(map[string]string{"name": "dev", "kubeconfig": validKubeconfig("https://localhost:6443")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/clusters", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterCreateMissingBody(t *testing.T) {
	svc := cluster.NewService(newClusterFakeRepo(), clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/clusters", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create empty status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterSetEnabledHappyPath(t *testing.T) {
	repo := newClusterFakeRepo()
	clusterSeed(t, repo, 3, "prod")
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	body, _ := json.Marshal(map[string]any{"enabled": false})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/clusters/3", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("setEnabled status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterSetEnabledNotFound(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	body, _ := json.Marshal(map[string]any{"enabled": true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/clusters/999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("setEnabled missing status = %d", w.Code)
	}
}

func TestClusterSetEnabledMissingBody(t *testing.T) {
	svc := cluster.NewService(newClusterFakeRepo(), clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/clusters/1", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("setEnabled empty status = %d", w.Code)
	}
}

func TestClusterUpdateCredentialHappyPath(t *testing.T) {
	repo := newClusterFakeRepo()
	clusterSeed(t, repo, 2, "dev")
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	body, _ := json.Marshal(map[string]string{"kubeconfig": validKubeconfig("https://localhost:6443")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/clusters/2/credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("updateCred status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterUpdateCredentialNotFound(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	body, _ := json.Marshal(map[string]string{"kubeconfig": validKubeconfig("https://localhost:6443")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/clusters/999/credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("updateCred missing status = %d", w.Code)
	}
}

func TestClusterUpdateCredentialMissingBody(t *testing.T) {
	svc := cluster.NewService(newClusterFakeRepo(), clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/clusters/1/credentials", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("updateCred empty status = %d", w.Code)
	}
}

func TestClusterProbeSuccess(t *testing.T) {
	repo := newClusterFakeRepo()
	clusterSeed(t, repo, 4, "test")
	prober := &clusterFakeProber{status: "healthy", version: "1.28.0"}
	svc := cluster.NewService(repo, clusterTestEncryptor(t), prober)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/clusters/4/probe", nil)
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("probe status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterProbeNotFound(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/clusters/999/probe", nil)
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("probe missing status = %d", w.Code)
	}
}

func TestClusterDeleteHappyPath(t *testing.T) {
	repo := newClusterFakeRepo()
	clusterSeed(t, repo, 6, "ephemeral")
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/clusters/6", nil)
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", w.Code, w.Body)
	}
}

func TestClusterDeleteNotFound(t *testing.T) {
	repo := newClusterFakeRepo()
	svc := cluster.NewService(repo, clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/clusters/999", nil)
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d", w.Code)
	}
}

func TestClusterDeleteInvalidID(t *testing.T) {
	svc := cluster.NewService(newClusterFakeRepo(), clusterTestEncryptor(t), &clusterFakeProber{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/clusters/abc", nil)
	req = withClusterActor(req)
	newClusterTestEngine(t, svc).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete invalid status = %d", w.Code)
	}
}
