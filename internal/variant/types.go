package variant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/platform"
)

var (
	ErrInvalidID         = errors.New("variant ID is invalid")
	ErrInvalidInput      = errors.New("variant request is invalid")
	ErrNotFound          = errors.New("variant not found")
	ErrInvalidTransition = errors.New("variant status transition is invalid")
	ErrVersionConflict   = errors.New("variant version is stale")
	ErrInvalidContent    = errors.New("variant content violates platform constraints")
)

const (
	DefaultPageSize = 50
	MaximumPageSize = 100
	MaximumOffset   = 10_000
	MaximumDrafts   = 3
)

type Draft struct {
	Platform domain.Platform
	Content  string
}

type Variant struct {
	ID         string               `json:"id"`
	PostID     string               `json:"post_id"`
	Platform   domain.Platform      `json:"platform"`
	Content    string               `json:"content"`
	Status     domain.VariantStatus `json:"status"`
	RejectedAt *time.Time           `json:"rejected_at,omitempty"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Version    int64                `json:"version"`
}

type InvalidContentError struct {
	Violations []platform.Violation
}

func (err *InvalidContentError) Error() string { return ErrInvalidContent.Error() }
func (err *InvalidContentError) Unwrap() error { return ErrInvalidContent }

type Store interface {
	CreateDrafts(context.Context, string, []Draft) ([]Variant, error)
	Get(context.Context, string) (Variant, error)
	ListByPost(context.Context, string, domain.VariantStatus, int, int) ([]Variant, error)
	UpdateContent(context.Context, string, int64, string) (Variant, error)
	Approve(context.Context, string, int64) (Variant, error)
	Reject(context.Context, string, int64) (Variant, error)
}

func invalidContent(violations []platform.Violation) error {
	return &InvalidContentError{Violations: violations}
}

func invalidTransition(current domain.VariantStatus) error {
	return fmt.Errorf("%w from %s", ErrInvalidTransition, current)
}
