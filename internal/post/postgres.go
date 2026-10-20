package post

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

func (store *PostgresStore) Create(ctx context.Context, value Post) (Post, error) {
	err := store.db.QueryRowContext(ctx, `INSERT INTO posts (source_type, source_url, content) VALUES ($1, NULLIF($2, ''), $3) RETURNING id::text, source_type::text, COALESCE(source_url, ''), content, created_at, updated_at, version`, value.SourceType, value.SourceURL, value.Content).Scan(&value.ID, &value.SourceType, &value.SourceURL, &value.Content, &value.CreatedAt, &value.UpdatedAt, &value.Version)
	if err != nil {
		return Post{}, fmt.Errorf("create post: %w", err)
	}
	return value, nil
}

func (store *PostgresStore) Get(ctx context.Context, id string) (Post, error) {
	var value Post
	err := store.db.QueryRowContext(ctx, `SELECT id::text, source_type::text, COALESCE(source_url, ''), content, created_at, updated_at, version FROM posts WHERE id = $1`, strings.TrimSpace(id)).Scan(&value.ID, &value.SourceType, &value.SourceURL, &value.Content, &value.CreatedAt, &value.UpdatedAt, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return Post{}, fmt.Errorf("get post: %w", err)
	}
	return value, nil
}
