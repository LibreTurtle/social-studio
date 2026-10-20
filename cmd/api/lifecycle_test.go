package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/LibreTurtle/social-studio/internal/ai"
)

type fakeShutdownServer struct {
	shutdownErr error
	closed      bool
}

func (f *fakeShutdownServer) Shutdown(context.Context) error {
	return f.shutdownErr
}

func (f *fakeShutdownServer) Close() error {
	f.closed = true
	return nil
}

func TestShutdownHTTPServerDoesNotForceCloseAfterGracefulShutdown(t *testing.T) {
	server := &fakeShutdownServer{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if err := shutdownHTTPServer(server, time.Second, logger); err != nil {
		t.Fatalf("shutdownHTTPServer returned error: %v", err)
	}
	if server.closed {
		t.Fatal("Close was called after a successful graceful shutdown")
	}
}

func TestShutdownHTTPServerForcesCloseWhenGracefulShutdownFails(t *testing.T) {
	server := &fakeShutdownServer{shutdownErr: context.DeadlineExceeded}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if err := shutdownHTTPServer(server, time.Millisecond, logger); err == nil {
		t.Fatal("expected shutdown error")
	}
	if !server.closed {
		t.Fatal("Close was not called after graceful shutdown failed")
	}
}

func TestServeHTTPReturnsUnexpectedListenError(t *testing.T) {
	server := &http.Server{Addr: "invalid address"}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if err := serveHTTP(context.Background(), server, logger, func() {}); err == nil {
		t.Fatal("expected invalid listen address to fail")
	}
}

func TestShutdownErrorCanBeIdentified(t *testing.T) {
	server := &fakeShutdownServer{shutdownErr: context.DeadlineExceeded}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	err := shutdownHTTPServer(server, time.Millisecond, logger)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
}

func TestHTTPWriteTimeoutCoversAllCandidateProviderFallbacks(t *testing.T) {
	settings := ai.Settings{Providers: make([]ai.ProviderSettings, 3), RequestTimeout: 20 * time.Second}
	if got, want := maxGenerationResponseTime(settings), 570*time.Second; got != want {
		t.Fatalf("write timeout = %s, want %s", got, want)
	}
}
