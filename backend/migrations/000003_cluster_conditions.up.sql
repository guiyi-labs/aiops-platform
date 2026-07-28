ALTER TABLE clusters ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE clusters DROP CONSTRAINT IF EXISTS clusters_status_check;
ALTER TABLE clusters ADD CONSTRAINT clusters_status_check
    CHECK (status IN ('disabled', 'unknown', 'ready', 'unreachable'));

CREATE TABLE IF NOT EXISTS cluster_conditions (
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    type VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    reason VARCHAR(128) NOT NULL,
    message VARCHAR(1024) NOT NULL DEFAULT '',
    last_transition_time TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (cluster_id, type),
    CONSTRAINT cluster_conditions_status_check CHECK (status IN ('True', 'False', 'Unknown'))
);

CREATE INDEX IF NOT EXISTS cluster_conditions_cluster_id_idx ON cluster_conditions (cluster_id);
