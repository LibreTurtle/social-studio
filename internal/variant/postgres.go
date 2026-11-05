package variant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

func (store *PostgresStore) CreateDrafts(ctx context.Context, postID string, drafts []Draft) ([]Variant, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin variant creation transaction: %w", err)
	}
	defer tx.Rollback()

	created := make([]Variant, 0, len(drafts))
	for _, draft := range drafts {
		value, err := scanVariant(tx.QueryRowContext(ctx, `
			INSERT INTO variants (post_id, platform, content, status)
			VALUES ($1, $2, $3, 'DRAFT')
			RETURNING id::text, post_id::text, platform::text, content, status::text,
			          rejected_at, created_at, updated_at, version`,
			postID, draft.Platform, draft.Content,
		))
		if err != nil {
			return nil, fmt.Errorf("insert draft variant: %w", err)
		}
		created = append(created, value)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit draft variants: %w", err)
	}
	return created, nil
}

func (store *PostgresStore) Get(ctx context.Context, id string) (Variant, error) {
	value, err := scanVariant(store.db.QueryRowContext(ctx, `
		SELECT id::text, post_id::text, platform::text, content, status::text,
		       rejected_at, created_at, updated_at, version
		FROM variants WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Variant{}, ErrNotFound
	}
	if err != nil {
		return Variant{}, fmt.Errorf("query variant: %w", err)
	}
	return value, nil
}

func (store *PostgresStore) ListByPost(ctx context.Context, postID string, status domain.VariantStatus, limit, offset int) ([]Variant, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id::text, post_id::text, platform::text, content, status::text,
		       rejected_at, created_at, updated_at, version
		FROM variants
		WHERE post_id = $1 AND ($2 = '' OR status = $2::variant_status)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4`, postID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query variants by post: %w", err)
	}
	defer rows.Close()

	values := make([]Variant, 0)
	for rows.Next() {
		value, err := scanVariant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate variants: %w", err)
	}
	return values, nil
}

func (store *PostgresStore) UpdateContent(ctx context.Context, id string, expectedVersion int64, content string) (Variant, error) {
	value, err := scanVariant(store.db.QueryRowContext(ctx, `
		UPDATE variants
		SET content = $3, status = 'DRAFT', rejected_at = NULL,
		    updated_at = NOW(), version = version + 1
		WHERE id = $1 AND version = $2 AND status IN ('DRAFT', 'REJECTED')
		RETURNING id::text, post_id::text, platform::text, content, status::text,
		          rejected_at, created_at, updated_at, version`, id, expectedVersion, content))
	if errors.Is(err, sql.ErrNoRows) {
		return Variant{}, store.classifyFailedMutation(ctx, id, expectedVersion)
	}
	if err != nil {
		return Variant{}, fmt.Errorf("update variant: %w", err)
	}
	return value, nil
}

func (store *PostgresStore) Approve(ctx context.Context, id string, expectedVersion int64) (Variant, error) {
	return store.updateStatus(ctx, id, expectedVersion, domain.VariantStatusApproved)
}

func (store *PostgresStore) Reject(ctx context.Context, id string, expectedVersion int64) (Variant, error) {
	return store.updateStatus(ctx, id, expectedVersion, domain.VariantStatusRejected)
}

func (store *PostgresStore) updateStatus(ctx context.Context, id string, expectedVersion int64, target domain.VariantStatus) (Variant, error) {
	var query string
	switch target {
	case domain.VariantStatusApproved:
		query = `
			UPDATE variants
			SET status = 'APPROVED', rejected_at = NULL,
			    updated_at = NOW(), version = version + 1
			WHERE id = $1 AND version = $2 AND status = 'DRAFT'
			RETURNING id::text, post_id::text, platform::text, content, status::text,
			          rejected_at, created_at, updated_at, version`
	case domain.VariantStatusRejected:
		query = `
			UPDATE variants
			SET status = 'REJECTED', rejected_at = NOW(),
			    updated_at = NOW(), version = version + 1
			WHERE id = $1 AND version = $2 AND status = 'DRAFT'
			RETURNING id::text, post_id::text, platform::text, content, status::text,
			          rejected_at, created_at, updated_at, version`
	default:
		return Variant{}, ErrInvalidInput
	}
	value, err := scanVariant(store.db.QueryRowContext(ctx, query, id, expectedVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return Variant{}, store.classifyFailedMutation(ctx, id, expectedVersion)
	}
	if err != nil {
		return Variant{}, fmt.Errorf("update variant review status: %w", err)
	}
	return value, nil
}

func (store *PostgresStore) classifyFailedMutation(ctx context.Context, id string, expectedVersion int64) error {
	current, err := store.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check variant after rejected update: %w", err)
	}
	if current.Version != expectedVersion {
		return ErrVersionConflict
	}
	return invalidTransition(current.Status)
}

type rowScanner interface{ Scan(...any) error }

func scanVariant(row rowScanner) (Variant, error) {
	var value Variant
	var platformName, status string
	var rejectedAt sql.NullTime
	err := row.Scan(
		&value.ID, &value.PostID, &platformName, &value.Content, &status,
		&rejectedAt, &value.CreatedAt, &value.UpdatedAt, &value.Version,
	)
	if err != nil {
		return Variant{}, err
	}
	value.Platform = domain.Platform(platformName)
	value.Status = domain.VariantStatus(status)
	if rejectedAt.Valid {
		value.RejectedAt = &rejectedAt.Time
	}
	return value, nil
}
