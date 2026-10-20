package middleware

import (
	"context"
	"net/http"
	"uuid"
)

type contextKey struct{}

const requestIDHeader = "X-Request-ID"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewV4()

		w.Header().Set(requestIDHeader, id.String())
		ctx := context.WithValue(r.Context(), contextKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(contextKey{}).(uuid.UUID)
	return id
}
