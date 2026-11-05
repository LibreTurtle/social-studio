package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LibreTurtle/social-studio/internal/ai"
	"github.com/LibreTurtle/social-studio/internal/config"
	"github.com/LibreTurtle/social-studio/internal/db"
	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/httpapi"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
	"github.com/LibreTurtle/social-studio/internal/logging"
	"github.com/LibreTurtle/social-studio/internal/post"
	"github.com/LibreTurtle/social-studio/internal/publishing"
	"github.com/LibreTurtle/social-studio/internal/variant"
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
	logger.Info("database connection established")

	posts := post.NewPostgresStore(database)
	postService := post.NewService(posts, ingestion.NewReader(ingestion.NewHTTPFetcher(), ingestion.NewHTMLExtractor()))
	variantService := variant.NewService(variant.NewPostgresStore(database))
	aiGenerator, err := ai.NewGenerator(configuration.AI, logger)
	if err != nil {
		return err
	}
	generationService := generation.NewService(posts, aiGenerator, variantService, 15*time.Minute, 10000)
	providerHTTPClient := publishing.NewHTTPClient()
	defer providerHTTPClient.CloseIdleConnections()
	publishers := publishing.NewPublishers(configuration.Publishing, providerHTTPClient)
	publishingService := publishing.NewService(publishing.NewPostgresStore(database), publishers, logger)
	server := &http.Server{
		Addr:              ":" + configuration.Port,
		Handler:           httpapi.NewServer(logger, httpapi.Dependencies{Posts: postService, Generations: generationService, Variants: variantService, Publishing: publishingService}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      max(maxGenerationResponseTime(configuration.AI), 50*time.Second),
		IdleTimeout:       60 * time.Second,
	}

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	return serveHTTP(signalContext, server, logger, stopSignals)
}

func maxGenerationResponseTime(settings ai.Settings) time.Duration {
	const responseOverhead = 30 * time.Second
	providerWindow := time.Duration(len(settings.Providers)) * settings.RequestTimeout
	maximumAttempts := generation.MaxPreferences * generation.MaxGenerationAttempts
	return time.Duration(maximumAttempts)*providerWindow + responseOverhead
}
