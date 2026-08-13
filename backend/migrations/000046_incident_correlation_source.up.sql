-- M108: allow incidents to be created from a correlation case.
-- A case aggregates N signal occurrences under one case_key; promoting the
-- case into the same collaborative incident workspace used by the other five
-- sources keeps dedup on (source_type, source_ref) — one incident per case.

ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_source_type_check;
ALTER TABLE incidents ADD CONSTRAINT incidents_source_type_check
    CHECK (source_type IN ('diagnosis', 'finding', 'alert', 'inspection', 'signal', 'correlation'));
