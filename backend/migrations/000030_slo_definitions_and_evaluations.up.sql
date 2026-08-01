-- M41: SLO, error budget and impact.
-- slo_definitions stores versioned SLO definitions (server-owned templates
-- only; never raw PromQL).
-- slo_evaluations stores append-only deterministic evaluations per (slo_id,
-- version, window). Historical evaluations are never rewritten.

CREATE TABLE IF NOT EXISTS slo_definitions (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL,
    service_kind VARCHAR(64) NOT NULL,
    service_namespace VARCHAR(63) NOT NULL DEFAULT '',
    service_name VARCHAR(253) NOT NULL,
    service_uid VARCHAR(64) NOT NULL DEFAULT '',
    service_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    template VARCHAR(64) NOT NULL,
    template_version VARCHAR(16) NOT NULL,
    objective DOUBLE PRECISION NOT NULL,
    rolling_window_seconds INTEGER NOT NULL,
    missing_data_policy VARCHAR(16) NOT NULL DEFAULT 'unavailable',
    latency_threshold_ms INTEGER NOT NULL DEFAULT 0,
    owner_id BIGINT NOT NULL,
    owner_name VARCHAR(128) NOT NULL DEFAULT '',
    fast_burn_rate DOUBLE PRECISION NOT NULL,
    fast_burn_window_seconds INTEGER NOT NULL,
    slow_burn_rate DOUBLE PRECISION NOT NULL,
    slow_burn_window_seconds INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version INTEGER NOT NULL DEFAULT 1,
    creator_id BIGINT NOT NULL,
    creator_name VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (template IN ('request_success_ratio','request_latency_target_ratio','workload_readiness')),
    CHECK (missing_data_policy IN ('unavailable','fail_open')),
    CHECK (objective >= 0.0 AND objective <= 1.0),
    CHECK (rolling_window_seconds >= 300 AND rolling_window_seconds <= 2592000),
    CHECK (fast_burn_rate >= 0.0),
    CHECK (slow_burn_rate >= 0.0),
    CHECK (fast_burn_window_seconds >= 60 AND fast_burn_window_seconds <= 86400),
    CHECK (slow_burn_window_seconds >= 60 AND slow_burn_window_seconds <= 86400),
    CHECK (fast_burn_window_seconds <= slow_burn_window_seconds),
    CHECK (latency_threshold_ms >= 0 AND latency_threshold_ms <= 60000),
    CHECK (version >= 1)
);

-- One active definition per (cluster, service, template). When a definition
-- is re-created after deletion, a new row is inserted.
CREATE UNIQUE INDEX IF NOT EXISTS uq_slo_definitions_active
    ON slo_definitions(cluster_id, service_namespace, service_name, template)
    WHERE enabled = TRUE;

CREATE INDEX IF NOT EXISTS idx_slo_definitions_cluster_ns
    ON slo_definitions(cluster_id, service_namespace);
CREATE INDEX IF NOT EXISTS idx_slo_definitions_owner
    ON slo_definitions(owner_id);
CREATE INDEX IF NOT EXISTS idx_slo_definitions_template
    ON slo_definitions(template, enabled);

CREATE TABLE IF NOT EXISTS slo_evaluations (
    id BIGSERIAL PRIMARY KEY,
    slo_id BIGINT NOT NULL,
    version INTEGER NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    good_events DOUBLE PRECISION NOT NULL,
    total_events DOUBLE PRECISION NOT NULL,
    ratio DOUBLE PRECISION NOT NULL,
    target_ratio DOUBLE PRECISION NOT NULL,
    error_budget DOUBLE PRECISION NOT NULL,
    remaining_budget DOUBLE PRECISION NOT NULL,
    burn_rate DOUBLE PRECISION NOT NULL,
    state VARCHAR(16) NOT NULL,
    coverage VARCHAR(16) NOT NULL,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (state IN ('healthy','burning_slow','burning_fast','breached','unavailable')),
    CHECK (coverage IN ('complete','partial','unavailable')),
    CHECK (window_end > window_start),
    CHECK (version >= 1),
    CHECK (total_events >= 0.0),
    CHECK (good_events >= 0.0 AND good_events <= total_events),
    CHECK (ratio >= 0.0 AND ratio <= 1.0),
    CHECK (target_ratio >= 0.0 AND target_ratio <= 1.0),
    CHECK (error_budget >= 0.0 AND error_budget <= 1.0)
);

-- Latest evaluation per (slo_id, version) for fast current-state lookup.
CREATE INDEX IF NOT EXISTS idx_slo_evaluations_slo_version
    ON slo_evaluations(slo_id, version, evaluated_at DESC);
CREATE INDEX IF NOT EXISTS idx_slo_evaluations_state
    ON slo_evaluations(state, evaluated_at DESC);
CREATE INDEX IF NOT EXISTS idx_slo_evaluations_window
    ON slo_evaluations(slo_id, window_start, window_end);
