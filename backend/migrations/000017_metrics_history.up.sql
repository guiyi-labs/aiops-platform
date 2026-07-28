CREATE TABLE IF NOT EXISTS metric_collection_runs (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL,
    nodes_status VARCHAR(16) NOT NULL,
    nodes_sampled INTEGER NOT NULL DEFAULT 0,
    nodes_total INTEGER NOT NULL DEFAULT 0,
    nodes_complete BOOLEAN NOT NULL DEFAULT FALSE,
    pods_status VARCHAR(16) NOT NULL,
    pods_sampled INTEGER NOT NULL DEFAULT 0,
    pods_total INTEGER NOT NULL DEFAULT 0,
    pods_complete BOOLEAN NOT NULL DEFAULT FALSE,
    failure_code VARCHAR(64) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT metric_collection_runs_status_check
        CHECK (status IN ('succeeded', 'partial', 'unavailable', 'timed_out', 'failed')),
    CONSTRAINT metric_collection_runs_target_status_check
        CHECK (
            nodes_status IN ('succeeded', 'unavailable', 'timed_out', 'failed')
            AND pods_status IN ('succeeded', 'unavailable', 'timed_out', 'failed')
        ),
    CONSTRAINT metric_collection_runs_nodes_coverage_check
        CHECK (
            (nodes_status = 'succeeded' AND nodes_sampled >= 0 AND nodes_total >= nodes_sampled
                AND nodes_complete = (nodes_sampled = nodes_total))
            OR (nodes_status <> 'succeeded' AND nodes_sampled = 0 AND nodes_total = 0 AND nodes_complete = FALSE)
        ),
    CONSTRAINT metric_collection_runs_pods_coverage_check
        CHECK (
            (pods_status = 'succeeded' AND pods_sampled >= 0 AND pods_total >= pods_sampled
                AND pods_complete = (pods_sampled = pods_total))
            OR (pods_status <> 'succeeded' AND pods_sampled = 0 AND pods_total = 0 AND pods_complete = FALSE)
        ),
    CONSTRAINT metric_collection_runs_time_check
        CHECK (started_at <= completed_at AND expires_at >= completed_at),
    CONSTRAINT metric_collection_runs_result_consistency_check CHECK (
        (status = 'succeeded' AND nodes_status = 'succeeded' AND pods_status = 'succeeded'
            AND nodes_complete AND pods_complete)
        OR (status = 'partial' AND (nodes_status = 'succeeded' OR pods_status = 'succeeded')
            AND NOT (nodes_status = 'succeeded' AND pods_status = 'succeeded'
                AND nodes_complete AND pods_complete))
        OR (status = 'unavailable' AND nodes_status = 'unavailable' AND pods_status = 'unavailable')
        OR (status = 'timed_out' AND nodes_status <> 'succeeded' AND pods_status <> 'succeeded'
            AND NOT (nodes_status = 'unavailable' AND pods_status = 'unavailable')
            AND (nodes_status = 'timed_out' OR pods_status = 'timed_out'))
        OR (status = 'failed' AND nodes_status <> 'succeeded' AND pods_status <> 'succeeded'
            AND nodes_status <> 'timed_out' AND pods_status <> 'timed_out'
            AND NOT (nodes_status = 'unavailable' AND pods_status = 'unavailable'))
    ),
    CONSTRAINT metric_collection_runs_failure_code_check
        CHECK (
            (status = 'succeeded' AND failure_code = '')
            OR (status <> 'succeeded' AND char_length(failure_code) BETWEEN 1 AND 64)
        ),
    CONSTRAINT metric_collection_runs_id_cluster_unique UNIQUE (id, cluster_id)
);

CREATE TABLE IF NOT EXISTS metric_samples (
    id BIGSERIAL PRIMARY KEY,
    collection_run_id BIGINT NOT NULL,
    cluster_id BIGINT NOT NULL,
    resource_kind VARCHAR(8) NOT NULL,
    resource_namespace VARCHAR(63) NOT NULL DEFAULT '',
    resource_name VARCHAR(253) NOT NULL,
    resource_uid VARCHAR(128) NOT NULL DEFAULT '',
    container_name VARCHAR(253) NOT NULL DEFAULT '',
    metric_name VARCHAR(16) NOT NULL,
    value BIGINT NOT NULL,
    unit VARCHAR(16) NOT NULL,
    source_timestamp TIMESTAMPTZ NOT NULL,
    window_milliseconds INTEGER NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT metric_samples_resource_kind_check CHECK (resource_kind IN ('Node', 'Pod')),
    CONSTRAINT metric_samples_resource_shape_check CHECK (
        (resource_kind = 'Node' AND resource_namespace = '' AND container_name = '')
        OR (resource_kind = 'Pod' AND resource_namespace <> '' AND container_name <> '')
    ),
    CONSTRAINT metric_samples_metric_check CHECK (
        (metric_name = 'cpu' AND unit = 'nanocores')
        OR (metric_name = 'memory' AND unit = 'bytes')
    ),
    CONSTRAINT metric_samples_value_check CHECK (value >= 0),
    CONSTRAINT metric_samples_window_check CHECK (window_milliseconds BETWEEN 1000 AND 3600000),
    CONSTRAINT metric_samples_name_check CHECK (
        char_length(resource_name) BETWEEN 1 AND 253
        AND resource_name = btrim(resource_name)
        AND char_length(resource_namespace) <= 63
        AND resource_namespace = btrim(resource_namespace)
        AND char_length(container_name) <= 253
        AND container_name = btrim(container_name)
    ),
    CONSTRAINT metric_samples_expiry_check CHECK (expires_at >= collected_at),
    CONSTRAINT metric_samples_run_cluster_fk FOREIGN KEY (collection_run_id, cluster_id)
        REFERENCES metric_collection_runs(id, cluster_id) ON DELETE CASCADE,
    CONSTRAINT metric_samples_collection_series_unique UNIQUE (
        collection_run_id, resource_kind, resource_namespace, resource_name,
        container_name, metric_name
    )
);

CREATE INDEX IF NOT EXISTS metric_collection_runs_cluster_completed_idx
    ON metric_collection_runs (cluster_id, completed_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS metric_collection_runs_expiry_idx
    ON metric_collection_runs (expires_at, id);
CREATE INDEX IF NOT EXISTS metric_samples_series_time_idx
    ON metric_samples (
        cluster_id, resource_kind, resource_namespace, resource_name,
        container_name, metric_name, collected_at DESC, id DESC
    );
