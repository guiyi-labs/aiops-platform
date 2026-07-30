CREATE TABLE IF NOT EXISTS promotion_plans (
    id VARCHAR(36) PRIMARY KEY,
    source_cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    destination_cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    source_namespace VARCHAR(63) NOT NULL,
    destination_namespace VARCHAR(63) NOT NULL,
    status VARCHAR(24) NOT NULL,
    bundle_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    dependency_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    confirmation_token_hash BYTEA NOT NULL,
    requested_by_user_id BIGINT,
    requested_by_name VARCHAR(128) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT promotion_plans_cluster_distinct_check
        CHECK (source_cluster_id <> destination_cluster_id),
    CONSTRAINT promotion_plans_status_check
        CHECK (status IN ('awaiting_confirmation', 'executing', 'succeeded', 'failed', 'partial', 'expired')),
    CONSTRAINT promotion_plans_namespace_length_check
        CHECK (char_length(source_namespace) BETWEEN 1 AND 63
            AND char_length(destination_namespace) BETWEEN 1 AND 63),
    CONSTRAINT promotion_plans_token_present_check
        CHECK (char_length(id) = 36 AND char_length(requested_by_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS promotion_plans_source_idx
    ON promotion_plans (source_cluster_id, source_namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS promotion_plans_destination_idx
    ON promotion_plans (destination_cluster_id, destination_namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS promotion_plans_claim_idx
    ON promotion_plans (status, expires_at, locked_at);

CREATE TABLE IF NOT EXISTS promotion_bundle_items (
    id BIGSERIAL PRIMARY KEY,
    plan_id VARCHAR(36) NOT NULL REFERENCES promotion_plans(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    kind VARCHAR(16) NOT NULL,
    source_namespace VARCHAR(63) NOT NULL,
    source_name VARCHAR(253) NOT NULL,
    source_uid VARCHAR(128) NOT NULL DEFAULT '',
    source_resource_version VARCHAR(128) NOT NULL DEFAULT '',
    destination_namespace VARCHAR(63) NOT NULL,
    destination_name VARCHAR(253) NOT NULL,
    manifest JSONB NOT NULL,
    diff JSONB NOT NULL DEFAULT '{}'::jsonb,
    item_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    CONSTRAINT promotion_bundle_items_kind_check
        CHECK (kind IN ('Deployment', 'Service', 'Ingress')),
    CONSTRAINT promotion_bundle_items_status_check
        CHECK (item_status IN ('pending', 'applied', 'failed', 'skipped')),
    CONSTRAINT promotion_bundle_items_ordinal_check
        CHECK (ordinal BETWEEN 0 AND 9),
    CONSTRAINT promotion_bundle_items_unique_ordinal UNIQUE (plan_id, ordinal)
);

CREATE INDEX IF NOT EXISTS promotion_bundle_items_plan_idx
    ON promotion_bundle_items (plan_id, ordinal);

CREATE TABLE IF NOT EXISTS promotion_dependency_mappings (
    id BIGSERIAL PRIMARY KEY,
    plan_id VARCHAR(36) NOT NULL REFERENCES promotion_plans(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL,
    source_namespace VARCHAR(63) NOT NULL,
    source_name VARCHAR(253) NOT NULL,
    destination_namespace VARCHAR(63) NOT NULL,
    destination_name VARCHAR(253) NOT NULL,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT promotion_dependency_mappings_kind_check
        CHECK (kind IN ('ConfigMap', 'Secret'))
);

CREATE INDEX IF NOT EXISTS promotion_dependency_mappings_plan_idx
    ON promotion_dependency_mappings (plan_id, kind);
