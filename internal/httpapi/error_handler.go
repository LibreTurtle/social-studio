package httpapi

import (
	"errors"
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
	"github.com/LibreTurtle/social-studio/internal/middleware"
	"github.com/LibreTurtle/social-studio/internal/post"
)

func (server *Server) handleError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ingestion.ErrInvalidInput), errors.Is(err, generation.ErrInvalidRequest):
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Request parameters are invalid", nil)
	case errors.Is(err, post.ErrNotFound), errors.Is(err, generation.ErrPostNotFound), errors.Is(err, generation.ErrSessionNotFound):
		server.writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "Requested resource was not found", nil)
	case errors.Is(err, generation.ErrAIUnavailable):
		server.writeError(writer, request, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "AI generation is temporarily unavailable", nil)
	case errors.Is(err, generation.ErrInvalidOutput):
		server.writeError(writer, request, http.StatusBadGateway, "GENERATED_CONTENT_INVALID", "Generated content could not satisfy platform constraints", nil)
	case errors.Is(err, ingestion.ErrFetchFailed):
		server.writeError(writer, request, http.StatusBadGateway, "SOURCE_FETCH_FAILED", "The source page could not be fetched", nil)
	case errors.Is(err, ingestion.ErrExtractionFailed):
		server.writeError(writer, request, http.StatusUnprocessableEntity, "SOURCE_CONTENT_INVALID", "The source page contains no usable content", nil)
	default:
		server.logger.ErrorContext(request.Context(), "request processing failed", "request_id", middleware.RequestIDFromContext(request.Context()), "error", err)
		server.writeError(writer, request, http.StatusInternalServerError, "INTERNAL_ERROR", "The request could not be completed", nil)
	}
}
