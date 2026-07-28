package cluster

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testKey = "ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE="

func TestEncryptorRoundTripAndRandomNonce(t *testing.T) {
	encryptor, err := NewEncryptor(testKey, "v1")
	if err != nil {
		t.Fatal(err)
	}
	first, version, err := encryptor.Encrypt([]byte("secret kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, _ := encryptor.Encrypt([]byte("secret kubeconfig"))
	if string(first) == "secret kubeconfig" || string(first) == string(second) {
		t.Fatal("encryption is plaintext or nonce was reused")
	}
	plaintext, err := encryptor.Decrypt(first)
	if err != nil || string(plaintext) != "secret kubeconfig" || version != "v1" {
		t.Fatalf("round trip = %q, %q, %v", plaintext, version, err)
	}
}

func TestParseKubeconfig(t *testing.T) {
	raw := strings.Replace(
		testKubeconfig("https://host.docker.internal:6443", "test-token", true),
		"    insecure-skip-tls-verify: true",
		"    insecure-skip-tls-verify: true\n    tls-server-name: 127.0.0.1",
		1,
	)
	config, err := ParseKubeconfig([]byte(raw))
	if err != nil {
		t.Fatalf("ParseKubeconfig() error = %v", err)
	}
	if config.Server != "https://host.docker.internal:6443" || config.Token != "test-token" || config.Transport.TLSClientConfig.ServerName != "127.0.0.1" {
		t.Fatalf("config = %#v", config)
	}
	if _, err := ParseKubeconfig([]byte("current-context: missing")); err == nil {
		t.Fatal("invalid kubeconfig accepted")
	}
}

func TestRegistryProbe(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" || r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"gitVersion":"v1.33.2"}`))
	}))
	defer server.Close()
	registry := NewRegistry(time.Second)
	version, err := registry.Probe(context.Background(), 1, []byte(testKubeconfig(server.URL, "test-token", true)))
	if err != nil || version != "v1.33.2" {
		t.Fatalf("Probe() = %q, %v", version, err)
	}
	registry.Invalidate(1)
}

func TestRegistryPatchUsesExactMethodBodyAndDryRun(t *testing.T) {
	patch := []byte(`{"metadata":{"resourceVersion":"17"}}`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPatch || r.URL.Path != "/apis/apps/v1/namespaces/demo/deployments/api" || r.URL.Query().Get("dryRun") != "All" || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Content-Type") != "application/strategic-merge-patch+json" || string(body) != string(patch) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"metadata":{"uid":"deployment-1","resourceVersion":"17"}}`))
	}))
	defer server.Close()
	registry := NewRegistry(time.Second)
	body, err := registry.Patch(context.Background(), 1, []byte(testKubeconfig(server.URL, "test-token", true)), "/apis/apps/v1/namespaces/demo/deployments/api", url.Values{"dryRun": {"All"}}, "application/strategic-merge-patch+json", patch, 1<<20)
	if err != nil || !strings.Contains(string(body), "deployment-1") {
		t.Fatalf("body=%s err=%v", body, err)
	}
	if _, err := registry.Patch(context.Background(), 1, []byte(testKubeconfig(server.URL, "test-token", true)), "/api", nil, "application/json", patch, 1<<20); err == nil {
		t.Fatal("unsupported patch content type accepted")
	}
}

func TestRegistryDoesNotFollowKubernetesRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	registry := NewRegistry(time.Second)
	_, err := registry.Patch(context.Background(), 2, []byte(testKubeconfig(source.URL, "test-token", true)), "/apis/apps/v1/namespaces/demo/deployments/api", nil, "application/strategic-merge-patch+json", []byte(`{"metadata":{"resourceVersion":"17"}}`), 1<<20)
	var status APIStatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTemporaryRedirect || redirected {
		t.Fatalf("error=%v redirected=%v", err, redirected)
	}
}

type clusterRepositoryStub struct {
	item          Cluster
	credential    Credential
	updatedServer string
	updatedCred   Credential
	updatedAt     time.Time
	conditions    []Condition
}

func (r *clusterRepositoryStub) List(context.Context) ([]Cluster, error) {
	return []Cluster{r.item}, nil
}
func (r *clusterRepositoryStub) Find(context.Context, int64) (Cluster, Credential, error) {
	if r.item.ID == 0 {
		return Cluster{}, Credential{}, ErrNotFound
	}
	return r.item, r.credential, nil
}
func (r *clusterRepositoryStub) Create(context.Context, *Cluster, Credential) error { return nil }
func (r *clusterRepositoryStub) UpdateCredential(_ context.Context, _ int64, apiServer string, credential Credential, now time.Time, conditions []Condition) error {
	r.updatedServer, r.updatedCred, r.updatedAt, r.conditions = apiServer, credential, now, conditions
	return nil
}
func (r *clusterRepositoryStub) SetEnabled(context.Context, int64, bool) error { return nil }
func (r *clusterRepositoryStub) UpdateProbe(context.Context, int64, string, string, time.Time, []Condition) error {
	return nil
}
func (r *clusterRepositoryStub) Delete(context.Context, int64) error { return nil }

type proberStub struct{ invalidated []int64 }

func (p *proberStub) Probe(context.Context, int64, []byte) (string, error) {
	return "", errors.New("not used")
}
func (p *proberStub) Invalidate(id int64) { p.invalidated = append(p.invalidated, id) }

func TestUpdateCredentialValidatesEncryptsAndInvalidatesCache(t *testing.T) {
	encryptor, err := NewEncryptor(testKey, "v1")
	if err != nil {
		t.Fatal(err)
	}
	repository := &clusterRepositoryStub{item: Cluster{ID: 7, Name: "dev", Enabled: true}}
	prober := &proberStub{}
	service := NewService(repository, encryptor, prober)
	raw := []byte(testKubeconfig("https://new.example.test:6443", "replacement-token", true))
	updated, err := service.UpdateCredential(context.Background(), 7, raw)
	if err != nil || updated.ID != 7 || repository.updatedServer != "https://new.example.test:6443" {
		t.Fatalf("updated=%#v server=%q err=%v", updated, repository.updatedServer, err)
	}
	if string(repository.updatedCred.EncryptedKubeconfig) == string(raw) || len(prober.invalidated) != 1 || prober.invalidated[0] != 7 {
		t.Fatalf("credential/cache state=%#v invalidated=%v", repository.updatedCred, prober.invalidated)
	}
	if len(repository.conditions) != 3 || repository.conditions[0].Reason != "CredentialsUpdated" || repository.conditions[0].Status != "Unknown" {
		t.Fatalf("conditions=%#v", repository.conditions)
	}
}

func TestUpdateCredentialRejectsInvalidConfigBeforeRepository(t *testing.T) {
	encryptor, _ := NewEncryptor(testKey, "v1")
	repository := &clusterRepositoryStub{item: Cluster{ID: 7, Name: "dev", Enabled: true}}
	service := NewService(repository, encryptor, &proberStub{})
	if _, err := service.UpdateCredential(context.Background(), 7, []byte("not-a-kubeconfig")); !errors.Is(err, ErrInvalidKubeconfig) {
		t.Fatalf("UpdateCredential() error=%v", err)
	}
	if repository.updatedServer != "" {
		t.Fatal("repository updated despite invalid kubeconfig")
	}
}

func testKubeconfig(server, token string, insecure bool) string {
	return strings.Join([]string{
		"apiVersion: v1", "kind: Config", "current-context: test", "clusters:", "- name: test-cluster",
		"  cluster:", "    server: " + server, "    insecure-skip-tls-verify: " + strings.ToLower(base64Marker(insecure)),
		"contexts:", "- name: test", "  context:", "    cluster: test-cluster", "    user: test-user",
		"users:", "- name: test-user", "  user:", "    token: " + token,
	}, "\n")
}

func base64Marker(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
