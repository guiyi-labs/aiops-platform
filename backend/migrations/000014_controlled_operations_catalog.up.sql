ALTER TABLE remediation_plans
    ALTER COLUMN diagnosis_id DROP NOT NULL,
    ALTER COLUMN restart_at DROP NOT NULL;

ALTER TABLE remediation_plans
    DROP CONSTRAINT IF EXISTS remediation_plans_action_check;

ALTER TABLE remediation_plans
    ADD COLUMN IF NOT EXISTS before_replicas INTEGER,
    ADD COLUMN IF NOT EXISTS desired_replicas INTEGER,
    ADD COLUMN IF NOT EXISTS before_suspended BOOLEAN,
    ADD COLUMN IF NOT EXISTS desired_suspended BOOLEAN;

ALTER TABLE remediation_plans
    ADD CONSTRAINT remediation_plans_action_check
        CHECK (action IN (
            'deployment.rollout_restart',
            'deployment.scale',
            'cronjob.suspend',
            'cronjob.resume'
        )),
    ADD CONSTRAINT remediation_plans_parameters_check
        CHECK (
            (action = 'deployment.rollout_restart'
                AND diagnosis_id IS NOT NULL
                AND restart_at IS NOT NULL
                AND before_replicas IS NULL AND desired_replicas IS NULL
                AND before_suspended IS NULL AND desired_suspended IS NULL)
            OR
            (action = 'deployment.scale'
                AND diagnosis_id IS NULL
                AND restart_at IS NULL
                AND before_replicas BETWEEN 0 AND 1000
                AND desired_replicas BETWEEN 0 AND 1000
                AND before_suspended IS NULL AND desired_suspended IS NULL)
            OR
            (action IN ('cronjob.suspend', 'cronjob.resume')
                AND diagnosis_id IS NULL
                AND restart_at IS NULL
                AND before_replicas IS NULL AND desired_replicas IS NULL
                AND before_suspended IS NOT NULL AND desired_suspended IS NOT NULL)
        );

CREATE INDEX IF NOT EXISTS remediation_plans_resource_idx
    ON remediation_plans (cluster_id, target_kind, target_namespace, target_name, created_at DESC)
    WHERE diagnosis_id IS NULL;
