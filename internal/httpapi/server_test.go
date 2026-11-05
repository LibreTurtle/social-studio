package httpapi

import (
	"context"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
	"github.com/LibreTurtle/social-studio/internal/post"
	"github.com/LibreTurtle/social-studio/internal/publishing"
	"github.com/LibreTurtle/social-studio/internal/variant"
)

type postServiceFake struct {
	request post.CreateRequest
	result  post.Post
	err     error
}

func testServer() http.Handler {
	return NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func TestHealthEndpoint(t *testing.T) {
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != true {
		t.Fatalf("unexpected response: %v", body)
	}
}

func TestPlatformProfilesEndpoint(t *testing.T) {
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/platforms", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Success || len(body.Data) != 3 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestUnknownEndpointReturnsStructuredNotFound(t *testing.T) {
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

type generationServiceFake struct {
	request   generation.Request
	result    generation.Result
	candidate generation.Candidate
	err       error
}

type variantServiceFake struct {
	value     variant.Variant
	listed    []variant.Variant
	status    domain.VariantStatus
	limit     int
	offset    int
	version   int64
	content   string
	operation string
	err       error
}

func (fake *variantServiceFake) Get(_ context.Context, _ string) (variant.Variant, error) {
	return fake.value, fake.err
}
func (fake *variantServiceFake) ListByPost(_ context.Context, _ string, status domain.VariantStatus, limit, offset int) ([]variant.Variant, error) {
	fake.status, fake.limit, fake.offset = status, limit, offset
	return fake.listed, fake.err
}
func (fake *variantServiceFake) UpdateContent(_ context.Context, _ string, version int64, content string) (variant.Variant, error) {
	fake.operation, fake.version, fake.content = "edit", version, content
	return fake.value, fake.err
}
func (fake *variantServiceFake) Approve(_ context.Context, _ string, version int64) (variant.Variant, error) {
	fake.operation, fake.version = "approve", version
	return fake.value, fake.err
}
func (fake *variantServiceFake) Reject(_ context.Context, _ string, version int64) (variant.Variant, error) {
	fake.operation, fake.version = "reject", version
	return fake.value, fake.err
}

func (fake *generationServiceFake) Generate(_ context.Context, request generation.Request) (generation.Result, error) {
	fake.request = request
	return fake.result, fake.err
}
func (fake *generationServiceFake) Get(string) (generation.Result, error) {
	return fake.result, fake.err
}
func (fake *generationServiceFake) Candidates(string) ([]generation.Candidate, error) {
	return fake.result.Candidates, fake.err
}
func (fake *generationServiceFake) Select(string, string) (generation.Candidate, error) {
	return fake.candidate, fake.err
}

func (fake *postServiceFake) Create(_ context.Context, request post.CreateRequest) (post.Post, error) {
	fake.request = request
	return fake.result, fake.err
}

func TestCreatePostEndpointPassesURLOrMarkdownToService(t *testing.T) {
	fake := &postServiceFake{result: post.Post{ID: "new-id", Content: "extracted"}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Posts: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"source_type":"MARKDOWN","content":"# Source"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	if fake.request.SourceType != domain.SourceTypeMarkdown || fake.request.Content != "# Source" {
		t.Fatalf("service request = %+v", fake.request)
	}
}

func TestCreatePostEndpointRejectsUnknownFields(t *testing.T) {
	fake := &postServiceFake{result: post.Post{ID: "new-id"}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Posts: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"source_type":"MARKDOWN","content":"body","extra":true}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if fake.request.Content != "" {
		t.Fatal("service called for request with unknown field")
	}
}

func TestCreatePostEndpointMapsIngestionInputError(t *testing.T) {
	fake := &postServiceFake{err: ingestion.ErrInvalidInput}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Posts: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"source_type":"URL","source_url":"file:///etc/passwd"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != false {
		t.Fatalf("unexpected error envelope: %v", body)
	}
}

func TestGenerateEndpointUsesStoredPostIDAndPreference(t *testing.T) {
	fake := &generationServiceFake{result: generation.Result{ID: "session-1", PostID: "post-1", Candidates: []generation.Candidate{{ID: "candidate-1", Content: "generated"}}}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Generations: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/variants", strings.NewReader(`{"post_id":"post-1","preferences":[{"platform":"X","tone":"CONCISE"}]}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	if fake.request.PostID != "post-1" || len(fake.request.Preferences) != 1 || fake.request.Preferences[0].Platform != domain.PlatformX {
		t.Fatalf("generation request = %+v", fake.request)
	}
	if got := fake.request.Preferences[0].TargetLength; got != 280 {
		t.Errorf("default target length = %d, want 280", got)
	}
	if got := fake.request.Preferences[0].HashtagCount; got != 0 {
		t.Errorf("default hashtag count = %d, want 0", got)
	}
}

func TestGenerateEndpointDoesNotReturnInvalidGeneratedText(t *testing.T) {
	fake := &generationServiceFake{err: generation.ErrInvalidOutput}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Generations: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/variants", strings.NewReader(`{"post_id":"post-1","preferences":[{"platform":"X","tone":"CONCISE"}]}`)))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "281") || strings.Contains(response.Body.String(), "invalid model output") {
		t.Fatalf("invalid generated content leaked to client: %s", response.Body.String())
	}
}

func TestGenerateEndpointDefaultsLengthsForEachPlatform(t *testing.T) {
	fake := &generationServiceFake{result: generation.Result{ID: "session-1"}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Generations: fake})
	response := httptest.NewRecorder()
	body := `{"post_id":"post-1","preferences":[{"platform":"X","tone":"CONCISE"},{"platform":"LINKEDIN","tone":"PROFESSIONAL"},{"platform":"TELEGRAM","tone":"CONVERSATIONAL"}]}`
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/variants", strings.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	want := []int{280, 3000, 4096}
	for i, preference := range fake.request.Preferences {
		if preference.TargetLength != want[i] || preference.HashtagCount != 0 {
			t.Errorf("preference %s defaults = length %d, hashtags %d", preference.Platform, preference.TargetLength, preference.HashtagCount)
		}
	}
}

func TestAccessLogRecordsActualRoute(t *testing.T) {
	var logs strings.Builder
	server := NewServer(slog.New(slog.NewJSONHandler(&logs, nil)))
	for _, path := range []string{"/healthz", "/api/v1/platforms"} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	}
	entries := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(entries) != 2 {
		t.Fatalf("got %d log entries, want one per request: %s", len(entries), logs.String())
	}
	for i, expected := range []struct{ path, route string }{
		{"/healthz", "GET /healthz"},
		{"/api/v1/platforms", "GET /api/v1/platforms"},
	} {
		if !strings.Contains(entries[i], `"path":"`+expected.path+`"`) || !strings.Contains(entries[i], `"route":"`+expected.route+`"`) || !strings.Contains(entries[i], `"msg":"HTTP request completed"`) {
			t.Errorf("log entry does not identify request route %s: %s", expected.route, entries[i])
		}
	}
}

func TestAccessLogRecoversFromPanicWithoutLoggingPanicValue(t *testing.T) {
	var logs strings.Builder
	server := &Server{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	handler := server.accessLog(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive source and provider secret")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if strings.Contains(logs.String(), "sensitive source") || strings.Contains(logs.String(), "provider secret") {
		t.Fatalf("panic value leaked into logs: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"msg":"request handler panicked"`) || !strings.Contains(logs.String(), `"status":500`) || !strings.Contains(logs.String(), `"msg":"HTTP request completed"`) {
		t.Fatalf("panic was not recorded with structured error and access logs: %s", logs.String())
	}
}

func TestGenerationResponsesDoNotExposeAIProviderOrModel(t *testing.T) {
	candidate := generation.Candidate{ID: "candidate-1", Content: "generated"}
	fake := &generationServiceFake{
		result:    generation.Result{ID: "session-1", Candidates: []generation.Candidate{candidate}},
		candidate: candidate,
	}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Generations: fake})
	for _, test := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/variants", `{"post_id":"post-1","preferences":[{"platform":"X","tone":"CONCISE"}]}`},
		{http.MethodGet, "/api/v1/generations/session-1", ""},
		{http.MethodGet, "/api/v1/generations/session-1/candidates", ""},
		{http.MethodPost, "/api/v1/generations/session-1/selection", `{"candidate_id":"candidate-1"}`},
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code < 200 || response.Code >= 300 {
			t.Fatalf("%s %s returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), `"provider"`) || strings.Contains(response.Body.String(), `"model"`) {
			t.Errorf("%s %s exposed internal AI implementation details: %s", test.method, test.path, response.Body.String())
		}
	}
}

func TestVariantReviewEndpoints(t *testing.T) {
	variantValue := variant.Variant{
		ID: "019a0000-0000-7000-8000-000000000001", PostID: "019a0000-0000-7000-8000-000000000002",
		Platform: domain.PlatformX, Content: "A draft", Status: domain.VariantStatusDraft,
		CreatedAt: time.Date(2026, time.October, 21, 0, 0, 0, 0, time.UTC), Version: 1,
	}
	fake := &variantServiceFake{value: variantValue, listed: []variant.Variant{variantValue}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Variants: fake})
	for _, test := range []struct {
		method, path, body, operation string
	}{
		{http.MethodGet, "/api/v1/posts/019a0000-0000-7000-8000-000000000002/variants?status=DRAFT&limit=20&offset=10", "", "list"},
		{http.MethodGet, "/api/v1/variants/019a0000-0000-7000-8000-000000000001", "", "get"},
		{http.MethodPatch, "/api/v1/variants/019a0000-0000-7000-8000-000000000001", `{"version":1,"content":"A revised draft"}`, "edit"},
		{http.MethodPost, "/api/v1/variants/019a0000-0000-7000-8000-000000000001/approve", `{"version":1}`, "approve"},
		{http.MethodPost, "/api/v1/variants/019a0000-0000-7000-8000-000000000001/reject", `{"version":1}`, "reject"},
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code < 200 || response.Code >= 300 {
			t.Fatalf("%s %s returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
		if test.operation != "list" && test.operation != "get" && fake.operation != test.operation {
			t.Errorf("%s %s operation = %q, want %q", test.method, test.path, fake.operation, test.operation)
		}
		if test.operation == "list" && (fake.status != domain.VariantStatusDraft || fake.limit != 20 || fake.offset != 10) {
			t.Errorf("list filter = %q, limit %d, offset %d", fake.status, fake.limit, fake.offset)
		}
		if test.operation == "edit" && (fake.version != 1 || fake.content != "A revised draft") {
			t.Errorf("edit request = version %d, content %q", fake.version, fake.content)
		}
	}
}

func TestVariantReviewEndpointMapsStaleVersionToConflict(t *testing.T) {
	fake := &variantServiceFake{err: variant.ErrVersionConflict}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Variants: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/variants/019a0000-0000-7000-8000-000000000001/approve", strings.NewReader(`{"version":2}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", response.Code, response.Body.String())
	}
}

type publishingServiceFake struct {
	schedule publishing.Schedule
	attempt  publishing.Attempt
	err      error
	created  bool
	called   bool
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

func TestGenerationSessionEndpoints(t *testing.T) {
	fake := &generationServiceFake{result: generation.Result{ID: "session-1", Candidates: []generation.Candidate{{ID: "candidate-1", Content: "generated"}}}, candidate: generation.Candidate{ID: "candidate-1", Content: "generated"}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Generations: fake})
	for _, test := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/generations/session-1", ""},
		{http.MethodGet, "/api/v1/generations/session-1/candidates", ""},
		{http.MethodPost, "/api/v1/generations/session-1/selection", `{"candidate_id":"candidate-1"}`},
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != http.StatusOK {
			t.Errorf("%s %s returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}
