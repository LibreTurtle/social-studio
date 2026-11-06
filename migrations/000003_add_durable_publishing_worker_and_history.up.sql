ALTER TYPE publish_attempt_status RENAME TO publish_attempt_status_previous;
CREATE TYPE publish_attempt_status AS ENUM ('PROCESSING', 'SUCCESS', 'FAILED', 'OUTCOME_UNKNOWN');
ALTER TABLE publish_attempts
    ALTER COLUMN status TYPE publish_attempt_status
    USING status::text::publish_attempt_status;
DROP TYPE publish_attempt_status_previous;

ALTER TABLE schedules
    ADD COLUMN processing_started_at TIMESTAMPTZ,
    ADD COLUMN dispatch_started_at TIMESTAMPTZ;

UPDATE schedules
SET processing_started_at = updated_at,
    dispatch_started_at = updated_at
WHERE status = 'PROCESSING';

INSERT INTO publish_attempts (schedule_id, variant_id, idempotency_key, status, error_message, attempted_at)
SELECT s.id,
       s.variant_id,
       s.variant_id::text || ':' || to_char(s.scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
       'OUTCOME_UNKNOWN',
       'provider outcome unknown during scheduler rollout',
       s.updated_at
FROM schedules s
WHERE s.status = 'PROCESSING'
  AND NOT EXISTS (SELECT 1 FROM publish_attempts a WHERE a.schedule_id = s.id);

ALTER TABLE schedules
    ADD CONSTRAINT schedules_processing_started_check
        CHECK (status <> 'PROCESSING' OR processing_started_at IS NOT NULL),
    ADD CONSTRAINT schedules_dispatch_started_check
        CHECK (dispatch_started_at IS NULL OR processing_started_at IS NOT NULL);

CREATE INDEX idx_schedules_stale_processing
    ON schedules (processing_started_at)
    WHERE status = 'PROCESSING';

DROP INDEX idx_publish_attempts_schedule;
DROP INDEX idx_publish_attempts_variant;
CREATE INDEX idx_publish_attempts_schedule_history
    ON publish_attempts (schedule_id, attempted_at DESC, id DESC);

CREATE INDEX idx_publish_attempts_variant_history
    ON publish_attempts (variant_id, attempted_at DESC, id DESC);
