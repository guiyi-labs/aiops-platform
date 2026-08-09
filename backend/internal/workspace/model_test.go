package workspace

import "testing"

func TestIsValidAuditAction(t *testing.T) {
	valid := []AuditAction{AuditActionGranted, AuditActionRevoked, AuditActionChanged}
	for _, a := range valid {
		if !IsValidAuditAction(a) {
			t.Errorf("IsValidAuditAction(%q) = false", a)
		}
	}
	if IsValidAuditAction("bogus") || IsValidAuditAction("") {
		t.Error("IsValidAuditAction accepted invalid action")
	}
}

func TestWorkspaceTableNames(t *testing.T) {
	cases := []struct {
		model interface{ TableName() string }
		want  string
	}{
		{Workspace{}, "workspaces"},
		{WorkspaceMembership{}, "workspace_memberships"},
		{WorkspaceQuota{}, "workspace_quotas"},
		{UserWorkspaceGrant{}, "user_workspace_grants"},
		{WorkspaceRoleBindingAudit{}, "workspace_role_bindings_audit"},
	}
	for _, tc := range cases {
		if got := tc.model.TableName(); got != tc.want {
			t.Errorf("%T.TableName = %q, want %q", tc.model, got, tc.want)
		}
	}
}
