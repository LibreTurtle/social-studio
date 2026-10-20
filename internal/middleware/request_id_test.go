package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"
)

func TestRequestIDAlwaysGeneratesItsOwnUUIDV4(t *testing.T) {
	clientID := uuid.NewV4()
	for _, incoming := range []string{"", "not-a-uuid", clientID.String()} {
		t.Run(incoming, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-Request-ID", incoming)
			response := httptest.NewRecorder()
			var contextID uuid.UUID

			RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextID = RequestIDFromContext(r.Context())
			})).ServeHTTP(response, request)

			got, err := uuid.Parse(response.Header().Get("X-Request-ID"))
			if err != nil {
				t.Fatalf("response request ID is invalid: %v", err)
			}
			if got != contextID {
				t.Fatalf("header request ID %s differs from context ID %s", got, contextID)
			}
			if incoming == clientID.String() && got == clientID {
				t.Fatal("server reused the client-supplied request ID")
			}
			if version := got[6] >> 4; version != 4 {
				t.Fatalf("generated UUID version = %d, want 4", version)
			}
		})
	}
}

func TestRequestIDFromContextReturnsNilForMissingID(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != uuid.Nil() {
		t.Fatalf("missing request ID = %s, want nil UUID", got)
	}
}
