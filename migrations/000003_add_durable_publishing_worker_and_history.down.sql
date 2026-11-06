DROP INDEX idx_publish_attempts_variant_history;
DROP INDEX idx_publish_attempts_schedule_history;
CREATE INDEX idx_publish_attempts_schedule ON publish_attempts (schedule_id);
CREATE INDEX idx_publish_attempts_variant ON publish_attempts (variant_id);
DROP INDEX idx_schedules_stale_processing;

ALTER TABLE schedules
    DROP CONSTRAINT schedules_dispatch_started_check,
    DROP CONSTRAINT schedules_processing_started_check,
    DROP COLUMN dispatch_started_at,
    DROP COLUMN processing_started_at;

ALTER TABLE publish_attempts ALTER COLUMN status TYPE TEXT USING status::text;
UPDATE publish_attempts SET status = 'FAILED' WHERE status IN ('PROCESSING', 'OUTCOME_UNKNOWN');
ALTER TYPE publish_attempt_status RENAME TO publish_attempt_status_next;
CREATE TYPE publish_attempt_status AS ENUM ('SUCCESS', 'FAILED');
ALTER TABLE publish_attempts
    ALTER COLUMN status TYPE publish_attempt_status
    USING status::publish_attempt_status;
DROP TYPE publish_attempt_status_next;
