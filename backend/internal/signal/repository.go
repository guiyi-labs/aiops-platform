package signal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Repository persists signal occurrences.
type Repository interface {
	// Upsert inserts a new occurrence or updates the existing row when the
	// (signal_id, fingerprint) pair already exists. Returns the stored
	// occurrence with its ID populated.
	Upsert(ctx context.Context, occ *Occurrence) error
	// List returns occurrences matching the filter, ordered by observed_at
	// DESC then id DESC. Total is the unconditional count for the same
	// filter (ignoring Limit) so callers can report truncation.
	List(ctx context.Context, filter ListFilter) (items []Occurrence, total int64, err error)
	// CountBySignal returns (signal_id, count, last_seen) tuples for the
	// overview's top-signals aggregation, filtered by the same scope.
	CountBySignal(ctx context.Context, clusterID *int64, namespace string, since time.Time, limit int) ([]OverviewSignal, error)
	// DeleteExpired removes rows whose expires_at <= now. Returns the count
	// removed. Bounded by an internal batch size to avoid long transactions.
	DeleteExpired(ctx context.Context, now time.Time, batchSize int) (int64, error)
}

// ComputeFingerprint produces a stable SHA256 over the identity fields that
// make a signal occurrence unique per (signal_id, fingerprint) contract.
// Two deliveries from the same producer for the same observed event must
// yield the same fingerprint so the ON CONFLICT path deduplicates them.
//
// The fingerprint is computed over: signal_id + cluster_id + resource
// (kind/namespace/name/uid) + window_start + window_end. ObservedAt is
// intentionally excluded: a re-delivery of the same event at a later time
// must not create a new row.
func ComputeFingerprint(req IngestRequest) string {
	uid := req.Resource.UID
	if uid == "" {
		// Name-only fallback: still stable but explicitly marked incomplete
		// so correlation can downgrade confidence.
		uid = "name-only:" + req.Resource.Name
	}
	ws := ""
	we := ""
	if req.WindowStart != nil {
		ws = req.WindowStart.UTC().Format(time.RFC3339Nano)
	}
	if req.WindowEnd != nil {
		we = req.WindowEnd.UTC().Format(time.RFC3339Nano)
	}
	payload := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%s",
		req.SignalID,
		req.ClusterID,
		req.Resource.Kind,
		req.Resource.Namespace,
		req.Resource.Name,
		uid,
		ws,
		we,
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// BuildOccurrence validates the ingest request against the catalog and
// produces a ready-to-persist Occurrence. Returns an error when the signal
// is unregistered (fail closed) or when required identity fields are empty.
func BuildOccurrence(req IngestRequest, now time.Time) (Occurrence, error) {
	desc, ok := Lookup(req.SignalID)
	if !ok {
		return Occurrence{}, fmt.Errorf("signal %q is not registered: ingestion rejected", req.SignalID)
	}
	if req.Fingerprint == "" {
		req.Fingerprint = ComputeFingerprint(req)
	}
	if req.Resource.Kind == "" || req.Resource.Name == "" {
		return Occurrence{}, fmt.Errorf("signal %q: resource kind and name are required", req.SignalID)
	}
	if req.ClusterID <= 0 {
		return Occurrence{}, fmt.Errorf("signal %q: cluster_id must be positive", req.SignalID)
	}
	if req.ObservedAt.IsZero() {
		return Occurrence{}, fmt.Errorf("signal %q: observed_at is required", req.SignalID)
	}
	if req.Freshness.IsZero() {
		req.Freshness = req.ObservedAt
	}
	severity := MapSeverity(desc, req.Severity)
	resource := req.Resource
	resource.Incomplete = resource.UID == ""
	ns := resource.Namespace
	if ns == "" {
		ns = req.Namespace
	}
	var expiresAt *time.Time
	if desc.Retention > 0 {
		t := now.Add(desc.Retention)
		expiresAt = &t
	}
	return Occurrence{
		SignalID:       req.SignalID,
		SignalCode:     req.SignalID, // alias for API parity
		SchemaVersion:  SchemaVersionV1,
		Producer:       req.Producer,
		ClusterID:      req.ClusterID,
		Namespace:      ns,
		Resource:       resource,
		Severity:       severity,
		State:          req.State,
		Fingerprint:    req.Fingerprint,
		Coverage:       req.Coverage,
		Freshness:      req.Freshness.UTC(),
		WindowStart:    req.WindowStart,
		WindowEnd:      req.WindowEnd,
		ObservedAt:     req.ObservedAt.UTC(),
		IngestedAt:     now.UTC(),
		ExpiresAt:      expiresAt,
		Attributes:     req.Attributes,
		Evidence:       req.Evidence,
		IngestionRunID: req.IngestionRunID,
	}, nil
}
