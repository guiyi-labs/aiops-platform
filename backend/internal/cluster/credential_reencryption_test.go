package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"sort"
	"testing"
	"time"
)

type credentialReencryptionRepositoryStub struct {
	credentials map[int64]Credential
	started     []CredentialReencryptionResult
	finished    []CredentialReencryptionResult
	writes      int
}

func (r *credentialReencryptionRepositoryStub) CredentialVersionCounts(context.Context) ([]CredentialVersionCount, error) {
	counts := map[string]int{}
	for _, credential := range r.credentials {
		counts[credential.EncryptionKeyVersion]++
	}
	versions := make([]CredentialVersionCount, 0, len(counts))
	for version, count := range counts {
		versions = append(versions, CredentialVersionCount{KeyVersion: version, Count: count})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].KeyVersion < versions[j].KeyVersion })
	return versions, nil
}

func (r *credentialReencryptionRepositoryStub) StartCredentialReencryption(_ context.Context, result CredentialReencryptionResult) error {
	r.started = append(r.started, result)
	return nil
}

func (r *credentialReencryptionRepositoryStub) InspectCredentialBatch(_ context.Context, targetVersion string, afterID int64, limit int) ([]Credential, error) {
	ids := r.candidateIDs(targetVersion)
	result := make([]Credential, 0, limit)
	for _, id := range ids {
		if id <= afterID {
			continue
		}
		result = append(result, cloneCredential(r.credentials[id]))
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (r *credentialReencryptionRepositoryStub) ReencryptCredentialBatch(
	_ context.Context,
	targetVersion string,
	limit int,
	_ time.Time,
	transform func(Credential) (Credential, error),
) (int, error) {
	ids := r.candidateIDs(targetVersion)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	staged := make(map[int64]Credential, len(ids))
	for _, id := range ids {
		credential, err := transform(cloneCredential(r.credentials[id]))
		if err != nil {
			return 0, err
		}
		staged[id] = cloneCredential(credential)
	}
	for id, credential := range staged {
		r.credentials[id] = credential
		r.writes++
	}
	return len(staged), nil
}

func (r *credentialReencryptionRepositoryStub) CountCredentialsOutsideVersion(_ context.Context, targetVersion string) (int, error) {
	return len(r.candidateIDs(targetVersion)), nil
}

func (r *credentialReencryptionRepositoryStub) FinishCredentialReencryption(_ context.Context, result CredentialReencryptionResult) error {
	r.finished = append(r.finished, result)
	return nil
}

func (r *credentialReencryptionRepositoryStub) candidateIDs(targetVersion string) []int64 {
	ids := make([]int64, 0, len(r.credentials))
	for id, credential := range r.credentials {
		if credential.EncryptionKeyVersion != targetVersion {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func cloneCredential(credential Credential) Credential {
	credential.EncryptedKubeconfig = bytes.Clone(credential.EncryptedKubeconfig)
	return credential
}

func TestCredentialReencryptionDryRunDoesNotWrite(t *testing.T) {
	repository, reencryptor, _, plaintext := newCredentialReencryptionFixture(t, 3)
	before := cloneCredential(repository.credentials[1])

	result, err := reencryptor.Run(context.Background(), CredentialReencryptionOptions{DryRun: true, BatchSize: 2, MaxRecords: 3})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != "succeeded" || result.ExaminedCount != 3 || result.ReencryptedCount != 0 || result.RemainingCount != 3 || result.BatchCount != 2 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	if repository.writes != 0 || !bytes.Equal(repository.credentials[1].EncryptedKubeconfig, before.EncryptedKubeconfig) {
		t.Fatal("dry-run modified a credential")
	}
	if len(repository.finished) != 1 || repository.finished[0].ErrorCode != "" || len(plaintext) == 0 {
		t.Fatalf("unexpected dry-run audit: %#v", repository.finished)
	}
}

func TestCredentialReencryptionApplyMovesCredentialsToActiveVersion(t *testing.T) {
	repository, reencryptor, active, plaintext := newCredentialReencryptionFixture(t, 3)

	result, err := reencryptor.Run(context.Background(), CredentialReencryptionOptions{BatchSize: 2, MaxRecords: 3})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != "succeeded" || result.ExaminedCount != 3 || result.ReencryptedCount != 3 || result.RemainingCount != 0 || result.BatchCount != 2 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	for id, credential := range repository.credentials {
		if credential.EncryptionKeyVersion != "v2" {
			t.Fatalf("credential %d version = %q", id, credential.EncryptionKeyVersion)
		}
		decrypted, err := active.Decrypt(credential.EncryptedKubeconfig, "v2")
		if err != nil || !bytes.Equal(decrypted, plaintext) {
			t.Fatalf("credential %d plaintext mismatch or decrypt error: %v", id, err)
		}
		clear(decrypted)
	}
}

func TestCredentialReencryptionUnknownVersionIsSanitized(t *testing.T) {
	repository, reencryptor, _, _ := newCredentialReencryptionFixture(t, 1)
	repository.credentials[1] = Credential{ClusterID: 1, EncryptedKubeconfig: []byte("opaque"), EncryptionKeyVersion: "missing"}

	result, err := reencryptor.Run(context.Background(), CredentialReencryptionOptions{BatchSize: 1, MaxRecords: 1})
	if !errors.Is(err, ErrUnknownEncryptionKeyVersion) {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != "failed" || result.ErrorCode != "UNKNOWN_KEY_VERSION" || result.RemainingCount != 1 || repository.writes != 0 {
		t.Fatalf("unexpected failed result: %#v", result)
	}
}

func TestCredentialReencryptionBatchFailureRollsBackEveryCredential(t *testing.T) {
	repository, reencryptor, _, _ := newCredentialReencryptionFixture(t, 2)
	firstBefore := cloneCredential(repository.credentials[1])
	repository.credentials[2] = Credential{ClusterID: 2, EncryptedKubeconfig: []byte("corrupt"), EncryptionKeyVersion: "v1"}

	result, err := reencryptor.Run(context.Background(), CredentialReencryptionOptions{BatchSize: 2, MaxRecords: 2})
	if err == nil {
		t.Fatal("Run() error = nil, want corrupt credential failure")
	}
	if result.Status != "failed" || result.ErrorCode != "REENCRYPTION_FAILED" || result.RemainingCount != 2 {
		t.Fatalf("unexpected failed result: %#v", result)
	}
	if repository.writes != 0 || repository.credentials[1].EncryptionKeyVersion != "v1" || !bytes.Equal(repository.credentials[1].EncryptedKubeconfig, firstBefore.EncryptedKubeconfig) {
		t.Fatal("failed batch committed a partial credential update")
	}
}

func TestCredentialReencryptionRejectsCandidateCountAboveReviewedMaximum(t *testing.T) {
	repository, reencryptor, _, _ := newCredentialReencryptionFixture(t, 3)

	result, err := reencryptor.Run(context.Background(), CredentialReencryptionOptions{BatchSize: 1, MaxRecords: 2})
	if !errors.Is(err, ErrCredentialReencryptionTooLarge) {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ErrorCode != "RECORD_LIMIT_EXCEEDED" || result.ExaminedCount != 0 || repository.writes != 0 {
		t.Fatalf("unexpected limit result: %#v", result)
	}
}

func TestCredentialReencryptionDryRunKeepsConcurrentCandidatesBounded(t *testing.T) {
	repository, reencryptor, _, _ := newCredentialReencryptionFixture(t, 2)
	repository.started = nil
	originalStart := reencryptor.repository
	reencryptor.repository = &credentialReencryptionConcurrentStub{
		credentialReencryptionRepositoryStub: repository,
		onStart: func() {
			concurrent := cloneCredential(repository.credentials[1])
			concurrent.ClusterID = 3
			repository.credentials[3] = concurrent
		},
	}
	defer func() { reencryptor.repository = originalStart }()

	result, err := reencryptor.Run(context.Background(), CredentialReencryptionOptions{DryRun: true, BatchSize: 2, MaxRecords: 2})
	if !errors.Is(err, ErrCredentialReencryptionTooLarge) {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExaminedCount != 2 || result.ErrorCode != "RECORD_LIMIT_EXCEEDED" || repository.writes != 0 {
		t.Fatalf("unexpected concurrent limit result: %#v", result)
	}
}

type credentialReencryptionConcurrentStub struct {
	*credentialReencryptionRepositoryStub
	onStart func()
}

func (r *credentialReencryptionConcurrentStub) StartCredentialReencryption(ctx context.Context, result CredentialReencryptionResult) error {
	if err := r.credentialReencryptionRepositoryStub.StartCredentialReencryption(ctx, result); err != nil {
		return err
	}
	r.onStart()
	return nil
}

func newCredentialReencryptionFixture(t *testing.T, count int) (*credentialReencryptionRepositoryStub, *CredentialReencryptor, *Encryptor, []byte) {
	t.Helper()
	legacyKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, 32))
	activeKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	legacy, err := NewEncryptor(legacyKey, "v1")
	if err != nil {
		t.Fatal(err)
	}
	active, err := NewEncryptor(activeKey, "v2", map[string]string{"v1": legacyKey})
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(testKubeconfig("https://rotation.invalid:6443", "rotation-token", true))
	repository := &credentialReencryptionRepositoryStub{credentials: map[int64]Credential{}}
	for index := 1; index <= count; index++ {
		ciphertext, version, err := legacy.Encrypt(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		repository.credentials[int64(index)] = Credential{ClusterID: int64(index), EncryptedKubeconfig: ciphertext, EncryptionKeyVersion: version}
	}
	reencryptor := NewCredentialReencryptor(repository, active)
	reencryptor.newID = func() (string, error) { return "00000000-0000-4000-8000-000000000001", nil }
	reencryptor.now = func() time.Time { return time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC) }
	return repository, reencryptor, active, plaintext
}
