package publishing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

func (store *PostgresStore) GetForSchedule(ctx context.Context, variantID string, scheduledAt time.Time) (domain.Platform, domain.VariantStatus, *Schedule, error) {
	var platform, status string
	var scheduleID, scheduleStatus sql.NullString
	var found Schedule
	var scheduleVariantID sql.NullString
	var scheduled, created, updated sql.NullTime
	var version sql.NullInt64
	err := store.db.QueryRowContext(ctx, `
		SELECT v.platform::text, v.status::text,
		       s.id::text, s.variant_id::text, s.scheduled_at, s.status::text,
		       s.created_at, s.updated_at, s.version
		FROM variants v
		LEFT JOIN schedules s ON s.variant_id = v.id AND s.scheduled_at = $2
		WHERE v.id = $1`, variantID, scheduledAt).Scan(
		&platform, &status, &scheduleID, &scheduleVariantID, &scheduled,
		&scheduleStatus, &created, &updated, &version,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil, ErrNotFound
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("query variant for publishing schedule: %w", err)
	}
	if scheduleID.Valid {
		found.ID = scheduleID.String
		found.VariantID = scheduleVariantID.String
		found.ScheduledAt = scheduled.Time
		found.Status = domain.ScheduleStatus(scheduleStatus.String)
		found.CreatedAt = created.Time
		found.UpdatedAt = updated.Time
		found.Version = version.Int64
		return domain.Platform(platform), domain.VariantStatus(status), &found, nil
	}
	return domain.Platform(platform), domain.VariantStatus(status), nil, nil
}

func (store *PostgresStore) CreateSchedule(ctx context.Context, variantID string, scheduledAt time.Time) (Schedule, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Schedule{}, fmt.Errorf("begin schedule transaction: %w", err)
	}
	defer tx.Rollback()
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status::text FROM variants WHERE id = $1 FOR UPDATE`, variantID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("load variant for schedule: %w", err)
	}
	existing, err := scanSchedule(tx.QueryRowContext(ctx, `
		SELECT id::text, variant_id::text, scheduled_at, status::text, created_at, updated_at, version
		FROM schedules WHERE variant_id = $1 AND scheduled_at = $2`, variantID, scheduledAt))
	if err == nil {
		if err := tx.Commit(); err != nil {
			return Schedule{}, fmt.Errorf("commit repeated schedule lookup: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Schedule{}, fmt.Errorf("load existing schedule: %w", err)
	}
	var occupied bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM schedules
			WHERE variant_id = $1 AND status IN ('PENDING', 'PROCESSING', 'COMPLETED')
		)`, variantID).Scan(&occupied)
	if err != nil {
		return Schedule{}, fmt.Errorf("check existing variant schedule: %w", err)
	}
	if occupied {
		return Schedule{}, ErrAlreadyScheduled
	}
	if domain.VariantStatus(status) != domain.VariantStatusApproved {
		return Schedule{}, ErrNotApproved
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO schedules (variant_id, scheduled_at, status)
		VALUES ($1, $2, 'PENDING')
		ON CONFLICT (variant_id, scheduled_at) DO NOTHING`, variantID, scheduledAt)
	if err != nil {
		return Schedule{}, fmt.Errorf("insert schedule: %w", err)
	}
	value, err := scanSchedule(tx.QueryRowContext(ctx, `
		SELECT id::text, variant_id::text, scheduled_at, status::text, created_at, updated_at, version
		FROM schedules WHERE variant_id = $1 AND scheduled_at = $2`, variantID, scheduledAt))
	if err != nil {
		return Schedule{}, fmt.Errorf("load schedule: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Schedule{}, fmt.Errorf("commit schedule transaction: %w", err)
	}
	return value, nil
}

func (store *PostgresStore) Claim(ctx context.Context, scheduleID string) (Claim, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Claim{}, fmt.Errorf("begin publishing claim transaction: %w", err)
	}
	defer tx.Rollback()
	var claim Claim
	var scheduleStatus, variantStatus, platform string
	var due bool
	err = tx.QueryRowContext(ctx, `
		SELECT s.id::text, s.variant_id::text, s.scheduled_at, s.status::text,
		       s.created_at, s.updated_at, s.version, v.platform::text, v.content, v.status::text,
	       s.scheduled_at <= NOW()
		FROM schedules s JOIN variants v ON v.id = s.variant_id
		WHERE s.id = $1 FOR UPDATE OF s`, scheduleID).Scan(
		&claim.Schedule.ID, &claim.Schedule.VariantID, &claim.Schedule.ScheduledAt, &scheduleStatus,
		&claim.Schedule.CreatedAt, &claim.Schedule.UpdatedAt, &claim.Schedule.Version,
		&platform, &claim.Content, &variantStatus, &due,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Claim{}, ErrNotFound
	}
	if err != nil {
		return Claim{}, fmt.Errorf("load publishing schedule: %w", err)
	}
	claim.Schedule.Status = domain.ScheduleStatus(scheduleStatus)
	claim.Platform = domain.Platform(platform)
	if claim.Schedule.Status == domain.ScheduleStatusCompleted || claim.Schedule.Status == domain.ScheduleStatusFailed {
		attempt, attemptErr := scanAttempt(tx.QueryRowContext(ctx, `
			SELECT id::text, schedule_id::text, variant_id::text, idempotency_key, status::text,
			       external_post_id, error_message, attempted_at
			FROM publish_attempts WHERE schedule_id = $1 ORDER BY attempted_at DESC LIMIT 1`, scheduleID))
		if attemptErr != nil {
			return Claim{}, fmt.Errorf("load prior publish attempt: %w", attemptErr)
		}
		claim.ExistingAttempt = &attempt
		return claim, nil
	}
	if claim.Schedule.Status == domain.ScheduleStatusProcessing {
		return Claim{}, ErrInProgress
	}
	if claim.Schedule.Status != domain.ScheduleStatusPending {
		return Claim{}, ErrInProgress
	}
	if domain.VariantStatus(variantStatus) != domain.VariantStatusApproved {
		return Claim{}, ErrNotApproved
	}
	if !due {
		return Claim{}, ErrNotDue
	}
	claim.IdempotencyKey = idempotencyKey(claim.Schedule.VariantID, claim.Schedule.ScheduledAt)
	err = tx.QueryRowContext(ctx, `
		UPDATE schedules SET status = 'PROCESSING', processing_started_at = NOW(), dispatch_started_at = NULL,
		       updated_at = NOW(), version = version + 1
		WHERE id = $1 AND status = 'PENDING'
		RETURNING updated_at, version`, scheduleID).Scan(&claim.Schedule.UpdatedAt, &claim.Schedule.Version)
	if err != nil {
		return Claim{}, fmt.Errorf("mark schedule processing: %w", err)
	}
	claim.Schedule.Status = domain.ScheduleStatusProcessing
	if err := tx.Commit(); err != nil {
		return Claim{}, fmt.Errorf("commit publishing claim: %w", err)
	}
	return claim, nil
}

func (store *PostgresStore) Complete(ctx context.Context, claim Claim, result PublishResult) (Attempt, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, fmt.Errorf("begin publish completion transaction: %w", err)
	}
	defer tx.Rollback()
	attempt, err := scanAttempt(tx.QueryRowContext(ctx, `
		UPDATE publish_attempts
		SET status = 'SUCCESS', external_post_id = $2, error_message = NULL
		WHERE id = $1 AND status IN ('PROCESSING', 'OUTCOME_UNKNOWN')
		RETURNING id::text, schedule_id::text, variant_id::text, idempotency_key, status::text,
		          external_post_id, error_message, attempted_at`, claim.AttemptID, result.ExternalPostID))
	if err != nil {
		return Attempt{}, fmt.Errorf("complete publish attempt: %w", err)
	}
	if err := updateOne(ctx, tx, `UPDATE schedules SET status = 'COMPLETED', updated_at = NOW(), version = version + 1 WHERE id = $1 AND status = 'PROCESSING'`, claim.Schedule.ID, "complete schedule"); err != nil {
		return Attempt{}, err
	}
	if err := updateOne(ctx, tx, `UPDATE variants SET status = 'PUBLISHED', updated_at = NOW(), version = version + 1 WHERE id = $1 AND status = 'APPROVED'`, claim.Schedule.VariantID, "mark variant published"); err != nil {
		return Attempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attempt{}, fmt.Errorf("commit publish completion: %w", err)
	}
	return attempt, nil
}

func (store *PostgresStore) Fail(ctx context.Context, claim Claim, reason FailureReason) (Attempt, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, fmt.Errorf("begin publish failure transaction: %w", err)
	}
	defer tx.Rollback()
	var attempt Attempt
	if claim.AttemptID == "" {
		attempt, err = insertAttempt(ctx, tx, claim, domain.PublishAttemptStatusFailed, "", string(reason))
	} else {
		attempt, err = scanAttempt(tx.QueryRowContext(ctx, `
			UPDATE publish_attempts
			SET status = 'FAILED', error_message = $2
			WHERE id = $1 AND status IN ('PROCESSING', 'OUTCOME_UNKNOWN')
			RETURNING id::text, schedule_id::text, variant_id::text, idempotency_key, status::text,
			          external_post_id, error_message, attempted_at`, claim.AttemptID, string(reason)))
	}
	if err != nil {
		return Attempt{}, fmt.Errorf("record publish failure: %w", err)
	}
	if err := updateOne(ctx, tx, `UPDATE schedules SET status = 'FAILED', updated_at = NOW(), version = version + 1 WHERE id = $1 AND status = 'PROCESSING'`, claim.Schedule.ID, "fail schedule"); err != nil {
		return Attempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attempt{}, fmt.Errorf("commit publish failure: %w", err)
	}
	return attempt, nil
}

func scanSchedule(row *sql.Row) (Schedule, error) {
	var value Schedule
	var status string
	err := row.Scan(&value.ID, &value.VariantID, &value.ScheduledAt, &status, &value.CreatedAt, &value.UpdatedAt, &value.Version)
	value.Status = domain.ScheduleStatus(status)
	return value, err
}

type rowScanner interface{ Scan(...any) error }

func scanAttempt(row rowScanner) (Attempt, error) {
	var value Attempt
	var status string
	var externalPostID, message sql.NullString
	err := row.Scan(&value.ID, &value.ScheduleID, &value.VariantID, &value.IdempotencyKey, &status, &externalPostID, &message, &value.AttemptedAt)
	if externalPostID.Valid {
		value.ExternalPostID = externalPostID.String
	}
	if message.Valid {
		value.ErrorMessage = message.String
	}
	value.Status = domain.PublishAttemptStatus(status)
	return value, err
}

func insertAttempt(ctx context.Context, tx *sql.Tx, claim Claim, status domain.PublishAttemptStatus, externalPostID, message string) (Attempt, error) {
	attempt, err := scanAttempt(tx.QueryRowContext(ctx, `
		INSERT INTO publish_attempts (schedule_id, variant_id, idempotency_key, status, external_post_id, error_message)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''))
		RETURNING id::text, schedule_id::text, variant_id::text, idempotency_key, status::text,
		          external_post_id, error_message, attempted_at`,
		claim.Schedule.ID, claim.Schedule.VariantID, claim.IdempotencyKey, status, externalPostID, message))
	if err != nil {
		return Attempt{}, fmt.Errorf("insert publish attempt: %w", err)
	}
	return attempt, nil
}

func updateOne(ctx context.Context, tx *sql.Tx, query string, id, operation string) error {
	result, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check %s result: %w", operation, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s: expected one affected row, got %d", operation, rows)
	}
	return nil
}

func idempotencyKey(variantID string, slot time.Time) string {
	return variantID + ":" + slot.UTC().Format(time.RFC3339Nano)
}
