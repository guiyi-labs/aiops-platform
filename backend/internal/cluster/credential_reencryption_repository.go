package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

func (r *GormRepository) CredentialVersionCounts(ctx context.Context) ([]CredentialVersionCount, error) {
	var versions []CredentialVersionCount
	err := r.db.WithContext(ctx).Raw(`
		SELECT encryption_key_version AS key_version, COUNT(*)::INTEGER AS count
		FROM cluster_credentials
		GROUP BY encryption_key_version
		ORDER BY encryption_key_version`).Scan(&versions).Error
	return versions, err
}

func (r *GormRepository) StartCredentialReencryption(ctx context.Context, result CredentialReencryptionResult) error {
	versions, err := json.Marshal(result.SourceKeyVersions)
	if err != nil {
		return fmt.Errorf("encode source key versions: %w", err)
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO credential_reencryption_runs (
			id, target_key_version, source_key_versions, dry_run, status,
			examined_count, reencrypted_count, remaining_count, batch_count,
			error_code, started_at
		) VALUES (?, ?, ?::jsonb, ?, 'running', 0, 0, ?, 0, '', ?)`,
		result.RunID, result.TargetKeyVersion, string(versions), result.DryRun,
		result.RemainingCount, result.StartedAt,
	).Error
}

func (r *GormRepository) InspectCredentialBatch(ctx context.Context, targetVersion string, afterID int64, limit int) ([]Credential, error) {
	var credentials []Credential
	err := r.db.WithContext(ctx).Raw(`
		SELECT cluster_id, encrypted_kubeconfig, encryption_key_version, created_at, updated_at
		FROM cluster_credentials
		WHERE encryption_key_version <> ? AND cluster_id > ?
		ORDER BY cluster_id
		LIMIT ?`, targetVersion, afterID, limit).Scan(&credentials).Error
	return credentials, err
}

func (r *GormRepository) ReencryptCredentialBatch(
	ctx context.Context,
	targetVersion string,
	limit int,
	updatedAt time.Time,
	transform func(Credential) (Credential, error),
) (int, error) {
	updated := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var credentials []Credential
		if err := tx.Raw(`
			SELECT cluster_id, encrypted_kubeconfig, encryption_key_version, created_at, updated_at
			FROM cluster_credentials
			WHERE encryption_key_version <> ?
			ORDER BY cluster_id
			FOR UPDATE SKIP LOCKED
			LIMIT ?`, targetVersion, limit).Scan(&credentials).Error; err != nil {
			return err
		}
		for _, credential := range credentials {
			reencrypted, err := transform(credential)
			if err != nil {
				return err
			}
			result := tx.Model(&Credential{}).
				Where("cluster_id = ? AND encryption_key_version = ?", credential.ClusterID, credential.EncryptionKeyVersion).
				Updates(map[string]any{
					"encrypted_kubeconfig":   reencrypted.EncryptedKubeconfig,
					"encryption_key_version": reencrypted.EncryptionKeyVersion,
					"updated_at":             updatedAt,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("credential %d changed during re-encryption", credential.ClusterID)
			}
			updated++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}

func (r *GormRepository) CountCredentialsOutsideVersion(ctx context.Context, targetVersion string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Credential{}).Where("encryption_key_version <> ?", targetVersion).Count(&count).Error
	return int(count), err
}

func (r *GormRepository) FinishCredentialReencryption(ctx context.Context, result CredentialReencryptionResult) error {
	update := r.db.WithContext(ctx).Exec(`
		UPDATE credential_reencryption_runs
		SET status = ?, examined_count = ?, reencrypted_count = ?, remaining_count = ?,
			batch_count = ?, error_code = ?, completed_at = ?
		WHERE id = ? AND status = 'running'`,
		result.Status, result.ExaminedCount, result.ReencryptedCount, result.RemainingCount,
		result.BatchCount, result.ErrorCode, result.CompletedAt, result.RunID,
	)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return fmt.Errorf("credential re-encryption run is not active")
	}
	return nil
}
