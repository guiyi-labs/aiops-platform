-- M43: Cited And Evaluated AI Investigator
--
-- Introduces ai_investigations (the case-level AI investigation output bound to
-- M42 correlation_cases). The investigation is a read-only advisory: it never
-- modifies the case, diagnosis or alert. One active investigation per case;
-- newer investigations mark older ones stale.
--
-- Design invariants:
-- * Every factual claim, impact statement and hypothesis cites an authorized
--   evidence ID. Fabricated, nonexistent or unauthorized citations reject
--   the entire output (enforced in the validator, not the DB).
-- * The model cannot upgrade a candidate to confirmed cause.
-- * Recommended runbook IDs must be eligible per the M42 Action Catalog.
-- * AI outage, budget exhaustion or schema/citation failure leaves
--   deterministic investigation and manual actions available.
-- * investigation_key = SHA256 over (case_id + investigator_version + prompt_hash)
--   so identical evidence + prompt + version reproduce identical investigations.

CREATE TABLE IF NOT EXISTS ai_investigations (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL,
    investigation_key VARCHAR(64) NOT NULL,
    investigator_version VARCHAR(16) NOT NULL DEFAULT '1.0',
    actor_id BIGINT NOT NULL,
    actor_name VARCHAR(253) NOT NULL,
    provider VARCHAR(64) NOT NULL,
    model VARCHAR(128) NOT NULL,
    provider_response_id VARCHAR(253) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'completed',
    summary TEXT NOT NULL DEFAULT '',
    impact TEXT NOT NULL DEFAULT '',
    hypotheses JSONB NOT NULL DEFAULT '[]',
    recommended_runbook_id VARCHAR(128) NOT NULL DEFAULT '',
    uncertainties JSONB NOT NULL DEFAULT '[]',
    citations JSONB NOT NULL DEFAULT '[]',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    failure_reason VARCHAR(253) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ai_investigations_status_chk CHECK (
        status IN ('completed', 'failed', 'stale')
    ),
    CONSTRAINT ai_investigations_tokens_chk CHECK (
        input_tokens >= 0 AND output_tokens >= 0
    ),
    CONSTRAINT ai_investigations_completed_summary_chk CHECK (
        status != 'completed' OR summary != ''
    ),
    CONSTRAINT ai_investigations_completed_citations_chk CHECK (
        status != 'completed' OR jsonb_array_length(citations) >= 1
    ),
    CONSTRAINT ai_investigations_failed_reason_chk CHECK (
        status != 'failed' OR failure_reason != ''
    )
);

-- One active (non-stale) investigation per case_key.
CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_investigations_active
    ON ai_investigations (case_id, investigation_key)
    WHERE status != 'stale';

-- Query indexes.
CREATE INDEX IF NOT EXISTS idx_ai_investigations_case_id
    ON ai_investigations (case_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_investigations_status
    ON ai_investigations (status, created_at DESC);

-- FK to correlation_cases. A case can have multiple investigations over time
-- (re-runs after new evidence); only one is non-stale per investigation_key.
ALTER TABLE ai_investigations
    ADD CONSTRAINT fk_ai_investigations_case
    FOREIGN KEY (case_id) REFERENCES correlation_cases (id) ON DELETE CASCADE;
