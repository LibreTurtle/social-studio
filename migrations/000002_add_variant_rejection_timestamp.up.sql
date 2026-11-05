ALTER TABLE variants
    ADD COLUMN rejected_at TIMESTAMPTZ,
    ADD CONSTRAINT variants_rejected_at_status_check
        CHECK ((status = 'REJECTED') = (rejected_at IS NOT NULL));
