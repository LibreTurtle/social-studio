package httpapi

import (
	"encoding/json/v2"
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/middleware"
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

func (server *Server) writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string, details any) {
	server.writeJSON(writer, request, status, envelope{Success: false, Message: message, Error: &errorDetail{Code: code, Details: details}})
}

func (server *Server) writeJSON(writer http.ResponseWriter, request *http.Request, status int, body envelope) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.MarshalWrite(writer, body); err != nil {
		server.logger.ErrorContext(request.Context(), "HTTP response write failed", "request_id", middleware.RequestIDFromContext(request.Context()), "status", status, "error", err)
	}
}
