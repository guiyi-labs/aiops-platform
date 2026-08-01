// Package federation implements the M48 KubeSphere-style multi-cluster
// federation model (host/member clusters).
//
// The federation layer is a SQL aggregation view over the existing clusters
// table: there is no Cluster Agent / Tower side channel, no inter-cluster
// resource sync controller, and no transparent K8s API proxy. Cross-cluster
// operations still go through the existing explicit cluster_id + fixed GVR
// whitelist pattern (ADR 0004). Federation state is composed of:
//
//   - `clusters.cluster_role` (host/member/standalone) — topology role.
//   - `clusters.federation_status` — federation-level health dimension
//     (registered/healthy/degraded/disconnected), orthogonal to the existing
//     `clusters.status` (enabled/disabled/unreachable).
//   - `cluster_federation_events` — append-only audit trail of federation
//     state transitions and operator actions.
//
// Design invariants (per ADR 0063):
//   - At most one host cluster (enforced by a partial unique index).
//   - 404 > 403 anti-leakage preserved: an unauthorized cluster is
//     indistinguishable from a missing one. Federation reads require
//     SystemAdmin or operations_admin; the role gate is enforced at the
//     route layer.
//   - cluster_role and federation_status are bounded enums; the CHECK
//     constraints are part of the public contract.
//   - Federation events are append-only; no UPDATE/DELETE path.
package federation

import (
	"errors"
	"time"
)

// Event types recorded in cluster_federation_events.event_type. The values
// are part of the public API contract (OpenAPI enum).
const (
	EventRegistered   = "registered"
	EventDeregistered = "deregistered"
	EventHeartbeat    = "heartbeat"
	EventStatusChange = "status_change"
	EventRoleChange   = "role_change"
)

// IsValidEvent reports whether eventType is a permitted federation event.
func IsValidEvent(eventType string) bool {
	switch eventType {
	case EventRegistered, EventDeregistered, EventHeartbeat, EventStatusChange, EventRoleChange:
		return true
	}
	return false
}

// FederationEvent is the append-only audit trail of federation state
// transitions and operator actions. Mirrors the platform audit pattern
// (ADR 0008): no UPDATE/DELETE path is exposed by the repository.
type FederationEvent struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	ClusterID  int64     `gorm:"not null;index" json:"cluster_id"`
	EventType  string    `gorm:"size:32;not null" json:"event_type"`
	Status     string    `gorm:"size:16;not null" json:"status"`
	Message    string    `gorm:"size:1024;not null;default:''" json:"message"`
	OccurredAt time.Time `gorm:"not null" json:"occurred_at"`
}

func (FederationEvent) TableName() string { return "cluster_federation_events" }

// ClusterSummary is the per-cluster entry in the federation overview. It
// composes the existing cluster.Cluster row with the federation fields.
type ClusterSummary struct {
	ClusterID         int64      `json:"cluster_id"`
	ClusterName       string     `json:"cluster_name"`
	APIServer         string     `json:"api_server"`
	Enabled           bool       `json:"enabled"`
	Status            string     `json:"status"`
	KubernetesVersion string     `json:"kubernetes_version,omitempty"`
	LastProbedAt      *time.Time `json:"last_probed_at,omitempty"`
	ClusterRole       string     `json:"cluster_role"`
	FederationStatus  string     `json:"federation_status"`
	RegisteredAt      *time.Time `json:"registered_at,omitempty"`
	LastHeartbeatAt   *time.Time `json:"last_heartbeat_at,omitempty"`
}

// Overview is the response shape for GET /api/v1/federation/overview. It is
// a pure aggregation over the clusters table — no live probing is performed
// by the federation service (the existing cluster probe path remains the
// source of truth for cluster.status).
type Overview struct {
	Host              *ClusterSummary  `json:"host,omitempty"`
	Members           []ClusterSummary `json:"members"`
	Standalone        []ClusterSummary `json:"standalone"`
	TotalClusters     int              `json:"total_clusters"`
	HealthyCount      int              `json:"healthy_count"`
	DegradedCount     int              `json:"degraded_count"`
	DisconnectedCount int              `json:"disconnected_count"`
	RegisteredCount   int              `json:"registered_count"`
	GeneratedAt       time.Time        `json:"generated_at"`
}

// ResourceSummaryEntry is one row of the cross-cluster resource summary. The
// GVR whitelist is fixed and operator-curated (M49 will refine). The counts
// are populated by the service from the existing kubernetes.Service list
// methods; missing/unreachable clusters contribute zero counts.
type ResourceSummaryEntry struct {
	Group      string         `json:"group"`
	Version    string         `json:"version"`
	Resource   string         `json:"resource"`
	Kind       string         `json:"kind"`
	ByCluster  []ClusterCount `json:"by_cluster"`
	TotalCount int            `json:"total_count"`
}

// ClusterCount is the per-cluster contribution to a ResourceSummaryEntry.
type ClusterCount struct {
	ClusterID   int64  `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Count       int    `json:"count"`
	// Error is empty on success; one of TIMEOUT / QUERY_FAILED when the
	// per-cluster list failed. The aggregate row is still returned so the
	// caller can see partial results.
	Error string `json:"error,omitempty"`
}

// ResourceSummary is the response shape for
// GET /api/v1/federation/resources/summary.
type ResourceSummary struct {
	Items       []ResourceSummaryEntry `json:"items"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// Sentinel errors returned by the service and repository. They are mapped to
// HTTP status codes by the handler layer.
var (
	// ErrClusterNotFound is returned when a cluster does not exist. Callers
	// should map this to 404.
	ErrClusterNotFound = errors.New("cluster not found")
	// ErrClusterAlreadyRegistered is returned when registering a cluster that
	// is already a federation member or host.
	ErrClusterAlreadyRegistered = errors.New("cluster already registered with federation")
	// ErrHostAlreadyExists is returned when promoting a cluster to host while
	// another host exists.
	ErrHostAlreadyExists = errors.New("a host cluster already exists")
	// ErrInvalidClusterRole is returned when an invalid cluster_role is
	// supplied.
	ErrInvalidClusterRole = errors.New("invalid cluster_role")
	// ErrInvalidFederationStatus is returned when an invalid federation_status
	// is supplied.
	ErrInvalidFederationStatus = errors.New("invalid federation_status")
	// ErrCannotDeregisterHost is returned when attempting to deregister the
	// host cluster without first demoting it to standalone/member.
	ErrCannotDeregisterHost = errors.New("host cluster cannot be deregistered; demote to standalone first")
)
