ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_source_type_check;
ALTER TABLE incidents ADD CONSTRAINT incidents_source_type_check
    CHECK (source_type IN ('diagnosis', 'finding', 'alert', 'inspection', 'signal'));
