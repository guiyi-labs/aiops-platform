CREATE TABLE IF NOT EXISTS diagnosis_records (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    rule_id VARCHAR(128) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    resource_kind VARCHAR(64) NOT NULL,
    resource_namespace VARCHAR(253) NOT NULL DEFAULT '',
    resource_name VARCHAR(253) NOT NULL,
    resource_uid VARCHAR(128) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'open',
    summary TEXT NOT NULL,
    root_causes JSONB NOT NULL DEFAULT '[]'::JSONB,
    recommendations JSONB NOT NULL DEFAULT '[]'::JSONB,
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT diagnosis_records_severity_check CHECK (severity IN ('info', 'warning', 'high', 'critical')),
    CONSTRAINT diagnosis_records_status_check CHECK (status IN ('open', 'confirmed', 'resolved', 'dismissed'))
);

CREATE TABLE IF NOT EXISTS diagnosis_evidence (
    id BIGSERIAL PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    evidence_type VARCHAR(64) NOT NULL,
    source VARCHAR(256) NOT NULL,
    content JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS diagnosis_records_cluster_created_idx ON diagnosis_records (cluster_id, created_at DESC);
CREATE INDEX IF NOT EXISTS diagnosis_records_resource_idx ON diagnosis_records (cluster_id, resource_kind, resource_namespace, resource_name);
CREATE INDEX IF NOT EXISTS diagnosis_evidence_diagnosis_idx ON diagnosis_evidence (diagnosis_id);
