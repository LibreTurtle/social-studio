package publishing

import (
	"context"
	"fmt"
)

func (store *PostgresStore) ListAttempts(ctx context.Context, filter HistoryFilter) ([]Attempt, error) {
	var variantID, scheduleID, beforeID any
	var beforeAt any
	if filter.VariantID != "" {
		variantID = filter.VariantID
	}
	if filter.ScheduleID != "" {
		scheduleID = filter.ScheduleID
	}
	if filter.Before != nil {
		beforeAt, beforeID = filter.Before.AttemptedAt, filter.Before.ID
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT id::text, schedule_id::text, variant_id::text, idempotency_key, status::text,
		       external_post_id, error_message, attempted_at
		FROM publish_attempts
		WHERE ($1::uuid IS NULL OR variant_id = $1::uuid)
		  AND ($2::uuid IS NULL OR schedule_id = $2::uuid)
		  AND ($3::timestamptz IS NULL OR (attempted_at, id) < ($3::timestamptz, $4::uuid))
		ORDER BY attempted_at DESC, id DESC
		LIMIT $5`, variantID, scheduleID, beforeAt, beforeID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("query publish history: %w", err)
	}
	defer rows.Close()
	attempts := make([]Attempt, 0, filter.Limit)
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("scan publish history: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publish history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close publish history rows: %w", err)
	}
	return attempts, nil
}
