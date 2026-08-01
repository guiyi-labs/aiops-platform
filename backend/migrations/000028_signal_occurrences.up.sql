-- M39: Unified signal model — append-only, TTL-bound signal occurrences.
-- Stores normalized envelopes from M21-M31 producers; not a raw telemetry
-- warehouse, full manifest or complete log body.
CREATE TABLE IF NOT EXISTS signal_occurrences (
    id BIGSERIAL PRIMARY KEY,
    signal_id VARCHAR(128) NOT NULL,
    signal_code VARCHAR(128) NOT NULL,
    schema_version VARCHAR(16) NOT NULL DEFAULT '1.0',
    producer VARCHAR(32) NOT NULL,
    cluster_id BIGINT NOT NULL,
    namespace VARCHAR(63) NOT NULL DEFAULT '',
    resource_kind VARCHAR(64) NOT NULL,
    resource_namespace VARCHAR(63) NOT NULL DEFAULT '',
    resource_name VARCHAR(253) NOT NULL,
    resource_uid VARCHAR(64) NOT NULL DEFAULT '',
    resource_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    severity VARCHAR(16) NOT NULL,
    state VARCHAR(16) NOT NULL,
    fingerprint VARCHAR(64) NOT NULL,
    coverage VARCHAR(16) NOT NULL,
    freshness TIMESTAMPTZ NOT NULL,
    window_start TIMESTAMPTZ,
    window_end TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    ingestion_run_id VARCHAR(64) NOT NULL DEFAULT '',
    CHECK (schema_version = '1.0'),
    CHECK (producer IN ('diagnosis','alert','metric','posture','audit','change')),
    CHECK (state IN ('active','resolved','expired','dismissed')),
    CHECK (coverage IN ('complete','partial','unavailable','truncated')),
    CHECK (severity IN ('critical','warning','info'))
);

-- Idempotent ingestion: one row per (signal_id, fingerprint) contract.
-- ON CONFLICT updates state/freshness/observed_at instead of duplicating.
CREATE UNIQUE INDEX IF NOT EXISTS uq_signal_occurrences_fingerprint
    ON signal_occurrences(signal_id, fingerprint);

-- Query indexes for the overview and list endpoints.
CREATE INDEX IF NOT EXISTS idx_signal_occurrences_cluster_ns
    ON signal_occurrences(cluster_id, namespace);
CREATE INDEX IF NOT EXISTS idx_signal_occurrences_state
    ON signal_occurrences(state, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_signal_occurrences_producer
    ON signal_occurrences(producer, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_signal_occurrences_expires
    ON signal_occurrences(expires_at) WHERE expires_at IS NOT NULL;
