-- Reverse M46: drop workspace multi-tenancy tables in reverse dependency order.

DROP TABLE IF EXISTS workspace_role_bindings_audit;
DROP TABLE IF EXISTS user_workspace_grants;
DROP TABLE IF EXISTS workspace_quotas;
DROP TABLE IF EXISTS workspace_memberships;
DROP TABLE IF EXISTS workspaces;
