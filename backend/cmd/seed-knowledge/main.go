// Command seed-knowledge populates the P1 RAG case library (knowledge_entries)
// with a curated set of verified, resolved diagnoses so the demo / replay loop
// has real historical cases to retrieve.
//
// Idempotency:
//   - Seed-owned diagnosis rows are prefixed with a provenance marker and cleared
//     before each run, so re-runs never duplicate.
//   - Knowledge entries cascade-delete via the source_diagnosis_id FK, and any
//     leftover entries from this tool's source diagnoses are overwritten by the
//     knowledge repository's natural-key dedup.
//
// Requires a reachable PostgreSQL matching the server's migrations. It runs the
// migration gate first so it never writes against a stale schema.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/gorm"

	"k8s-aiops.local/backend/internal/config"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/knowledge"
	"k8s-aiops.local/backend/internal/store"
	"k8s-aiops.local/backend/migrations"
)

func main() {
	os.Exit(run())
}

func run() int {
	dryRun := flag.Bool("dry-run", false, "print the cases that would be seeded without touching the database")
	timeout := flag.Duration("timeout", 2*time.Minute, "complete command timeout")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "seed-knowledge: unexpected arguments")
		return 2
	}

	entries := buildSeedEntries(time.Now().Add(-30 * 24 * time.Hour))

	if *dryRun {
		fmt.Printf("would seed %d case-library entries:\n", len(entries))
		for _, e := range entries {
			fmt.Printf("  [%s] %s/%s %s — %s\n", e.Severity, e.ResourceKind, e.ResourceName, e.RuleID, e.Summary)
		}
		return 0
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed-knowledge: configuration is invalid")
		return 1
	}
	database, err := store.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed-knowledge: database connection failed")
		return 1
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := migrations.Apply(ctx, database.SQL()); err != nil {
		fmt.Fprintln(os.Stderr, "seed-knowledge: migration gate failed")
		return 1
	}

	if err := seed(ctx, database.GORM(), entries); err != nil {
		fmt.Fprintf(os.Stderr, "seed-knowledge: %v\n", err)
		return 1
	}
	fmt.Printf("seed-knowledge: seeded %d case-library entries into %q\n", len(entries), cfg.DatabaseURL)
	return 0
}

// seed upserts a synthetic cluster plus one resolved diagnosis per entry, then
// distills each into the knowledge case library. Errors are surfaced so the
// caller can exit non-zero.
func seed(ctx context.Context, db *gorm.DB, entries []knowledge.Entry) error {
	clusterID, err := upsertSeedCluster(ctx, db)
	if err != nil {
		return fmt.Errorf("upsert seed cluster: %w", err)
	}
	if err := clearSeedRows(ctx, db); err != nil {
		return fmt.Errorf("clear seed rows: %w", err)
	}

	diagRepo := diagnosis.NewGormRepository(db)
	knowRepo := knowledge.NewGormRepository(db)

	// Stagger observed_at so live diagnoses (resolved after seeding) sort ahead
	// of seed entries in the newest-first knowledge list.
	base := time.Now().Add(-30 * 24 * time.Hour)
	for i, e := range entries {
		observedAt := base.Add(time.Duration(i) * time.Minute)
		record := &diagnosis.Record{
			ClusterID:  clusterID,
			RuleID:     e.RuleID,
			Severity:   e.Severity,
			Status:     "resolved",
			ObservedAt: observedAt,
			Resource: diagnosis.ResourceRef{
				Kind:      e.ResourceKind,
				Namespace: e.ResourceNamespace,
				Name:      e.ResourceName,
			},
			Summary:        seedProvenanceMarker + e.Summary,
			RootCauses:     e.RootCauses,
			Recommendations: e.Recommendations,
		}
		if err := diagRepo.Save(ctx, record); err != nil {
			return fmt.Errorf("save diagnosis %s: %w", e.ResourceName, err)
		}
		entry := e
		entry.SourceDiagnosisID = record.ID
		entry.NotedAt = observedAt
		entry.Summary = e.Summary // strip provenance marker from the distilled case
		if _, err := knowRepo.Insert(ctx, entry); err != nil {
			return fmt.Errorf("insert knowledge entry %s: %w", e.ResourceName, err)
		}
		log.Printf("seeded %s/%s (%s)", e.ResourceKind, e.ResourceName, e.RuleID)
	}
	return nil
}

// upsertSeedCluster returns the cluster id for the synthetic seed cluster,
// creating it by name if absent.
func upsertSeedCluster(ctx context.Context, db *gorm.DB) (int64, error) {
	var id int64
	if err := db.WithContext(ctx).Raw(
		`INSERT INTO clusters (name, api_server, status, kubernetes_version, created_at, updated_at)
		 VALUES (?, 'https://seed.invalid:6443', 'disabled', 'v1.36.1', NOW(), NOW())
		 ON CONFLICT (name) DO UPDATE SET updated_at = NOW()
		 RETURNING id`, seedClusterName,
	).Scan(&id).Error; err != nil {
		return 0, err
	}
	return id, nil
}

// clearSeedRows removes the diagnosis records this tool owns. Knowledge entries
// cascade via the source_diagnosis_id FK, so the case library stays consistent.
func clearSeedRows(ctx context.Context, db *gorm.DB) error {
	// The diagnosis summary carries the provenance marker for seed-owned rows.
	if err := db.WithContext(ctx).Exec(
		`DELETE FROM diagnosis_records WHERE summary LIKE ?`,
		seedProvenanceMarker+"%",
	).Error; err != nil {
		return err
	}
	// Defensive: any knowledge entry whose source diagnosis no longer exists is
	// removed explicitly (guards against a FK-deferred or disabled cascade).
	return db.WithContext(ctx).Exec(
		`DELETE FROM knowledge_entries ke
		 WHERE NOT EXISTS (SELECT 1 FROM diagnosis_records d WHERE d.id = ke.source_diagnosis_id)`,
	).Error
}
