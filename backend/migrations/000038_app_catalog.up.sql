-- M57: Helm application catalog + controlled deploy plans.
-- Stores registered Helm repository metadata and M19-style controlled-
-- operation plans for chart deployment. Chart metadata is fetched live
-- from each repo's index.yaml (read-only HTTP); the deploy preview builds
-- a Flux HelmRelease CR manifest and validates it via server-side dry-run,
-- then execute creates the CR through the bounded kubernetes gateway.
-- No Helm SDK dependency; credentials are stored as JSONB and redacted in
-- all API responses (ADR 0008 / project invariant: credentials never in
-- API/UI/logs/audit).

CREATE TABLE IF NOT EXISTS helm_repositories (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(128) NOT NULL DEFAULT '',
    url VARCHAR(512) NOT NULL,
    -- credentials_json stores basic-auth / TLS client cert material as JSONB.
    -- Sensitive fields (username, password, tls_client_cert, tls_client_key)
    -- are NEVER returned in API responses — the handler redacts them.
    credentials_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT helm_repositories_name_unique UNIQUE (name),
    CONSTRAINT helm_repositories_url_length_check
        CHECK (char_length(url) BETWEEN 8 AND 512),
    CONSTRAINT helm_repositories_name_format_check
        CHECK (name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')
);

CREATE INDEX IF NOT EXISTS idx_helm_repositories_name ON helm_repositories(name);

-- M19 controlled-operation plan for Helm chart deployment.
-- Mirrors promotion_plans (000019): confirmation_token_hash + idempotency_key
-- + Claim state machine. Preview builds a HelmRelease CR manifest and
-- validates via server-side dry-run; execute creates the CR on the target
-- cluster through the bounded kubernetes gateway.
CREATE TABLE IF NOT EXISTS app_catalog_plans (
    id VARCHAR(36) PRIMARY KEY,
    status VARCHAR(32) NOT NULL,
    repo_id BIGINT NOT NULL REFERENCES helm_repositories(id) ON DELETE CASCADE,
    chart_name VARCHAR(253) NOT NULL,
    chart_version VARCHAR(128) NOT NULL,
    target_cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    target_namespace VARCHAR(63) NOT NULL,
    release_name VARCHAR(253) NOT NULL,
    -- values_yaml stores the user-supplied Helm values as raw YAML text.
    values_yaml TEXT NOT NULL DEFAULT '',
    -- chart_metadata captures the chart's index.yaml entry (description, icon,
    -- home, maintainers, etc.) at preview time so execute sees a consistent
    -- snapshot even if the repo index changes between preview and execute.
    chart_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- release_manifest stores the rendered HelmRelease CR manifest JSON that
    -- will be applied at execute time. Built at preview; applied verbatim at
    -- execute (no re-rendering — deterministic).
    release_manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- deploy_diff is the typed preview diff (chart name, version, namespace,
    -- release name, values overrides) shown to the operator before confirm.
    deploy_diff JSONB NOT NULL DEFAULT '{}'::jsonb,
    confirmation_token_hash BYTEA NOT NULL,
    requested_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    requested_by_name VARCHAR(128) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT app_catalog_plans_status_check
        CHECK (status IN ('awaiting_confirmation', 'executing', 'succeeded', 'failed', 'expired')),
    CONSTRAINT app_catalog_plans_namespace_length_check
        CHECK (char_length(target_namespace) BETWEEN 1 AND 63),
    CONSTRAINT app_catalog_plans_release_name_check
        CHECK (char_length(release_name) BETWEEN 1 AND 253),
    CONSTRAINT app_catalog_plans_token_present_check
        CHECK (char_length(id) = 36 AND char_length(requested_by_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS app_catalog_plans_claim_idx
    ON app_catalog_plans (status, expires_at, locked_at);
CREATE INDEX IF NOT EXISTS app_catalog_plans_cluster_idx
    ON app_catalog_plans (target_cluster_id, target_namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS app_catalog_plans_repo_idx
    ON app_catalog_plans (repo_id, created_at DESC);
