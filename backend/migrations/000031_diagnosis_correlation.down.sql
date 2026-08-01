-- M42: Multi-Signal Correlation And Deterministic RCA (down migration)
--
-- Drops the four correlation tables in reverse dependency order. Safe to run
-- at any time; re-running the up migration recreates the schema.

DROP TABLE IF EXISTS correlation_change_candidates;
DROP TABLE IF EXISTS correlation_resource_links;
DROP TABLE IF EXISTS correlation_signal_links;
DROP TABLE IF EXISTS correlation_cases;
