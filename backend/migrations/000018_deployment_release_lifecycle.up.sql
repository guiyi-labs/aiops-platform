ALTER TABLE remediation_plans
    ADD COLUMN IF NOT EXISTS container_name VARCHAR(253),
    ADD COLUMN IF NOT EXISTS before_image VARCHAR(512),
    ADD COLUMN IF NOT EXISTS desired_image VARCHAR(512),
    ADD COLUMN IF NOT EXISTS rollback_revision INTEGER,
    ADD COLUMN IF NOT EXISTS rollback_replicaset_name VARCHAR(253),
    ADD COLUMN IF NOT EXISTS rollback_replicaset_uid VARCHAR(128),
    ADD COLUMN IF NOT EXISTS rollback_replicaset_resource_version VARCHAR(128);

ALTER TABLE remediation_plans
    DROP CONSTRAINT IF EXISTS remediation_plans_action_check;

ALTER TABLE remediation_plans
    ADD CONSTRAINT remediation_plans_action_check
        CHECK (action IN (
            'deployment.rollout_restart',
            'deployment.scale',
            'deployment.image_update',
            'deployment.rollback',
            'cronjob.suspend',
            'cronjob.resume'
        ));

ALTER TABLE remediation_plans
    DROP CONSTRAINT IF EXISTS remediation_plans_parameters_check;

ALTER TABLE remediation_plans
    ADD CONSTRAINT remediation_plans_parameters_check
        CHECK (
            (action = 'deployment.rollout_restart'
                AND diagnosis_id IS NOT NULL
                AND restart_at IS NOT NULL
                AND before_replicas IS NULL AND desired_replicas IS NULL
                AND before_suspended IS NULL AND desired_suspended IS NULL
                AND container_name = '' AND before_image = '' AND desired_image = ''
                AND rollback_revision IS NULL AND rollback_replicaset_uid = '')
            OR
            (action = 'deployment.scale'
                AND diagnosis_id IS NULL
                AND restart_at IS NULL
                AND before_replicas BETWEEN 0 AND 1000
                AND desired_replicas BETWEEN 0 AND 1000
                AND before_suspended IS NULL AND desired_suspended IS NULL
                AND container_name = '' AND before_image = '' AND desired_image = ''
                AND rollback_revision IS NULL AND rollback_replicaset_uid = '')
            OR
            (action IN ('cronjob.suspend', 'cronjob.resume')
                AND diagnosis_id IS NULL
                AND restart_at IS NULL
                AND before_replicas IS NULL AND desired_replicas IS NULL
                AND before_suspended IS NOT NULL AND desired_suspended IS NOT NULL
                AND container_name = '' AND before_image = '' AND desired_image = ''
                AND rollback_revision IS NULL AND rollback_replicaset_uid = '')
            OR
            (action = 'deployment.image_update'
                AND diagnosis_id IS NULL
                AND restart_at IS NULL
                AND before_replicas IS NULL AND desired_replicas IS NULL
                AND before_suspended IS NULL AND desired_suspended IS NULL
                AND container_name IS NOT NULL AND container_name <> ''
                AND before_image IS NOT NULL AND before_image <> ''
                AND desired_image IS NOT NULL AND desired_image <> ''
                AND rollback_revision IS NULL AND rollback_replicaset_uid = '')
            OR
            (action = 'deployment.rollback'
                AND diagnosis_id IS NULL
                AND restart_at IS NULL
                AND before_replicas IS NULL AND desired_replicas IS NULL
                AND before_suspended IS NULL AND desired_suspended IS NULL
                AND container_name = '' AND before_image = '' AND desired_image = ''
                AND rollback_revision BETWEEN 1 AND 2147483647
                AND rollback_replicaset_uid IS NOT NULL AND rollback_replicaset_uid <> '')
        );
