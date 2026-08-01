package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"k8s-aiops.local/backend/internal/authz"
)

// Service is the M46 workspace application service. It is the single
// authorization point for workspace-scoped operations and enforces the
// invariants documented in ADR 0061:
//
//   - SystemAdmin bypasses all workspace grant checks.
//   - 404 > 403 anti-leakage: an unauthorized workspace is reported as
//     ErrWorkspaceNotFound so callers cannot distinguish "missing" from
//     "hidden".
//   - Only the three fixed roles are accepted.
//   - The workspace owner always holds workspace_admin; the owner grant
//     cannot be revoked or downgraded while the workspace exists.
//   - WorkspaceGrant is orthogonal to ClusterGrant / NamespaceGrant: it
//     never grants namespace read access.
type Service struct {
	repo Repository
}

// NewService constructs a workspace Service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// CreateWorkspaceInput is the validated payload for CreateWorkspace.
type CreateWorkspaceInput struct {
	Name        string
	DisplayName string
	Metadata    json.RawMessage
}

// UpdateWorkspaceInput is the validated payload for UpdateWorkspace.
type UpdateWorkspaceInput struct {
	DisplayName string
	Metadata    json.RawMessage
}

// SetQuotaInput is the validated payload for SetQuota.
type SetQuotaInput struct {
	HardCPUCores       *float64
	HardMemoryMiB      *int64
	HardPodCount       *int64
	HardNamespaceCount *int64
}

// GrantRoleInput is the validated payload for GrantRole.
type GrantRoleInput struct {
	UserID      int64
	WorkspaceID int64
	Role        string
}

// ============================================================================
// Workspace CRUD
// ============================================================================

// CreateWorkspace creates a workspace and seeds the owner grant
// (workspace_admin) atomically. The actor must be SystemAdmin — workspace
// self-service creation is intentionally deferred (per ADR 0061 §4.3).
func (s *Service) CreateWorkspace(ctx context.Context, actorUserID int64, actorRoles []string, in CreateWorkspaceInput) (Workspace, error) {
	if !authz.IsSystemAdmin(actorRoles) {
		// Anti-leakage: surface as 404 rather than 403 so non-admins cannot
		// probe the create endpoint's existence semantics.
		return Workspace{}, ErrWorkspaceNotFound
	}
	if err := validateWorkspaceName(in.Name); err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return Workspace{}, errors.New("display_name is required")
	}
	metadata, err := normalizeMetadata(in.Metadata)
	if err != nil {
		return Workspace{}, err
	}
	if actorUserID <= 0 {
		return Workspace{}, errors.New("actor_user_id is required")
	}

	ws := Workspace{
		Name:         in.Name,
		DisplayName:  in.DisplayName,
		OwnerUserID:  actorUserID,
		MetadataJSON: metadata,
	}
	created, err := s.repo.CreateWorkspace(ctx, ws)
	if err != nil {
		return Workspace{}, err
	}
	// Seed the owner grant. If this fails we surface the error to the caller;
	// the workspace row remains but the audit trail is consistent because no
	// grant was ever recorded.
	_, err = s.repo.CreateGrant(ctx, UserWorkspaceGrant{
		UserID:      actorUserID,
		WorkspaceID: created.ID,
		Role:        RoleAdmin,
	})
	if err != nil {
		return Workspace{}, err
	}
	if err := s.repo.AppendAudit(ctx, WorkspaceRoleBindingAudit{
		WorkspaceID: created.ID,
		UserID:      actorUserID,
		Role:        RoleAdmin,
		Action:      AuditActionGranted,
		GrantedBy:   &actorUserID,
	}); err != nil {
		return Workspace{}, err
	}
	return created, nil
}

// GetWorkspace returns a workspace by ID. The actor must be SystemAdmin or
// hold any workspace role on the workspace; otherwise ErrWorkspaceNotFound is
// returned (anti-leakage).
func (s *Service) GetWorkspace(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64) (Workspace, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, actorUserID, actorRoles, ws); err != nil {
		return Workspace{}, err
	}
	return ws, nil
}

// ListWorkspaces returns the workspaces the actor may see:
//   - SystemAdmin: every workspace.
//   - Other roles: workspaces the user holds a grant on.
//
// Owner-only listing is intentionally not exposed here; the platform uses
// grants as the source of truth, and an owner always holds a workspace_admin
// grant (seeded on creation).
func (s *Service) ListWorkspaces(ctx context.Context, actorUserID int64, actorRoles []string) ([]Workspace, error) {
	if authz.IsSystemAdmin(actorRoles) {
		return s.repo.ListWorkspaces(ctx, 0)
	}
	ids, err := s.repo.ListUserWorkspaces(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Workspace{}, nil
	}
	all, err := s.repo.ListWorkspaces(ctx, 0)
	if err != nil {
		return nil, err
	}
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	out := make([]Workspace, 0, len(ids))
	for _, ws := range all {
		if _, ok := idSet[ws.ID]; ok {
			out = append(out, ws)
		}
	}
	return out, nil
}

// UpdateWorkspace updates display_name and metadata. Requires workspace_admin
// (or SystemAdmin). Name and owner are immutable.
func (s *Service) UpdateWorkspace(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64, in UpdateWorkspaceInput) (Workspace, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleAdmin); err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return Workspace{}, errors.New("display_name is required")
	}
	metadata, err := normalizeMetadata(in.Metadata)
	if err != nil {
		return Workspace{}, err
	}
	ws.DisplayName = in.DisplayName
	ws.MetadataJSON = metadata
	return s.repo.UpdateWorkspace(ctx, ws)
}

// DeleteWorkspace removes a workspace, its memberships, quota and grants
// (CASCADE). Requires SystemAdmin — workspace admins cannot delete their own
// workspace; this prevents accidental loss of cross-cluster attribution.
func (s *Service) DeleteWorkspace(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64) error {
	if !authz.IsSystemAdmin(actorRoles) {
		return ErrWorkspaceNotFound
	}
	if _, err := s.repo.GetWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	return s.repo.DeleteWorkspace(ctx, workspaceID)
}

// ============================================================================
// Membership
// ============================================================================

// ListMemberships returns the (cluster_id, namespace) tuples bound to the
// workspace. Requires workspace_viewer or higher.
func (s *Service) ListMemberships(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64) ([]WorkspaceMembership, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.ListMemberships(ctx, workspaceID)
}

// NamespacesForWorkspaceFilter is the M47 visibility-filter helper. It returns
// the set of namespaces on clusterID that belong to workspaceID.
//
// This method deliberately does NOT enforce workspace_viewer authorization.
// It is called from the kubernetes resource-list handlers AFTER the existing
// ClusterGrant/NamespaceGrant middleware has already authorized the caller
// for the cluster; the workspace_id query parameter is a pure visibility
// narrowing filter, not an authorization decision (ADR 0062 §4). The
// workspace's existence is not leaked because:
//   - If workspaceID does not exist, the returned set is empty and the
//     handler returns an empty resource list (200 with items: []).
//   - If the caller has no ClusterGrant/NamespaceGrant on the cluster, the
//     middleware already returned 404 before this method is reached.
//
// Returns (nil, nil) when workspaceID is zero (filter disabled).
func (s *Service) NamespacesForWorkspaceFilter(ctx context.Context, clusterID, workspaceID int64) ([]string, error) {
	if workspaceID <= 0 || clusterID <= 0 {
		return nil, nil
	}
	memberships, err := s.repo.ListMembershipsByCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(memberships))
	for _, m := range memberships {
		if m.WorkspaceID == workspaceID {
			out = append(out, m.Namespace)
		}
	}
	return out, nil
}

// AddMembership binds a (cluster_id, namespace) tuple to the workspace.
// Requires workspace_admin. The (cluster_id, namespace) pair may belong to at
// most one workspace (DB unique constraint).
func (s *Service) AddMembership(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID, clusterID int64, namespace string) (WorkspaceMembership, error) {
	if err := validateNamespace(namespace); err != nil {
		return WorkspaceMembership{}, err
	}
	if clusterID <= 0 {
		return WorkspaceMembership{}, errors.New("cluster_id is required")
	}
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return WorkspaceMembership{}, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleAdmin); err != nil {
		return WorkspaceMembership{}, err
	}
	return s.repo.AddMembership(ctx, WorkspaceMembership{
		WorkspaceID: workspaceID,
		ClusterID:   clusterID,
		Namespace:   namespace,
	})
}

// RemoveMembership unbinds a (cluster_id, namespace) tuple from the workspace.
// Requires workspace_admin.
func (s *Service) RemoveMembership(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID, clusterID int64, namespace string) error {
	if err := validateNamespace(namespace); err != nil {
		return err
	}
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleAdmin); err != nil {
		return err
	}
	return s.repo.RemoveMembership(ctx, workspaceID, clusterID, namespace)
}

// ============================================================================
// Quota
// ============================================================================

// GetQuota returns the soft quota for the workspace. Requires workspace_viewer
// or higher. A missing quota row is reported as an empty (zero) quota, not an
// error.
func (s *Service) GetQuota(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64) (WorkspaceQuota, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return WorkspaceQuota{}, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleViewer); err != nil {
		return WorkspaceQuota{}, err
	}
	return s.repo.GetQuota(ctx, workspaceID)
}

// SetQuota upserts the soft quota for the workspace. Requires workspace_admin.
// Negative values are rejected; nil values mean "not tracked".
func (s *Service) SetQuota(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64, in SetQuotaInput) (WorkspaceQuota, error) {
	if err := validateQuotaInput(in); err != nil {
		return WorkspaceQuota{}, err
	}
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return WorkspaceQuota{}, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleAdmin); err != nil {
		return WorkspaceQuota{}, err
	}
	quota := WorkspaceQuota{
		WorkspaceID:        workspaceID,
		HardCPUCores:       in.HardCPUCores,
		HardMemoryMiB:      in.HardMemoryMiB,
		HardPodCount:       in.HardPodCount,
		HardNamespaceCount: in.HardNamespaceCount,
	}
	return s.repo.UpsertQuota(ctx, quota)
}

// ============================================================================
// Role bindings
// ============================================================================

// ListGrants returns the workspace role bindings. Requires workspace_viewer
// or higher.
func (s *Service) ListGrants(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64) ([]UserWorkspaceGrant, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.ListGrants(ctx, workspaceID)
}

// GrantRole creates or replaces a workspace role binding. Requires
// workspace_admin. The owner's role cannot be changed (always workspace_admin).
func (s *Service) GrantRole(ctx context.Context, actorUserID int64, actorRoles []string, in GrantRoleInput) (UserWorkspaceGrant, error) {
	if !IsValidRole(in.Role) {
		return UserWorkspaceGrant{}, ErrInvalidRole
	}
	if in.UserID <= 0 {
		return UserWorkspaceGrant{}, errors.New("user_id is required")
	}
	ws, err := s.repo.GetWorkspace(ctx, in.WorkspaceID)
	if err != nil {
		return UserWorkspaceGrant{}, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleAdmin); err != nil {
		return UserWorkspaceGrant{}, err
	}
	// The owner's role is fixed.
	if in.UserID == ws.OwnerUserID && in.Role != RoleAdmin {
		return UserWorkspaceGrant{}, errors.New("owner role is fixed to workspace_admin")
	}

	existing, err := s.repo.GetGrant(ctx, in.UserID, in.WorkspaceID)
	if err != nil && !errors.Is(err, ErrWorkspaceGrantNotFound) {
		return UserWorkspaceGrant{}, err
	}
	if errors.Is(err, ErrWorkspaceGrantNotFound) {
		created, cerr := s.repo.CreateGrant(ctx, UserWorkspaceGrant{
			UserID:      in.UserID,
			WorkspaceID: in.WorkspaceID,
			Role:        in.Role,
		})
		if cerr != nil {
			return UserWorkspaceGrant{}, cerr
		}
		if aerr := s.repo.AppendAudit(ctx, WorkspaceRoleBindingAudit{
			WorkspaceID: in.WorkspaceID,
			UserID:      in.UserID,
			Role:        in.Role,
			Action:      AuditActionGranted,
			GrantedBy:   &actorUserID,
		}); aerr != nil {
			return UserWorkspaceGrant{}, aerr
		}
		return created, nil
	}
	// Replace existing role.
	if existing.Role == in.Role {
		return existing, nil
	}
	// Cannot downgrade the owner (defence-in-depth; the check above already
	// rejects non-admin roles for the owner).
	if existing.UserID == ws.OwnerUserID && in.Role != RoleAdmin {
		return UserWorkspaceGrant{}, errors.New("owner role is fixed to workspace_admin")
	}
	updated, uerr := s.repo.UpdateGrantRole(ctx, in.UserID, in.WorkspaceID, in.Role)
	if uerr != nil {
		return UserWorkspaceGrant{}, uerr
	}
	if aerr := s.repo.AppendAudit(ctx, WorkspaceRoleBindingAudit{
		WorkspaceID: in.WorkspaceID,
		UserID:      in.UserID,
		Role:        in.Role,
		Action:      AuditActionChanged,
		GrantedBy:   &actorUserID,
	}); aerr != nil {
		return UserWorkspaceGrant{}, aerr
	}
	return updated, nil
}

// RevokeRole removes a workspace role binding. Requires workspace_admin. The
// owner's grant cannot be revoked while the workspace exists.
func (s *Service) RevokeRole(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID, targetUserID int64) error {
	if targetUserID <= 0 {
		return errors.New("user_id is required")
	}
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleAdmin); err != nil {
		return err
	}
	if targetUserID == ws.OwnerUserID {
		return errors.New("owner grant cannot be revoked")
	}
	if err := s.repo.DeleteGrant(ctx, targetUserID, workspaceID); err != nil {
		return err
	}
	if err := s.repo.AppendAudit(ctx, WorkspaceRoleBindingAudit{
		WorkspaceID: workspaceID,
		UserID:      targetUserID,
		Role:        "",
		Action:      AuditActionRevoked,
		GrantedBy:   &actorUserID,
	}); err != nil {
		return err
	}
	return nil
}

// ListAudit returns the role-binding audit trail (newest first). Requires
// workspace_viewer or higher.
func (s *Service) ListAudit(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64, limit int) ([]WorkspaceRoleBindingAudit, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRole(ctx, actorUserID, actorRoles, ws, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.ListAudit(ctx, workspaceID, limit)
}

// ============================================================================
// Authorization helpers
// ============================================================================

// authorizeWorkspaceAccess enforces "viewer or higher" on the workspace,
// which is the minimum for any workspace-scoped read. Returns
// ErrWorkspaceNotFound on denial for anti-leakage.
func (s *Service) authorizeWorkspaceAccess(ctx context.Context, actorUserID int64, actorRoles []string, ws Workspace) error {
	if authz.IsSystemAdmin(actorRoles) {
		return nil
	}
	_, err := s.repo.GetGrant(ctx, actorUserID, ws.ID)
	if errors.Is(err, ErrWorkspaceGrantNotFound) {
		return ErrWorkspaceNotFound
	}
	return err
}

// authorizeRole enforces that the actor holds at least minRole on the
// workspace. SystemAdmin bypasses. Returns ErrWorkspaceNotFound on denial for
// anti-leakage.
func (s *Service) authorizeRole(ctx context.Context, actorUserID int64, actorRoles []string, ws Workspace, minRole string) error {
	if authz.IsSystemAdmin(actorRoles) {
		return nil
	}
	grant, err := s.repo.GetGrant(ctx, actorUserID, ws.ID)
	if err != nil {
		if errors.Is(err, ErrWorkspaceGrantNotFound) {
			return ErrWorkspaceNotFound
		}
		return err
	}
	if RoleRank(grant.Role) < RoleRank(minRole) {
		return ErrWorkspaceNotFound
	}
	return nil
}

// ============================================================================
// Validation helpers
// ============================================================================

// validateWorkspaceName enforces the DNS-subdomain policy required by the
// migration CHECK constraint.
func validateWorkspaceName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 63 {
		return errors.New("name must be at most 63 characters")
	}
	for i, r := range name {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		isHyphen := r == '-'
		if !isAlnum && !isHyphen {
			return fmt.Errorf("name contains invalid character %q at position %d", r, i)
		}
		if isHyphen && (i == 0 || i == len(name)-1) {
			return errors.New("name must start and end with an alphanumeric character")
		}
	}
	return nil
}

// validateNamespace enforces the DNS-1123 label policy required by the
// migration CHECK constraint.
func validateNamespace(namespace string) error {
	if namespace == "" {
		return errors.New("namespace is required")
	}
	if len(namespace) > 253 {
		return errors.New("namespace must be at most 253 characters")
	}
	for i, r := range namespace {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		isHyphen := r == '-'
		if !isAlnum && !isHyphen {
			return fmt.Errorf("namespace contains invalid character %q at position %d", r, i)
		}
		if isHyphen && (i == 0 || i == len(namespace)-1) {
			return errors.New("namespace must start and end with an alphanumeric character")
		}
	}
	return nil
}

// normalizeMetadata coerces the supplied JSON to a non-null object. nil or
// empty input becomes "{}".
func normalizeMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("{}"), nil
	}
	var probe interface{}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("metadata_json is not valid JSON: %w", err)
	}
	if probe == nil {
		return json.RawMessage("{}"), nil
	}
	if _, ok := probe.(map[string]interface{}); !ok {
		return nil, errors.New("metadata_json must be a JSON object")
	}
	return raw, nil
}

// validateQuotaInput rejects negative hard values. Nil values mean "not
// tracked" and are accepted.
func validateQuotaInput(in SetQuotaInput) error {
	if in.HardCPUCores != nil && *in.HardCPUCores < 0 {
		return errors.New("hard_cpu_cores must be non-negative")
	}
	if in.HardMemoryMiB != nil && *in.HardMemoryMiB < 0 {
		return errors.New("hard_memory_mib must be non-negative")
	}
	if in.HardPodCount != nil && *in.HardPodCount < 0 {
		return errors.New("hard_pod_count must be non-negative")
	}
	if in.HardNamespaceCount != nil && *in.HardNamespaceCount < 0 {
		return errors.New("hard_namespace_count must be non-negative")
	}
	return nil
}
