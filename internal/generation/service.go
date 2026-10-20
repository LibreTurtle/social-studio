package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/LibreTurtle/social-studio/internal/ai"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/platform"
	"github.com/LibreTurtle/social-studio/internal/post"
)

type Service struct {
	posts     PostStore
	generator ContentGenerator
	sessions  *SessionCache
}

func NewService(posts PostStore, generator ContentGenerator, ttl time.Duration, maxSessions int) *Service {
	return &Service{posts: posts, generator: generator, sessions: NewSessionCache(ttl, maxSessions)}
}

func (service *Service) Generate(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	stored, err := service.posts.Get(ctx, request.PostID)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			return Result{}, ErrPostNotFound
		}
		return Result{}, fmt.Errorf("load post: %w", err)
	}
	result := newResult(stored.ID, service.sessions.TTL(), len(request.Preferences))
	profiles := platform.DefaultProfiles()
	for _, preference := range request.Preferences {
		prompt := ai.Prompt{Platform: preference.Platform, Tone: preference.Tone, Source: stored.Content, TargetLength: preference.TargetLength, MaxLength: profiles[preference.Platform].MaxLength, HashtagCount: preference.HashtagCount, Instructions: preference.Instructions}
		var content, provider string
		var violations []platform.Violation
		for attempt := 0; attempt < MaxGenerationAttempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return Result{}, fmt.Errorf("generate %s variant: %w", preference.Platform, err)
			}
			var generateErr error
			content, provider, generateErr = service.generator.Generate(ctx, prompt)
			if generateErr != nil {
				return Result{}, fmt.Errorf("generate %s variant: %w", preference.Platform, generateErr)
			}
			violations = platform.Validate(content, profiles[preference.Platform], preference.HashtagCount)
			if len(violations) == 0 {
				break
			}
			prompt.ConstraintFeedback = prompt.ConstraintFeedback[:0]
			for _, violation := range violations {
				prompt.ConstraintFeedback = append(prompt.ConstraintFeedback, violation.Message)
			}
		}
		if len(violations) != 0 {
			feedback := make([]string, 0, len(violations))
			for _, violation := range violations {
				feedback = append(feedback, violation.Code+": "+violation.Message)
			}
			return Result{}, fmt.Errorf("%w for %s: %s", ErrInvalidOutput, preference.Platform, strings.Join(feedback, "; "))
		}
		result.Candidates = append(result.Candidates, Candidate{ID: newID(), Platform: preference.Platform, Content: content, Provider: provider})
	}
	service.sessions.Put(result)
	return result, nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.PostID) == "" || len(request.Preferences) == 0 || len(request.Preferences) > MaxPreferences {
		return ErrInvalidRequest
	}
	if _, err := uuid.Parse(request.PostID); err != nil {
		return ErrInvalidRequest
	}
	profiles := platform.DefaultProfiles()
	seen := make(map[domain.Platform]struct{}, len(request.Preferences))
	for _, preference := range request.Preferences {
		profile, supported := profiles[preference.Platform]
		if !supported || !preference.Tone.Valid() || preference.TargetLength <= 0 || preference.TargetLength > profile.MaxLength || preference.HashtagCount < 0 || preference.HashtagCount > profile.MaxHashtags || len([]rune(preference.Instructions)) > 1000 {
			return ErrInvalidRequest
		}
		if _, duplicate := seen[preference.Platform]; duplicate {
			return ErrInvalidRequest
		}
		seen[preference.Platform] = struct{}{}
	}
	return nil
}
