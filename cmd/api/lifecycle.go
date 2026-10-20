package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const shutdownTimeout = 30 * time.Second

type shutdownController interface {
	Shutdown(context.Context) error
	Close() error
}

func serveHTTP(ctx context.Context, server *http.Server, logger *slog.Logger, stopSignals context.CancelFunc) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	logger.Info("HTTP server is starting", "address", server.Addr)

	select {
	case err := <-serverErrors:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server failed: %w", err)
	case <-ctx.Done():
		stopSignals()
		logger.InfoContext(ctx, "shutdown requested", "cause", context.Cause(ctx))
		shutdownErr := shutdownHTTPServer(server, shutdownTimeout, logger)
		listenErr := <-serverErrors
		if shutdownErr != nil {
			if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
				return errors.Join(shutdownErr, fmt.Errorf("HTTP server failed while shutting down: %w", listenErr))
			}
			return shutdownErr
		}
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server failed while shutting down: %w", listenErr)
		}
		return nil
	}
}

func shutdownHTTPServer(server shutdownController, timeout time.Duration, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.WarnContext(ctx, "graceful HTTP shutdown failed; closing active connections", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return errors.Join(
				fmt.Errorf("graceful HTTP shutdown failed: %w", err),
				fmt.Errorf("force-closing HTTP server failed: %w", closeErr),
			)
		}
		return fmt.Errorf("graceful HTTP shutdown failed; active connections were closed: %w", err)
	}

	logger.Info("HTTP server shutdown completed")
	return nil
}
