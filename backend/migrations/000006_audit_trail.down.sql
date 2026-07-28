DROP INDEX IF EXISTS audit_logs_actor_created_idx;
DROP INDEX IF EXISTS audit_logs_result_created_idx;
DROP INDEX IF EXISTS audit_logs_action_created_idx;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_agent;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS ip_address;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS status_code;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_name;
