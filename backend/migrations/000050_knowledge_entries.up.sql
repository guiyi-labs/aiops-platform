-- P1: Knowledge base for RAG-backed diagnosis.
--
-- Stores distilled, validated entries taken from resolved diagnosis records
-- (root causes + recommendations + summary). The retriever searches this
-- table to inject historical, verified cases into AI prompts (aiexplain /
-- aiinvestigator). Design invariants:
--
-- * Entries are written only when a diagnosis transitions to resolved and
--   carries non-empty root causes / recommendations (ingest hook, not a
--   trigger, so the write is explicit and testable).
-- * Dedup key: (rule_id, resource_kind, resource_name, resolved_at date)
--   lets the same recurring defect keep the newest resolved entry while a
--   genuinely distinct failure on the same resource still lands.
-- * The table is a distilled snapshot, NOT a live view: source diagnosis
--   edits never mutate knowledge entries retroactively.

CREATE TABLE IF NOT EXISTS knowledge_entries (
    id BIGSERIAL PRIMARY KEY,
    source_diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    rule_id VARCHAR(128) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    resource_kind VARCHAR(64) NOT NULL,
    resource_namespace VARCHAR(253) NOT NULL DEFAULT '',
    resource_name VARCHAR(253) NOT NULL,
    summary TEXT NOT NULL,
    root_causes JSONB NOT NULL DEFAULT '[]'::JSONB,
    recommendations JSONB NOT NULL DEFAULT '[]'::JSONB,
    noted_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT knowledge_entries_severity_check CHECK (severity IN ('info', 'warning', 'high', 'critical'))
);

-- Retrieval index: rule + severity + resource kind narrows the candidate
-- set before the LLM re-ranks (Phase-1 structured retrieval).
CREATE INDEX IF NOT EXISTS knowledge_entries_rule_idx ON knowledge_entries (rule_id, severity, resource_kind, noted_at DESC);
-- Dedup lookup index.
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_entries_dedup_idx ON knowledge_entries (rule_id, resource_kind, resource_name, noted_at);