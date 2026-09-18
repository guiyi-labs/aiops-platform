package cluster

import (
	"context"
	"errors"
	"testing"
	"time"
)

// recordingRepository is a hermetic Repository stub that records the calls
// Service makes, so tests can assert on registration / probe behaviour without
// a database. It intentionally does not persist state across calls.
type recordingRepository struct {
	cluster        Cluster
	cred           Credential
	findErr        error
	createdCluster *Cluster
	createdCred    *Credential
	updatedStatus  string
	updatedVersion string
	updatedCred    Credential
	updatedCond    []Condition
}

func (r *recordingRepository) List(context.Context) ([]Cluster, error) {
	return []Cluster{r.cluster}, nil
}
func (r *recordingRepository) Find(_ context.Context, _ int64) (Cluster, Credential, error) {
	if r.findErr != nil {
		return Cluster{}, Credential{}, r.findErr
	}
	return r.cluster, r.cred, nil
}
func (r *recordingRepository) Create(_ context.Context, c *Cluster, cred Credential) error {
	rc, rcred := c, cred
	r.createdCluster, r.createdCred = rc, &rcred
	return nil
}
func (r *recordingRepository) UpdateCredential(_ context.Context, _ int64, apiServer string, cred Credential, _ time.Time, cond []Condition) error {
	r.updatedVersion, r.updatedCred, r.updatedCond = apiServer, cred, cond
	return nil
}
func (r *recordingRepository) SetEnabled(context.Context, int64, bool) error { return nil }
func (r *recordingRepository) UpdateProbe(_ context.Context, _ int64, status, version string, _ time.Time, cond []Condition) error {
	r.updatedStatus, r.updatedVersion, r.updatedCond = status, version, cond
	return nil
}
func (r *recordingRepository) Delete(context.Context, int64) error { return nil }

// fakeProber is a Prober stub whose result is fully controllable per test.
type fakeProber struct {
	version     string
	err         error
	invalidated []int64
}

func (p *fakeProber) Probe(context.Context, int64, []byte) (string, error) {
	return p.version, p.err
}
func (p *fakeProber) Invalidate(id int64) {
	p.invalidated = append(p.invalidated, id)
}

func TestServiceCreateRegistersClusterAndEncryptsCredential(t *testing.T) {
	encryptor, _ := NewEncryptor(testKey, "v1")
	repo := &recordingRepository{}
	svc := NewService(repo, encryptor, &fakeProber{})

	if _, err := svc.Create(context.Background(), "  ", []byte(testKubeconfig("https://h:6443", "tok", true))); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("empty name error = %v", err)
	}
	if _, err := svc.Create(context.Background(), "dev", []byte("not-a-kubeconfig")); err == nil {
		t.Fatal("invalid kubeconfig accepted")
	}
	raw := []byte(testKubeconfig("https://h.example:6443", "tok", true))
	cluster, err := svc.Create(context.Background(), "dev", raw)
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Name != "dev" || cluster.APIServer != "https://h.example:6443" || cluster.Enabled || cluster.Status != StatusDisabled {
		t.Fatalf("cluster = %#v", cluster)
	}
	if repo.createdCred == nil || string(repo.createdCred.EncryptedKubeconfig) == string(raw) || repo.createdCred.EncryptionKeyVersion != "v1" {
		t.Fatalf("credential not encrypted as expected: %#v", repo.createdCred)
	}
}

func TestServiceProbeUpdatesConditionsOnSuccessAndFailure(t *testing.T) {
	encryptor, _ := NewEncryptor(testKey, "v1")
	raw := []byte(testKubeconfig("https://h:6443", "tok", true))
	enc, ver, _ := encryptor.Encrypt(raw)
	repo := &recordingRepository{cluster: Cluster{ID: 1, Enabled: true}, cred: Credential{EncryptedKubeconfig: enc, EncryptionKeyVersion: ver}}
	prober := &fakeProber{version: "v1.36.1"}
	svc := NewService(repo, encryptor, prober)

	if _, err := svc.Probe(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if repo.updatedStatus != StatusReady || repo.updatedVersion != "v1.36.1" {
		t.Fatalf("status=%q version=%q", repo.updatedStatus, repo.updatedVersion)
	}
	if repo.updatedCond[0].Status != "True" || repo.updatedCond[1].Status != "True" || repo.updatedCond[2].Status != "True" {
		t.Fatalf("conditions=%#v", repo.updatedCond)
	}

	prober.err = errors.New("unreachable")
	if _, err := svc.Probe(context.Background(), 1); err == nil {
		t.Fatal("probe failure not propagated")
	}
	if repo.updatedStatus != StatusUnreachable {
		t.Fatalf("status=%q", repo.updatedStatus)
	}
	if repo.updatedCond[1].Status != "False" {
		t.Fatalf("reachable condition = %q", repo.updatedCond[1].Status)
	}
}

func TestServiceAccessRejectsDisabledAndReturnsPlaintext(t *testing.T) {
	encryptor, _ := NewEncryptor(testKey, "v1")
	raw := []byte(testKubeconfig("https://h:6443", "tok", true))
	enc, ver, _ := encryptor.Encrypt(raw)
	repo := &recordingRepository{cluster: Cluster{ID: 1, Enabled: false}, cred: Credential{EncryptedKubeconfig: enc, EncryptionKeyVersion: ver}}
	svc := NewService(repo, encryptor, &fakeProber{})

	if _, _, err := svc.Access(context.Background(), 1); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
	repo.cluster.Enabled = true
	c, plaintext, err := svc.Access(context.Background(), 1)
	if err != nil || c.ID != 1 || string(plaintext) != string(raw) {
		t.Fatalf("access = %v %q %v", c, plaintext, err)
	}
}

func TestValidatePathRejectsBadPrefix(t *testing.T) {
	if err := validatePath("/api/v1"); err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	if err := validatePath("api/v1"); err == nil {
		t.Fatal("missing leading slash accepted")
	}
	if err := validatePath("//api"); err == nil {
		t.Fatal("double slash accepted")
	}
}

func TestTypesPatchTypeMapsStrategicMerge(t *testing.T) {
	if got := typesPatchType("application/strategic-merge-patch+json"); got != "application/strategic-merge-patch+json" {
		t.Fatalf("strategic merge = %q", got)
	}
	if got := typesPatchType("application/json"); got != "application/json" {
		t.Fatalf("fallback = %q", got)
	}
}
