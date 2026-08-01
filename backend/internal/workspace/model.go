// Package workspace implements the M46 lightweight KubeSphere-style Workspace
// multi-tenancy layer.
//
// A Workspace is an aggregation dimension that groups cluster namespaces across
// the fleet for UI grouping, quota display and cross-cluster namespace
// attribution. It carries its own three-role model (workspace_admin /
// workspace_editor / workspace_viewer) that is independent of the four platform
// roles. The WorkspaceRole does NOT grant namespace read access — namespace
// reads still require the existing ClusterGrant or NamespaceGrant. WorkspaceRole
// only authorizes workspace metadata / membership / quota / role-binding edits.
//
// Design invariants (per ADR 0061):
//   - The 2D authorization matrix (four platform roles x ClusterGrant /
//     NamespaceGrant) is unchanged. WorkspaceGrant is a third, orthogonal grant
//     type that does not affect namespace read access.
//   - SystemAdmin bypasses all grants (including WorkspaceGrant).
//   - 404 > 403 anti-leakage: an unauthorized workspace is indistinguishable
//     from a missing one.
//   - Only the three fixed roles are permitted. Custom workspace roles are
//     deferred.
//   - Workspace quota is display-only; it is NOT enforced against actual
//     cluster ResourceQuota.
package workspace

import (
	"encoding/json"
	"errors"
	"time"
)

// Fixed workspace roles. Custom workspace roles are explicitly deferred; only
// these three values are persisted by user_workspace_grants.role.
const (
	// RoleAdmin can edit workspace metadata, add/remove memberships, set quota
	// and manage workspace role bindings.
	RoleAdmin = "workspace_admin"
	// RoleEditor can view workspace resources and trigger controlled write
	// operations (actual execution still requires the platform operations_admin
	// role).
	RoleEditor = "workspace_editor"
	// RoleViewer can read workspace resources.
	RoleViewer = "workspace_viewer"
)

// AllowedRoles is the fixed catalog of workspace roles. It is the single source
// of truth for validation in the service layer.
var AllowedRoles = []string{RoleAdmin, RoleEditor, RoleViewer}

// IsValidRole reports whether role is one of the three fixed workspace roles.
func IsValidRole(role string) bool {
	for _, allowed := range AllowedRoles {
		if role == allowed {
			return true
		}
	}
	return false
}

// AuditAction is the type of role-binding audit event.
type AuditAction string

const (
	AuditActionGranted AuditAction = "granted"
	AuditActionRevoked AuditAction = "revoked"
	AuditActionChanged AuditAction = "changed"
)

// IsValidAuditAction reports whether action is a permitted audit action.
func IsValidAuditAction(action AuditAction) bool {
	switch action {
	case AuditActionGranted, AuditActionRevoked, AuditActionChanged:
		return true
	}
	return false
}

// Workspace is the SQL-side mirror of the KubeSphere Workspace CRD. The
// owner_user_id is the user who created the workspace; ownership does not
// imply platform-level privileges — it is the default workspace_admin grant
// target.
type Workspace struct {
	ID           int64           `gorm:"primaryKey" json:"id"`
	Name         string          `gorm:"size:63;uniqueIndex;not null" json:"name"`
	DisplayName  string          `gorm:"size:128;not null" json:"display_name"`
	OwnerUserID  int64           `gorm:"not null" json:"owner_user_id"`
	MetadataJSON json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (Workspace) TableName() string { return "workspaces" }

// WorkspaceMembership binds a workspace to one (cluster_id, namespace) tuple.
// A (cluster_id, namespace) pair may belong to at most one workspace.
type WorkspaceMembership struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	WorkspaceID int64     `gorm:"not null;uniqueIndex:idx_ws_membership_unique,priority:1" json:"workspace_id"`
	ClusterID   int64     `gorm:"not null;uniqueIndex:idx_ws_membership_unique,priority:2" json:"cluster_id"`
	Namespace   string    `gorm:"size:253;not null;uniqueIndex:idx_ws_membership_unique,priority:3" json:"namespace"`
	CreatedAt   time.Time `json:"created_at"`
}

func (WorkspaceMembership) TableName() string { return "workspace_memberships" }

// WorkspaceQuota is the soft quota display for a workspace. Mirrors KubeSphere
// Workspace ResourceQuota but is display-only — the platform does NOT enforce
// it against cluster ResourceQuota. All Hard* fields are nullable so a
// workspace can omit fields it does not track.
type WorkspaceQuota struct {
	WorkspaceID        int64     `gorm:"primaryKey" json:"workspace_id"`
	HardCPUCores       *float64  `gorm:"type:numeric(12,3)" json:"hard_cpu_cores,omitempty"`
	HardMemoryMiB      *int64    `json:"hard_memory_mib,omitempty"`
	HardPodCount       *int64    `json:"hard_pod_count,omitempty"`
	HardNamespaceCount *int64    `json:"hard_namespace_count,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (WorkspaceQuota) TableName() string { return "workspace_quotas" }

// UserWorkspaceGrant binds a user to a workspace with a fixed workspace role.
// One role per (user_id, workspace_id) pair.
type UserWorkspaceGrant struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	UserID      int64     `gorm:"not null;uniqueIndex:idx_user_workspace_unique,priority:1" json:"user_id"`
	WorkspaceID int64     `gorm:"not null;uniqueIndex:idx_user_workspace_unique,priority:2" json:"workspace_id"`
	Role        string    `gorm:"size:32;not null" json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

func (UserWorkspaceGrant) TableName() string { return "user_workspace_grants" }

// WorkspaceRoleBindingAudit is the append-only audit trail of workspace role
// binding changes. Mirrors the platform audit pattern but scoped to workspace
// role management actions.
type WorkspaceRoleBindingAudit struct {
	ID          int64       `gorm:"primaryKey" json:"id"`
	WorkspaceID int64       `gorm:"not null" json:"workspace_id"`
	UserID      int64       `gorm:"not null" json:"user_id"`
	Role        string      `gorm:"size:32;not null" json:"role"`
	Action      AuditAction `gorm:"size:16;not null" json:"action"`
	GrantedBy   *int64      `json:"granted_by,omitempty"`
	GrantedAt   time.Time   `json:"granted_at"`
}

func (WorkspaceRoleBindingAudit) TableName() string { return "workspace_role_bindings_audit" }

// AccessDecision is the result of a workspace policy evaluation.
type AccessDecision struct {
	Allowed bool
	// Reason is a stable machine-readable code for denied decisions, suitable
	// for audit logging. It must not leak hidden workspace names.
	Reason string
}

// Sentinel errors returned by the service and repository. They are mapped to
// HTTP status codes by the handler layer.
var (
	// ErrWorkspaceNotFound is returned when a workspace does not exist or is
	// not authorized (anti-leakage). Callers should map this to 404.
	ErrWorkspaceNotFound = errors.New("workspace not found")
	// ErrWorkspaceAlreadyExists is returned on duplicate workspace name.
	ErrWorkspaceAlreadyExists = errors.New("workspace already exists")
	// ErrMembershipNotFound is returned when a membership does not exist.
	ErrMembershipNotFound = errors.New("workspace membership not found")
	// ErrMembershipAlreadyExists is returned when the (cluster_id, namespace)
	// pair is already bound to a workspace.
	ErrMembershipAlreadyExists = errors.New("workspace membership already exists")
	// ErrWorkspaceGrantNotFound is returned when a workspace role binding does
	// not exist.
	ErrWorkspaceGrantNotFound = errors.New("workspace grant not found")
	// ErrWorkspaceGrantAlreadyExists is returned when a user already holds a
	// role on the workspace.
	ErrWorkspaceGrantAlreadyExists = errors.New("workspace grant already exists")
	// ErrInvalidRole is returned when a role is not one of the three fixed
	// workspace roles.
	ErrInvalidRole = errors.New("invalid workspace role")
	// ErrAccessDenied is returned when the policy evaluator denies access.
	// Callers should map this to 404 (not 403) for anti-leakage.
	ErrAccessDenied = errors.New("workspace access denied")
)

// RoleRank returns the rank of a workspace role for hierarchy comparisons. The
// rank is only used by the service layer to decide whether a caller may grant
// or revoke a role equal to or above their own (a workspace_admin may manage
// all roles; a workspace_editor may not manage roles at all).
func RoleRank(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}
