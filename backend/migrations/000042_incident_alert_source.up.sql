-- M103: allow incidents to be created from a firing alert instance.
-- The alert -> incident path promotes an alert instance into the same
-- collaborative workspace used by diagnosis/finding sources; dedup is still
-- enforced by (source_type, source_ref).

ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_source_type_check;
ALTER TABLE incidents ADD CONSTRAINT incidents_source_type_check
    CHECK (source_type IN ('diagnosis', 'finding', 'alert'));
