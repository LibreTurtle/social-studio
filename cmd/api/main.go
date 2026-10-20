package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LibreTurtle/social-studio/internal/db"
	"github.com/LibreTurtle/social-studio/internal/httpapi"
	"github.com/LibreTurtle/social-studio/internal/logging"
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("application stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	database, err := db.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("database connection pool close failed", "error", err)
		}
	}()
	logger.Info("database connection established")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           httpapi.NewServer(logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	return serveHTTP(signalContext, server, logger, stopSignals)
}
