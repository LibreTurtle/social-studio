package httpapi

import (
	"context"
	"encoding/json/v2"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
	"github.com/LibreTurtle/social-studio/internal/post"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type postServiceFake struct {
	request post.CreateRequest
	result  post.Post
	err     error
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
