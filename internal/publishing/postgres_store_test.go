package publishing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresStoreSchedulesOnlyApprovedVariantsAndReusesSlots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database := newPublishingTestDatabase(t, ctx)
	store := NewPostgresStore(database)

	draftID := insertPublishingTestVariant(t, ctx, database, "DRAFT", "draft content")
	if _, err := store.CreateSchedule(ctx, draftID, time.Now().UTC()); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("CreateSchedule() error = %v, want ErrNotApproved", err)
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM schedules WHERE variant_id = $1`, draftID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("created schedules for an unapproved variant = %d, want 0", count)
	}

	approvedID := insertPublishingTestVariant(t, ctx, database, "APPROVED", "approved content")
	slot := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	first, err := store.CreateSchedule(ctx, approvedID, slot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSchedule(ctx, approvedID, slot)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Status != domain.ScheduleStatusPending {
		t.Fatalf("repeated schedule creation returned IDs/status %q/%q %q, want same pending schedule", first.ID, second.ID, first.Status)
	}
	if _, err := store.CreateSchedule(ctx, approvedID, slot.Add(time.Hour)); !errors.Is(err, ErrAlreadyScheduled) {
		t.Fatalf("second slot for the same variant error = %v, want ErrAlreadyScheduled", err)
	}
}

func TestPostgresStoreClaimsConcurrentPublishOnceAndPersistsSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database := newPublishingTestDatabase(t, ctx)
	store := NewPostgresStore(database)
	variantID := insertPublishingTestVariant(t, ctx, database, "APPROVED", "approved content")
	schedule, err := store.CreateSchedule(ctx, variantID, time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	const callers = 12
	start := make(chan struct{})
	claims := make(chan Claim, callers)
	errorsFound := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			<-start
			claim, claimErr := store.Claim(ctx, schedule.ID)
			if claimErr != nil {
				errorsFound <- claimErr
				return
			}
			claims <- claim
		}()
	}
	close(start)
	workers.Wait()
	close(claims)
	close(errorsFound)

	var claim Claim
	claimCount := 0
	for candidate := range claims {
		claim = candidate
		claimCount++
	}
	if claimCount != 1 {
		t.Fatalf("successful claims = %d, want exactly 1", claimCount)
	}
	for claimErr := range errorsFound {
		if !errors.Is(claimErr, ErrInProgress) {
			t.Fatalf("competing claim error = %v, want ErrInProgress", claimErr)
		}
	}

	attempt, err := store.Complete(ctx, claim, PublishResult{ExternalPostID: "remote-post-1"})
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status != domain.PublishAttemptStatusSuccess || attempt.ExternalPostID != "remote-post-1" {
		t.Fatalf("completed attempt = %+v", attempt)
	}
	prior, err := store.Claim(ctx, schedule.ID)
	if err != nil || prior.ExistingAttempt == nil || prior.ExistingAttempt.ID != attempt.ID {
		t.Fatalf("repeated claim = %+v, %v; want existing attempt %q", prior, err, attempt.ID)
	}
	var status string
	if err := database.QueryRowContext(ctx, `SELECT status::text FROM variants WHERE id = $1`, variantID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.VariantStatusPublished) {
		t.Fatalf("variant status = %q, want PUBLISHED", status)
	}
	repeatedSchedule, err := store.CreateSchedule(ctx, variantID, schedule.ScheduledAt)
	if err != nil || repeatedSchedule.ID != schedule.ID || repeatedSchedule.Status != domain.ScheduleStatusCompleted {
		t.Fatalf("repeated completed schedule = %+v, %v; want existing schedule %q", repeatedSchedule, err, schedule.ID)
	}
}

func newPublishingTestDatabase(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL persistence tests against a disposable PostgreSQL 18 database")
	}
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	adminDatabase := stdlib.OpenDB(*adminConfig)
	adminDatabase.SetMaxOpenConns(2)
	if err := adminDatabase.PingContext(ctx); err != nil {
		adminDatabase.Close()
		t.Fatalf("connect TEST_DATABASE_URL: %v", err)
	}

	generatedID := uuid.NewV4().String()
	schema := "publishing_test_" + strings.ReplaceAll(generatedID, "-", "")
	if _, err := adminDatabase.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminDatabase.Close()
		t.Fatalf("create isolated publishing test schema: %v", err)
	}

	applicationConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		adminDatabase.Close()
		t.Fatalf("parse application test connection: %v", err)
	}
	if applicationConfig.RuntimeParams == nil {
		applicationConfig.RuntimeParams = make(map[string]string)
	}
	applicationConfig.RuntimeParams["search_path"] = schema + ",public"
	database := stdlib.OpenDB(*applicationConfig)
	database.SetMaxOpenConns(16)
	database.SetMaxIdleConns(16)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		_, _ = adminDatabase.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
		adminDatabase.Close()
		t.Fatalf("connect to isolated schema: %v", err)
	}

	statements := []string{
		`CREATE TYPE social_platform AS ENUM ('X', 'LINKEDIN', 'TELEGRAM')`,
		`CREATE TYPE variant_status AS ENUM ('DRAFT', 'APPROVED', 'REJECTED', 'PUBLISHED')`,
		`CREATE TYPE schedule_status AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED')`,
		`CREATE TYPE publish_attempt_status AS ENUM ('SUCCESS', 'FAILED')`,
		`CREATE TABLE variants (id uuid PRIMARY KEY, platform social_platform NOT NULL, content text NOT NULL, status variant_status NOT NULL, updated_at timestamptz NOT NULL, version bigint NOT NULL)`,
		`CREATE TABLE schedules (id uuid PRIMARY KEY DEFAULT uuidv7(), variant_id uuid NOT NULL REFERENCES variants(id), scheduled_at timestamptz NOT NULL, status schedule_status NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), version bigint NOT NULL DEFAULT 1, UNIQUE (variant_id, scheduled_at))`,
		`CREATE TABLE publish_attempts (id uuid PRIMARY KEY DEFAULT uuidv7(), schedule_id uuid NOT NULL REFERENCES schedules(id), variant_id uuid NOT NULL REFERENCES variants(id), idempotency_key text NOT NULL UNIQUE, status publish_attempt_status NOT NULL, external_post_id text, error_message text, attempted_at timestamptz NOT NULL DEFAULT now())`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			database.Close()
			_, _ = adminDatabase.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
			adminDatabase.Close()
			t.Fatalf("create isolated persistence schema: %v", err)
		}
	}
	t.Cleanup(func() {
		_ = database.Close()
		cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := adminDatabase.ExecContext(cleanupContext, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop isolated persistence schema: %v", err)
		}
		if err := adminDatabase.Close(); err != nil {
			t.Errorf("close test database connection: %v", err)
		}
	})
	return database
}

func insertPublishingTestVariant(t *testing.T, ctx context.Context, database *sql.DB, status, content string) string {
	t.Helper()
	id := uuid.NewV4().String()
	if _, err := database.ExecContext(ctx, `
		INSERT INTO variants (id, platform, content, status, updated_at, version)
		VALUES ($1, 'X', $2, $3, now(), 1)`, id, content, status); err != nil {
		t.Fatalf("insert test variant: %v", err)
	}
	return id
}
