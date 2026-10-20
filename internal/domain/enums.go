package domain

type Platform string

const (
	PlatformX        Platform = "X"
	PlatformLinkedIn Platform = "LINKEDIN"
	PlatformTelegram Platform = "TELEGRAM"
)

func (value Platform) Valid() bool {
	switch value {
	case PlatformX, PlatformLinkedIn, PlatformTelegram:
		return true
	default:
		return false
	}
}

type Tone string

const (
	ToneConcise        Tone = "CONCISE"
	ToneProfessional   Tone = "PROFESSIONAL"
	ToneConversational Tone = "CONVERSATIONAL"
)

func (value Tone) Valid() bool {
	switch value {
	case ToneConcise, ToneProfessional, ToneConversational:
		return true
	default:
		return false
	}
}

type SourceType string

const (
	SourceTypeURL      SourceType = "URL"
	SourceTypeMarkdown SourceType = "MARKDOWN"
)

func (value SourceType) Valid() bool {
	switch value {
	case SourceTypeURL, SourceTypeMarkdown:
		return true
	default:
		return false
	}
}

type VariantStatus string

const (
	VariantStatusDraft     VariantStatus = "DRAFT"
	VariantStatusApproved  VariantStatus = "APPROVED"
	VariantStatusRejected  VariantStatus = "REJECTED"
	VariantStatusPublished VariantStatus = "PUBLISHED"
)

func (value VariantStatus) Valid() bool {
	switch value {
	case VariantStatusDraft, VariantStatusApproved, VariantStatusRejected, VariantStatusPublished:
		return true
	default:
		return false
	}
}

type ScheduleStatus string

const (
	ScheduleStatusPending    ScheduleStatus = "PENDING"
	ScheduleStatusProcessing ScheduleStatus = "PROCESSING"
	ScheduleStatusCompleted  ScheduleStatus = "COMPLETED"
	ScheduleStatusFailed     ScheduleStatus = "FAILED"
)

func (value ScheduleStatus) Valid() bool {
	switch value {
	case ScheduleStatusPending, ScheduleStatusProcessing, ScheduleStatusCompleted, ScheduleStatusFailed:
		return true
	default:
		return false
	}
}

type PublishAttemptStatus string

const (
	PublishAttemptStatusSuccess PublishAttemptStatus = "SUCCESS"
	PublishAttemptStatusFailed  PublishAttemptStatus = "FAILED"
)

func (value PublishAttemptStatus) Valid() bool {
	switch value {
	case PublishAttemptStatusSuccess, PublishAttemptStatusFailed:
		return true
	default:
		return false
	}
}
