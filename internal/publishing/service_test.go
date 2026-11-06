package publishing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/middleware"
)

type fakeStore struct {
	mu              sync.Mutex
	claim           Claim
	claimErr        error
	complete        int
	fail            int
	started         int
	unknown         int
	failReason      FailureReason
	platform        domain.Platform
	status          domain.VariantStatus
	creates         int
	existing        *Schedule
	historyAttempts []Attempt
	historyFilter   HistoryFilter
	historyErr      error
}

func (store *fakeStore) CreateSchedule(context.Context, string, time.Time) (Schedule, error) {
	store.creates++
	return Schedule{ID: testID(10), VariantID: testID(1), ScheduledAt: time.Now().UTC(), Status: domain.ScheduleStatusPending}, nil
}
func (store *fakeStore) GetForSchedule(context.Context, string, time.Time) (domain.Platform, domain.VariantStatus, *Schedule, error) {
	return store.platform, store.status, store.existing, nil
}
func (store *fakeStore) Claim(context.Context, string) (Claim, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.claim, store.claimErr
}
func (store *fakeStore) ClaimDue(context.Context, int) ([]Claim, error) { return nil, nil }
func (store *fakeStore) RecoverStale(context.Context, time.Duration) (Recovery, error) {
	return Recovery{}, nil
}
func (store *fakeStore) StartDispatch(_ context.Context, claim Claim) (Attempt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.started++
	return Attempt{ID: testID(12), ScheduleID: claim.Schedule.ID, VariantID: claim.Schedule.VariantID, IdempotencyKey: claim.IdempotencyKey, Status: domain.PublishAttemptStatusProcessing, AttemptedAt: time.Now().UTC()}, nil
}
func (store *fakeStore) MarkOutcomeUnknown(context.Context, Claim) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.unknown++
	return nil
}
func (store *fakeStore) Complete(context.Context, Claim, PublishResult) (Attempt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.complete++
	return Attempt{ScheduleID: testID(10), Status: domain.PublishAttemptStatusSuccess, ExternalPostID: "external-1"}, nil
}
func (store *fakeStore) Fail(_ context.Context, _ Claim, reason FailureReason) (Attempt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.fail++
	store.failReason = reason
	return Attempt{ScheduleID: testID(10), Status: domain.PublishAttemptStatusFailed, ErrorMessage: string(reason)}, nil
}
func (store *fakeStore) ListAttempts(_ context.Context, filter HistoryFilter) ([]Attempt, error) {
	store.historyFilter = filter
	return store.historyAttempts, store.historyErr
}

type fakePublisher struct {
	mu     sync.Mutex
	calls  int
	result PublishResult
	err    error
}

func (publisher *fakePublisher) Publish(context.Context, PublishCommand) (PublishResult, error) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.calls++
	return publisher.result, publisher.err
}

func TestPublishUsesStoredClaimAndPersistsSuccess(t *testing.T) {
	store := &fakeStore{claim: Claim{Schedule: Schedule{ID: testID(10), VariantID: testID(1), Status: domain.ScheduleStatusProcessing}, Platform: domain.PlatformX, Content: "approved content", IdempotencyKey: "variant-slot"}}
	provider := &fakePublisher{result: PublishResult{ExternalPostID: "external-1"}}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	result, err := service.Publish(context.Background(), testID(10))
	if err != nil || result.ExternalPostID != "external-1" || store.complete != 1 || provider.calls != 1 {
		t.Fatalf("Publish() = %+v, %v; complete=%d provider calls=%d", result, err, store.complete, provider.calls)
	}
}

func TestPublishLeavesAmbiguousProviderOutcomeClaimed(t *testing.T) {
	store := &fakeStore{claim: Claim{Schedule: Schedule{ID: testID(10), VariantID: testID(1), Status: domain.ScheduleStatusProcessing}, Platform: domain.PlatformX, Content: "content"}}
	provider := &fakePublisher{err: ErrUncertain}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	_, err := service.Publish(context.Background(), testID(10))
	if !errors.Is(err, ErrUncertain) || store.complete != 0 || store.fail != 0 || store.unknown != 1 {
		t.Fatalf("Publish() error=%v, complete=%d fail=%d unknown=%d", err, store.complete, store.fail, store.unknown)
	}
}

func TestProviderFailureLogsOnlySanitizedCategory(t *testing.T) {
	var logs bytes.Buffer
	store := &fakeStore{claim: Claim{Schedule: Schedule{ID: testID(10), VariantID: testID(1)}, Platform: domain.PlatformX, Content: "content"}}
	provider := &fakePublisher{err: errors.New("upstream body includes private token")}
	service := NewService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider}, slog.New(slog.NewJSONHandler(&logs, nil)))
	var publishErr error
	handler := middleware.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		_, publishErr = service.Publish(request.Context(), testID(10))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/publish", nil))
	if !errors.Is(publishErr, ErrUncertain) {
		t.Fatalf("Publish() error = %v, want uncertain outcome", publishErr)
	}
	if !strings.Contains(logs.String(), `"error_category":"provider_outcome_unknown"`) || !strings.Contains(logs.String(), `"request_id":"`+response.Header().Get("X-Request-ID")+`"`) || strings.Contains(logs.String(), "private token") || strings.Contains(logs.String(), "upstream body") {
		t.Fatalf("provider log was missing category or exposed raw error: %s", logs.String())
	}
}

func TestPublishPersistsDefinitiveProviderFailure(t *testing.T) {
	store := &fakeStore{claim: Claim{Schedule: Schedule{ID: testID(10), VariantID: testID(1), Status: domain.ScheduleStatusProcessing}, Platform: domain.PlatformX, Content: "content"}}
	provider := &fakePublisher{err: ErrRejected}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	result, err := service.Publish(context.Background(), testID(10))
	if err != nil || result.Status != domain.PublishAttemptStatusFailed || store.fail != 1 {
		t.Fatalf("Publish() = %+v, %v; fail=%d", result, err, store.fail)
	}
}

func TestPublishDoesNotCallProviderWhenScheduleClaimFails(t *testing.T) {
	store := &fakeStore{claimErr: ErrInProgress}
	provider := &fakePublisher{}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	_, err := service.Publish(context.Background(), testID(10))
	if !errors.Is(err, ErrInProgress) || provider.calls != 0 {
		t.Fatalf("Publish() error=%v, provider calls=%d", err, provider.calls)
	}
}

func TestRepeatedCompletedPublishReturnsStoredAttempt(t *testing.T) {
	prior := &Attempt{ID: testID(11), ScheduleID: testID(10), VariantID: testID(1), IdempotencyKey: "same-slot", Status: domain.PublishAttemptStatusSuccess, ExternalPostID: "remote-existing"}
	store := &fakeStore{claim: Claim{ExistingAttempt: prior}}
	provider := &fakePublisher{}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	result, err := service.Publish(context.Background(), testID(10))
	if err != nil || result.ExternalPostID != "remote-existing" || provider.calls != 0 {
		t.Fatalf("repeat Publish() = %+v, %v; provider calls=%d", result, err, provider.calls)
	}
}

func TestPublishRejectsStoredContentThatViolatesPlatformProfile(t *testing.T) {
	store := &fakeStore{claim: Claim{Schedule: Schedule{ID: testID(10), VariantID: testID(1), Status: domain.ScheduleStatusProcessing}, Platform: domain.PlatformX, Content: strings.Repeat("x", 281)}}
	provider := &fakePublisher{}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	result, err := service.Publish(context.Background(), testID(10))
	if err != nil || result.ErrorMessage != string(FailureContentInvalid) || provider.calls != 0 || store.fail != 1 || store.failReason != FailureContentInvalid {
		t.Fatalf("Publish() = %+v, %v; calls=%d failures=%d", result, err, provider.calls, store.fail)
	}
}

func TestScheduleRequiresValidVariantID(t *testing.T) {
	service := newTestService(&fakeStore{}, nil)
	if _, err := service.CreateSchedule(context.Background(), "not-a-uuid", time.Now()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateSchedule() error = %v, want invalid input", err)
	}
}

func TestScheduleRequiresConfiguredPlatformPublisher(t *testing.T) {
	store := &fakeStore{platform: domain.PlatformX, status: domain.VariantStatusApproved}
	service := newTestService(store, nil)
	_, err := service.CreateSchedule(context.Background(), testID(1), time.Now().UTC())
	if !errors.Is(err, ErrPublisherUnavailable) || store.creates != 0 {
		t.Fatalf("CreateSchedule() error=%v store creates=%d; want unavailable and no schedule", err, store.creates)
	}
}

func TestSchedulePersistsForApprovedConfiguredPlatform(t *testing.T) {
	store := &fakeStore{platform: domain.PlatformX, status: domain.VariantStatusApproved}
	provider := &fakePublisher{}
	service := newTestService(store, map[domain.Platform]SocialPublisher{domain.PlatformX: provider})
	schedule, err := service.CreateSchedule(context.Background(), testID(1), time.Now().UTC())
	if err != nil || schedule.Status != domain.ScheduleStatusPending || store.creates != 1 {
		t.Fatalf("CreateSchedule() = %+v, %v; store creates=%d", schedule, err, store.creates)
	}
}

func TestRepeatedScheduleRequestReturnsExistingSlotWithoutPublisher(t *testing.T) {
	existing := &Schedule{ID: testID(10), VariantID: testID(1), Status: domain.ScheduleStatusCompleted}
	store := &fakeStore{platform: domain.PlatformX, status: domain.VariantStatusPublished, existing: existing}
	service := newTestService(store, nil)
	schedule, err := service.CreateSchedule(context.Background(), testID(1), time.Now().UTC())
	if err != nil || schedule.ID != existing.ID || store.creates != 0 {
		t.Fatalf("CreateSchedule() = %+v, %v; store creates=%d", schedule, err, store.creates)
	}
}

func testID(last byte) string {
	value := uuid.MustParse("019a0000-0000-7000-8000-000000000000")
	value[15] = last
	return value.String()
}

func newTestService(store Store, publishers map[domain.Platform]SocialPublisher) *Service {
	return NewService(store, publishers, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}
