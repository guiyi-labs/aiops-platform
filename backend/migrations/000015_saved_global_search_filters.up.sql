CREATE TABLE IF NOT EXISTS saved_global_search_filters (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(40) NOT NULL,
    query_text VARCHAR(64) NOT NULL,
    namespace VARCHAR(63) NOT NULL DEFAULT '',
    kinds VARCHAR(64) NOT NULL,
    schema_version SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT saved_global_search_filters_name_check
        CHECK (char_length(name) BETWEEN 1 AND 40 AND name = btrim(name)),
    CONSTRAINT saved_global_search_filters_query_check
        CHECK (char_length(query_text) BETWEEN 2 AND 64 AND query_text = btrim(query_text)),
    CONSTRAINT saved_global_search_filters_namespace_check
        CHECK (namespace = '' OR namespace ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
    CONSTRAINT saved_global_search_filters_kinds_check
        CHECK (kinds IN (
            'Pod', 'Deployment', 'Service', 'Ingress',
            'Pod,Deployment', 'Pod,Service', 'Pod,Ingress',
            'Deployment,Service', 'Deployment,Ingress', 'Service,Ingress',
            'Pod,Deployment,Service', 'Pod,Deployment,Ingress',
            'Pod,Service,Ingress', 'Deployment,Service,Ingress',
            'Pod,Deployment,Service,Ingress'
        )),
    CONSTRAINT saved_global_search_filters_schema_check CHECK (schema_version = 1)
);

CREATE UNIQUE INDEX IF NOT EXISTS saved_global_search_filters_user_name_idx
    ON saved_global_search_filters (user_id, LOWER(name));
CREATE INDEX IF NOT EXISTS saved_global_search_filters_user_created_idx
    ON saved_global_search_filters (user_id, created_at, id);
