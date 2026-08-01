-- M41: SLO, error budget and impact (down migration).

DROP INDEX IF EXISTS idx_slo_evaluations_window;
DROP INDEX IF EXISTS idx_slo_evaluations_state;
DROP INDEX IF EXISTS idx_slo_evaluations_slo_version;
DROP TABLE IF EXISTS slo_evaluations;

DROP INDEX IF EXISTS idx_slo_definitions_template;
DROP INDEX IF EXISTS idx_slo_definitions_owner;
DROP INDEX IF EXISTS idx_slo_definitions_cluster_ns;
DROP INDEX IF EXISTS uq_slo_definitions_active;
DROP TABLE IF EXISTS slo_definitions;
