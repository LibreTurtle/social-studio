package httpapi

import (
	"context"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/variant"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type variantServiceFake struct {
	value     variant.Variant
	listed    []variant.Variant
	status    domain.VariantStatus
	limit     int
	offset    int
	version   int64
	content   string
	operation string
	err       error
}

func (fake *variantServiceFake) Get(_ context.Context, _ string) (variant.Variant, error) {
	return fake.value, fake.err
}

func (fake *variantServiceFake) ListByPost(_ context.Context, _ string, status domain.VariantStatus, limit, offset int) ([]variant.Variant, error) {
	fake.status, fake.limit, fake.offset = status, limit, offset
	return fake.listed, fake.err
}

func (fake *variantServiceFake) UpdateContent(_ context.Context, _ string, version int64, content string) (variant.Variant, error) {
	fake.operation, fake.version, fake.content = "edit", version, content
	return fake.value, fake.err
}

func (fake *variantServiceFake) Approve(_ context.Context, _ string, version int64) (variant.Variant, error) {
	fake.operation, fake.version = "approve", version
	return fake.value, fake.err
}

func (fake *variantServiceFake) Reject(_ context.Context, _ string, version int64) (variant.Variant, error) {
	fake.operation, fake.version = "reject", version
	return fake.value, fake.err
}

func TestVariantReviewEndpoints(t *testing.T) {
	variantValue := variant.Variant{
		ID: "019a0000-0000-7000-8000-000000000001", PostID: "019a0000-0000-7000-8000-000000000002",
		Platform: domain.PlatformX, Content: "A draft", Status: domain.VariantStatusDraft,
		CreatedAt: time.Date(2026, time.October, 21, 0, 0, 0, 0, time.UTC), Version: 1,
	}
	fake := &variantServiceFake{value: variantValue, listed: []variant.Variant{variantValue}}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Variants: fake})
	for _, test := range []struct {
		method, path, body, operation string
	}{
		{http.MethodGet, "/api/v1/posts/019a0000-0000-7000-8000-000000000002/variants?status=DRAFT&limit=20&offset=10", "", "list"},
		{http.MethodGet, "/api/v1/variants/019a0000-0000-7000-8000-000000000001", "", "get"},
		{http.MethodPatch, "/api/v1/variants/019a0000-0000-7000-8000-000000000001", `{"version":1,"content":"A revised draft"}`, "edit"},
		{http.MethodPost, "/api/v1/variants/019a0000-0000-7000-8000-000000000001/approve", `{"version":1}`, "approve"},
		{http.MethodPost, "/api/v1/variants/019a0000-0000-7000-8000-000000000001/reject", `{"version":1}`, "reject"},
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code < 200 || response.Code >= 300 {
			t.Fatalf("%s %s returned %d: %s", test.method, test.path, response.Code, response.Body.String())
		}
		if test.operation != "list" && test.operation != "get" && fake.operation != test.operation {
			t.Errorf("%s %s operation = %q, want %q", test.method, test.path, fake.operation, test.operation)
		}
		if test.operation == "list" && (fake.status != domain.VariantStatusDraft || fake.limit != 20 || fake.offset != 10) {
			t.Errorf("list filter = %q, limit %d, offset %d", fake.status, fake.limit, fake.offset)
		}
		if test.operation == "edit" && (fake.version != 1 || fake.content != "A revised draft") {
			t.Errorf("edit request = version %d, content %q", fake.version, fake.content)
		}
	}
}

func TestVariantReviewEndpointMapsStaleVersionToConflict(t *testing.T) {
	fake := &variantServiceFake{err: variant.ErrVersionConflict}
	server := NewServer(slog.New(slog.NewJSONHandler(io.Discard, nil)), Dependencies{Variants: fake})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/variants/019a0000-0000-7000-8000-000000000001/approve", strings.NewReader(`{"version":2}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", response.Code, response.Body.String())
	}
}
