ALTER TABLE correlation_signal_links
    DROP COLUMN IF EXISTS coverage,
    DROP COLUMN IF EXISTS freshness,
    DROP COLUMN IF EXISTS window_start,
    DROP COLUMN IF EXISTS window_end;
