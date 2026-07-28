package cluster

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"time"
)

const (
	MinCredentialReencryptionBatchSize = 1
	MaxCredentialReencryptionBatchSize = 100
	MinCredentialReencryptionRecords   = 1
	MaxCredentialReencryptionRecords   = 10000
	credentialReencryptionAuditTimeout = 5 * time.Second
)

var (
	ErrCredentialReencryptionLimit      = errors.New("credential re-encryption options are outside the allowed bounds")
	ErrCredentialReencryptionTooLarge   = errors.New("credential re-encryption candidate count exceeds the reviewed maximum")
	ErrCredentialReencryptionIncomplete = errors.New("credential re-encryption completed with credentials still on legacy versions")
)

type CredentialReencryptionOptions struct {
	DryRun     bool
	BatchSize  int
	MaxRecords int
}

type CredentialVersionCount struct {
	KeyVersion string
	Count      int
}

type CredentialReencryptionResult struct {
	RunID             string    `json:"run_id"`
	TargetKeyVersion  string    `json:"target_key_version"`
	SourceKeyVersions []string  `json:"source_key_versions"`
	DryRun            bool      `json:"dry_run"`
	Status            string    `json:"status"`
	ExaminedCount     int       `json:"examined_count"`
	ReencryptedCount  int       `json:"reencrypted_count"`
	RemainingCount    int       `json:"remaining_count"`
	BatchCount        int       `json:"batch_count"`
	ErrorCode         string    `json:"error_code,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	CompletedAt       time.Time `json:"completed_at"`
}

type CredentialReencryptionRepository interface {
	CredentialVersionCounts(context.Context) ([]CredentialVersionCount, error)
	StartCredentialReencryption(context.Context, CredentialReencryptionResult) error
	InspectCredentialBatch(context.Context, string, int64, int) ([]Credential, error)
	ReencryptCredentialBatch(context.Context, string, int, time.Time, func(Credential) (Credential, error)) (int, error)
	CountCredentialsOutsideVersion(context.Context, string) (int, error)
	FinishCredentialReencryption(context.Context, CredentialReencryptionResult) error
}

type CredentialReencryptor struct {
	repository CredentialReencryptionRepository
	encryptor  *Encryptor
	now        func() time.Time
	newID      func() (string, error)
}

func NewCredentialReencryptor(repository CredentialReencryptionRepository, encryptor *Encryptor) *CredentialReencryptor {
	return &CredentialReencryptor{repository: repository, encryptor: encryptor, now: time.Now, newID: newCredentialReencryptionID}
}

func (r *CredentialReencryptor) Run(ctx context.Context, options CredentialReencryptionOptions) (CredentialReencryptionResult, error) {
	if options.BatchSize < MinCredentialReencryptionBatchSize || options.BatchSize > MaxCredentialReencryptionBatchSize ||
		options.MaxRecords < MinCredentialReencryptionRecords || options.MaxRecords > MaxCredentialReencryptionRecords {
		return CredentialReencryptionResult{}, ErrCredentialReencryptionLimit
	}
	runID, err := r.newID()
	if err != nil {
		return CredentialReencryptionResult{}, fmt.Errorf("generate credential re-encryption run ID: %w", err)
	}
	versions, err := r.repository.CredentialVersionCounts(ctx)
	if err != nil {
		return CredentialReencryptionResult{}, fmt.Errorf("count credential key versions: %w", err)
	}
	targetVersion := r.encryptor.ActiveVersion()
	sourceVersions := make([]string, 0, len(versions))
	pending := 0
	for _, version := range versions {
		if version.KeyVersion == targetVersion || version.Count == 0 {
			continue
		}
		sourceVersions = append(sourceVersions, version.KeyVersion)
		pending += version.Count
	}
	sort.Strings(sourceVersions)
	result := CredentialReencryptionResult{
		RunID: runID, TargetKeyVersion: targetVersion, SourceKeyVersions: sourceVersions,
		DryRun: options.DryRun, Status: "running", RemainingCount: pending, StartedAt: r.now().UTC(),
	}
	if err := r.repository.StartCredentialReencryption(ctx, result); err != nil {
		return CredentialReencryptionResult{}, fmt.Errorf("start credential re-encryption audit: %w", err)
	}

	fail := func(runErr error) (CredentialReencryptionResult, error) {
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), credentialReencryptionAuditTimeout)
		defer cancel()
		if remaining, countErr := r.repository.CountCredentialsOutsideVersion(auditCtx, targetVersion); countErr == nil {
			result.RemainingCount = remaining
		}
		result.Status = "failed"
		result.ErrorCode = credentialReencryptionErrorCode(runErr)
		result.CompletedAt = r.now().UTC()
		if finishErr := r.repository.FinishCredentialReencryption(auditCtx, result); finishErr != nil {
			return result, fmt.Errorf("%w; finish credential re-encryption audit: %v", runErr, finishErr)
		}
		return result, runErr
	}

	if pending > options.MaxRecords {
		return fail(ErrCredentialReencryptionTooLarge)
	}
	if options.DryRun {
		if err := r.inspect(ctx, options, &result); err != nil {
			return fail(err)
		}
	} else {
		if err := r.apply(ctx, options, &result); err != nil {
			return fail(err)
		}
	}

	remaining, err := r.repository.CountCredentialsOutsideVersion(ctx, targetVersion)
	if err != nil {
		return fail(fmt.Errorf("count remaining credentials: %w", err))
	}
	result.RemainingCount = remaining
	if !options.DryRun && remaining != 0 {
		return fail(ErrCredentialReencryptionIncomplete)
	}
	result.Status = "succeeded"
	result.CompletedAt = r.now().UTC()
	if err := r.repository.FinishCredentialReencryption(ctx, result); err != nil {
		return result, fmt.Errorf("finish credential re-encryption audit: %w", err)
	}
	return result, nil
}

func (r *CredentialReencryptor) inspect(ctx context.Context, options CredentialReencryptionOptions, result *CredentialReencryptionResult) error {
	var afterID int64
	for {
		limit := min(options.BatchSize, options.MaxRecords-result.ExaminedCount+1)
		credentials, err := r.repository.InspectCredentialBatch(ctx, r.encryptor.ActiveVersion(), afterID, limit)
		if err != nil {
			return fmt.Errorf("inspect credential batch: %w", err)
		}
		if len(credentials) == 0 {
			return nil
		}
		result.BatchCount++
		for _, credential := range credentials {
			if result.ExaminedCount == options.MaxRecords {
				return ErrCredentialReencryptionTooLarge
			}
			plaintext, err := r.encryptor.Decrypt(credential.EncryptedKubeconfig, credential.EncryptionKeyVersion)
			if err != nil {
				return err
			}
			_, parseErr := ParseKubeconfig(plaintext)
			clear(plaintext)
			if parseErr != nil {
				return ErrInvalidKubeconfig
			}
			result.ExaminedCount++
			afterID = credential.ClusterID
		}
	}
}

func (r *CredentialReencryptor) apply(ctx context.Context, options CredentialReencryptionOptions, result *CredentialReencryptionResult) error {
	for result.ReencryptedCount < options.MaxRecords {
		limit := min(options.BatchSize, options.MaxRecords-result.ReencryptedCount)
		count, err := r.repository.ReencryptCredentialBatch(ctx, r.encryptor.ActiveVersion(), limit, r.now().UTC(), func(credential Credential) (Credential, error) {
			plaintext, err := r.encryptor.Decrypt(credential.EncryptedKubeconfig, credential.EncryptionKeyVersion)
			if err != nil {
				return Credential{}, err
			}
			if _, err := ParseKubeconfig(plaintext); err != nil {
				clear(plaintext)
				return Credential{}, ErrInvalidKubeconfig
			}
			encrypted, version, err := r.encryptor.Encrypt(plaintext)
			clear(plaintext)
			if err != nil {
				return Credential{}, err
			}
			credential.EncryptedKubeconfig = encrypted
			credential.EncryptionKeyVersion = version
			return credential, nil
		})
		if err != nil {
			return fmt.Errorf("re-encrypt credential batch: %w", err)
		}
		if count == 0 {
			return nil
		}
		result.BatchCount++
		result.ExaminedCount += count
		result.ReencryptedCount += count
	}
	return nil
}

func credentialReencryptionErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrUnknownEncryptionKeyVersion):
		return "UNKNOWN_KEY_VERSION"
	case errors.Is(err, ErrInvalidKubeconfig):
		return "INVALID_KUBECONFIG"
	case errors.Is(err, ErrCredentialReencryptionTooLarge):
		return "RECORD_LIMIT_EXCEEDED"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "CANCELED"
	default:
		return "REENCRYPTION_FAILED"
	}
}

func newCredentialReencryptionID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
