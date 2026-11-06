package httpapi

import (
	"context"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/generation"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
