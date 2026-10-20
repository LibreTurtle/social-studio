package ai

import (
	"net/http"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

const (
	ProviderGroq       = "groq"
	ProviderOpenRouter = "openrouter"
	ProviderGemini     = "gemini"
)

type ProviderSettings struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

type Settings struct {
	Providers       []ProviderSettings
	RequestTimeout  time.Duration
	MaxOutputTokens int
	Transport       http.RoundTripper
}

type Prompt struct {
	Platform           domain.Platform
	Tone               domain.Tone
	Source             string
	TargetLength       int
	MaxLength          int
	HashtagCount       int
	Instructions       string
	ConstraintFeedback []string
}
