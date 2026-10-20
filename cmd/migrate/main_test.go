package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestMigrationCommandRequiresExactlyOneArgument(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, arguments := range [][]string{nil, {"up", "down"}, {"invalid"}} {
		if code := run(logger, arguments, func(string) string { return "" }); code != 2 {
			t.Errorf("run(%v) exit code = %d, want 2", arguments, code)
		}
	}
}

func TestMigrationCommandRequiresDatabaseURL(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if code := run(logger, []string{"up"}, func(string) string { return "" }); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
}

func TestPGXMigrationURLValidatesAndRewritesScheme(t *testing.T) {
	got, err := pgxMigrationURL("postgresql://user:password@localhost:5432/social_studio?sslmode=disable")
	if err != nil || got != "pgx5://user:password@localhost:5432/social_studio?sslmode=disable" {
		t.Fatalf("pgxMigrationURL() = (%q, %v)", got, err)
	}
	if _, err := pgxMigrationURL("https://example.com/db"); err == nil {
		t.Fatal("pgxMigrationURL() accepted a non-PostgreSQL URL")
	}
}
