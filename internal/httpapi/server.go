package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/middleware"
	"github.com/LibreTurtle/social-studio/internal/post"
	"github.com/LibreTurtle/social-studio/internal/publishing"
	"github.com/LibreTurtle/social-studio/internal/variant"
)

type Server struct {
	logger      *slog.Logger
	posts       PostService
	generations GenerationService
	variants    ReviewService
	publishing  PublishingService
}

type PostService interface {
	Create(context.Context, post.CreateRequest) (post.Post, error)
}
type GenerationService interface {
	Generate(context.Context, generation.Request) (generation.Result, error)
	Get(string) (generation.Result, error)
	Candidates(string) ([]generation.Candidate, error)
	Select(string, string) (generation.Candidate, error)
}
type ReviewService interface {
	Get(context.Context, string) (variant.Variant, error)
	ListByPost(context.Context, string, domain.VariantStatus, int, int) ([]variant.Variant, error)
	UpdateContent(context.Context, string, int64, string) (variant.Variant, error)
	Approve(context.Context, string, int64) (variant.Variant, error)
	Reject(context.Context, string, int64) (variant.Variant, error)
}

type PublishingService interface {
	CreateSchedule(context.Context, string, time.Time) (publishing.Schedule, error)
	Publish(context.Context, string) (publishing.Attempt, error)
	ListHistory(context.Context, publishing.HistoryRequest) (publishing.HistoryPage, error)
}

type Dependencies struct {
	Posts       PostService
	Generations GenerationService
	Variants    ReviewService
	Publishing  PublishingService
}

func NewServer(logger *slog.Logger, dependencies ...Dependencies) http.Handler {
	server := &Server{logger: loggerOrDefault(logger)}
	if len(dependencies) != 0 {
		server.posts, server.generations, server.variants = dependencies[0].Posts, dependencies[0].Generations, dependencies[0].Variants
		server.publishing = dependencies[0].Publishing
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /api/v1/platforms", server.listPlatforms)
	mux.HandleFunc("POST /api/v1/posts", server.createPost)
	mux.HandleFunc("POST /api/v1/variants", server.generateVariants)
	mux.HandleFunc("GET /api/v1/posts/{postID}/variants", server.listVariants)
	mux.HandleFunc("GET /api/v1/variants/{variantID}", server.getVariant)
	mux.HandleFunc("PATCH /api/v1/variants/{variantID}", server.editVariant)
	mux.HandleFunc("POST /api/v1/variants/{variantID}/approve", server.approveVariant)
	mux.HandleFunc("POST /api/v1/variants/{variantID}/reject", server.rejectVariant)
	mux.HandleFunc("POST /api/v1/schedules", server.createSchedule)
	mux.HandleFunc("POST /api/v1/schedules/{scheduleID}/publish", server.publishSchedule)
	mux.HandleFunc("GET /api/v1/publish-history", server.listPublishHistory)
	mux.HandleFunc("GET /api/v1/generations/{sessionID}", server.getGeneration)
	mux.HandleFunc("GET /api/v1/generations/{sessionID}/candidates", server.getCandidates)
	mux.HandleFunc("POST /api/v1/generations/{sessionID}/selection", server.selectCandidate)
	return middleware.RequestID(server.accessLog(mux))
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
