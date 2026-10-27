-- 回滚 000051：先删除 Deployment 形态的样本（旧约束不接受该形态），再收紧列宽与约束。
DELETE FROM metric_samples WHERE resource_kind = 'Deployment';
DELETE FROM metric_samples_downsampled WHERE resource_kind = 'Deployment';

ALTER TABLE metric_samples DROP CONSTRAINT IF EXISTS metric_samples_resource_kind_check;
ALTER TABLE metric_samples DROP CONSTRAINT IF EXISTS metric_samples_resource_shape_check;
ALTER TABLE metric_samples DROP CONSTRAINT IF EXISTS metric_samples_metric_check;

ALTER TABLE metric_samples ALTER COLUMN resource_kind TYPE VARCHAR(8);
ALTER TABLE metric_samples_downsampled ALTER COLUMN resource_kind TYPE VARCHAR(8);

ALTER TABLE metric_samples
    ADD CONSTRAINT metric_samples_resource_kind_check
        CHECK (resource_kind IN ('Node', 'Pod')),
    ADD CONSTRAINT metric_samples_resource_shape_check CHECK (
        (resource_kind = 'Node' AND resource_namespace = '' AND container_name = '')
        OR (resource_kind = 'Pod' AND resource_namespace <> '' AND container_name <> '')
    ),
    ADD CONSTRAINT metric_samples_metric_check CHECK (
        (metric_name = 'cpu' AND unit = 'nanocores')
        OR (metric_name = 'memory' AND unit = 'bytes')
    );
