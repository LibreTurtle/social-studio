package publishing

import (
	"context"
	"errors"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

var (
	ErrInvalidInput         = errors.New("publishing input is invalid")
	ErrNotFound             = errors.New("schedule not found")
	ErrNotApproved          = errors.New("variant is not approved")
	ErrNotDue               = errors.New("schedule is not due")
	ErrInProgress           = errors.New("schedule is already processing")
	ErrPublisherUnavailable = errors.New("publisher is unavailable")
	ErrAlreadyScheduled     = errors.New("variant already has a schedule")
	ErrRejected             = errors.New("provider rejected publish request")
	ErrUncertain            = errors.New("publish outcome is uncertain")
)

type FailureReason string

const (
	FailureContentInvalid       FailureReason = "content_invalid"
	FailurePublisherUnavailable FailureReason = "publisher_unavailable"
	FailureProviderRejected     FailureReason = "provider_rejected"
)

type Schedule struct {
	ID          string                `json:"id"`
	VariantID   string                `json:"variant_id"`
	ScheduledAt time.Time             `json:"scheduled_at"`
	Status      domain.ScheduleStatus `json:"status"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	Version     int64                 `json:"version"`
}

type Attempt struct {
	ID             string                      `json:"id"`
	ScheduleID     string                      `json:"schedule_id"`
	VariantID      string                      `json:"variant_id"`
	IdempotencyKey string                      `json:"-"`
	Status         domain.PublishAttemptStatus `json:"status"`
	ExternalPostID string                      `json:"external_post_id,omitempty"`
	ErrorMessage   string                      `json:"error_message,omitempty"`
	AttemptedAt    time.Time                   `json:"attempted_at"`
}

type PublishCommand struct {
	VariantID      string
	ScheduleID     string
	Content        string
	IdempotencyKey string
}

type PublishResult struct{ ExternalPostID string }

type SocialPublisher interface {
	Publish(context.Context, PublishCommand) (PublishResult, error)
}

type Claim struct {
	Schedule        Schedule
	Platform        domain.Platform
	Content         string
	IdempotencyKey  string
	ExistingAttempt *Attempt
}

type Store interface {
	GetForSchedule(context.Context, string, time.Time) (domain.Platform, domain.VariantStatus, *Schedule, error)
	CreateSchedule(context.Context, string, time.Time) (Schedule, error)
	Claim(context.Context, string) (Claim, error)
	Complete(context.Context, Claim, PublishResult) (Attempt, error)
	Fail(context.Context, Claim, FailureReason) (Attempt, error)
}
