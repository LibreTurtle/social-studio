package httpapi

import (
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"sort"

	"github.com/LibreTurtle/social-studio/internal/middleware"
	"github.com/LibreTurtle/social-studio/internal/platform"
)

type envelope struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    any          `json:"data,omitempty"`
	Error   *errorDetail `json:"error,omitempty"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}

type Server struct {
	logger *slog.Logger
}

func NewServer(logger *slog.Logger) http.Handler {
	server := &Server{logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /api/v1/platforms", server.listPlatforms)
	return middleware.RequestID(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoContext(r.Context(), "health check served",
		"request_id", middleware.RequestIDFromContext(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
	)
	s.writeJSON(w, r, http.StatusOK, envelope{Success: true, Message: "Service is healthy", Data: map[string]string{"status": "ok"}})
}

func (s *Server) listPlatforms(w http.ResponseWriter, r *http.Request) {
	profiles := platform.DefaultProfiles()
	list := make([]platform.Profile, 0, len(profiles))
	for _, profile := range profiles {
		list = append(list, profile)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	s.logger.InfoContext(r.Context(), "platform profiles retrieved",
		"request_id", middleware.RequestIDFromContext(r.Context()),
		"count", len(list),
	)
	s.writeJSON(w, r, http.StatusOK, envelope{Success: true, Message: "Platform profiles retrieved", Data: list})
}

func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, body); err != nil {
		s.logger.ErrorContext(r.Context(), "HTTP response write failed",
			"request_id", middleware.RequestIDFromContext(r.Context()),
			"status", status,
			"error", err,
		)
	}
}
