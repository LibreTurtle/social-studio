package config

import (
	"strings"
	"testing"

	"github.com/LibreTurtle/social-studio/internal/ai"
)

func TestLoadFromUsesConfiguredProviderOrderAndDefaults(t *testing.T) {
	config, err := LoadFrom(lookup(map[string]string{
		"DATABASE_URL":       "postgres://db.example/social_studio",
		"GROQ_ENABLED":       "true",
		"GROQ_API_KEY":       "groq-key",
		"OPENROUTER_ENABLED": "true",
		"OPENROUTER_API_KEY": "router-key",
		"GEMINI_ENABLED":     "true",
		"GEMINI_API_KEY":     "gemini-key",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if config.Port != "8080" || config.DatabaseURL != "postgres://db.example/social_studio" {
		t.Fatalf("unexpected base config: %+v", config)
	}
	providers := config.AI.Providers
	if len(providers) != 3 || providers[0].Name != ai.ProviderGroq || providers[1].Name != ai.ProviderGemini || providers[2].Name != ai.ProviderOpenRouter {
		t.Fatalf("provider order = %+v", providers)
	}
	if providers[0].Model != "openai/gpt-oss-20b" || providers[1].Model != "gemini-3.5-flash-lite" || providers[2].Model != "openrouter/free" {
		t.Fatalf("unexpected model defaults: %+v", providers)
	}
}

func TestLoadFromAllowsApplicationWithoutConfiguredAIProviders(t *testing.T) {
	config, err := LoadFrom(lookup(map[string]string{"DATABASE_URL": "postgres://db.example/social_studio"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AI.Providers) != 0 {
		t.Fatalf("providers = %+v, want none", config.AI.Providers)
	}
}

func TestLoadFromUsesBoundedSchedulerDefaultsAndOverrides(t *testing.T) {
	defaults, err := LoadFrom(lookup(map[string]string{"DATABASE_URL": "postgres://db.example/social_studio"}))
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Scheduler.PollInterval <= 0 || defaults.Scheduler.ClaimTimeout <= 0 || defaults.Scheduler.Concurrency < 1 {
		t.Fatalf("scheduler defaults are not bounded: %+v", defaults.Scheduler)
	}
	configured, err := LoadFrom(lookup(map[string]string{
		"DATABASE_URL":            "postgres://db.example/social_studio",
		"SCHEDULER_POLL_INTERVAL": "2s",
		"SCHEDULER_CLAIM_TIMEOUT": "90s",
		"SCHEDULER_CONCURRENCY":   "6",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if configured.Scheduler.PollInterval.String() != "2s" || configured.Scheduler.ClaimTimeout.String() != "1m30s" || configured.Scheduler.Concurrency != 6 {
		t.Fatalf("scheduler settings = %+v", configured.Scheduler)
	}
}

func TestLoadFromRejectsInvalidSchedulerBounds(t *testing.T) {
	for _, values := range []map[string]string{
		{"DATABASE_URL": "postgres://db.example/social_studio", "SCHEDULER_POLL_INTERVAL": "0s"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "SCHEDULER_POLL_INTERVAL": "1ns"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "SCHEDULER_CLAIM_TIMEOUT": "nope"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "SCHEDULER_CLAIM_TIMEOUT": "48h"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "SCHEDULER_CONCURRENCY": "129"},
	} {
		if _, err := LoadFrom(lookup(values)); err == nil {
			t.Errorf("LoadFrom(%v) accepted invalid scheduler settings", values)
		}
	}
}

func TestLoadFromAllowsProviderBaseURLOverrides(t *testing.T) {
	config, err := LoadFrom(lookup(map[string]string{
		"DATABASE_URL":        "postgres://db.example/social_studio",
		"GROQ_ENABLED":        "true",
		"GROQ_API_KEY":        "groq-key",
		"GROQ_BASE_URL":       "https://proxy.example/groq/v1",
		"GEMINI_ENABLED":      "true",
		"GEMINI_API_KEY":      "gemini-key",
		"GEMINI_BASE_URL":     "https://proxy.example/gemini/v1beta",
		"OPENROUTER_ENABLED":  "true",
		"OPENROUTER_API_KEY":  "router-key",
		"OPENROUTER_BASE_URL": "https://proxy.example/openrouter/api/v1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://proxy.example/groq/v1", "https://proxy.example/gemini/v1beta", "https://proxy.example/openrouter/api/v1"}
	for index, provider := range config.AI.Providers {
		if provider.BaseURL != want[index] {
			t.Errorf("%s base URL = %q, want %q", provider.Name, provider.BaseURL, want[index])
		}
	}
}

func TestLoadFromRejectsEnabledProviderWithoutKey(t *testing.T) {
	_, err := LoadFrom(lookup(map[string]string{
		"DATABASE_URL": "postgres://db.example/social_studio",
		"GROQ_ENABLED": "true",
		"GROQ_API_KEY": "",
	}))
	if err == nil || !strings.Contains(err.Error(), "GROQ_API_KEY") {
		t.Fatalf("LoadFrom() error = %v, want missing GROQ_API_KEY", err)
	}
}

func TestLoadFromRejectsInvalidDurationsAndProviderFlags(t *testing.T) {
	tests := []map[string]string{
		{"DATABASE_URL": "postgres://db.example/social_studio", "AI_REQUEST_TIMEOUT": "later"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "GEMINI_ENABLED": "sometimes"},
	}
	for _, values := range tests {
		if _, err := LoadFrom(lookup(values)); err == nil {
			t.Fatalf("LoadFrom(%v) succeeded", values)
		}
	}
}

func TestLoadFromBuildsOnlyConfiguredPublishingAdapters(t *testing.T) {
	config, err := LoadFrom(lookup(map[string]string{
		"DATABASE_URL":             "postgres://db.example/social_studio",
		"X_PUBLISH_ENABLED":        "true",
		"X_ACCESS_TOKEN":           "x-secret",
		"LINKEDIN_PUBLISH_ENABLED": "true",
		"LINKEDIN_ACCESS_TOKEN":    "li-secret",
		"LINKEDIN_AUTHOR_URN":      "urn:li:person:member-1",
		"LINKEDIN_API_VERSION":     "202610",
		"TELEGRAM_PUBLISH_ENABLED": "true",
		"TELEGRAM_BOT_TOKEN":       "telegram-secret",
		"TELEGRAM_CHAT_ID":         "@channel",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !config.Publishing.X.Enabled || config.Publishing.X.AccessToken != "x-secret" ||
		!config.Publishing.LinkedIn.Enabled || config.Publishing.LinkedIn.AuthorURN != "urn:li:person:member-1" ||
		!config.Publishing.Telegram.Enabled || config.Publishing.Telegram.ChatID != "@channel" {
		t.Fatalf("publishing configuration = %+v", config.Publishing)
	}
}

func TestLoadFromRequiresCompleteConfigurationForEnabledPublisher(t *testing.T) {
	tests := []map[string]string{
		{"DATABASE_URL": "postgres://db.example/social_studio", "X_PUBLISH_ENABLED": "true"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "LINKEDIN_PUBLISH_ENABLED": "true", "LINKEDIN_ACCESS_TOKEN": "token"},
		{"DATABASE_URL": "postgres://db.example/social_studio", "TELEGRAM_PUBLISH_ENABLED": "true", "TELEGRAM_BOT_TOKEN": "token"},
	}
	for _, values := range tests {
		if _, err := LoadFrom(lookup(values)); err == nil {
			t.Errorf("LoadFrom(%v) succeeded without required publisher settings", values)
		}
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
