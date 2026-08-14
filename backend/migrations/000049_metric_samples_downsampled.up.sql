-- M114-3 metric history downsampled archive: 30-day coarse-grained storage.
-- When precise metric_samples rows expire (>7d), their data is aggregated
-- into hourly buckets and inserted here before deletion. Queries beyond the
-- precise window route to this table.  The archive contains only one summary
-- row per (cluster_id, resource_kind, resource_namespace, resource_name,
-- container_name, metric_name, bucket_hour), so total rows are bounded by
-- 30d * 24h * series_count, which is small.
--
-- NOTE: The original metric_samples CHECK constrains resource_kind to
-- Node/Pod and metric_name to cpu/memory, but the codebase also writes
-- Deployment/readiness samples (M99-B). This archive table is intentionally
-- more permissive to avoid carrying that pre-existing inconsistency forward.

CREATE TABLE IF NOT EXISTS metric_samples_downsampled (
    id               BIGSERIAL PRIMARY KEY,
    cluster_id       BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    resource_kind    VARCHAR(8) NOT NULL,
    resource_namespace VARCHAR(63) NOT NULL DEFAULT '',
    resource_name    VARCHAR(253) NOT NULL,
    resource_uid     VARCHAR(128) NOT NULL DEFAULT '',
    container_name   VARCHAR(253) NOT NULL DEFAULT '',
    metric_name      VARCHAR(16) NOT NULL,
    unit             VARCHAR(16) NOT NULL,
    bucket_hour      TIMESTAMPTZ NOT NULL,
    value_avg        BIGINT NOT NULL,
    value_max        BIGINT NOT NULL,
    sample_count     INTEGER NOT NULL,
    window_milliseconds INTEGER NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT metric_samples_downsampled_value_check CHECK (value_avg >= 0 AND value_max >= 0),
    CONSTRAINT metric_samples_downsampled_sample_check CHECK (sample_count >= 1),
    CONSTRAINT metric_samples_downsampled_bucket_window_check CHECK (window_milliseconds = 3600000)
);

CREATE UNIQUE INDEX IF NOT EXISTS metric_samples_downsampled_bucket_hour_idx
    ON metric_samples_downsampled (
        cluster_id, resource_kind, resource_namespace, resource_name,
        container_name, metric_name, bucket_hour
    );

CREATE INDEX IF NOT EXISTS metric_samples_downsampled_range_idx
    ON metric_samples_downsampled (
        cluster_id, resource_kind, resource_namespace, resource_name,
        container_name, metric_name, bucket_hour DESC
    );
