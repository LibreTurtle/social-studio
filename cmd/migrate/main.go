package main

import (
	"errors"
	"fmt"
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
	os.Exit(run(logger, os.Args[1:], os.Getenv))
}

func run(logger *slog.Logger, arguments []string, getenv func(string) string) int {
	if len(arguments) != 1 || arguments[0] != "up" && arguments[0] != "down" {
		logger.Error("migration command required", "usage", "migrate <up | down>")
		return 2
	}
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		return 2
	}

	databaseURLForMigrations, err := pgxMigrationURL(databaseURL)
	if err != nil {
		logger.Error("DATABASE_URL must be a postgres or postgresql URL")
		return 2
	}
	instance, err := migrate.New("file://migrations", databaseURLForMigrations)
	if err != nil {
		logger.Error("migration initialization failed", "error_type", fmt.Sprintf("%T", err))
		return 1
	}
	defer func() {
		sourceErr, databaseErr := instance.Close()
		if sourceErr != nil || databaseErr != nil {
			logger.Warn("migration resources did not close cleanly", "source_error_type", errorType(sourceErr), "database_error_type", errorType(databaseErr))
		}
	}()

	var runErr error
	if arguments[0] == "up" {
		runErr = instance.Up()
	} else {
		runErr = instance.Steps(-1)
	}
	if errors.Is(runErr, migrate.ErrNoChange) {
		logger.Info("database schema is already at the requested version", "command", arguments[0])
		return 0
	}
	if runErr != nil {
		logger.Error("migration failed", "command", arguments[0], "error_type", fmt.Sprintf("%T", runErr))
		return 1
	}
	logger.Info("migration completed", "command", arguments[0])
	return 0
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
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
