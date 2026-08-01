-- M37B: Alert route receivers, routes, silences and deliveries
CREATE TABLE IF NOT EXISTS alert_route_receivers (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (creator_id, name)
);

CREATE TABLE IF NOT EXISTS alert_routes (
    id BIGSERIAL PRIMARY KEY,
    receiver_id BIGINT NOT NULL REFERENCES alert_route_receivers(id) ON DELETE CASCADE,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 50 CHECK (priority >= 1 AND priority <= 100),
    cluster_id BIGINT,
    rule_name VARCHAR(256) NOT NULL DEFAULT '',
    severity VARCHAR(32) NOT NULL DEFAULT '',
    dedupe_key VARCHAR(256) NOT NULL,
    -- time.Duration values are persisted as nanoseconds so GORM can scan them
    -- without a lossy/custom conversion layer.
    group_interval BIGINT,
    repeat_interval BIGINT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_alert_routes_creator ON alert_routes(creator_id);
CREATE INDEX idx_alert_routes_enabled ON alert_routes(enabled) WHERE enabled = TRUE;

CREATE TABLE IF NOT EXISTS alert_silences (
    id BIGSERIAL PRIMARY KEY,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cluster_id BIGINT,
    rule_name VARCHAR(256) NOT NULL DEFAULT '',
    severity VARCHAR(32) NOT NULL DEFAULT '',
    reason VARCHAR(500) NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (ends_at > starts_at),
    CHECK (EXTRACT(EPOCH FROM (ends_at - starts_at)) <= 604800)
);
CREATE INDEX idx_alert_silences_creator ON alert_silences(creator_id);
-- Do not use NOW() in a partial-index predicate: PostgreSQL requires index
-- predicates to be IMMUTABLE while NOW() is only STABLE.  The leading
-- starts_at/ends_at columns still support the bounded active-silence query.
CREATE INDEX idx_alert_silences_window ON alert_silences(starts_at, ends_at);

CREATE TABLE IF NOT EXISTS alert_route_deliveries (
    id BIGSERIAL PRIMARY KEY,
    route_id BIGINT NOT NULL REFERENCES alert_routes(id) ON DELETE CASCADE,
    receiver_id BIGINT NOT NULL REFERENCES alert_route_receivers(id) ON DELETE CASCADE,
    alert_instance_id BIGINT NOT NULL,
    cluster_id BIGINT NOT NULL,
    rule_name VARCHAR(256) NOT NULL DEFAULT '',
    severity VARCHAR(32) NOT NULL DEFAULT '',
    event_type VARCHAR(32) NOT NULL,
    dedupe_key VARCHAR(256) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    last_error TEXT,
    locked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_alert_route_deliveries_status ON alert_route_deliveries(status, next_attempt_at);
CREATE UNIQUE INDEX idx_alert_route_deliveries_inflight_dedupe
    ON alert_route_deliveries(route_id, dedupe_key, event_type)
    WHERE status IN ('pending', 'delivering');
