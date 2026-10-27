-- 修正 metric_samples 与采集器之间的口径漂移。
--
-- 背景：采集器自 M99-B 起会写入 Deployment 的 readiness 样本
-- （internal/metricshistory/collector.go: ResourceDeployment = "Deployment"，
-- 指标名为 readiness_ready / readiness_total，单位 count），
-- 但 000017 建表时把 resource_kind 限制为 VARCHAR(8) 且 CHECK 只允许 ('Node','Pod')，
-- 于是每一批插入都因 'Deployment' 超长而整体回滚——metric_samples 长期为空，
-- 连带指标历史、告警规则评估、SLO 评估全部拿不到数据。
--
-- 000049 的注释已记录这处不一致，但当时只让新表（downsampled）更宽松，
-- 未修正原表。本迁移补齐：
--   1) resource_kind 放宽到 VARCHAR(32)（两张表）；
--   2) kind 约束纳入 Deployment；
--   3) 形态约束区分三种资源：Node(namespace='' container='')、
--      Pod(namespace<>'' container<>'')、Deployment(namespace<>'' container='')；
--   4) 指标约束纳入 readiness_* + count。

ALTER TABLE metric_samples DROP CONSTRAINT IF EXISTS metric_samples_resource_kind_check;
ALTER TABLE metric_samples DROP CONSTRAINT IF EXISTS metric_samples_resource_shape_check;
ALTER TABLE metric_samples DROP CONSTRAINT IF EXISTS metric_samples_metric_check;

ALTER TABLE metric_samples ALTER COLUMN resource_kind TYPE VARCHAR(32);
ALTER TABLE metric_samples_downsampled ALTER COLUMN resource_kind TYPE VARCHAR(32);

ALTER TABLE metric_samples
    ADD CONSTRAINT metric_samples_resource_kind_check
        CHECK (resource_kind IN ('Node', 'Pod', 'Deployment')),
    ADD CONSTRAINT metric_samples_resource_shape_check CHECK (
        (resource_kind = 'Node' AND resource_namespace = '' AND container_name = '')
        OR (resource_kind = 'Pod' AND resource_namespace <> '' AND container_name <> '')
        OR (resource_kind = 'Deployment' AND resource_namespace <> '' AND container_name = '')
    ),
    ADD CONSTRAINT metric_samples_metric_check CHECK (
        (metric_name = 'cpu' AND unit = 'nanocores')
        OR (metric_name = 'memory' AND unit = 'bytes')
        OR (metric_name IN ('readiness_ready', 'readiness_total') AND unit = 'count')
    );
