package publishing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

func (store *PostgresStore) ClaimDue(ctx context.Context, limit int) ([]Claim, error) {
	if limit < 1 || limit > 128 {
		return nil, ErrInvalidInput
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin due schedule claim: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT s.id::text, s.variant_id::text, s.scheduled_at, s.created_at, s.updated_at,
		       s.version, v.platform::text, v.content
		FROM schedules s
		JOIN variants v ON v.id = s.variant_id
		WHERE s.status = 'PENDING' AND s.scheduled_at <= NOW() AND v.status = 'APPROVED'
		ORDER BY s.scheduled_at, s.id
		LIMIT $1
		FOR UPDATE OF s SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("select due schedules: %w", err)
	}
	claims := make([]Claim, 0, limit)
	for rows.Next() {
		var claim Claim
		if err := rows.Scan(
			&claim.Schedule.ID, &claim.Schedule.VariantID, &claim.Schedule.ScheduledAt,
			&claim.Schedule.CreatedAt, &claim.Schedule.UpdatedAt, &claim.Schedule.Version,
			&claim.Platform, &claim.Content,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan due schedule: %w", err)
		}
		claim.Schedule.Status = domain.ScheduleStatusPending
		claim.IdempotencyKey = idempotencyKey(claim.Schedule.VariantID, claim.Schedule.ScheduledAt)
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate due schedules: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close due schedule rows: %w", err)
	}
	for index := range claims {
		claim := &claims[index]
		err := tx.QueryRowContext(ctx, `
			UPDATE schedules
			SET status = 'PROCESSING', processing_started_at = NOW(), dispatch_started_at = NULL,
			    updated_at = NOW(), version = version + 1
			WHERE id = $1 AND status = 'PENDING'
			RETURNING updated_at, version`, claim.Schedule.ID).Scan(&claim.Schedule.UpdatedAt, &claim.Schedule.Version)
		if err != nil {
			return nil, fmt.Errorf("mark due schedule processing: %w", err)
		}
		claim.Schedule.Status = domain.ScheduleStatusProcessing
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit due schedule claims: %w", err)
	}
	return claims, nil
}

func (store *PostgresStore) RecoverStale(ctx context.Context, staleAfter time.Duration) (Recovery, error) {
	if staleAfter <= 0 {
		return Recovery{}, ErrInvalidInput
	}
	staleMicros := staleAfter.Microseconds()
	if staleMicros < 1 {
		return Recovery{}, ErrInvalidInput
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Recovery{}, fmt.Errorf("begin stale claim recovery: %w", err)
	}
	defer tx.Rollback()
	var recovered Recovery
	result, err := tx.ExecContext(ctx, `
		UPDATE schedules
		SET status = 'PENDING', processing_started_at = NULL, updated_at = NOW(), version = version + 1
		WHERE status = 'PROCESSING' AND dispatch_started_at IS NULL
		  AND processing_started_at < NOW() - ($1::double precision * INTERVAL '1 microsecond')`, staleMicros)
	if err != nil {
		return Recovery{}, fmt.Errorf("requeue undispatched claims: %w", err)
	}
	recovered.Requeued, err = result.RowsAffected()
	if err != nil {
		return Recovery{}, fmt.Errorf("count requeued claims: %w", err)
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE publish_attempts a
		SET status = 'OUTCOME_UNKNOWN', error_message = 'provider outcome unknown after worker interruption'
		FROM schedules s
		WHERE a.schedule_id = s.id AND a.status = 'PROCESSING'
		  AND s.status = 'PROCESSING'
		  AND s.dispatch_started_at < NOW() - ($1::double precision * INTERVAL '1 microsecond')`, staleMicros)
	if err != nil {
		return Recovery{}, fmt.Errorf("mark interrupted provider outcomes unknown: %w", err)
	}
	recovered.Unknown, err = result.RowsAffected()
	if err != nil {
		return Recovery{}, fmt.Errorf("count unknown provider outcomes: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Recovery{}, fmt.Errorf("commit stale claim recovery: %w", err)
	}
	return recovered, nil
}

func (store *PostgresStore) StartDispatch(ctx context.Context, claim Claim) (Attempt, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, fmt.Errorf("begin provider dispatch transaction: %w", err)
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		UPDATE schedules
		SET dispatch_started_at = NOW(), updated_at = NOW(), version = version + 1
		WHERE id = $1 AND status = 'PROCESSING' AND version = $2 AND dispatch_started_at IS NULL
		RETURNING id`, claim.Schedule.ID, claim.Schedule.Version).Scan(new(string))
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, ErrClaimLost
	}
	if err != nil {
		return Attempt{}, fmt.Errorf("mark provider dispatch started: %w", err)
	}
	attempt, err := scanAttempt(tx.QueryRowContext(ctx, `
		INSERT INTO publish_attempts (schedule_id, variant_id, idempotency_key, status)
		VALUES ($1, $2, $3, 'PROCESSING')
		RETURNING id::text, schedule_id::text, variant_id::text, idempotency_key, status::text,
		          external_post_id, error_message, attempted_at`,
		claim.Schedule.ID, claim.Schedule.VariantID, claim.IdempotencyKey))
	if err != nil {
		return Attempt{}, fmt.Errorf("record provider dispatch attempt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Attempt{}, fmt.Errorf("commit provider dispatch transaction: %w", err)
	}
	return attempt, nil
}

func (store *PostgresStore) MarkOutcomeUnknown(ctx context.Context, claim Claim) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE publish_attempts
		SET status = 'OUTCOME_UNKNOWN', error_message = 'provider outcome unknown; automatic retry disabled'
		WHERE id = $1 AND status = 'PROCESSING'`, claim.AttemptID)
	if err != nil {
		return fmt.Errorf("mark provider attempt outcome unknown: %w", err)
	}
	return nil
}
