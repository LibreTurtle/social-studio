package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/LibreTurtle/social-studio/internal/config"
	"github.com/LibreTurtle/social-studio/internal/db"
	"github.com/LibreTurtle/social-studio/internal/logging"
	"github.com/LibreTurtle/social-studio/internal/publishing"
	"github.com/LibreTurtle/social-studio/internal/scheduler"
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("scheduler worker stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	database, err := db.Connect(configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("database connection pool close failed", "error", err)
		}
	}()

	providerHTTPClient := publishing.NewHTTPClient()
	defer providerHTTPClient.CloseIdleConnections()
	providers := publishing.NewPublishers(configuration.Publishing, providerHTTPClient)
	store := publishing.NewPostgresStore(database)
	publisher := publishing.NewService(store, providers, logger)
	worker, err := scheduler.NewWorker(store, publisher, scheduler.Options{
		PollInterval: configuration.Scheduler.PollInterval,
		ClaimTimeout: configuration.Scheduler.ClaimTimeout,
		Concurrency:  configuration.Scheduler.Concurrency,
	}, logger)
	if err != nil {
		return err
	}
	logger.Info("scheduler worker started", "poll_interval", configuration.Scheduler.PollInterval, "claim_timeout", configuration.Scheduler.ClaimTimeout, "concurrency", configuration.Scheduler.Concurrency)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-shutdownDone:
		}
	}()
	err = worker.Run(ctx)
	close(shutdownDone)
	stop()
	if err != nil {
		return err
	}
	logger.Info("scheduler worker stopped")
	return nil
}
