package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/buildinfo"
	"k8s-aiops.local/backend/internal/config"
	"k8s-aiops.local/backend/internal/store"
	"k8s-aiops.local/backend/migrations"
)

func main() {
	os.Exit(run())
}

func run() int {
	verify := flag.Bool("verify", false, "verify an existing archive instead of creating one")
	printPublicKey := flag.Bool("print-public-key", false, "print the public key derived from a private key file")
	archivePath := flag.String("archive", "", "explicit archive payload path")
	manifestPath := flag.String("manifest", "", "archive manifest path; defaults to <archive>.manifest.json")
	privateKeyPath := flag.String("private-key-file", "", "base64 Ed25519 seed/private key file for archive creation")
	trustedPublicKeyPath := flag.String("trusted-public-key-file", "", "base64 Ed25519 public key file required for verification")
	firstID := flag.Int64("from-id", 0, "first inclusive audit ID for archive creation")
	lastID := flag.Int64("to-id", 0, "last inclusive audit ID for archive creation")
	maxRecords := flag.Int("max-records", audit.MaxArchiveRecords, "maximum records in one archive (1..10000)")
	timeout := flag.Duration("timeout", 5*time.Minute, "complete command timeout")
	flag.Parse()

	if flag.NArg() != 0 || *timeout <= 0 || *maxRecords < 1 || *maxRecords > audit.MaxArchiveRecords {
		fmt.Fprintln(os.Stderr, "invalid audit archive arguments")
		return 2
	}
	if *printPublicKey {
		if *verify || *archivePath != "" || *manifestPath != "" || *trustedPublicKeyPath != "" || *privateKeyPath == "" || *firstID != 0 || *lastID != 0 {
			fmt.Fprintln(os.Stderr, "invalid audit archive public key arguments")
			return 2
		}
		privateKey, err := audit.LoadPrivateKey(*privateKeyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "audit archive signing key is invalid")
			return 1
		}
		if err := writeJSON(map[string]string{"public_key": base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))}); err != nil {
			return 1
		}
		return 0
	}
	if *archivePath == "" {
		fmt.Fprintln(os.Stderr, "invalid audit archive arguments")
		return 2
	}
	if *manifestPath == "" {
		*manifestPath = audit.ManifestPath(*archivePath)
	}
	if *verify {
		if *privateKeyPath != "" || *firstID != 0 || *lastID != 0 || *trustedPublicKeyPath == "" {
			fmt.Fprintln(os.Stderr, "invalid audit archive verification arguments")
			return 2
		}
		trustedPublicKey, err := audit.LoadPublicKey(*trustedPublicKeyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "audit archive trusted public key is invalid")
			return 1
		}
		result, err := audit.VerifyArchiveFiles(*archivePath, *manifestPath, trustedPublicKey)
		if err != nil {
			fmt.Fprintln(os.Stderr, "audit archive verification failed")
			return 1
		}
		if err := writeJSON(result); err != nil {
			return 1
		}
		return 0
	}
	if *privateKeyPath == "" || *trustedPublicKeyPath != "" || *firstID < 1 || *lastID < *firstID || *manifestPath != audit.ManifestPath(*archivePath) {
		fmt.Fprintln(os.Stderr, "invalid audit archive creation arguments")
		return 2
	}
	privateKey, err := audit.LoadPrivateKey(*privateKeyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit archive signing key is invalid")
		return 1
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit archive configuration is invalid")
		return 1
	}
	database, err := store.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit archive database connection failed")
		return 1
	}
	defer func() { _ = database.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := migrations.Apply(ctx, database.SQL()); err != nil {
		fmt.Fprintln(os.Stderr, "audit archive migration check failed")
		return 1
	}
	records, err := audit.NewGormRepository(database.GORM()).ArchiveRange(ctx, *firstID, *lastID, *maxRecords)
	if err != nil {
		if errors.Is(err, audit.ErrArchiveRecordLimitExceeded) {
			fmt.Fprintln(os.Stderr, "audit archive record limit exceeded")
		} else {
			fmt.Fprintln(os.Stderr, "audit archive record selection failed")
		}
		return 1
	}
	result, err := audit.WriteSignedArchive(*archivePath, records, privateKey, time.Now(), buildinfo.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit archive write failed")
		return 1
	}
	if err := writeJSON(result); err != nil {
		return 1
	}
	return 0
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, "audit archive result encoding failed")
		return err
	}
	return nil
}
