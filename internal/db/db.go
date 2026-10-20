package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const pingTimeout = 5 * time.Second

func Connect(databaseURL string) (*sql.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	connection, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	connection.SetMaxOpenConns(25)
	connection.SetMaxIdleConns(25)
	connection.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := connection.PingContext(ctx); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("db.Ping: %w", err)
	}

	return connection, nil
}
