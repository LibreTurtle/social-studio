package variant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/platform"
)

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (service *Service) CreateDrafts(ctx context.Context, postID string, drafts []Draft) ([]Variant, error) {
	postID, err := canonicalID(postID)
	if err != nil {
		return nil, fmt.Errorf("create variants: %w", err)
	}
	if len(drafts) == 0 || len(drafts) > MaximumDrafts {
		return nil, ErrInvalidInput
	}
	profiles := platform.DefaultProfiles()
	seen := make(map[domain.Platform]struct{}, len(drafts))
	for _, draft := range drafts {
		profile, supported := profiles[draft.Platform]
		if !supported {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[draft.Platform]; duplicate {
			return nil, ErrInvalidInput
		}
		seen[draft.Platform] = struct{}{}
		if violations := platform.ValidateContent(draft.Content, profile); len(violations) != 0 {
			return nil, invalidContent(violations)
		}
	}
	created, err := service.store.CreateDrafts(ctx, postID, drafts)
	if err != nil {
		return nil, fmt.Errorf("persist variants: %w", err)
	}
	return created, nil
}

func (service *Service) Get(ctx context.Context, id string) (Variant, error) {
	id, err := canonicalID(id)
	if err != nil {
		return Variant{}, err
	}
	value, err := service.store.Get(ctx, id)
	if err != nil {
		return Variant{}, fmt.Errorf("get variant: %w", err)
	}
	return value, nil
}

func (service *Service) ListByPost(ctx context.Context, postID string, status domain.VariantStatus, limit, offset int) ([]Variant, error) {
	postID, err := canonicalID(postID)
	if err != nil {
		return nil, err
	}
	if status != "" && !status.Valid() {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > MaximumPageSize || offset < 0 || offset > MaximumOffset {
		return nil, ErrInvalidInput
	}
	values, err := service.store.ListByPost(ctx, postID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	return values, nil
}

func (service *Service) UpdateContent(ctx context.Context, id string, expectedVersion int64, content string) (Variant, error) {
	id, err := canonicalID(id)
	if err != nil {
		return Variant{}, err
	}
	if expectedVersion <= 0 {
		return Variant{}, ErrInvalidInput
	}
	current, err := service.store.Get(ctx, id)
	if err != nil {
		return Variant{}, fmt.Errorf("load variant for edit: %w", err)
	}
	if !current.Platform.Valid() {
		return Variant{}, errors.New("stored variant has an unsupported platform")
	}
	profile := platform.DefaultProfiles()[current.Platform]
	if violations := platform.ValidateContent(content, profile); len(violations) != 0 {
		return Variant{}, invalidContent(violations)
	}
	updated, err := service.store.UpdateContent(ctx, id, expectedVersion, content)
	if err != nil {
		return Variant{}, fmt.Errorf("update variant content: %w", err)
	}
	return updated, nil
}

func (service *Service) Approve(ctx context.Context, id string, expectedVersion int64) (Variant, error) {
	return service.transition(ctx, id, expectedVersion, service.store.Approve)
}

func (service *Service) Reject(ctx context.Context, id string, expectedVersion int64) (Variant, error) {
	return service.transition(ctx, id, expectedVersion, service.store.Reject)
}

func (service *Service) transition(ctx context.Context, id string, expectedVersion int64, action func(context.Context, string, int64) (Variant, error)) (Variant, error) {
	id, err := canonicalID(id)
	if err != nil {
		return Variant{}, err
	}
	if expectedVersion <= 0 {
		return Variant{}, ErrInvalidInput
	}
	value, err := action(ctx, id, expectedVersion)
	if err != nil {
		return Variant{}, fmt.Errorf("change variant review status: %w", err)
	}
	return value, nil
}

func canonicalID(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", ErrInvalidID
	}
	return parsed.String(), nil
}
