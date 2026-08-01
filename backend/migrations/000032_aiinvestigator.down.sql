-- M43: Cited And Evaluated AI Investigator (down migration)

DROP INDEX IF EXISTS idx_ai_investigations_status;
DROP INDEX IF EXISTS idx_ai_investigations_case_id;
DROP INDEX IF EXISTS uq_ai_investigations_active;

ALTER TABLE ai_investigations
    DROP CONSTRAINT IF EXISTS fk_ai_investigations_case;

DROP TABLE IF EXISTS ai_investigations;
