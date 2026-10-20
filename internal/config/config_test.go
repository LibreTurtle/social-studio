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

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
