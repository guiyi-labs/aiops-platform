-- M51: Alert inhibit rules (source_match -> target_match suppression).
-- Extends M37B alertroute: when an alert matching the source is firing, alerts
-- matching the target are suppressed (inhibited). Unlike silences, inhibits are
-- not time-bounded — they are active while the source alert is firing and are
-- removed only by explicit deletion. A source alert is considered firing when a
-- non-resolved alert_route_delivery exists for the source match within the
-- active window (default 5m, configurable per-inhibit via equal_to optional).
CREATE TABLE IF NOT EXISTS alert_inhibits (
    id BIGSERIAL PRIMARY KEY,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Source match: when a firing alert matches, the target is suppressed.
    source_cluster_id BIGINT,
    source_rule_name VARCHAR(256) NOT NULL DEFAULT '',
    source_severity VARCHAR(32) NOT NULL DEFAULT '',
    -- Target match: alerts matching this are inhibited while source is firing.
    target_cluster_id BIGINT,
    target_rule_name VARCHAR(256) NOT NULL DEFAULT '',
    target_severity VARCHAR(32) NOT NULL DEFAULT '',
    -- Reason is mandatory (mirrors silences) for auditability.
    reason VARCHAR(500) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (source_rule_name <> '' OR source_cluster_id IS NOT NULL OR source_severity <> ''),
    CHECK (target_rule_name <> '' OR target_cluster_id IS NOT NULL OR target_severity <> '')
);
CREATE INDEX idx_alert_inhibits_creator ON alert_inhibits(creator_id);
CREATE INDEX idx_alert_inhibits_enabled ON alert_inhibits(enabled) WHERE enabled = TRUE;
