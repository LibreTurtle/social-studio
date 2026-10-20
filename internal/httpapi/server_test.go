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

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
	"github.com/LibreTurtle/social-studio/internal/post"
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
	candidate := generation.Candidate{ID: "candidate-1", Content: "generated", Provider: "internal-provider-name"}
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
		if strings.Contains(response.Body.String(), "internal-provider-name") || strings.Contains(response.Body.String(), `"provider"`) || strings.Contains(response.Body.String(), `"model"`) {
			t.Errorf("%s %s exposed internal AI implementation details: %s", test.method, test.path, response.Body.String())
		}
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
