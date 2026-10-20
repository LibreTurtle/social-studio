package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/LibreTurtle/social-studio/internal/middleware"
)

type responseStatusWriter struct {
	http.ResponseWriter
	status int
}

func (writer *responseStatusWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *responseStatusWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}

func (writer *responseStatusWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (server *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		tracked := &responseStatusWriter{ResponseWriter: writer}
		defer func() {
			if recovered := recover(); recovered != nil {
				server.logger.ErrorContext(request.Context(), "request handler panicked",
					"request_id", middleware.RequestIDFromContext(request.Context()),
					"panic_type", fmt.Sprintf("%T", recovered),
				)
				if tracked.status == 0 {
					server.writeError(tracked, request, http.StatusInternalServerError, "INTERNAL_ERROR", "The request could not be completed", nil)
				}
			}
			status := tracked.status
			if status == 0 {
				status = http.StatusOK
			}
			server.logger.InfoContext(request.Context(), "HTTP request completed",
				"request_id", middleware.RequestIDFromContext(request.Context()),
				"method", request.Method,
				"route", request.Pattern,
				"path", request.URL.Path,
				"status", status,
				"duration", time.Since(started),
			)
		}()
		next.ServeHTTP(tracked, request)
	})
}
