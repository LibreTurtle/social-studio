package publishing

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestListHistoryUsesKeysetCursorAndBoundsPageSize(t *testing.T) {
	firstAttempt := Attempt{ID: testID(1), AttemptedAt: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)}
	secondAttempt := Attempt{ID: testID(2), AttemptedAt: firstAttempt.AttemptedAt.Add(-time.Minute)}
	store := &fakeStore{historyAttempts: []Attempt{firstAttempt, secondAttempt}}
	service := NewService(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	page, err := service.ListHistory(context.Background(), HistoryRequest{VariantID: testID(3), Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attempts) != 1 || page.Attempts[0].ID != firstAttempt.ID || page.NextCursor == "" {
		t.Fatalf("first history page = %+v", page)
	}
	if store.historyFilter.Limit != 2 {
		t.Fatalf("store fetch limit=%d, want requested limit plus one", store.historyFilter.Limit)
	}

	store.historyAttempts = []Attempt{secondAttempt}
	_, err = service.ListHistory(context.Background(), HistoryRequest{VariantID: testID(3), Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if store.historyFilter.Before == nil || store.historyFilter.Before.ID != firstAttempt.ID || !store.historyFilter.Before.AttemptedAt.Equal(firstAttempt.AttemptedAt) {
		t.Fatalf("decoded history cursor = %+v, want first attempt cursor", store.historyFilter.Before)
	}
}

func TestListHistoryRejectsInvalidIDsAndCursor(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, request := range []HistoryRequest{
		{VariantID: "not-a-uuid"},
		{Cursor: "invalid-cursor"},
		{Limit: 101},
	} {
		if _, err := service.ListHistory(context.Background(), request); err != ErrInvalidInput {
			t.Errorf("ListHistory(%+v) error=%v, want ErrInvalidInput", request, err)
		}
	}
}
