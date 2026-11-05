package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LibreTurtle/social-studio/internal/publishing"
)

type createScheduleRequest struct {
	VariantID   string `json:"variant_id"`
	ScheduledAt string `json:"scheduled_at"`
}

func (server *Server) createSchedule(writer http.ResponseWriter, request *http.Request) {
	if server.publishing == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Publishing is unavailable", nil)
		return
	}
	var body createScheduleRequest
	if !server.decodeRequest(writer, request, &body) {
		return
	}
	scheduledAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(body.ScheduledAt))
	if err != nil || strings.TrimSpace(body.VariantID) == "" {
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "variant_id and an RFC3339 scheduled_at are required", nil)
		return
	}
	schedule, err := server.publishing.CreateSchedule(request.Context(), body.VariantID, scheduledAt)
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusCreated, envelope{Success: true, Message: "Schedule created", Data: schedule})
}

func (server *Server) publishSchedule(writer http.ResponseWriter, request *http.Request) {
	if server.publishing == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Publishing is unavailable", nil)
		return
	}
	attempt, err := server.publishing.Publish(request.Context(), request.PathValue("scheduleID"))
	if errors.Is(err, publishing.ErrUncertain) {
		server.writeError(writer, request, http.StatusAccepted, "PUBLISH_OUTCOME_UNKNOWN", "The provider outcome is unknown; this schedule will not be retried automatically", nil)
		return
	}
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Publish attempt recorded", Data: attempt})
}
