package httpapi

import (
	"context"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/publishing"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type publishingServiceFake struct {
	schedule       publishing.Schedule
	attempt        publishing.Attempt
	historyRequest publishing.HistoryRequest
	history        publishing.HistoryPage
	err            error
	created        bool
	called         bool
}

func (fake *publishingServiceFake) CreateSchedule(_ context.Context, variantID string, scheduledAt time.Time) (publishing.Schedule, error) {
	fake.created = true
	fake.schedule = publishing.Schedule{ID: "019a0000-0000-7000-8000-000000000010", VariantID: variantID, ScheduledAt: scheduledAt, Status: domain.ScheduleStatusPending}
	return fake.schedule, fake.err
}

func (fake *publishingServiceFake) Publish(_ context.Context, scheduleID string) (publishing.Attempt, error) {
	fake.called = true
	fake.attempt.ScheduleID = scheduleID
	return fake.attempt, fake.err
}

func (fake *publishingServiceFake) ListHistory(_ context.Context, request publishing.HistoryRequest) (publishing.HistoryPage, error) {
	fake.historyRequest = request
	return fake.history, fake.err
}

func TestScheduleAndPublishEndpoints(t *testing.T) {
	fake := &publishingServiceFake{attempt: publishing.Attempt{Status: domain.PublishAttemptStatusSuccess, ExternalPostID: "remote-1", IdempotencyKey: "internal-key"}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	scheduledAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	create := httptest.NewRecorder()
	server.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"variant_id":"019a0000-0000-7000-8000-000000000001","scheduled_at":"`+scheduledAt+`"}`)))
	if create.Code != http.StatusCreated || !fake.created {
		t.Fatalf("create schedule status=%d called=%t body=%s", create.Code, fake.created, create.Body.String())
	}
	publish := httptest.NewRecorder()
	server.ServeHTTP(publish, httptest.NewRequest(http.MethodPost, "/api/v1/schedules/019a0000-0000-7000-8000-000000000010/publish", nil))
	if publish.Code != http.StatusOK || !fake.called || !strings.Contains(publish.Body.String(), "remote-1") || strings.Contains(publish.Body.String(), "internal-key") || strings.Contains(publish.Body.String(), "idempotency_key") {
		t.Fatalf("publish status=%d called=%t body=%s", publish.Code, fake.called, publish.Body.String())
	}
}

func TestScheduleEndpointRefusesUnapprovedVariant(t *testing.T) {
	fake := &publishingServiceFake{err: publishing.ErrNotApproved}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	response := httptest.NewRecorder()
	body := `{"variant_id":"019a0000-0000-7000-8000-000000000001","scheduled_at":"2026-11-05T10:00:00Z"}`
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(body)))
	if response.Code != http.StatusConflict || !fake.created {
		t.Fatalf("status=%d service-called=%t body=%s", response.Code, fake.created, response.Body.String())
	}
}

func TestScheduleEndpointReturnsUnavailableWithoutPlatformPublisher(t *testing.T) {
	fake := &publishingServiceFake{err: publishing.ErrPublisherUnavailable}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	response := httptest.NewRecorder()
	body := `{"variant_id":"019a0000-0000-7000-8000-000000000001","scheduled_at":"2026-11-05T10:00:00Z"}`
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(body)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestScheduleEndpointRejectsAnotherSlotForSameVariant(t *testing.T) {
	fake := &publishingServiceFake{err: publishing.ErrAlreadyScheduled}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	response := httptest.NewRecorder()
	body := `{"variant_id":"019a0000-0000-7000-8000-000000000001","scheduled_at":"2026-11-05T10:00:00Z"}`
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(body)))
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPublishEndpointReturnsAcceptedForUnknownProviderOutcome(t *testing.T) {
	fake := &publishingServiceFake{err: publishing.ErrUncertain}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules/019a0000-0000-7000-8000-000000000010/publish", nil))
	if response.Code != http.StatusAccepted || strings.Contains(response.Body.String(), "provider body") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestScheduleEndpointRejectsInvalidSlot(t *testing.T) {
	fake := &publishingServiceFake{}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"variant_id":"019a0000-0000-7000-8000-000000000001","scheduled_at":"tomorrow"}`)))
	if response.Code != http.StatusBadRequest || fake.created {
		t.Fatalf("status=%d called=%t body=%s", response.Code, fake.created, response.Body.String())
	}
}

func TestPublishHistoryEndpointPassesBoundedFiltersAndHidesIdempotencyKey(t *testing.T) {
	fake := &publishingServiceFake{
		history: publishing.HistoryPage{
			Attempts:   []publishing.Attempt{{ID: "019a0000-0000-7000-8000-000000000011", ScheduleID: "019a0000-0000-7000-8000-000000000010", VariantID: "019a0000-0000-7000-8000-000000000001", IdempotencyKey: "private-key", Status: domain.PublishAttemptStatusSuccess, ExternalPostID: "remote-1"}},
			NextCursor: "opaque-cursor",
		},
	}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/publish-history?variant_id=019a0000-0000-7000-8000-000000000001&limit=25&cursor=page-token", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.historyRequest.VariantID != "019a0000-0000-7000-8000-000000000001" || fake.historyRequest.Limit != 25 || fake.historyRequest.Cursor != "page-token" {
		t.Fatalf("status=%d request=%+v body=%s", response.Code, fake.historyRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "remote-1") || !strings.Contains(response.Body.String(), "opaque-cursor") || strings.Contains(response.Body.String(), "private-key") || strings.Contains(response.Body.String(), "idempotency_key") {
		t.Fatalf("history response leaked internal data or omitted page data: %s", response.Body.String())
	}
}

func TestPublishHistoryEndpointRejectsInvalidFilters(t *testing.T) {
	fake := &publishingServiceFake{}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Publishing: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/publish-history?variant_id=not-a-uuid&limit=many", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if fake.historyRequest.VariantID != "" {
		t.Fatal("publishing service called for invalid history filters")
	}
}
