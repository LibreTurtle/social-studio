package publishing

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"uuid"
)

const (
	defaultHistoryLimit = 50
	maximumHistoryLimit = 100
)

func (service *Service) ListHistory(ctx context.Context, request HistoryRequest) (HistoryPage, error) {
	if request.Limit == 0 {
		request.Limit = defaultHistoryLimit
	}
	if request.Limit < 1 || request.Limit > maximumHistoryLimit {
		return HistoryPage{}, ErrInvalidInput
	}
	variantID, err := optionalCanonicalID(request.VariantID)
	if err != nil {
		return HistoryPage{}, ErrInvalidInput
	}
	scheduleID, err := optionalCanonicalID(request.ScheduleID)
	if err != nil {
		return HistoryPage{}, ErrInvalidInput
	}
	if variantID == "" && scheduleID == "" {
		return HistoryPage{}, ErrInvalidInput
	}
	var before *AttemptCursor
	if request.Cursor != "" {
		before, err = decodeAttemptCursor(request.Cursor)
		if err != nil {
			return HistoryPage{}, ErrInvalidInput
		}
	}

	attempts, err := service.store.ListAttempts(ctx, HistoryFilter{
		VariantID: variantID, ScheduleID: scheduleID, Limit: request.Limit + 1, Before: before,
	})
	if err != nil {
		return HistoryPage{}, fmt.Errorf("list publish history: %w", err)
	}
	page := HistoryPage{Attempts: attempts}
	if len(attempts) > request.Limit {
		page.Attempts = attempts[:request.Limit]
		page.NextCursor = encodeAttemptCursor(page.Attempts[len(page.Attempts)-1])
	}
	return page, nil
}

func optionalCanonicalID(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return canonicalID(value)
}

func encodeAttemptCursor(attempt Attempt) string {
	value := attempt.AttemptedAt.UTC().Format(time.RFC3339Nano) + "\n" + attempt.ID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeAttemptCursor(value string) (*AttemptCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrInvalidInput
	}
	parts := strings.SplitN(string(decoded), "\n", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidInput
	}
	attemptedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil || attemptedAt.IsZero() {
		return nil, ErrInvalidInput
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, ErrInvalidInput
	}
	return &AttemptCursor{AttemptedAt: attemptedAt.UTC(), ID: id.String()}, nil
}
