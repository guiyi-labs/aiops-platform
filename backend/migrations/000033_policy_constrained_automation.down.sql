-- M44: Policy-Constrained Automation And Post-Action Verification (rollback)
--
-- Drops action_verifications and action_plans in dependency order. The
-- verification_id back-reference on action_plans is not a hard FK and needs
-- no explicit drop.

DROP TABLE IF EXISTS action_verifications;
DROP TABLE IF EXISTS action_plans;
