-- M99-D: persist signal data metadata on correlation_signal_links so cases
-- expose missing samples (coverage) and data latency (freshness/window)
-- explicitly in the UI. Historical rows keep the default 'complete' coverage;
-- the upsert is idempotent per (case, occurrence, relation), so re-runs of
-- the correlation worker never overwrite recorded links.

ALTER TABLE correlation_signal_links
    ADD COLUMN IF NOT EXISTS coverage VARCHAR(16) NOT NULL DEFAULT 'complete',
    ADD COLUMN IF NOT EXISTS freshness TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS window_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS window_end TIMESTAMPTZ;
