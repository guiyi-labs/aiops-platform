-- M35: Lightweight cluster and namespace access grants.
-- Adds user-to-cluster and optional exact-namespace grants as the
-- resource-scope dimension on top of the existing four global platform roles
-- (the action dimension). SystemAdmin bypasses these grants entirely.

CREATE TABLE IF NOT EXISTS user_cluster_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_cluster_grants_unique UNIQUE (user_id, cluster_id)
);

CREATE INDEX IF NOT EXISTS user_cluster_grants_user_idx ON user_cluster_grants (user_id);
CREATE INDEX IF NOT EXISTS user_cluster_grants_cluster_idx ON user_cluster_grants (cluster_id);

CREATE TABLE IF NOT EXISTS user_namespace_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace VARCHAR(253) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_namespace_grants_unique UNIQUE (user_id, cluster_id, namespace)
);

CREATE INDEX IF NOT EXISTS user_namespace_grants_user_cluster_idx ON user_namespace_grants (user_id, cluster_id);
