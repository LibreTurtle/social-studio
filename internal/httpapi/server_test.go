package httpapi

import (
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
