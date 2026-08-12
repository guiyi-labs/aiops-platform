-- M98: Incident workspace for handoff, follow-up and postmortem.
-- An incident wraps a persisted diagnosis (or a client-observed finding) into
-- a collaborative workspace with a stable incident number, assignee, followers,
-- a system/note-separated timeline, an explicit status machine with CAS
-- versioning, and a read-only postmortem view. Creation from the same source
-- is deduplicated by (source_type, source_ref); every state-changing write
-- requires an expected_version and fails with 409 on conflict.

CREATE SEQUENCE IF NOT EXISTS incident_number_seq;

CREATE TABLE IF NOT EXISTS incidents (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE DEFAULT ('INC-' || LPAD(nextval('incident_number_seq')::TEXT, 6, '0')),
    title TEXT NOT NULL,
    source_type VARCHAR(16) NOT NULL CHECK (source_type IN ('diagnosis', 'finding')),
    source_ref TEXT NOT NULL,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    resource_kind VARCHAR(64) NOT NULL,
    resource_namespace VARCHAR(253) NOT NULL DEFAULT '',
    resource_name VARCHAR(253) NOT NULL,
    resource_uid VARCHAR(128) NOT NULL DEFAULT '',
    severity VARCHAR(16) NOT NULL CHECK (severity IN ('info', 'warning', 'high', 'critical')),
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'confirmed', 'resolved', 'dismissed')),
    summary TEXT NOT NULL,
    postmortem TEXT NOT NULL DEFAULT '',
    assigned_to_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    version BIGINT NOT NULL DEFAULT 1,
    observed_at TIMESTAMPTZ NOT NULL,
    sla_due_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT incidents_source_unique UNIQUE (source_type, source_ref)
);

CREATE INDEX IF NOT EXISTS incidents_cluster_idx ON incidents (cluster_id);
CREATE INDEX IF NOT EXISTS incidents_status_idx ON incidents (status);
CREATE INDEX IF NOT EXISTS incidents_assignee_idx ON incidents (assigned_to_user_id);
CREATE INDEX IF NOT EXISTS incidents_created_idx ON incidents (created_at DESC);

CREATE TABLE IF NOT EXISTS incident_followers (
    incident_id BIGINT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (incident_id, user_id)
);

CREATE TABLE IF NOT EXISTS incident_timeline_events (
    id BIGSERIAL PRIMARY KEY,
    incident_id BIGINT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    event_type VARCHAR(16) NOT NULL CHECK (event_type IN ('system', 'note')),
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_name VARCHAR(128) NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS incident_timeline_incident_idx ON incident_timeline_events (incident_id, created_at, id);
