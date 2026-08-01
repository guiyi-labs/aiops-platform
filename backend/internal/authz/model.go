package authz

import "time"

// ClusterGrant authorizes a user to access all namespaces in a cluster. It is
// the coarse-grained resource-scope dimension; the action dimension remains the
// four global platform roles (system_admin, operations_admin, security_auditor,
// viewer). SystemAdmin bypasses grants entirely.
type ClusterGrant struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"not null;uniqueIndex:idx_user_cluster,priority:1" json:"user_id"`
	ClusterID int64     `gorm:"not null;uniqueIndex:idx_user_cluster,priority:2" json:"cluster_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (ClusterGrant) TableName() string { return "user_cluster_grants" }

// NamespaceGrant authorizes a user to access one exact namespace in a cluster.
// If the user also holds a ClusterGrant for the same cluster, the NamespaceGrant
// is redundant but not harmful. Namespace grants allow finer-grained access
// without exposing the whole cluster.
type NamespaceGrant struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"not null;uniqueIndex:idx_user_cluster_ns,priority:1" json:"user_id"`
	ClusterID int64     `gorm:"not null;uniqueIndex:idx_user_cluster_ns,priority:2" json:"cluster_id"`
	Namespace string    `gorm:"size:253;not null;uniqueIndex:idx_user_cluster_ns,priority:3" json:"namespace"`
	CreatedAt time.Time `json:"created_at"`
}

func (NamespaceGrant) TableName() string { return "user_namespace_grants" }

// ClusterScope summarizes a user's access to one cluster. If AllNamespaces is
// true, the user holds a cluster-level grant and NamespaceGrants is empty. If
// AllNamespaces is false, NamespaceGrants lists the exact namespaces the user
// may access; an empty slice means the user has no access to that cluster.
type ClusterScope struct {
	ClusterID       int64
	AllNamespaces   bool
	NamespaceGrants []string
}

// AccessDecision is the result of a policy evaluation.
type AccessDecision struct {
	Allowed bool
	// Reason is a stable machine-readable code for denied decisions, suitable
	// for audit logging. It must not leak hidden cluster or resource names.
	Reason string
}
