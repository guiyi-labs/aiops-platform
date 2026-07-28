package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNameRequired = errors.New("cluster name is required")
	ErrDisabled     = errors.New("cluster is disabled")
)

type Prober interface {
	Probe(context.Context, int64, []byte) (string, error)
	Invalidate(int64)
}

type Service struct {
	repository Repository
	encryptor  *Encryptor
	prober     Prober
	now        func() time.Time
}

func NewService(repository Repository, encryptor *Encryptor, prober Prober) *Service {
	return &Service{repository: repository, encryptor: encryptor, prober: prober, now: time.Now}
}

func (s *Service) List(ctx context.Context) ([]Cluster, error) { return s.repository.List(ctx) }

func (s *Service) Get(ctx context.Context, id int64) (Cluster, error) {
	item, _, err := s.repository.Find(ctx, id)
	return item, err
}

func (s *Service) Access(ctx context.Context, id int64) (Cluster, []byte, error) {
	item, credential, err := s.repository.Find(ctx, id)
	if err != nil {
		return Cluster{}, nil, err
	}
	if !item.Enabled {
		return item, nil, ErrDisabled
	}
	plaintext, err := s.encryptor.Decrypt(credential.EncryptedKubeconfig)
	if err != nil {
		return item, nil, err
	}
	return item, plaintext, nil
}

func (s *Service) Create(ctx context.Context, name string, rawKubeconfig []byte) (Cluster, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Cluster{}, ErrNameRequired
	}
	config, err := ParseKubeconfig(rawKubeconfig)
	if err != nil {
		return Cluster{}, err
	}
	encrypted, version, err := s.encryptor.Encrypt(rawKubeconfig)
	if err != nil {
		return Cluster{}, err
	}
	item := Cluster{Name: name, APIServer: config.Server, Enabled: false, Status: StatusDisabled, Conditions: []Condition{}}
	credential := Credential{EncryptedKubeconfig: encrypted, EncryptionKeyVersion: version}
	if err := s.repository.Create(ctx, &item, credential); err != nil {
		return Cluster{}, fmt.Errorf("create cluster: %w", err)
	}
	return item, nil
}

func (s *Service) UpdateCredential(ctx context.Context, id int64, rawKubeconfig []byte) (Cluster, error) {
	config, err := ParseKubeconfig(rawKubeconfig)
	if err != nil {
		return Cluster{}, err
	}
	encrypted, version, err := s.encryptor.Encrypt(rawKubeconfig)
	if err != nil {
		return Cluster{}, err
	}
	now := s.now().UTC()
	conditions := credentialUpdateConditions(now)
	if err := s.repository.UpdateCredential(ctx, id, config.Server, Credential{EncryptedKubeconfig: encrypted, EncryptionKeyVersion: version}, now, conditions); err != nil {
		return Cluster{}, err
	}
	s.prober.Invalidate(id)
	updated, _, err := s.repository.Find(ctx, id)
	return updated, err
}

func credentialUpdateConditions(now time.Time) []Condition {
	return []Condition{
		{Type: ConditionCredentialValid, Status: "Unknown", Reason: "CredentialsUpdated", Message: "Credentials were replaced; probe is required", LastTransitionTime: now},
		{Type: ConditionReachable, Status: "Unknown", Reason: "CredentialsUpdated", Message: "Credentials were replaced; probe is required", LastTransitionTime: now},
		{Type: ConditionReady, Status: "Unknown", Reason: "CredentialsUpdated", Message: "Credentials were replaced; probe is required", LastTransitionTime: now},
	}
}

func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	if err := s.repository.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	s.prober.Invalidate(id)
	return nil
}

func (s *Service) Probe(ctx context.Context, id int64) (Cluster, error) {
	item, credential, err := s.repository.Find(ctx, id)
	if err != nil {
		return Cluster{}, err
	}
	now := s.now().UTC()
	plaintext, err := s.encryptor.Decrypt(credential.EncryptedKubeconfig)
	if err != nil {
		conditions := probeConditions(now, false, false, "CredentialDecryptFailed", err.Error())
		_ = s.repository.UpdateProbe(ctx, id, StatusUnreachable, "", now, conditions)
		return Cluster{}, err
	}
	version, probeErr := s.prober.Probe(ctx, id, plaintext)
	status := StatusReady
	conditions := probeConditions(now, true, probeErr == nil, "ProbeSucceeded", "Kubernetes API is reachable")
	if probeErr != nil {
		status = StatusUnreachable
		conditions = probeConditions(now, true, false, "ProbeFailed", probeErr.Error())
	}
	if !item.Enabled && probeErr == nil {
		status = StatusDisabled
	}
	if err := s.repository.UpdateProbe(ctx, id, status, version, now, conditions); err != nil {
		return Cluster{}, err
	}
	updated, _, err := s.repository.Find(ctx, id)
	if probeErr != nil {
		return updated, probeErr
	}
	return updated, err
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	s.prober.Invalidate(id)
	return nil
}

func probeConditions(now time.Time, credentialValid, reachable bool, reason, message string) []Condition {
	truth := func(value bool) string {
		if value {
			return "True"
		}
		return "False"
	}
	return []Condition{
		{Type: ConditionCredentialValid, Status: truth(credentialValid), Reason: reason, Message: message, LastTransitionTime: now},
		{Type: ConditionReachable, Status: truth(reachable), Reason: reason, Message: message, LastTransitionTime: now},
		{Type: ConditionReady, Status: truth(credentialValid && reachable), Reason: reason, Message: message, LastTransitionTime: now},
	}
}
