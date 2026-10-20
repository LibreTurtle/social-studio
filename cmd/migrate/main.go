package main

import (
	"errors"
	"log/slog"
	"net/url"
	"os"

	"github.com/LibreTurtle/social-studio/internal/logging"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)
	if len(os.Args) != 2 {
		logger.Error("migration command required", "usage", "migrate <up | down>")
		os.Exit(2)
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(2)
	}

	databaseURLForMigrations, err := pgxMigrationURL(databaseURL)
	if err != nil {
		logger.Error("DATABASE_URL must be a postgres or postgresql URL")
		os.Exit(2)
	}
	instance, err := migrate.New("file://migrations", databaseURLForMigrations)
	if err != nil {
		logger.Error("migration initialization failed", "error", err)
		os.Exit(1)
	}
	defer instance.Close()

	var runErr error
	switch os.Args[1] {
	case "up":
		runErr = instance.Up()
	case "down":
		runErr = instance.Steps(-1)
	default:
		logger.Error("unknown migration command", "command", os.Args[1], "usage", "migrate <up | down>")
		os.Exit(2)
	}
	if errors.Is(runErr, migrate.ErrNoChange) {
		logger.Info("database schema is already at the requested version", "command", os.Args[1])
		return
	}
	if runErr != nil {
		logger.Error("migration failed", "command", os.Args[1], "error", runErr)
		os.Exit(1)
	}
	logger.Info("migration completed", "command", os.Args[1])
}

func pgxMigrationURL(databaseURL string) (string, error) {
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	if parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql" {
		return "", errors.New("unsupported database URL scheme")
	}
	parsedURL.Scheme = "pgx5"
	return parsedURL.String(), nil
}
