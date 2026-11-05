package httpapi

import (
	"errors"
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
	"github.com/LibreTurtle/social-studio/internal/middleware"
	"github.com/LibreTurtle/social-studio/internal/post"
	"github.com/LibreTurtle/social-studio/internal/publishing"
	"github.com/LibreTurtle/social-studio/internal/variant"
)

func (server *Server) handleError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ingestion.ErrInvalidInput), errors.Is(err, generation.ErrInvalidRequest), errors.Is(err, variant.ErrInvalidID), errors.Is(err, variant.ErrInvalidInput), errors.Is(err, publishing.ErrInvalidInput):
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Request parameters are invalid", nil)
	case errors.Is(err, post.ErrNotFound), errors.Is(err, generation.ErrPostNotFound), errors.Is(err, generation.ErrSessionNotFound), errors.Is(err, variant.ErrNotFound), errors.Is(err, publishing.ErrNotFound):
		server.writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "Requested resource was not found", nil)
	case errors.Is(err, variant.ErrVersionConflict):
		server.writeError(writer, request, http.StatusConflict, "VERSION_CONFLICT", "Variant changed since it was retrieved; fetch it again", nil)
	case errors.Is(err, variant.ErrInvalidTransition):
		server.writeError(writer, request, http.StatusConflict, "INVALID_VARIANT_TRANSITION", "Variant cannot be changed from its current status", nil)
	case errors.Is(err, publishing.ErrNotApproved):
		server.writeError(writer, request, http.StatusConflict, "VARIANT_NOT_APPROVED", "Only approved variants can be scheduled", nil)
	case errors.Is(err, publishing.ErrAlreadyScheduled):
		server.writeError(writer, request, http.StatusConflict, "VARIANT_ALREADY_SCHEDULED", "Variant already has an active or completed publish schedule", nil)
	case errors.Is(err, publishing.ErrNotDue):
		server.writeError(writer, request, http.StatusConflict, "SCHEDULE_NOT_DUE", "The scheduled time has not arrived", nil)
	case errors.Is(err, publishing.ErrInProgress):
		server.writeError(writer, request, http.StatusConflict, "PUBLISH_IN_PROGRESS", "This schedule is already being processed", nil)
	case errors.Is(err, publishing.ErrPublisherUnavailable):
		server.writeError(writer, request, http.StatusServiceUnavailable, "PUBLISHER_UNAVAILABLE", "Publishing is not configured for this platform", nil)
	case errors.Is(err, variant.ErrInvalidContent):
		var contentErr *variant.InvalidContentError
		var details any
		if errors.As(err, &contentErr) {
			details = contentErr.Violations
		}
		server.writeError(writer, request, http.StatusUnprocessableEntity, "INVALID_VARIANT_CONTENT", "Edited content violates platform constraints", details)
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
