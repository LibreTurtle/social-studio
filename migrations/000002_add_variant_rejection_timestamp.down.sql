ALTER TABLE variants
    DROP CONSTRAINT variants_rejected_at_status_check,
    DROP COLUMN rejected_at;
