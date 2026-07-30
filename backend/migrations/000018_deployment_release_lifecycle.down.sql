ALTER TABLE remediation_plans
    DROP CONSTRAINT IF EXISTS remediation_plans_parameters_check;

ALTER TABLE remediation_plans
    DROP CONSTRAINT IF EXISTS remediation_plans_action_check;

ALTER TABLE remediation_plans
    ADD CONSTRAINT remediation_plans_action_check
        CHECK (action IN (
            'deployment.rollout_restart',
            'deployment.scale',
            'cronjob.suspend',
            'cronjob.resume'
        ));

ALTER TABLE remediation_plans
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

ALTER TABLE remediation_plans
    DROP COLUMN IF EXISTS container_name,
    DROP COLUMN IF EXISTS before_image,
    DROP COLUMN IF EXISTS desired_image,
    DROP COLUMN IF EXISTS rollback_revision,
    DROP COLUMN IF EXISTS rollback_replicaset_name,
    DROP COLUMN IF EXISTS rollback_replicaset_uid,
    DROP COLUMN IF EXISTS rollback_replicaset_resource_version;
