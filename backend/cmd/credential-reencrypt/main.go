package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/config"
	"k8s-aiops.local/backend/internal/store"
	"k8s-aiops.local/backend/migrations"
)

const credentialReencryptionLockID int64 = 741029

func main() {
	os.Exit(run())
}

func run() int {
	apply := flag.Bool("apply", false, "persist credential re-encryption with --apply; omitted means dry-run")
	batchSize := flag.Int("batch-size", 50, "credentials per transaction (1..100)")
	maxRecords := flag.Int("max-records", 1000, "maximum reviewed credentials for one run (1..10000)")
	timeout := flag.Duration("timeout", 15*time.Minute, "complete command timeout")
	flag.Parse()
	if flag.NArg() != 0 || *timeout <= 0 ||
		*batchSize < cluster.MinCredentialReencryptionBatchSize || *batchSize > cluster.MaxCredentialReencryptionBatchSize ||
		*maxRecords < cluster.MinCredentialReencryptionRecords || *maxRecords > cluster.MaxCredentialReencryptionRecords {
		fmt.Fprintln(os.Stderr, "invalid credential re-encryption arguments")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential re-encryption configuration is invalid")
		return 1
	}
	encryptor, err := cluster.NewEncryptor(cfg.CredentialEncryptionKey, cfg.CredentialKeyVersion, cfg.CredentialDecryptionKeys)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential re-encryption keyring is invalid")
		return 1
	}
	database, err := store.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential re-encryption database connection failed")
		return 1
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := migrations.Apply(ctx, database.SQL()); err != nil {
		fmt.Fprintln(os.Stderr, "credential re-encryption migration check failed")
		return 1
	}
	connection, err := database.SQL().Conn(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential re-encryption lock connection failed")
		return 1
	}
	defer func() { _ = connection.Close() }()
	var locked bool
	if err := connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, credentialReencryptionLockID).Scan(&locked); err != nil || !locked {
		fmt.Fprintln(os.Stderr, "another credential re-encryption command is active")
		return 1
	}
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unlockCancel()
		_, _ = connection.ExecContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, credentialReencryptionLockID)
	}()

	reencryptor := cluster.NewCredentialReencryptor(cluster.NewGormRepository(database.GORM()), encryptor)
	result, runErr := reencryptor.Run(ctx, cluster.CredentialReencryptionOptions{
		DryRun: !*apply, BatchSize: *batchSize, MaxRecords: *maxRecords,
	})
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "credential re-encryption result encoding failed")
		return 1
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "credential re-encryption failed (%s)\n", result.ErrorCode)
		return 1
	}
	return 0
}
