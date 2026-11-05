package publishing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/middleware"
	"github.com/LibreTurtle/social-studio/internal/platform"
)

type Service struct {
	store      Store
	publishers map[domain.Platform]SocialPublisher
	logger     *slog.Logger
}

func NewService(store Store, publishers map[domain.Platform]SocialPublisher, logger *slog.Logger) *Service {
	configured := make(map[domain.Platform]SocialPublisher, len(publishers))
	for platform, publisher := range publishers {
		if platform.Valid() && publisher != nil {
			configured[platform] = publisher
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, publishers: configured, logger: logger}
}

func (service *Service) CreateSchedule(ctx context.Context, variantID string, scheduledAt time.Time) (Schedule, error) {
	variantID, err := canonicalID(variantID)
	if err != nil || scheduledAt.IsZero() {
		return Schedule{}, ErrInvalidInput
	}
	scheduledAt = scheduledAt.UTC().Truncate(time.Microsecond)
	platform, status, existing, err := service.store.GetForSchedule(ctx, variantID, scheduledAt)
	if err != nil {
		return Schedule{}, fmt.Errorf("load variant for publishing schedule: %w", err)
	}
	if existing != nil {
		return *existing, nil
	}
	if status != domain.VariantStatusApproved {
		return Schedule{}, ErrNotApproved
	}
	if _, configured := service.publishers[platform]; !configured {
		service.logger.WarnContext(ctx, "cannot schedule without a configured publisher", "request_id", middleware.RequestIDFromContext(ctx), "platform", platform, "error_category", "publisher_unavailable")
		return Schedule{}, ErrPublisherUnavailable
	}
	schedule, err := service.store.CreateSchedule(ctx, variantID, scheduledAt)
	if err != nil {
		return Schedule{}, fmt.Errorf("create publishing schedule: %w", err)
	}
	return schedule, nil
}

func (service *Service) Publish(ctx context.Context, scheduleID string) (Attempt, error) {
	scheduleID, err := canonicalID(scheduleID)
	if err != nil {
		return Attempt{}, ErrInvalidInput
	}
	claim, err := service.store.Claim(ctx, scheduleID)
	if err != nil {
		return Attempt{}, fmt.Errorf("claim publishing schedule: %w", err)
	}
	if claim.ExistingAttempt != nil {
		return *claim.ExistingAttempt, nil
	}
	profile, supported := platform.DefaultProfiles()[claim.Platform]
	if !supported || len(platform.ValidateContent(claim.Content, profile)) != 0 {
		service.logger.WarnContext(ctx, "scheduled variant failed publishing validation", "request_id", middleware.RequestIDFromContext(ctx), "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", "content_invalid")
		attempt, failErr := service.store.Fail(ctx, claim, FailureContentInvalid)
		if failErr != nil {
			return Attempt{}, fmt.Errorf("record invalid scheduled content: %w", failErr)
		}
		return attempt, nil
	}
	publisher, ok := service.publishers[claim.Platform]
	if !ok {
		service.logger.WarnContext(ctx, "social publisher is not configured", "request_id", middleware.RequestIDFromContext(ctx), "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", "publisher_unavailable")
		attempt, failErr := service.store.Fail(ctx, claim, FailurePublisherUnavailable)
		if failErr != nil {
			return Attempt{}, fmt.Errorf("record unavailable publisher: %w", failErr)
		}
		return attempt, nil
	}
	result, publishErr := publisher.Publish(ctx, PublishCommand{
		VariantID: claim.Schedule.VariantID, ScheduleID: claim.Schedule.ID,
		Content: claim.Content, IdempotencyKey: claim.IdempotencyKey,
	})
	if publishErr != nil {
		if errors.Is(publishErr, ErrRejected) {
			service.logger.WarnContext(ctx, "social provider rejected publish request", "request_id", middleware.RequestIDFromContext(ctx), "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", "provider_rejected")
			attempt, failErr := service.store.Fail(ctx, claim, FailureProviderRejected)
			if failErr != nil {
				return Attempt{}, fmt.Errorf("record rejected publish: %w", failErr)
			}
			return attempt, nil
		}
		service.logger.ErrorContext(ctx, "social provider publish outcome is uncertain", "request_id", middleware.RequestIDFromContext(ctx), "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", "provider_outcome_unknown")
		return Attempt{}, ErrUncertain
	}
	if strings.TrimSpace(result.ExternalPostID) == "" {
		service.logger.ErrorContext(ctx, "social provider returned no post identifier", "request_id", middleware.RequestIDFromContext(ctx), "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", "provider_response_invalid")
		return Attempt{}, ErrUncertain
	}
	result.ExternalPostID = strings.TrimSpace(result.ExternalPostID)
	attempt, err := service.store.Complete(ctx, claim, result)
	if err != nil {
		// The provider may already have published. Leave the claim in PROCESSING to prevent a duplicate.
		service.logger.ErrorContext(ctx, "publish result could not be persisted", "request_id", middleware.RequestIDFromContext(ctx), "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", "result_persistence_failed")
		return Attempt{}, fmt.Errorf("persist publish result: %w", err)
	}
	return attempt, nil
}

func canonicalID(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", ErrInvalidInput
	}
	return parsed.String(), nil
}
