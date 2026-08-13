-- M104: allow incidents to be created from an inspection result.
-- Inspection findings (M52 intelligent inspection) promote into the same
-- collaborative incident workspace; dedup is still enforced by
-- (source_type, source_ref).

ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_source_type_check;
ALTER TABLE incidents ADD CONSTRAINT incidents_source_type_check
    CHECK (source_type IN ('diagnosis', 'finding', 'alert', 'inspection'));
