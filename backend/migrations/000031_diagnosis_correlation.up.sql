-- M42: Multi-Signal Correlation And Deterministic RCA
--
-- Introduces correlation_cases (the aggregation unit), correlation_signal_links
-- (signal→case relations), correlation_resource_links (topology path links)
-- and correlation_change_candidates (root-cause candidates with factors and
-- confidence). diagnosis_records remains the human status/SLA/feedback source
-- of truth; these tables are the deterministic correlation layer.
--
-- Design invariants:
-- * One active case per deterministic case_key (partial unique index).
-- * case_key = SHA256 over (cluster_id + resource_uid + rule_id + correlation_version).
-- * Different UID, authorization scope or unrelated topology never merges.
-- * Confidence never self-upgrades; AI cannot promote candidate→confirmed.
-- * Temporal proximity alone is never causality — a change-symptom rule must
--   match for ConfidenceConfirmed.

CREATE TABLE IF NOT EXISTS correlation_cases (
    id BIGSERIAL PRIMARY KEY,
    case_key VARCHAR(64) NOT NULL,
    cluster_id BIGINT NOT NULL,
    rule_id VARCHAR(128) NOT NULL,
    correlation_version VARCHAR(16) NOT NULL DEFAULT '1.0',
    primary_kind VARCHAR(64) NOT NULL,
    primary_namespace VARCHAR(253) NOT NULL DEFAULT '',
    primary_name VARCHAR(253) NOT NULL,
    primary_uid VARCHAR(128) NOT NULL DEFAULT '',
    primary_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    confidence VARCHAR(16) NOT NULL DEFAULT 'unknown',
    evidence_completeness VARCHAR(16) NOT NULL DEFAULT 'insufficient',
    factors JSONB NOT NULL DEFAULT '[]'::JSONB,
    diagnosis_ids JSONB NOT NULL DEFAULT '[]'::JSONB,
    root_change_candidate_id BIGINT,
    first_observed_at TIMESTAMPTZ NOT NULL,
    last_observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('active','resolved','stale')),
    CHECK (confidence IN ('confirmed','candidate','contradicted','unknown')),
    CHECK (evidence_completeness IN ('complete','partial','insufficient')),
    CHECK (correlation_version = '1.0'),
    CHECK (case_key ~ '^[0-9a-f]{64}$'),
    CHECK (primary_kind <> ''),
    CHECK (primary_name <> '')
);

-- At most one active case per case_key. Resolved/stale cases retain their
-- rows so historical queries remain possible; a new active case for the same
-- key is admitted only after the previous one is resolved or stale.
CREATE UNIQUE INDEX IF NOT EXISTS uq_correlation_cases_active
    ON correlation_cases (case_key)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_correlation_cases_cluster_time
    ON correlation_cases (cluster_id, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_correlation_cases_resource
    ON correlation_cases (cluster_id, primary_kind, primary_namespace, primary_name);
CREATE INDEX IF NOT EXISTS idx_correlation_cases_primary_uid
    ON correlation_cases (cluster_id, primary_uid)
    WHERE primary_uid <> '';
CREATE INDEX IF NOT EXISTS idx_correlation_cases_rule
    ON correlation_cases (rule_id, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_correlation_cases_status
    ON correlation_cases (status, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_correlation_cases_confidence
    ON correlation_cases (confidence, last_observed_at DESC);

CREATE TABLE IF NOT EXISTS correlation_signal_links (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES correlation_cases(id) ON DELETE CASCADE,
    signal_occurrence_id BIGINT NOT NULL,
    relation VARCHAR(16) NOT NULL,
    signal_id VARCHAR(128) NOT NULL,
    producer VARCHAR(32) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (relation IN ('trigger','context','change','outcome')),
    CHECK (producer IN ('diagnosis','alert','metric','posture','audit','change')),
    CHECK (signal_id <> '')
);

-- One link per (case, occurrence, relation) — duplicate delivery yields the
-- same row, mirroring signal_occurrences fingerprint dedup.
CREATE UNIQUE INDEX IF NOT EXISTS uq_correlation_signal_links
    ON correlation_signal_links (case_id, signal_occurrence_id, relation);

CREATE INDEX IF NOT EXISTS idx_correlation_signal_links_case
    ON correlation_signal_links (case_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_correlation_signal_links_occurrence
    ON correlation_signal_links (signal_occurrence_id);

CREATE TABLE IF NOT EXISTS correlation_resource_links (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES correlation_cases(id) ON DELETE CASCADE,
    kind VARCHAR(64) NOT NULL,
    namespace VARCHAR(253) NOT NULL DEFAULT '',
    name VARCHAR(253) NOT NULL,
    uid VARCHAR(128) NOT NULL DEFAULT '',
    incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    relation VARCHAR(16) NOT NULL,
    topology_path JSONB NOT NULL DEFAULT '[]'::JSONB,
    edge_ids JSONB NOT NULL DEFAULT '[]'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (relation IN ('primary','upstream','downstream','related')),
    CHECK (kind <> ''),
    CHECK (name <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_correlation_resource_links
    ON correlation_resource_links (case_id, kind, namespace, name, uid, relation);

CREATE INDEX IF NOT EXISTS idx_correlation_resource_links_case
    ON correlation_resource_links (case_id);
CREATE INDEX IF NOT EXISTS idx_correlation_resource_links_uid
    ON correlation_resource_links (kind, uid)
    WHERE uid <> '';

CREATE TABLE IF NOT EXISTS correlation_change_candidates (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES correlation_cases(id) ON DELETE CASCADE,
    change_event_id BIGINT NOT NULL,
    rule_id VARCHAR(128) NOT NULL,
    confidence VARCHAR(16) NOT NULL DEFAULT 'unknown',
    rank INTEGER NOT NULL DEFAULT 1,
    factors JSONB NOT NULL DEFAULT '[]'::JSONB,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::JSONB,
    contradicting_refs JSONB NOT NULL DEFAULT '[]'::JSONB,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (confidence IN ('confirmed','candidate','contradicted','unknown')),
    CHECK (rank >= 1 AND rank <= 10),
    CHECK (rule_id <> '')
);

-- At most one candidate per (case, change_event) — a change event is ranked
-- once per case, not multiple times.
CREATE UNIQUE INDEX IF NOT EXISTS uq_correlation_change_candidates
    ON correlation_change_candidates (case_id, change_event_id);

CREATE INDEX IF NOT EXISTS idx_correlation_change_candidates_case
    ON correlation_change_candidates (case_id, rank);
CREATE INDEX IF NOT EXISTS idx_correlation_change_candidates_confidence
    ON correlation_change_candidates (confidence, rank);
CREATE INDEX IF NOT EXISTS idx_correlation_change_candidates_event
    ON correlation_change_candidates (change_event_id);
