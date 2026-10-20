package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/middleware"
	"github.com/LibreTurtle/social-studio/internal/post"
)

type Server struct {
	logger      *slog.Logger
	posts       PostService
	generations GenerationService
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
type Dependencies struct {
	Posts       PostService
	Generations GenerationService
}

func NewServer(logger *slog.Logger, dependencies ...Dependencies) http.Handler {
	server := &Server{logger: loggerOrDefault(logger)}
	if len(dependencies) != 0 {
		server.posts, server.generations = dependencies[0].Posts, dependencies[0].Generations
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /api/v1/platforms", server.listPlatforms)
	mux.HandleFunc("POST /api/v1/posts", server.createPost)
	mux.HandleFunc("POST /api/v1/variants", server.generateVariants)
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
