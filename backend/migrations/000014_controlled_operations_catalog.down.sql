DROP INDEX IF EXISTS remediation_plans_resource_idx;

ALTER TABLE remediation_plans
    DROP CONSTRAINT IF EXISTS remediation_plans_parameters_check,
    DROP CONSTRAINT IF EXISTS remediation_plans_action_check;

DELETE FROM remediation_plans WHERE diagnosis_id IS NULL;

ALTER TABLE remediation_plans
    DROP COLUMN IF EXISTS before_replicas,
    DROP COLUMN IF EXISTS desired_replicas,
    DROP COLUMN IF EXISTS before_suspended,
    DROP COLUMN IF EXISTS desired_suspended,
    ALTER COLUMN diagnosis_id SET NOT NULL,
    ALTER COLUMN restart_at SET NOT NULL,
    ADD CONSTRAINT remediation_plans_action_check
        CHECK (action IN ('deployment.rollout_restart'));
