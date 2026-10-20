package generation

import (
	"context"
	"errors"
	"time"

	"github.com/LibreTurtle/social-studio/internal/ai"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/platform"
	"github.com/LibreTurtle/social-studio/internal/post"
)

var (
	ErrPostNotFound    = errors.New("post not found")
	ErrSessionNotFound = errors.New("generation session not found")
	ErrInvalidRequest  = errors.New("invalid generation request")
	ErrInvalidOutput   = errors.New("AI providers could not produce content satisfying platform constraints")
	ErrAIUnavailable   = ai.ErrProvidersUnavailable
)

const (
	MaxPreferences        = 3
	MaxGenerationAttempts = 3
)

type Preference struct {
	Platform     domain.Platform `json:"platform"`
	Tone         domain.Tone     `json:"tone"`
	TargetLength int             `json:"target_length"`
	HashtagCount int             `json:"hashtag_count"`
	Instructions string          `json:"instructions,omitempty"`
}
type Request struct {
	PostID      string       `json:"post_id"`
	Preferences []Preference `json:"preferences"`
}
type Candidate struct {
	ID         string               `json:"id"`
	Platform   domain.Platform      `json:"platform"`
	Content    string               `json:"content"`
	Provider   string               `json:"-"`
	Violations []platform.Violation `json:"violations"`
}
type Result struct {
	ID         string      `json:"id"`
	PostID     string      `json:"post_id"`
	CreatedAt  time.Time   `json:"created_at"`
	ExpiresAt  time.Time   `json:"expires_at"`
	Candidates []Candidate `json:"candidates"`
}
type PostStore interface {
	Get(context.Context, string) (post.Post, error)
}
type ContentGenerator interface {
	Generate(context.Context, ai.Prompt) (string, string, error)
}
