package httpapi

import (
	"net/http"
	"strconv"

	"github.com/LibreTurtle/social-studio/internal/publishing"
)

func (server *Server) listPublishHistory(writer http.ResponseWriter, request *http.Request) {
	if server.publishing == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Publishing is unavailable", nil)
		return
	}
	query := request.URL.Query()
	limit := 0
	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "History limit must be an integer", nil)
			return
		}
		limit = parsed
	}
	page, err := server.publishing.ListHistory(request.Context(), publishing.HistoryRequest{
		VariantID: query.Get("variant_id"), ScheduleID: query.Get("schedule_id"),
		Limit: limit, Cursor: query.Get("cursor"),
	})
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Publish history retrieved", Data: page})
}
